package registry

import (
	"fmt"
	"os"

	dcontext "github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/driver/factory"
	"github.com/docker/distribution/version"
	"github.com/docker/libtrust"
	"github.com/spf13/cobra"
)

var showVersion bool

func init() {
	RootCmd.AddCommand(ServeCmd)
	RootCmd.AddCommand(GCCmd)
	GCCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "do everything except remove the blobs")
	GCCmd.Flags().BoolVarP(&removeUntagged, "delete-untagged", "m", false, "delete manifests that are not currently referenced via tag")
	GCCmd.Flags().BoolVarP(&removeRepositories, "delete-repositories", "r", false, "delete repositories without manifests")
	GCCmd.Flags().Float64VarP(&modificationTimeout, "modification-timeout", "t", 0, "don't remove objects modified less than this number of seconds ago")
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

var dryRun bool
var removeUntagged bool
var removeRepositories bool
var modificationTimeout float64

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

		k, err := libtrust.GenerateECP256PrivateKey()
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}

		registry, err := storage.NewRegistry(ctx, driver, storage.Schema1SigningKey(k))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to construct registry: %v", err)
			os.Exit(1)
		}

		err = storage.MarkAndSweep(ctx, driver, registry, storage.GCOpts{
			DryRun:              dryRun,
			RemoveUntagged:      removeUntagged,
			RemoveRepositories:  removeRepositories,
			ModificationTimeout: modificationTimeout,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to garbage collect: %v", err)
			os.Exit(1)
		}
	},
}
