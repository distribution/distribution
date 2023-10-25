package registry

import (
	"fmt"
	"os"
	"time"

	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/registry/purge"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	"github.com/distribution/distribution/v3/version"
	"github.com/spf13/cobra"
)

var showVersion bool

func init() {
	RootCmd.AddCommand(ServeCmd)

	RootCmd.AddCommand(GCCmd)
	GCCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "do everything except remove the blobs")
	GCCmd.Flags().BoolVarP(&removeUntagged, "delete-untagged", "m", false, "delete manifests that are not currently referenced via tag")

	RootCmd.AddCommand(PurgeUploadsCmd)
	PurgeUploadsCmd.Flags().BoolVarP(&purgeDryRun, "dry-run", "d", false, "do everything except remove the upload files")
	PurgeUploadsCmd.Flags().DurationVarP(&purgeAgeDuration, "age", "a", 0, "the age of temporary files created during upload")

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
			cmd.Usage()
			os.Exit(1)
		}

		driver, err := factory.Create(config.Storage.Type(), config.Storage.Parameters())
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to construct %s driver: %v", config.Storage.Type(), err)
			os.Exit(1)
		}

		ctx := dcontext.Background()
		ctx, err = configureLogging(ctx, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to configure logging with config: %s", err)
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

var (
	purgeAgeDuration time.Duration
	purgeDryRun      bool
)

// PurgeUploadsCmd is the cobra command that corresponds to the purge-uploads subcommand
var PurgeUploadsCmd = &cobra.Command{
	Use:   "purge-uploads <config>",
	Short: "`purge-uploads` deletes outdated upload files",
	Long:  "`purge-uploads` deletes outdated upload files",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := resolveConfiguration(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
			cmd.Usage()
			os.Exit(1)
		}

		driver, err := factory.Create(config.Storage.Type(), config.Storage.Parameters())
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to construct %s driver: %v", config.Storage.Type(), err)
			os.Exit(1)
		}

		ctx := dcontext.Background()
		ctx, err = configureLogging(ctx, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to configure logging with config: %s", err)
			os.Exit(1)
		}

		// default configs
		purgeConfig := purge.UploadPurgeDefaultConfig()
		if mc, ok := config.Storage["maintenance"]; ok {
			if v, ok := mc["uploadpurging"]; ok {
				purgeConfig, ok = v.(map[interface{}]interface{})
				if !ok {
					panic("uploadpurging config key must contain additional keys")
				}
			}
		}

		// parse configs from config file
		purgeOption, err := purge.ParseConfig(purgeConfig)
		if err != nil {
			panic(fmt.Sprintf("Unable to parse upload purge configuration: %s", err.Error()))
		}

		// overwrite confis by command line options
		if cmd.Flags().Changed("age") {
			purgeOption.Age = purgeAgeDuration
		}

		if cmd.Flags().Changed("dry-run") {
			purgeOption.DryRun = purgeDryRun
		}

		fmt.Println(purgeOption.String())

		_, errs := storage.PurgeUploads(ctx, driver, time.Now().Add(-purgeOption.Age), !purgeOption.DryRun)
		if len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "failed to purge uploads: %v", err)
			os.Exit(1)
		}
	},
}
