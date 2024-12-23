package configuration

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Configuration is a versioned registry configuration, intended to be provided by a yaml file, and
// optionally modified by environment variables.
//
// Note that yaml field names should never include _ characters, since this is the separator used
// in environment variable names.
type Configuration struct {
	// Version is the version which defines the format of the rest of the configuration
	Version Version `yaml:"version"`

	// Log supports setting various parameters related to the logging
	// subsystem.
	Log Log `yaml:"log"`

	// Loglevel is the level at which registry operations are logged.
	//
	// Deprecated: Use Log.Level instead.
	Loglevel Loglevel `yaml:"loglevel,omitempty"`

	// Storage is the configuration for the registry's storage driver
	Storage Storage `yaml:"storage"`

	// Auth allows configuration of various authorization methods that may be
	// used to gate requests.
	Auth Auth `yaml:"auth,omitempty"`

	// Middleware lists all middlewares to be used by the registry.
	Middleware map[string][]Middleware `yaml:"middleware,omitempty"`

	// HTTP contains configuration parameters for the registry's http
	// interface.
	HTTP HTTP `yaml:"http,omitempty"`

	// Notifications specifies configuration about various endpoint to which
	// registry events are dispatched.
	Notifications Notifications `yaml:"notifications,omitempty"`

	// Redis configures the redis pool available to the registry webapp.
	Redis Redis `yaml:"redis,omitempty"`

	// Health provides the configuration section for health checks.
	// It allows defining various checks to monitor the health of different subsystems.
	Health Health `yaml:"health,omitempty"`

	// Catalog is composed of MaxEntries.
	// Catalog endpoint (/v2/_catalog) configuration, it provides the configuration
	// options to control the maximum number of entries returned by the catalog endpoint.
	Catalog Catalog `yaml:"catalog,omitempty"`

	// Proxy defines the configuration options for using the registry as a pull-through cache.
	Proxy Proxy `yaml:"proxy,omitempty"`

	// Validation configures validation options for the registry.
	Validation Validation `yaml:"validation,omitempty"`

	// Policy configures registry policy options.
	Policy Policy `yaml:"policy,omitempty"`
}

// Policy defines configuration options for managing registry policies.
type Policy struct {
	// Repository configures policies for repositories
	Repository Repository `yaml:"repository,omitempty"`
}

// Repository defines configuration options related to repository policies in the registry.
type Repository struct {
	// Classes is a list of repository classes that the registry allows content for.
	// This value is matched against the media type in uploaded manifests.
	// If this field is non-empty, the registry enforces that all uploaded
	// content belongs to one of the specified classes.
	Classes []string `yaml:"classes"`
}

// Catalog provides configuration options for the /v2/_catalog endpoint.
type Catalog struct {
	// Max number of entries returned by the catalog endpoint. Requesting n entries
	// to the catalog endpoint will return at most MaxEntries entries.
	// An empty or a negative value will set a default of 1000 maximum entries by default.
	MaxEntries int `yaml:"maxentries,omitempty"`
}

// Log represents the configuration for logging within the application.
type Log struct {
	// AccessLog configures access logging.
	AccessLog AccessLog `yaml:"accesslog,omitempty"`

	// Level is the granularity at which registry operations are logged.
	Level Loglevel `yaml:"level,omitempty"`

	// Formatter overrides the default formatter with another. Options
	// include "text", "json" and "logstash".
	Formatter string `yaml:"formatter,omitempty"`

	// Fields allows users to specify static string fields to include in
	// the logger context.
	Fields map[string]interface{} `yaml:"fields,omitempty"`

	// Hooks allows users to configure the log hooks, to enabling the
	// sequent handling behavior, when defined levels of log message emit.
	Hooks []LogHook `yaml:"hooks,omitempty"`

	// ReportCaller allows user to configure the log to report the caller
	ReportCaller bool `yaml:"reportcaller,omitempty"`
}

// AccessLog configures options for access logging.
type AccessLog struct {
	// Disabled disables access logging.
	Disabled bool `yaml:"disabled,omitempty"`
}

// HTTP defines configuration options for the HTTP interface of the registry.
type HTTP struct {
	// Addr specifies the bind address for the registry instance.
	Addr string `yaml:"addr,omitempty"`

	// Net specifies the net portion of the bind address. A default empty value means tcp.
	Net string `yaml:"net,omitempty"`

	// Host specifies an externally-reachable address for the registry, as a fully
	// qualified URL.
	Host string `yaml:"host,omitempty"`

	// Prefix specifies a URL path prefix for the HTTP interface.
	// This can be used to serve the registry under a specific path
	// rather than at the root of the domain (e.g., "/registry").
	Prefix string `yaml:"prefix,omitempty"`

	// Secret specifies the secret key which HMAC tokens are created with.
	Secret string `yaml:"secret,omitempty"`

	// RelativeURLs specifies that relative URLs should be returned in
	// Location headers
	RelativeURLs bool `yaml:"relativeurls,omitempty"`

	// Amount of time to wait for connection to drain before shutting down when registry
	// receives a stop signal
	DrainTimeout time.Duration `yaml:"draintimeout,omitempty"`

	// TLS instructs the http server to listen with a TLS configuration.
	// This only support simple tls configuration with a cert and key.
	// Mostly, this is useful for testing situations or simple deployments
	// that require tls. If more complex configurations are required, use
	// a proxy or make a proposal to add support here.
	TLS TLS `yaml:"tls,omitempty"`

	// Headers is a set of headers to include in HTTP responses. A common
	// use case for this would be security headers such as
	// Strict-Transport-Security. The map keys are the header names, and
	// the values are the associated header payloads.
	Headers http.Header `yaml:"headers,omitempty"`

	// Debug configures the http debug interface, if specified. This can
	// include services such as pprof, expvar and other data that should
	// not be exposed externally. Left disabled by default.
	Debug Debug `yaml:"debug,omitempty"`

	// HTTP2 configures options for HTTP/2 support.
	HTTP2 HTTP2 `yaml:"http2,omitempty"`

	// H2C configures support for HTTP/2 without requiring TLS (HTTP/2 Cleartext).
	H2C H2C `yaml:"h2c,omitempty"`
}

// Debug defines the configuration options for the registry's debug interface.
// It allows administrators to enable or disable the debug server and configure
// telemetry and monitoring endpoints such as Prometheus.
type Debug struct {
	// Addr specifies the bind address for the debug server.
	Addr string `yaml:"addr,omitempty"`

	// Prometheus configures the Prometheus telemetry endpoint for monitoring purposes.
	Prometheus Prometheus `yaml:"prometheus,omitempty"`
}

// Prometheus configures the Prometheus telemetry endpoint for the registry.
// It allows administrators to enable Prometheus monitoring and customize
// the scrape path for metric collection.
type Prometheus struct {
	// Enabled determines whether Prometheus telemetry is enabled or not.
	Enabled bool `yaml:"enabled,omitempty"`

	// Path specifies the URL path where the Prometheus metrics are exposed.
	// The default is "/metrics", but it can be customized here.
	Path string `yaml:"path,omitempty"`
}

// HTTP2 configures options.
type HTTP2 struct {
	// Specifies whether the registry should disallow clients attempting
	// to connect via HTTP/2. If set to true, only HTTP/1.1 is supported.
	Disabled bool `yaml:"disabled,omitempty"`
}

// H2C configures support for HTTP/2 Cleartext.
type H2C struct {
	// Enables H2C (HTTP/2 Cleartext). Enable to support HTTP/2 without needing to configure TLS
	// Useful when deploying the registry behind a load balancer (e.g. Cloud Run)
	Enabled bool `yaml:"enabled,omitempty"`
}

// TLS defines the configuration options for enabling and configuring TLS (Transport Layer Security)
// for secure communication between the registry and clients. It allows the registry to listen for
// HTTPS connections with a specified certificate, key, and optional client authentication settings.
type TLS struct {
	// Certificate specifies the path to an x509 certificate file to
	// be used for TLS.
	Certificate string `yaml:"certificate,omitempty"`

	// Key specifies the path to the x509 key file, which should
	// contain the private portion for the file specified in
	// Certificate.
	Key string `yaml:"key,omitempty"`

	// Specifies the CA certs for client authentication
	// A file may contain multiple CA certificates encoded as PEM
	ClientCAs []string `yaml:"clientcas,omitempty"`

	// Client certificate authentication mode
	// One of: request-client-cert, require-any-client-cert, verify-client-cert-if-given, require-and-verify-client-cert
	ClientAuth ClientAuth `yaml:"clientauth,omitempty"`

	// Specifies the lowest TLS version allowed
	MinimumTLS string `yaml:"minimumtls,omitempty"`

	// Specifies a list of cipher suites allowed
	CipherSuites []string `yaml:"ciphersuites,omitempty"`

	// LetsEncrypt is used to configuration setting up TLS through
	// Let's Encrypt instead of manually specifying certificate and
	// key. If a TLS certificate is specified, the Let's Encrypt
	// section will not be used.
	LetsEncrypt LetsEncrypt `yaml:"letsencrypt,omitempty"`
}

// LetsEncrypt configures automatic TLS certificate provisioning using Let's Encrypt.
type LetsEncrypt struct {
	// CacheFile specifies cache file to use for lets encrypt
	// certificates and keys.
	CacheFile string `yaml:"cachefile,omitempty"`

	// Email is the email to use during Let's Encrypt registration
	Email string `yaml:"email,omitempty"`

	// Hosts specifies the hosts which are allowed to obtain Let's
	// Encrypt certificates.
	Hosts []string `yaml:"hosts,omitempty"`

	// DirectoryURL points to the CA directory endpoint.
	// If empty, LetsEncrypt is used.
	DirectoryURL string `yaml:"directoryurl,omitempty"`
}

// LogHook is composed of hook Level and Type.
// After hooks configuration, it can execute the next handling automatically,
// when defined levels of log message emitted.
// Example: hook can sending an email notification when error log happens in app.
type LogHook struct {
	// Disable lets user select to enable hook or not.
	Disabled bool `yaml:"disabled,omitempty"`

	// Type allows user to select which type of hook handler they want.
	Type string `yaml:"type,omitempty"`

	// Levels set which levels of log message will let hook executed.
	Levels []string `yaml:"levels,omitempty"`

	// MailOptions allows user to configure email parameters.
	MailOptions MailOptions `yaml:"options,omitempty"`
}

// MailOptions provides the configuration sections to user, for specific handler.
type MailOptions struct {
	// SMTP defines the configuration options for the SMTP server used for sending email notifications.
	SMTP SMTP `yaml:"smtp,omitempty"`

	// From defines mail sending address
	From string `yaml:"from,omitempty"`

	// To defines mail receiving address
	To []string `yaml:"to,omitempty"`
}

// SMTP represents the configuration for an SMTP (Simple Mail Transfer Protocol) server
// used for sending emails. It includes settings for the SMTP server's address, authentication,
// and other relevant configurations needed to connect and send emails.
type SMTP struct {
	// Addr defines smtp host address
	Addr string `yaml:"addr,omitempty"`

	// Username defines user name to smtp host
	Username string `yaml:"username,omitempty"`

	// Password defines password of login user
	Password string `yaml:"password,omitempty"`

	// Insecure defines if smtp login skips the secure certification.
	Insecure bool `yaml:"insecure,omitempty"`
}

// FileChecker is a type of entry in the health section for checking files.
type FileChecker struct {
	// Interval is the duration in between checks
	Interval time.Duration `yaml:"interval,omitempty"`

	// File is the path to check
	File string `yaml:"file,omitempty"`

	// Threshold is the number of times a check must fail to trigger an
	// unhealthy state
	Threshold int `yaml:"threshold,omitempty"`
}

// HTTPChecker is a type of entry in the health section for checking HTTP URIs.
type HTTPChecker struct {
	// Timeout is the duration to wait before timing out the HTTP request
	Timeout time.Duration `yaml:"timeout,omitempty"`

	// StatusCode is the expected status code
	StatusCode int

	// Interval is the duration in between checks
	Interval time.Duration `yaml:"interval,omitempty"`

	// URI is the HTTP URI to check
	URI string `yaml:"uri,omitempty"`

	// Headers lists static headers that should be added to all requests
	Headers http.Header `yaml:"headers"`

	// Threshold is the number of times a check must fail to trigger an
	// unhealthy state
	Threshold int `yaml:"threshold,omitempty"`
}

// TCPChecker is a type of entry in the health section for checking TCP servers.
type TCPChecker struct {
	// Timeout is the duration to wait before timing out the TCP connection
	Timeout time.Duration `yaml:"timeout,omitempty"`

	// Interval is the duration in between checks
	Interval time.Duration `yaml:"interval,omitempty"`

	// Addr is the TCP address to check
	Addr string `yaml:"addr,omitempty"`

	// Threshold is the number of times a check must fail to trigger an
	// unhealthy state
	Threshold int `yaml:"threshold,omitempty"`
}

// Health provides the configuration section for health checks.
type Health struct {
	// FileCheckers is a list of paths to check
	FileCheckers []FileChecker `yaml:"file,omitempty"`

	// HTTPCheckers is a list of URIs to check
	HTTPCheckers []HTTPChecker `yaml:"http,omitempty"`

	// TCPCheckers is a list of URIs to check
	TCPCheckers []TCPChecker `yaml:"tcp,omitempty"`

	// StorageDriver configures a health check on the configured storage
	// driver
	StorageDriver StorageDriver `yaml:"storagedriver,omitempty"`
}

// StorageDriver configures health checks specific to the storage driver.
type StorageDriver struct {
	// Enabled turns on the health check for the storage driver
	Enabled bool `yaml:"enabled,omitempty"`

	// Interval is the duration in between checks
	Interval time.Duration `yaml:"interval,omitempty"`

	// Threshold is the number of times a check must fail to trigger an
	// unhealthy state
	Threshold int `yaml:"threshold,omitempty"`
}

// Platform specifies the characteristics of a computing environment
// and allows registry administrators to define required platforms for image validation.
// Administrators can select specific architectures and operating systems that must exist
// in the registry. This ensures that all image indexes uploaded to the registry are valid
// for the specified platforms.
type Platform struct {
	// Architecture is the architecture for this platform
	Architecture string `yaml:"architecture,omitempty"`

	// OS is the operating system for this platform
	OS string `yaml:"os,omitempty"`
}

// v0_1Configuration is a Version 0.1 Configuration struct
// This is currently aliased to Configuration, as it is the current version
type v0_1Configuration Configuration

// UnmarshalYAML implements the yaml.Unmarshaler interface
// Unmarshals a string of the form X.Y into a Version, validating that X and Y can represent unsigned integers
func (version *Version) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var versionString string
	err := unmarshal(&versionString)
	if err != nil {
		return err
	}

	newVersion := Version(versionString)
	if _, err := newVersion.major(); err != nil {
		return err
	}

	if _, err := newVersion.minor(); err != nil {
		return err
	}

	*version = newVersion
	return nil
}

// CurrentVersion is the most recent Version that can be parsed
var CurrentVersion = MajorMinorVersion(0, 1)

// Loglevel is the level at which operations are logged
// This can be error, warn, info, or debug
type Loglevel string

// UnmarshalYAML implements the yaml.Umarshaler interface
// Unmarshals a string into a Loglevel, lowercasing the string and validating that it represents a
// valid loglevel
func (loglevel *Loglevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var loglevelString string
	err := unmarshal(&loglevelString)
	if err != nil {
		return err
	}

	loglevelString = strings.ToLower(loglevelString)
	switch loglevelString {
	case "error", "warn", "info", "debug":
	default:
		return fmt.Errorf("invalid loglevel %s Must be one of [error, warn, info, debug]", loglevelString)
	}

	*loglevel = Loglevel(loglevelString)
	return nil
}

// Parameters defines a key-value parameters mapping
type Parameters map[string]interface{}

// Storage defines the configuration for registry object storage
type Storage map[string]Parameters

// Type returns the storage driver type, such as filesystem or s3
func (storage Storage) Type() string {
	var storageType []string

	// Return only key in this map
	for k := range storage {
		switch k {
		case "maintenance":
			// allow configuration of maintenance
		case "cache":
			// allow configuration of caching
		case "delete":
			// allow configuration of delete
		case "redirect":
			// allow configuration of redirect
		case "tag":
			// allow configuration of tag
		default:
			storageType = append(storageType, k)
		}
	}
	if len(storageType) > 1 {
		panic("multiple storage drivers specified in configuration or environment: " + strings.Join(storageType, ", "))
	}
	if len(storageType) == 1 {
		return storageType[0]
	}
	return ""
}

// TagParameters returns the Parameters map for a Storage tag configuration
func (storage Storage) TagParameters() Parameters {
	return storage["tag"]
}

// setTagParameter changes the parameter at the provided key to the new value
func (storage Storage) setTagParameter(key string, value interface{}) {
	if _, ok := storage["tag"]; !ok {
		storage["tag"] = make(Parameters)
	}
	storage["tag"][key] = value
}

// Parameters returns the Parameters map for a Storage configuration
func (storage Storage) Parameters() Parameters {
	return storage[storage.Type()]
}

// setParameter changes the parameter at the provided key to the new value
func (storage Storage) setParameter(key string, value interface{}) {
	storage[storage.Type()][key] = value
}

// UnmarshalYAML implements the yaml.Unmarshaler interface
// Unmarshals a single item map into a Storage or a string into a Storage type with no parameters
func (storage *Storage) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var storageMap map[string]Parameters
	err := unmarshal(&storageMap)
	if err == nil {
		if len(storageMap) > 1 {
			types := make([]string, 0, len(storageMap))
			for k := range storageMap {
				switch k {
				case "maintenance":
					// allow for configuration of maintenance
				case "cache":
					// allow configuration of caching
				case "delete":
					// allow configuration of delete
				case "redirect":
					// allow configuration of redirect
				case "tag":
					// allow configuration of tag
				default:
					types = append(types, k)
				}
			}

			if len(types) > 1 {
				return fmt.Errorf("must provide exactly one storage type. Provided: %v", types)
			}
		}
		*storage = storageMap
		return nil
	}

	var storageType string
	err = unmarshal(&storageType)
	if err == nil {
		*storage = Storage{storageType: Parameters{}}
		return nil
	}

	return err
}

// MarshalYAML implements the yaml.Marshaler interface
func (storage Storage) MarshalYAML() (interface{}, error) {
	if storage.Parameters() == nil {
		return storage.Type(), nil
	}
	return map[string]Parameters(storage), nil
}

// Auth defines the configuration for registry authorization.
type Auth map[string]Parameters

// Type returns the auth type, such as htpasswd or token
func (auth Auth) Type() string {
	// Return only key in this map
	for k := range auth {
		return k
	}
	return ""
}

// Parameters returns the Parameters map for an Auth configuration
func (auth Auth) Parameters() Parameters {
	return auth[auth.Type()]
}

// setParameter changes the parameter at the provided key to the new value
func (auth Auth) setParameter(key string, value interface{}) {
	auth[auth.Type()][key] = value
}

// UnmarshalYAML implements the yaml.Unmarshaler interface
// Unmarshals a single item map into a Storage or a string into a Storage type with no parameters
func (auth *Auth) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var m map[string]Parameters
	err := unmarshal(&m)
	if err == nil {
		if len(m) > 1 {
			types := make([]string, 0, len(m))
			for k := range m {
				types = append(types, k)
			}

			// TODO(stevvooe): May want to change this slightly for
			// authorization to allow multiple challenges.
			return fmt.Errorf("must provide exactly one type. Provided: %v", types)

		}
		*auth = m
		return nil
	}

	var authType string
	err = unmarshal(&authType)
	if err == nil {
		*auth = Auth{authType: Parameters{}}
		return nil
	}

	return err
}

// MarshalYAML implements the yaml.Marshaler interface
func (auth Auth) MarshalYAML() (interface{}, error) {
	if auth.Parameters() == nil {
		return auth.Type(), nil
	}
	return map[string]Parameters(auth), nil
}

// Notifications configures multiple http endpoints.
type Notifications struct {
	// EventConfig is the configuration for the event format that is sent to each Endpoint.
	EventConfig Events `yaml:"events,omitempty"`
	// Endpoints is a list of http configurations for endpoints that
	// respond to webhook notifications. In the future, we may allow other
	// kinds of endpoints, such as external queues.
	Endpoints []Endpoint `yaml:"endpoints,omitempty"`
}

// Endpoint describes the configuration of an http webhook notification
// endpoint.
type Endpoint struct {
	Name              string        `yaml:"name"`              // identifies the endpoint in the registry instance.
	Disabled          bool          `yaml:"disabled"`          // disables the endpoint
	URL               string        `yaml:"url"`               // post url for the endpoint.
	Headers           http.Header   `yaml:"headers"`           // static headers that should be added to all requests
	Timeout           time.Duration `yaml:"timeout"`           // HTTP timeout
	Threshold         int           `yaml:"threshold"`         // circuit breaker threshold before backing off on failure
	Backoff           time.Duration `yaml:"backoff"`           // backoff duration
	IgnoredMediaTypes []string      `yaml:"ignoredmediatypes"` // target media types to ignore
	Ignore            Ignore        `yaml:"ignore"`            // ignore event types
}

// Events configures notification events.
type Events struct {
	IncludeReferences bool `yaml:"includereferences"` // include reference data in manifest events
}

// Ignore configures mediaTypes and actions of the event, that it won't be propagated
type Ignore struct {
	MediaTypes []string `yaml:"mediatypes"` // target media types to ignore
	Actions    []string `yaml:"actions"`    // ignore action types
}

// Middleware configures named middlewares to be applied at injection points.
type Middleware struct {
	// Name the middleware registers itself as
	Name string `yaml:"name"`
	// Flag to disable middleware easily
	Disabled bool `yaml:"disabled,omitempty"`
	// Map of parameters that will be passed to the middleware's initialization function
	Options Parameters `yaml:"options"`
}

// Proxy configures the registry as a pull through cache
type Proxy struct {
	// RemoteURL is the URL of the remote registry
	RemoteURL string `yaml:"remoteurl"`

	// Username of the hub user
	Username string `yaml:"username"`

	// Password of the hub user
	Password string `yaml:"password"`

	// Exec specifies a custom exec-based command to retrieve credentials.
	// If set, Username and Password are ignored.
	Exec *ExecConfig `yaml:"exec,omitempty"`

	// TTL is the expiry time of the content and will be cleaned up when it expires
	// if not set, defaults to 7 * 24 hours
	// If set to zero, will never expire cache
	TTL *time.Duration `yaml:"ttl,omitempty"`
}

// ExecConfig defines the configuration for executing a command as a credential helper.
// This allows the registry to authenticate against an upstream registry by executing a
// specified command to obtain credentials. The command can be re-executed based on the
// configured lifetime, enabling the registry to run as a pull-through cache that manages
// its authentication dynamically.
type ExecConfig struct {
	// Command is the command to execute.
	Command string `yaml:"command"`

	// Lifetime is the expiry period of the credentials. The credentials
	// returned by the command is reused through the configured lifetime, then
	// the command will be re-executed to retrieve new credentials.
	// If set to zero, the command will be executed for every request.
	// If not set, the command will only be executed once.
	Lifetime *time.Duration `yaml:"lifetime,omitempty"`
}

// Validation configures validation options for the registry.
type Validation struct {
	// Enabled enables the other options in this section. This field is
	// deprecated in favor of Disabled.
	Enabled bool `yaml:"enabled,omitempty"`

	// Disabled disables the other options in this section.
	Disabled bool `yaml:"disabled,omitempty"`

	// Manifests configures manifest validation.
	Manifests ValidationManifests `yaml:"manifests,omitempty"`
}

// ValidationManifests configures validation rules for manifests pushed to the registry.
type ValidationManifests struct {
	// URLs configures validation for URLs in pushed manifests.
	URLs URLs `yaml:"urls,omitempty"`

	// ImageIndexes configures validation of image indexes
	Indexes ValidationIndexes `yaml:"indexes,omitempty"`
}

// URLs defines validation rules for URLs found in the manifests pushed to the registry.
type URLs struct {
	// Allow specifies regular expressions (https://godoc.org/regexp/syntax)
	// that URLs in pushed manifests must match.
	Allow []string `yaml:"allow,omitempty"`

	// Deny specifies regular expressions (https://godoc.org/regexp/syntax)
	// that URLs in pushed manifests must not match.
	Deny []string `yaml:"deny,omitempty"`
}

// ValidationIndexes configures validation rules for image indexes within the manifest.
type ValidationIndexes struct {
	// Platforms configures the validation applies to the platform images included in an image index
	Platforms Platforms `yaml:"platforms"`

	// PlatformList filters the set of platforms to validate for image existence.
	PlatformList []Platform `yaml:"platformlist,omitempty"`
}

// Platforms configures the validation applies to the platform images included in an image index
// This can be all, none, or list
type Platforms string

// UnmarshalYAML implements the yaml.Umarshaler interface
// Unmarshals a string into a Platforms option, lowercasing the string and validating that it represents a
// valid option
func (platforms *Platforms) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var platformsString string
	err := unmarshal(&platformsString)
	if err != nil {
		return err
	}

	platformsString = strings.ToLower(platformsString)
	switch platformsString {
	case "all", "none", "list":
	default:
		return fmt.Errorf("invalid platforms option %s Must be one of [all, none, list]", platformsString)
	}

	*platforms = Platforms(platformsString)
	return nil
}

// Parse parses an input configuration yaml document into a Configuration struct
// This should generally be capable of handling old configuration format versions
//
// Environment variables may be used to override configuration parameters other than version,
// following the scheme below:
// Configuration.Abc may be replaced by the value of REGISTRY_ABC,
// Configuration.Abc.Xyz may be replaced by the value of REGISTRY_ABC_XYZ, and so forth
func Parse(rd io.Reader) (*Configuration, error) {
	in, err := io.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	p := NewParser("registry", []VersionedParseInfo{
		{
			Version: MajorMinorVersion(0, 1),
			ParseAs: reflect.TypeOf(v0_1Configuration{}),
			ConversionFunc: func(c interface{}) (interface{}, error) {
				if v0_1, ok := c.(*v0_1Configuration); ok {
					if v0_1.Log.Level == Loglevel("") {
						if v0_1.Loglevel != Loglevel("") {
							v0_1.Log.Level = v0_1.Loglevel
						} else {
							v0_1.Log.Level = Loglevel("info")
						}
					}
					if v0_1.Loglevel != Loglevel("") {
						v0_1.Loglevel = Loglevel("")
					}

					if v0_1.Catalog.MaxEntries <= 0 {
						v0_1.Catalog.MaxEntries = 1000
					}

					if v0_1.Storage.Type() == "" {
						return nil, errors.New("no storage configuration provided")
					}
					return (*Configuration)(v0_1), nil
				}
				return nil, fmt.Errorf("expected *v0_1Configuration, received %#v", c)
			},
		},
	})

	config := new(Configuration)
	err = p.Parse(in, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// RedisOptions represents the configuration options for Redis, which are
// provided by the redis package. This struct can be used to configure the
// connection to Redis in a universal (clustered or standalone) setup.
type RedisOptions = redis.UniversalOptions

// RedisTLSOptions configures the TLS (Transport Layer Security) settings for
// Redis connections, allowing secure communication over the network.
type RedisTLSOptions struct {
	// Certificate specifies the path to the certificate file for TLS authentication.
	// This certificate is used to establish a secure connection with the Redis server.
	Certificate string `yaml:"certificate,omitempty"`

	// Key specifies the path to the private key file associated with the certificate.
	// This key is used to authenticate the client during the TLS handshake.
	Key string `yaml:"key,omitempty"`

	// ClientCAs specifies a list of certificates to be used to verify the server's
	// certificate during the TLS handshake. This can be used for mutual TLS authentication.
	ClientCAs []string `yaml:"clientcas,omitempty"`
}

// Redis represents the configuration for connecting to a Redis server. It includes
// both the basic connection options and optional TLS settings to secure the connection.
type Redis struct {
	// Options provides the configuration for connecting to Redis, including
	// options for both clustered and standalone Redis setups. It is provided inline
	// from the `redis.UniversalOptions` struct.
	Options RedisOptions `yaml:",inline"`

	// TLS contains the TLS settings for secure communication with the Redis server.
	// If specified, these settings will enable encryption and authentication via TLS.
	TLS RedisTLSOptions `yaml:"tls,omitempty"`
}

func (c Redis) MarshalYAML() (interface{}, error) {
	fields := make(map[string]interface{})

	val := reflect.ValueOf(c.Options)
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldValue := val.Field(i)

		// ignore funcs fields in redis.UniversalOptions
		if fieldValue.Kind() == reflect.Func {
			continue
		}

		fields[strings.ToLower(field.Name)] = fieldValue.Interface()
	}

	// Add TLS fields if they're not empty
	if c.TLS.Certificate != "" || c.TLS.Key != "" || len(c.TLS.ClientCAs) > 0 {
		fields["tls"] = c.TLS
	}

	return fields, nil
}

func (c *Redis) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var fields map[string]interface{}
	err := unmarshal(&fields)
	if err != nil {
		return err
	}

	val := reflect.ValueOf(&c.Options).Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldName := strings.ToLower(field.Name)

		if value, ok := fields[fieldName]; ok {
			fieldValue := val.Field(i)
			if fieldValue.CanSet() {
				switch field.Type {
				case reflect.TypeOf(time.Duration(0)):
					durationStr, ok := value.(string)
					if !ok {
						return fmt.Errorf("invalid duration value for field: %s", fieldName)
					}
					duration, err := time.ParseDuration(durationStr)
					if err != nil {
						return fmt.Errorf("failed to parse duration for field: %s, error: %v", fieldName, err)
					}
					fieldValue.Set(reflect.ValueOf(duration))
				default:
					if err := setFieldValue(fieldValue, value); err != nil {
						return fmt.Errorf("failed to set value for field: %s, error: %v", fieldName, err)
					}
				}
			}
		}
	}

	// Handle TLS fields
	if tlsData, ok := fields["tls"]; ok {
		tlsMap, ok := tlsData.(map[interface{}]interface{})
		if !ok {
			return fmt.Errorf("invalid TLS data structure")
		}

		if cert, ok := tlsMap["certificate"]; ok {
			var isString bool
			c.TLS.Certificate, isString = cert.(string)
			if !isString {
				return fmt.Errorf("Redis TLS certificate must be a string")
			}
		}
		if key, ok := tlsMap["key"]; ok {
			var isString bool
			c.TLS.Key, isString = key.(string)
			if !isString {
				return fmt.Errorf("Redis TLS (private) key must be a string")
			}
		}
		if cas, ok := tlsMap["clientcas"]; ok {
			caList, ok := cas.([]interface{})
			if !ok {
				return fmt.Errorf("invalid clientcas data structure")
			}
			for _, ca := range caList {
				if caStr, ok := ca.(string); ok {
					c.TLS.ClientCAs = append(c.TLS.ClientCAs, caStr)
				}
			}
		}
	}

	return nil
}

func setFieldValue(field reflect.Value, value interface{}) error {
	if value == nil {
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		stringValue, ok := value.(string)
		if !ok {
			return fmt.Errorf("failed to convert value to string")
		}
		field.SetString(stringValue)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intValue, ok := value.(int)
		if !ok {
			return fmt.Errorf("failed to convert value to integer")
		}
		field.SetInt(int64(intValue))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintValue, ok := value.(uint)
		if !ok {
			return fmt.Errorf("failed to convert value to unsigned integer")
		}
		field.SetUint(uint64(uintValue))
	case reflect.Float32, reflect.Float64:
		floatValue, ok := value.(float64)
		if !ok {
			return fmt.Errorf("failed to convert value to float")
		}
		field.SetFloat(floatValue)
	case reflect.Bool:
		boolValue, ok := value.(bool)
		if !ok {
			return fmt.Errorf("failed to convert value to boolean")
		}
		field.SetBool(boolValue)
	case reflect.Slice:
		slice := reflect.MakeSlice(field.Type(), 0, 0)
		valueSlice, ok := value.([]interface{})
		if !ok {
			return fmt.Errorf("failed to convert value to slice")
		}
		for _, item := range valueSlice {
			sliceValue := reflect.New(field.Type().Elem()).Elem()
			if err := setFieldValue(sliceValue, item); err != nil {
				return err
			}
			slice = reflect.Append(slice, sliceValue)
		}
		field.Set(slice)
	default:
		return fmt.Errorf("unsupported field type: %v", field.Type())
	}
	return nil
}

const (
	ClientAuthRequestClientCert          = "request-client-cert"
	ClientAuthRequireAnyClientCert       = "require-any-client-cert"
	ClientAuthVerifyClientCertIfGiven    = "verify-client-cert-if-given"
	ClientAuthRequireAndVerifyClientCert = "require-and-verify-client-cert"
)

type ClientAuth string

// UnmarshalYAML implements the yaml.Umarshaler interface
// Unmarshals a string into a ClientAuth, validating that it represents a valid ClientAuth mod
func (clientAuth *ClientAuth) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var clientAuthString string
	err := unmarshal(&clientAuthString)
	if err != nil {
		return err
	}

	switch clientAuthString {
	case ClientAuthRequestClientCert:
	case ClientAuthRequireAnyClientCert:
	case ClientAuthVerifyClientCertIfGiven:
	case ClientAuthRequireAndVerifyClientCert:
	default:
		return fmt.Errorf("invalid ClientAuth %s Must be one of: %s, %s, %s, %s", clientAuthString, ClientAuthRequestClientCert, ClientAuthRequireAnyClientCert, ClientAuthVerifyClientCertIfGiven, ClientAuthRequireAndVerifyClientCert)
	}

	*clientAuth = ClientAuth(clientAuthString)

	return nil
}
