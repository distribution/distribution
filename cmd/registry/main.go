package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/gorilla/handlers"

	log "github.com/Sirupsen/logrus"

	"github.com/docker/docker-registry"
	"github.com/docker/docker-registry/configuration"
	_ "github.com/docker/docker-registry/storagedriver/filesystem"
	_ "github.com/docker/docker-registry/storagedriver/inmemory"
)

func main() {
	flag.Usage = usage
	flag.Parse()

	config, err := resolveConfiguration()
	if err != nil {
		fatalf("configuration error: %v", err)
	}

	app := registry.NewApp(*config)
	handler := handlers.CombinedLoggingHandler(os.Stdout, app)
	log.SetLevel(logLevel(config.Loglevel))

	log.Infof("listening on %v", config.HTTP.Addr)
	if err := http.ListenAndServe(config.HTTP.Addr, handler); err != nil {
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

	config, err := configuration.Parse(fp)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %v", configurationPath, err)
	}

	return config, nil
}

func logLevel(level configuration.Loglevel) log.Level {
	l, err := log.ParseLevel(string(level))
	if err != nil {
		log.Warnf("error parsing level %q: %v", level, err)
		l = log.InfoLevel
	}

	return l
}
