package main

import (
	"crypto/tls"
	"crypto/x509"
	_ "expvar"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/Sirupsen/logrus/formatters/logstash"
	"github.com/bugsnag/bugsnag-go"
	"github.com/docker/distribution/configuration"
	ctxu "github.com/docker/distribution/context"
	_ "github.com/docker/distribution/health"
	_ "github.com/docker/distribution/registry/auth/silly"
	_ "github.com/docker/distribution/registry/auth/token"
	"github.com/docker/distribution/registry/handlers"
	_ "github.com/docker/distribution/registry/storage/driver/azure"
	_ "github.com/docker/distribution/registry/storage/driver/filesystem"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
	_ "github.com/docker/distribution/registry/storage/driver/middleware/cloudfront"
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

	ctx, err = configureLogging(ctx, config)
	if err != nil {
		fatalf("error configuring logger: %v", err)
	}

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
		tlsConf := &tls.Config{
			ClientAuth: tls.NoClientCert,
		}

		if len(config.HTTP.TLS.ClientCAs) != 0 {
			pool := x509.NewCertPool()

			for _, ca := range config.HTTP.TLS.ClientCAs {
				caPem, err := ioutil.ReadFile(ca)
				if err != nil {
					ctxu.GetLogger(app).Fatalln(err)
				}

				if ok := pool.AppendCertsFromPEM(caPem); !ok {
					ctxu.GetLogger(app).Fatalln(fmt.Errorf("Could not add CA to pool"))
				}
			}

			for _, subj := range pool.Subjects() {
				ctxu.GetLogger(app).Debugf("CA Subject: %s", string(subj))
			}

			tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
			tlsConf.ClientCAs = pool
		}

		ctxu.GetLogger(app).Infof("listening on %v, tls", config.HTTP.Addr)
		server := &http.Server{
			Addr:      config.HTTP.Addr,
			Handler:   handler,
			TLSConfig: tlsConf,
		}

		if err := server.ListenAndServeTLS(config.HTTP.TLS.Certificate, config.HTTP.TLS.Key); err != nil {
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

// configureLogging prepares the context with a logger using the
// configuration.
func configureLogging(ctx context.Context, config *configuration.Configuration) (context.Context, error) {
	if config.Log.Level == "" && config.Log.Formatter == "" {
		// If no config for logging is set, fallback to deprecated "Loglevel".
		log.SetLevel(logLevel(config.Loglevel))
		ctx = ctxu.WithLogger(ctx, ctxu.GetLogger(ctx, "version"))
		return ctx, nil
	}

	log.SetLevel(logLevel(config.Log.Level))

	switch config.Log.Formatter {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	case "text":
		log.SetFormatter(&log.TextFormatter{})
	case "logstash":
		log.SetFormatter(&logstash.LogstashFormatter{})
	default:
		// just let the library use default on empty string.
		if config.Log.Formatter != "" {
			return ctx, fmt.Errorf("unsupported logging formatter: %q", config.Log.Formatter)
		}
	}

	if config.Log.Formatter != "" {
		log.Debugf("using %q logging formatter", config.Log.Formatter)
	}

	// log the application version with messages
	ctx = context.WithValue(ctx, "version", version.Version)

	if len(config.Log.Fields) > 0 {
		// build up the static fields, if present.
		var fields []interface{}
		for k := range config.Log.Fields {
			fields = append(fields, k)
		}

		ctx = ctxu.WithValues(ctx, config.Log.Fields)
		ctx = ctxu.WithLogger(ctx, ctxu.GetLogger(ctx, fields...))
	}

	return ctx, nil
}

func logLevel(level configuration.Loglevel) log.Level {
	l, err := log.ParseLevel(string(level))
	if err != nil {
		l = log.InfoLevel
		log.Warnf("error parsing level %q: %v, using %q	", level, err, l)
	}

	return l
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
