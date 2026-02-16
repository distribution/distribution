package registry

import (
	"fmt"
	"os"
	"time"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	"github.com/distribution/distribution/v3/version"
	"github.com/spf13/cobra"
)

var showVersion bool

var (
	dryRun         bool
	removeUntagged bool
	quiet          bool
	workers        int
	timeout        time.Duration
	checkpointDir  string
	markOnly       bool
	sweepOnly      bool
)

func init() {
	RootCmd.AddCommand(ServeCmd)
	RootCmd.AddCommand(GCCmd)
	GCCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "do everything except remove the blobs")
	GCCmd.Flags().BoolVarP(&removeUntagged, "delete-untagged", "m", false, "delete manifests that are not currently referenced via tag")
	GCCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "silence output")
	GCCmd.Flags().IntVarP(&workers, "workers", "w", 4, "number of concurrent workers")
	GCCmd.Flags().DurationVarP(&timeout, "timeout", "t", 24*time.Hour, "maximum runtime before stopping")
	GCCmd.Flags().StringVar(&checkpointDir, "checkpoint-dir", "", "directory for checkpoint/resume and two-pass mode")
	GCCmd.Flags().BoolVar(&markOnly, "mark-only", false, "only run mark phase and save candidates (requires --checkpoint-dir)")
	GCCmd.Flags().BoolVar(&sweepOnly, "sweep", false, "only run sweep phase from checkpoint (requires --checkpoint-dir)")
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
			Quiet:          quiet,
			MaxConcurrency: workers,
			Timeout:        timeout,
			CheckpointDir:  checkpointDir,
			MarkOnly:       markOnly,
			SweepOnly:      sweepOnly,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to garbage collect: %v", err)
			os.Exit(1)
		}
	},
}
