package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/bugsnag/bugsnag-go"
	"github.com/gorilla/handlers"
	"github.com/yvasiyarov/gorelic"

	_ "github.com/docker/distribution/auth/silly"
	_ "github.com/docker/distribution/auth/token"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/registry"
	_ "github.com/docker/distribution/storagedriver/filesystem"
	_ "github.com/docker/distribution/storagedriver/inmemory"
)

func main() {
	flag.Usage = usage
	flag.Parse()

	config, err := resolveConfiguration()
	if err != nil {
		fatalf("configuration error: %v", err)
	}

	app := registry.NewApp(*config)
	handler := configureReporting(app)
	handler = handlers.CombinedLoggingHandler(os.Stdout, handler)
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

func configureReporting(app *registry.App) http.Handler {
	var handler http.Handler = app

	if app.Config.Reporting.Bugsnag.APIKey != "" {
		bugsnagConfig := bugsnag.Configuration{
			APIKey: app.Config.Reporting.Bugsnag.APIKey,
			// TODO(brianbland): provide the registry version here
			// AppVersion: "2.0",
		}
		if app.Config.Reporting.Bugsnag.ReleaseStage != "" {
			bugsnagConfig.ReleaseStage = app.Config.Reporting.Bugsnag.ReleaseStage
		}
		if app.Config.Reporting.Bugsnag.Endpoint != "" {
			bugsnagConfig.Endpoint = app.Config.Reporting.Bugsnag.Endpoint
		}
		bugsnag.Configure(bugsnagConfig)

		handler = bugsnag.Handler(handler)
	}

	if app.Config.Reporting.NewRelic.LicenseKey != "" {
		agent := gorelic.NewAgent()
		agent.NewrelicLicense = app.Config.Reporting.NewRelic.LicenseKey
		if app.Config.Reporting.NewRelic.Name != "" {
			agent.NewrelicName = app.Config.Reporting.NewRelic.Name
		}
		agent.CollectHTTPStat = true
		agent.Verbose = true
		agent.Run()

		handler = agent.WrapHTTPHandler(handler)
	}

	return handler
}
