package registry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	memorycache "github.com/distribution/distribution/v3/registry/storage/cache/memory"
	rediscache "github.com/distribution/distribution/v3/registry/storage/cache/redis"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	"github.com/distribution/distribution/v3/version"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
)

var showVersion bool

func init() {
	RootCmd.AddCommand(ServeCmd)
	RootCmd.AddCommand(GCCmd)
	GCCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "do everything except remove the blobs")
	GCCmd.Flags().BoolVarP(&removeUntagged, "delete-untagged", "m", false, "delete manifests that are not currently referenced via tag")
	RootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "show the version and exit")
}

// RootCmd is the main command for the 'registry' binary.
var RootCmd = &cobra.Command{
	Use:   "registry",
	Short: "`registry`",
	Long:  "`registry`",
	Run: func(cmd *cobra.Command, args []string) {
		if showVersion {
			version.PrintVersion()
			return
		}
		// nolint:errcheck
		cmd.Usage()
	},
}

var (
	dryRun         bool
	removeUntagged bool
)

func configRedis(config *configuration.Configuration) (redis.UniversalClient, error) {
	if len(config.Redis.Options.Addrs) == 0 {
		return nil, fmt.Errorf("redis not configured")
	}

	// redis TLS config
	if config.Redis.TLS.Certificate != "" || config.Redis.TLS.Key != "" {
		var err error
		tlsConf := &tls.Config{}
		tlsConf.Certificates = make([]tls.Certificate, 1)
		tlsConf.Certificates[0], err = tls.LoadX509KeyPair(config.Redis.TLS.Certificate, config.Redis.TLS.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to load redis client certificate: %v", err)
		}
		if len(config.Redis.TLS.ClientCAs) != 0 {
			pool := x509.NewCertPool()
			for _, ca := range config.Redis.TLS.ClientCAs {
				caPem, err := os.ReadFile(ca)
				if err != nil {
					return nil, fmt.Errorf("Error: failed reading redis client CA: %w", err)
				}

				if ok := pool.AppendCertsFromPEM(caPem); !ok {
					return nil, fmt.Errorf("Error: could not add CA to pool")
				}
			}
			tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
			tlsConf.ClientCAs = pool
		}
		config.Redis.Options.TLSConfig = tlsConf
	}

	config.Redis.Options.OnConnect = func(ctx context.Context, cn *redis.Conn) error {
		res := cn.Ping(ctx)
		return res.Err()
	}

	return redis.NewUniversalClient(&config.Redis.Options), nil

}

func getGCCacheProvider(config *configuration.Configuration) (cache.BlobDescriptorCacheProvider, error) {
	if cc, ok := config.Storage["cache"]; ok {
		v, ok := cc["blobdescriptor"]
		if !ok {
			// Backwards compatible: "layerinfo" == "blobdescriptor"
			v = cc["layerinfo"]
		}

		switch v {
		case "redis":
			redisClient, err := configRedis(config)
			if err != nil {
				return nil, err
			}
			return rediscache.NewRedisBlobDescriptorCacheProvider(redisClient), nil

		case "inmemory":
			blobDescriptorSize := memorycache.DefaultSize
			configuredSize, ok := cc["blobdescriptorsize"]
			if ok {
				var err error
				// Since Parameters is not strongly typed, render to a string and convert back
				blobDescriptorSize, err = strconv.Atoi(fmt.Sprint(configuredSize))
				if err != nil {
					panic(fmt.Sprintf("invalid blobdescriptorsize value %s: %s", configuredSize, err))
				}
			}

			return memorycache.NewInMemoryBlobDescriptorCacheProvider(blobDescriptorSize), nil

		default:
			return nil, nil
		}
	}

	return nil, nil
}

// GCCmd is the cobra command that corresponds to the garbage-collect subcommand
var GCCmd = &cobra.Command{
	Use:   "garbage-collect <config>",
	Short: "`garbage-collect` deletes layers not referenced by any manifests",
	Long:  "`garbage-collect` deletes layers not referenced by any manifests",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := resolveConfiguration(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
			// nolint:errcheck
			cmd.Usage()
			os.Exit(1)
		}

		ctx := dcontext.Background()
		ctx, err = configureLogging(ctx, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to configure logging with config: %s", err)
			os.Exit(1)
		}

		driver, err := factory.Create(ctx, config.Storage.Type(), config.Storage.Parameters())
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to construct %s driver: %v", config.Storage.Type(), err)
			os.Exit(1)
		}

		registry, err := storage.NewRegistry(ctx, driver)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to construct registry: %v", err)
			os.Exit(1)
		}

		cacheProvider, err := getGCCacheProvider(config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get cache provider: %v", err)
			os.Exit(1)
		}

		err = storage.MarkAndSweep(ctx, driver, cacheProvider, registry, storage.GCOpts{
			DryRun:         dryRun,
			RemoveUntagged: removeUntagged,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to garbage collect: %v", err)
			os.Exit(1)
		}
	},
}
