package registry

import (
	"fmt"
	"os"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	"github.com/distribution/distribution/v3/version"
	"github.com/opencontainers/go-digest"
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

		err = storage.MarkAndSweep(ctx, driver, registry, storage.GCOpts{
			DryRun:         dryRun,
			RemoveUntagged: removeUntagged,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to garbage collect: %v", err)
			os.Exit(1)
		}
	},
}

func GetUsedBlobs(args []string) (map[storage.UsedBlob]struct{}, []storage.ManifestDel, error) {
	config, err := resolveConfiguration(args)
	if err != nil {
		return nil, nil, fmt.Errorf("configuration error: %v", err)
	}

	ctx := dcontext.Background()
	ctx, err = configureLogging(ctx, config)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to configure logging with config: %s", err)
	}

	driver, err := factory.Create(ctx, config.Storage.Type(), config.Storage.Parameters())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to construct %s driver: %v", config.Storage.Type(), err)
	}

	registry, err := storage.NewRegistry(ctx, driver)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to construct registry: %v", err)
	}

	usedBlobs, manifests, err := storage.GetUsedBlobs(ctx, registry)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get used blobs: %v", err)
	}

	return usedBlobs, manifests, nil
}

func GetBlobs(args []string) (map[digest.Digest]struct{}, error) {
	config, err := resolveConfiguration(args)
	if err != nil {
		return nil, fmt.Errorf("configuration error: %v", err)
	}

	ctx := dcontext.Background()
	ctx, err = configureLogging(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to configure logging with config: %s", err)
	}

	driver, err := factory.Create(ctx, config.Storage.Type(), config.Storage.Parameters())
	if err != nil {
		return nil, fmt.Errorf("failed to construct %s driver: %v", config.Storage.Type(), err)
	}

	registry, err := storage.NewRegistry(ctx, driver)
	if err != nil {
		return nil, fmt.Errorf("failed to construct registry: %v", err)
	}

	blobService := registry.Blobs()
	blobs := make(map[digest.Digest]struct{})
	err = blobService.Enumerate(ctx, func(dgst digest.Digest) error {
		blobs[dgst] = struct{}{}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get blobs: %v", err)
	}

	return blobs, nil
}

// GCCmd is the cobra command that corresponds to the garbage-collect subcommand
var TractoGCCmd = &cobra.Command{
	Use:   "tracto-gc <config>",
	Short: "`tracto-gc` deletes layers not referenced by any manifests",
	Long:  "`tracto-gc` deletes layers not referenced by any manifests",
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

		usedBlobs, manifets, err := storage.GetUsedBlobs(ctx, registry)

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to garbage collect: %v", err)
			os.Exit(1)
		}

		print(usedBlobs, manifets)
	},
}
