package registry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	logstash "github.com/bshuster-repo/logrus-logstash-hook"
	"github.com/docker/go-metrics"
	gorhandlers "github.com/gorilla/handlers"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/health"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/handlers"
	"github.com/distribution/distribution/v3/registry/listener"
	"github.com/distribution/distribution/v3/tracing"
	"github.com/distribution/distribution/v3/version"
)

// a map of TLS cipher suite names to constants in https://golang.org/pkg/crypto/tls/#pkg-constants
var cipherSuites = map[string]uint16{
	// TLS 1.0 - 1.2 cipher suites
	"TLS_RSA_WITH_3DES_EDE_CBC_SHA":                 tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	"TLS_RSA_WITH_AES_128_CBC_SHA":                  tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"TLS_RSA_WITH_AES_256_CBC_SHA":                  tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"TLS_RSA_WITH_AES_128_GCM_SHA256":               tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_RSA_WITH_AES_256_GCM_SHA384":               tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":          tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":          tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":           tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":            tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":            tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":         tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256":       tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":         tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384":       tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256":   tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256": tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	// TLS 1.3 cipher suites
	"TLS_AES_128_GCM_SHA256":       tls.TLS_AES_128_GCM_SHA256,
	"TLS_AES_256_GCM_SHA384":       tls.TLS_AES_256_GCM_SHA384,
	"TLS_CHACHA20_POLY1305_SHA256": tls.TLS_CHACHA20_POLY1305_SHA256,
}

// a list of default ciphersuites to utilize
var defaultCipherSuites = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_AES_128_GCM_SHA256,
	tls.TLS_CHACHA20_POLY1305_SHA256,
	tls.TLS_AES_256_GCM_SHA384,
}

const defaultTLSVersionStr = "tls1.2"

// tlsVersions maps user-specified values to tls version constants.
var tlsVersions = map[string]uint16{
	"tls1.2": tls.VersionTLS12,
	"tls1.3": tls.VersionTLS13,
}

// defaultLogFormatter is the default formatter to use for logs.
const defaultLogFormatter = "text"

// HandlerFunc defines an http middleware
type HandlerFunc func(config *configuration.Configuration, handler http.Handler) http.Handler

var handlerMiddlewares []HandlerFunc

// RegisterHandler is used to register http middlewares to the registry service
func RegisterHandler(handlerFunc HandlerFunc) {
	handlerMiddlewares = append(handlerMiddlewares, handlerFunc)
}

// ServeCmd is a cobra command for running the registry.
var ServeCmd = &cobra.Command{
	Use:   "serve <config>",
	Short: "`serve` stores and distributes Docker images",
	Long:  "`serve` stores and distributes Docker images.",
	Run: func(cmd *cobra.Command, args []string) {
		// setup context
		ctx := dcontext.WithVersion(dcontext.Background(), version.Version())

		config, err := resolveConfiguration(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
			// nolint:errcheck
			cmd.Usage()
			os.Exit(1)
		}
		registry, err := NewRegistry(ctx, config)
		if err != nil {
			logrus.Fatalln(err)
		}

		configureDebugServer(config)

		if err = registry.ListenAndServe(); err != nil {
			logrus.Fatalln(err)
		}
	},
}

// A Registry represents a complete instance of the registry.
//
// TODO(aaronl): It might make sense for Registry to become an interface.
type Registry struct {
	config *configuration.Configuration
	app    *handlers.App
	server *http.Server
	quit   chan os.Signal
}

// NewRegistry creates a new registry from a context and configuration struct.
func NewRegistry(ctx context.Context, config *configuration.Configuration) (*Registry, error) {
	var err error
	ctx, err = configureLogging(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("error configuring logger: %v", err)
	}

	app := handlers.NewApp(ctx, config)
	// TODO(aaronl): The global scope of the health checks means NewRegistry
	// can only be called once per process.
	app.RegisterHealthChecks()
	var handler http.Handler = app
	handler = alive("/", handler)
	handler = health.Handler(handler)
	handler = panicHandler(handler)
	if !config.Log.AccessLog.Disabled {
		handler = gorhandlers.CombinedLoggingHandler(os.Stdout, handler)
	}

	for _, applyHandlerMiddleware := range handlerMiddlewares {
		handler = applyHandlerMiddleware(config, handler)
	}

	err = tracing.InitOpenTelemetry(app.Context)
	if err != nil {
		return nil, fmt.Errorf("error during open telemetry initialization: %v", err)
	}
	if config.HTTP.H2C.Enabled {
		handler = h2c.NewHandler(handler, &http2.Server{})
	}
	handler = otelHandler(handler)

	server := &http.Server{
		Handler: handler,
	}

	return &Registry{
		app:    app,
		config: config,
		server: server,
		quit:   make(chan os.Signal, 1),
	}, nil
}

// otelHandler returns an http.Handler that wraps the provided `next` handler with OpenTelemetry instrumentation.
// This instrumentation tracks each HTTP request, creating spans with names derived from the request method and URL path.
func otelHandler(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return r.Method + " " + r.URL.Path }))
}

// takes a list of cipher suites and converts it to a list of respective tls constants
// if an empty list is provided, then the defaults will be used
func getCipherSuites(names []string) ([]uint16, error) {
	if len(names) == 0 {
		return defaultCipherSuites, nil
	}
	cipherSuiteConsts := make([]uint16, len(names))
	for i, name := range names {
		cipherSuiteConst, ok := cipherSuites[name]
		if !ok {
			return nil, fmt.Errorf("unknown TLS cipher suite '%s' specified for http.tls.cipherSuites", name)
		}
		cipherSuiteConsts[i] = cipherSuiteConst
	}
	return cipherSuiteConsts, nil
}

// takes a list of cipher suite ids and converts it to a list of respective names
func getCipherSuiteNames(ids []uint16) []string {
	if len(ids) == 0 {
		return nil
	}
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = tls.CipherSuiteName(id)
	}
	return names
}

// set ACME-server/DirectoryURL, if provided
func setDirectoryURL(directoryurl string) *acme.Client {
	if len(directoryurl) > 0 {
		return &acme.Client{DirectoryURL: directoryurl}
	}
	return nil
}

// ListenAndServe runs the registry's HTTP server.
func (registry *Registry) ListenAndServe() error {
	config := registry.config

	ln, err := listener.NewListener(config.HTTP.Net, config.HTTP.Addr)
	if err != nil {
		return err
	}

	if config.HTTP.TLS.Certificate != "" || config.HTTP.TLS.LetsEncrypt.CacheFile != "" {
		if config.HTTP.TLS.MinimumTLS == "" {
			config.HTTP.TLS.MinimumTLS = defaultTLSVersionStr
		}
		tlsMinVersion, ok := tlsVersions[config.HTTP.TLS.MinimumTLS]
		if !ok {
			return fmt.Errorf("unknown minimum TLS level '%s' specified for http.tls.minimumtls", config.HTTP.TLS.MinimumTLS)
		}
		dcontext.GetLogger(registry.app).Infof("restricting TLS version to %s or higher", config.HTTP.TLS.MinimumTLS)

		var tlsCipherSuites []uint16
		// configuring cipher suites are no longer supported after the tls1.3.
		// (https://go.dev/blog/tls-cipher-suites)
		if tlsMinVersion > tls.VersionTLS12 {
			dcontext.GetLogger(registry.app).Warnf("restricting TLS cipher suites to empty. Because configuring cipher suites is no longer supported in %s", config.HTTP.TLS.MinimumTLS)
		} else {
			tlsCipherSuites, err = getCipherSuites(config.HTTP.TLS.CipherSuites)
			if err != nil {
				return err
			}
			dcontext.GetLogger(registry.app).Infof("restricting TLS cipher suites to: %s", strings.Join(getCipherSuiteNames(tlsCipherSuites), ","))
		}

		tlsConf := &tls.Config{
			ClientAuth:   tls.NoClientCert,
			NextProtos:   nextProtos(config),
			MinVersion:   tlsMinVersion,
			CipherSuites: tlsCipherSuites,
		}

		if config.HTTP.TLS.LetsEncrypt.CacheFile != "" {
			if config.HTTP.TLS.Certificate != "" {
				return fmt.Errorf("cannot specify both certificate and Let's Encrypt")
			}
			m := &autocert.Manager{
				HostPolicy: autocert.HostWhitelist(config.HTTP.TLS.LetsEncrypt.Hosts...),
				Cache:      autocert.DirCache(config.HTTP.TLS.LetsEncrypt.CacheFile),
				Email:      config.HTTP.TLS.LetsEncrypt.Email,
				Prompt:     autocert.AcceptTOS,
				Client:     setDirectoryURL(config.HTTP.TLS.LetsEncrypt.DirectoryURL),
			}
			tlsConf.GetCertificate = m.GetCertificate
			tlsConf.NextProtos = append(tlsConf.NextProtos, acme.ALPNProto)
		} else {
			tlsConf.Certificates = make([]tls.Certificate, 1)
			tlsConf.Certificates[0], err = tls.LoadX509KeyPair(config.HTTP.TLS.Certificate, config.HTTP.TLS.Key)
			if err != nil {
				return err
			}
		}

		if len(config.HTTP.TLS.ClientCAs) != 0 {
			pool := x509.NewCertPool()

			for _, ca := range config.HTTP.TLS.ClientCAs {
				caPem, err := os.ReadFile(ca)
				if err != nil {
					return err
				}

				if ok := pool.AppendCertsFromPEM(caPem); !ok {
					return fmt.Errorf("could not add CA to pool")
				}
			}

			for _, subj := range pool.Subjects() { //nolint:staticcheck // FIXME(thaJeztah): ignore SA1019: ac.(*accessController).rootCerts.Subjects has been deprecated since Go 1.18: if s was returned by SystemCertPool, Subjects will not include the system roots. (staticcheck)
				dcontext.GetLogger(registry.app).Debugf("CA Subject: %s", string(subj))
			}

			tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
			tlsConf.ClientCAs = pool
		}

		ln = tls.NewListener(ln, tlsConf)
		dcontext.GetLogger(registry.app).Infof("listening on %v, tls", ln.Addr())
	} else {
		dcontext.GetLogger(registry.app).Infof("listening on %v", ln.Addr())
	}

	if config.HTTP.DrainTimeout == 0 {
		return registry.server.Serve(ln)
	}

	// setup channel to get notified on SIGTERM signal
	signal.Notify(registry.quit, os.Interrupt, syscall.SIGTERM)
	serveErr := make(chan error)

	// Start serving in goroutine and listen for stop signal in main thread
	go func() {
		serveErr <- registry.server.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		return err
	case <-registry.quit:
		dcontext.GetLogger(registry.app).Info("stopping server gracefully. Draining connections for ", config.HTTP.DrainTimeout)
		// shutdown the server with a grace period of configured timeout
		c, cancel := context.WithTimeout(context.Background(), config.HTTP.DrainTimeout)
		defer cancel()
		return registry.Shutdown(c)
	}
}

// Shutdown gracefully shuts down the registry's HTTP server and application object.
func (registry *Registry) Shutdown(ctx context.Context) error {
	err := registry.server.Shutdown(ctx)
	if appErr := registry.app.Shutdown(); appErr != nil {
		err = errors.Join(err, appErr)
	}
	return err
}

func configureDebugServer(config *configuration.Configuration) {
	if config.HTTP.Debug.Addr != "" {
		go func(addr string) {
			logrus.Infof("debug server listening %v", addr)
			if err := http.ListenAndServe(addr, nil); err != nil {
				logrus.Fatalf("error listening on debug interface: %v", err)
			}
		}(config.HTTP.Debug.Addr)
		configurePrometheus(config)
	}
}

func configurePrometheus(config *configuration.Configuration) {
	if config.HTTP.Debug.Prometheus.Enabled {
		path := config.HTTP.Debug.Prometheus.Path
		if path == "" {
			path = "/metrics"
		}
		logrus.Info("providing prometheus metrics on ", path)
		http.Handle(path, metrics.Handler())
	}
}

// configureLogging prepares the context with a logger using the
// configuration.
func configureLogging(ctx context.Context, config *configuration.Configuration) (context.Context, error) {
	logrus.SetLevel(logLevel(config.Log.Level))
	logrus.SetReportCaller(config.Log.ReportCaller)

	formatter := config.Log.Formatter
	if formatter == "" {
		formatter = defaultLogFormatter
	}

	switch formatter {
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat:   time.RFC3339Nano,
			DisableHTMLEscape: true,
		})
	case "text":
		logrus.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	case "logstash":
		logrus.SetFormatter(&logstash.LogstashFormatter{
			Formatter: &logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano},
		})
	default:
		return ctx, fmt.Errorf("unsupported logging formatter: %q", formatter)
	}

	logrus.Debugf("using %q logging formatter", formatter)
	if len(config.Log.Fields) > 0 {
		// build up the static fields, if present.
		var fields []interface{}
		for k := range config.Log.Fields {
			fields = append(fields, k)
		}

		ctx = dcontext.WithValues(ctx, config.Log.Fields)
		ctx = dcontext.WithLogger(ctx, dcontext.GetLogger(ctx, fields...))
	}

	dcontext.SetDefaultLogger(dcontext.GetLogger(ctx))
	return ctx, nil
}

func logLevel(level configuration.Loglevel) logrus.Level {
	l, err := logrus.ParseLevel(string(level))
	if err != nil {
		l = logrus.InfoLevel
		logrus.Warnf("error parsing level %q: %v, using %q	", level, err, l)
	}

	return l
}

// panicHandler add an HTTP handler to web app. The handler recover the happening
// panic. logrus.Panic transmits panic message to pre-config log hooks, which is
// defined in config.yml.
func panicHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logrus.Panic(fmt.Sprintf("%v", err))
			}
		}()
		handler.ServeHTTP(w, r)
	})
}

// alive simply wraps the handler with a route that always returns an http 200
// response when the path is matched. If the path is not matched, the request
// is passed to the provided handler. There is no guarantee of anything but
// that the server is up. Wrap with other handlers (such as health.Handler)
// for greater affect.
func alive(path string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			return
		}

		handler.ServeHTTP(w, r)
	})
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

func nextProtos(config *configuration.Configuration) []string {
	switch config.HTTP.HTTP2.Disabled {
	case true:
		return []string{"http/1.1"}
	default:
		return []string{"h2", "http/1.1"}
	}
}
