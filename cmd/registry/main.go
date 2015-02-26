package main

import (
	_ "expvar"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/bugsnag/bugsnag-go"
	"github.com/docker/distribution/configuration"
	ctxu "github.com/docker/distribution/context"
	_ "github.com/docker/distribution/health"
	_ "github.com/docker/distribution/registry/auth/silly"
	_ "github.com/docker/distribution/registry/auth/token"
	"github.com/docker/distribution/registry/handlers"
	_ "github.com/docker/distribution/registry/storage/driver/filesystem"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
	_ "github.com/docker/distribution/registry/storage/driver/s3"
	"github.com/docker/distribution/version"
	gorhandlers "github.com/gorilla/handlers"
	"github.com/yvasiyarov/gorelic"
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

	ctx := context.Background()

	config, err := resolveConfiguration()
	if err != nil {
		fatalf("configuration error: %v", err)
	}

	log.SetLevel(logLevel(config.Loglevel))
	ctx = context.WithValue(ctx, "version", version.Version)
	ctx = ctxu.WithLogger(ctx, ctxu.GetLogger(ctx, "version"))

	app := handlers.NewApp(ctx, *config)
	handler := configureReporting(app)
	handler = gorhandlers.CombinedLoggingHandler(os.Stdout, handler)

	if config.HTTP.Debug.Addr != "" {
		go debugServer(config.HTTP.Debug.Addr)
	}

	if config.HTTP.TLS.Certificate == "" {
		ctxu.GetLogger(app).Infof("listening on %v", config.HTTP.Addr)
		if err := http.ListenAndServe(config.HTTP.Addr, handler); err != nil {
			ctxu.GetLogger(app).Fatalln(err)
		}
	} else {
		ctxu.GetLogger(app).Infof("listening on %v, tls", config.HTTP.Addr)
		if err := http.ListenAndServeTLS(config.HTTP.Addr, config.HTTP.TLS.Certificate, config.HTTP.TLS.Key, handler); err != nil {
			ctxu.GetLogger(app).Fatalln(err)
		}
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

func configureReporting(app *handlers.App) http.Handler {
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

// debugServer starts the debug server with pprof, expvar among other
// endpoints. The addr should not be exposed externally. For most of these to
// work, tls cannot be enabled on the endpoint, so it is generally separate.
func debugServer(addr string) {
	log.Infof("debug server listening %v", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("error listening on debug interface: %v", err)
	}
}
