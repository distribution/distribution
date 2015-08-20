package main

import (
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/registry"
	_ "github.com/docker/distribution/registry/auth/htpasswd"
	_ "github.com/docker/distribution/registry/auth/silly"
	_ "github.com/docker/distribution/registry/auth/token"
	_ "github.com/docker/distribution/registry/proxy"
	_ "github.com/docker/distribution/registry/storage/driver/azure"
	_ "github.com/docker/distribution/registry/storage/driver/filesystem"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
	_ "github.com/docker/distribution/registry/storage/driver/middleware/cloudfront"
	_ "github.com/docker/distribution/registry/storage/driver/oss"
	_ "github.com/docker/distribution/registry/storage/driver/s3"
	_ "github.com/docker/distribution/registry/storage/driver/swift"
	"github.com/docker/distribution/version"
	"golang.org/x/net/context"
)

var showVersion bool

func init() {
	flag.BoolVar(&showVersion, "version", false, "show the version and exit")
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if showVersion {
		version.PrintVersion()
		return
	}

	config, err := resolveConfiguration()
	if err != nil {
		fatalf("configuration error: %v", err)
	}

	registry, err := registry.NewRegistry(context.Background(), config)
	if err != nil {
		log.Fatalln(err)
	}

	err = registry.Serve()
	if err != nil {
		log.Fatalln(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:", os.Args[0], "<config>")
	flag.PrintDefaults()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	usage()
	os.Exit(1)
}

func resolveConfiguration() (*configuration.Configuration, error) {
	var configurationPath string

	if flag.NArg() > 0 {
		configurationPath = flag.Arg(0)
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
