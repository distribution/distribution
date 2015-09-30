package pruner

import (
	"fmt"
	"io"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/uuid"
	"github.com/docker/distribution/version"
	"github.com/spf13/cobra"
)

const (
	binaryName      = "pruner"
	helpMessageTmpl = `%s allows to print and/or delete registry's orphaned blobs.

Make sure that registry instance isn't running or is running in read-only mode
before launching this executable.`
)

// Cmd is a cobra command for running the registry.
var Cmd = &cobra.Command{
	Use:   "<config>",
	Short: fmt.Sprintf("%s deletes orphaned blobs", binaryName),
	Long:  fmt.Sprintf(helpMessageTmpl, binaryName),
	Run: func(cmd *cobra.Command, args []string) {
		if showVersion {
			version.PrintVersion()
			return
		}

		// setup context
		ctx := context.WithVersion(context.Background(), version.Version)

		registryConfig, err := resolveConfiguration(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
			cmd.Usage()
			os.Exit(1)
		}

		registry, err := NewRegistry(ctx, registryConfig)
		if err != nil {
			log.Fatalln(err)
		}

		prunerConfig.In = os.Stdin
		prunerConfig.Out = cmd.Out()
		if prunerConfig.Out == nil {
			prunerConfig.Out = os.Stdout
		}

		pruner := NewPruner(&prunerConfig, ctx)
		rg, err := pruner.LoadRegistryGraph(registry)
		if err != nil {
			log.Fatalln(err)
		}
		err = pruner.ProcessRegistryGraph(registry, rg)
		if err != nil {
			log.Fatalln(err)
		}
	},
}

// Config holds parameters for Pruner loaded from program arguments and config
// file.
type Config struct {
	RemoveEmpty bool
	DryRun      bool
	Confirm     bool
	Verbose     bool
	Out         io.Writer
	In          io.Reader
}

var (
	showVersion  bool
	prunerConfig Config
)

func init() {
	Cmd.PersistentFlags().BoolVarP(&prunerConfig.RemoveEmpty, "remove-empty", "e", false, "remove empty repositories (containing 0 manifest revisions)")
	Cmd.PersistentFlags().BoolVarP(&prunerConfig.DryRun, "dry-run", "n", false, "perform a trial prune with no changes to registry")
	Cmd.PersistentFlags().BoolVarP(&prunerConfig.Confirm, "confirm", "c", true, "ask for confirmation before making changes")
	Cmd.PersistentFlags().BoolVarP(&prunerConfig.Verbose, "verbose", "v", false, "turn on verbosive output")
	Cmd.PersistentFlags().BoolVarP(&showVersion, "version", "V", false, "show the version and exit")
}

// A Registry represents a complete instance of the registry.
type Registry struct {
	config *configuration.Configuration
}

// NewRegistry creates a new registry from a context and configuration struct.
func NewRegistry(ctx context.Context, config *configuration.Configuration) (*Registry, error) {
	var err error

	if prunerConfig.Verbose {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	ctx = context.WithLogger(ctx, context.GetLogger(ctx))
	if err != nil {
		return nil, fmt.Errorf("error configuring logger: %v", err)
	}

	// inject a logger into the uuid library. warns us if there is a problem
	// with uuid generation under low entropy.
	uuid.Loggerf = context.GetLogger(ctx).Warnf

	return &Registry{
		config: config,
	}, nil
}

func resolveConfiguration(args []string) (*configuration.Configuration, error) {
	var configurationPath string

	if len(args) > 0 {
		configurationPath = args[0]
	} else if os.Getenv("REGISTRY_CONFIGURATION_PATH") != "" {
		configurationPath = os.Getenv("REGISTRY_CONFIGURATION_PATH")
	}

	if configurationPath == "" {
		return nil, fmt.Errorf("configuration path unspecified")
	}

	fp, err := os.Open(configurationPath)
	if err != nil {
		return nil, err
	}

	defer fp.Close()

	config, err := configuration.Parse(fp)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %v", configurationPath, err)
	}

	return config, nil
}
