package configuration

import (
	"bytes"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"
)

// configStruct is a canonical example configuration, which should map to configYamlV0_1
var configStruct = Configuration{
	Version: "0.1",
	Log: Log{
		Level:  "info",
		Fields: map[string]interface{}{"environment": "test"},
	},
	Storage: Storage{
		"somedriver": Parameters{
			"string1": "string-value1",
			"string2": "string-value2",
			"bool1":   true,
			"bool2":   false,
			"nil1":    nil,
			"int1":    42,
			"url1":    "https://foo.example.com",
			"path1":   "/some-path",
		},
		"tag": Parameters{
			"concurrencylimit": 10,
		},
	},
	Auth: Auth{
		"silly": Parameters{
			"realm":   "silly",
			"service": "silly",
		},
	},
	Notifications: Notifications{
		Endpoints: []Endpoint{
			{
				Name: "endpoint-1",
				URL:  "http://example.com",
				Headers: http.Header{
					"Authorization": []string{"Bearer <example>"},
				},
				IgnoredMediaTypes: []string{"application/octet-stream"},
				Ignore: Ignore{
					MediaTypes: []string{"application/octet-stream"},
					Actions:    []string{"pull"},
				},
			},
		},
	},
	Catalog: Catalog{
		MaxEntries: 1000,
	},
	HTTP: HTTP{
		TLS: TLS{
			ClientCAs:  []string{"/path/to/ca.pem"},
			ClientAuth: ClientAuthVerifyClientCertIfGiven,
		},
		Headers: http.Header{
			"X-Content-Type-Options": []string{"nosniff"},
		},
		HTTP2: HTTP2{
			Disabled: false,
		},
		H2C: H2C{
			Enabled: true,
		},
	},
	Redis: Redis{
		Options: RedisOptions{
			Addrs:           []string{"localhost:6379"},
			Username:        "alice",
			Password:        "123456",
			DB:              1,
			MaxIdleConns:    16,
			PoolSize:        64,
			ConnMaxIdleTime: time.Second * 300,
			DialTimeout:     time.Millisecond * 10,
			ReadTimeout:     time.Millisecond * 10,
			WriteTimeout:    time.Millisecond * 10,
		},
		TLS: RedisTLSOptions{
			Certificate: "/foo/cert.crt",
			Key:         "/foo/key.pem",
			ClientCAs:   []string{"/path/to/ca.pem"},
		},
	},
	Validation: Validation{
		Manifests: ValidationManifests{
			Indexes: ValidationIndexes{
				Platforms: "none",
			},
		},
	},
}

// configYamlV0_1 is a Version 0.1 yaml document representing configStruct
const configYamlV0_1 = `
version: 0.1
log:
  level: info
  fields:
    environment: test
storage:
  somedriver:
    string1: string-value1
    string2: string-value2
    bool1: true
    bool2: false
    nil1: ~
    int1: 42
    url1: "https://foo.example.com"
    path1: "/some-path"
  tag:
    concurrencylimit: 10
auth:
  silly:
    realm: silly
    service: silly
notifications:
  endpoints:
    - name: endpoint-1
      url:  http://example.com
      headers:
        Authorization: [Bearer <example>]
      ignoredmediatypes:
        - application/octet-stream
      ignore:
        mediatypes:
           - application/octet-stream
        actions:
           - pull
http:
  tls:
    clientcas:
      - /path/to/ca.pem
    clientauth: verify-client-cert-if-given
  headers:
    X-Content-Type-Options: [nosniff]
redis:
  tls:
    certificate: /foo/cert.crt
    key: /foo/key.pem
    clientcas:
      - /path/to/ca.pem
  addrs: [localhost:6379]
  username: alice
  password: "123456"
  db: 1
  maxidleconns: 16
  poolsize: 64
  connmaxidletime: 300s
  dialtimeout: 10ms
  readtimeout: 10ms
  writetimeout: 10ms
validation:
  manifests:
    indexes:
      platforms: none
`

// inmemoryConfigYamlV0_1 is a Version 0.1 yaml document specifying an inmemory
// storage driver with no parameters
const inmemoryConfigYamlV0_1 = `
version: 0.1
log:
  level: info
storage: inmemory
auth:
  silly:
    realm: silly
    service: silly
notifications:
  endpoints:
    - name: endpoint-1
      url:  http://example.com
      headers:
        Authorization: [Bearer <example>]
      ignoredmediatypes:
        - application/octet-stream
      ignore:
        mediatypes:
           - application/octet-stream
        actions:
           - pull
http:
  headers:
    X-Content-Type-Options: [nosniff]
validation:
  manifests:
    indexes:
      platforms: none
`

type ConfigSuite struct {
	suite.Suite
	expectedConfig *Configuration
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}

func (suite *ConfigSuite) SetupTest() {
	suite.expectedConfig = copyConfig(configStruct)
}

// TestMarshalRoundtrip validates that configStruct can be marshaled and
// unmarshaled without changing any parameters
func (suite *ConfigSuite) TestMarshalRoundtrip() {
	configBytes, err := yaml.Marshal(suite.expectedConfig)
	suite.Require().NoError(err)
	config, err := Parse(bytes.NewReader(configBytes))
	suite.T().Log(string(configBytes))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseSimple validates that configYamlV0_1 can be parsed into a struct
// matching configStruct
func (suite *ConfigSuite) TestParseSimple() {
	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseInmemory validates that configuration yaml with storage provided as
// a string can be parsed into a Configuration struct with no storage parameters
func (suite *ConfigSuite) TestParseInmemory() {
	suite.expectedConfig.Storage = Storage{"inmemory": Parameters{}}
	suite.expectedConfig.Log.Fields = nil
	suite.expectedConfig.HTTP.TLS.ClientCAs = nil
	suite.expectedConfig.HTTP.TLS.ClientAuth = ""
	suite.expectedConfig.Redis = Redis{}

	config, err := Parse(bytes.NewReader([]byte(inmemoryConfigYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseIncomplete validates that an incomplete yaml configuration cannot
// be parsed without providing environment variables to fill in the missing
// components.
func (suite *ConfigSuite) TestParseIncomplete() {
	incompleteConfigYaml := "version: 0.1"
	_, err := Parse(bytes.NewReader([]byte(incompleteConfigYaml)))
	suite.Require().Error(err)

	suite.expectedConfig.Log.Fields = nil
	suite.expectedConfig.Storage = Storage{"filesystem": Parameters{"rootdirectory": "/tmp/testroot"}}
	suite.expectedConfig.Auth = Auth{"silly": Parameters{"realm": "silly"}}
	suite.expectedConfig.Notifications = Notifications{}
	suite.expectedConfig.HTTP.Headers = nil
	suite.expectedConfig.HTTP.TLS.ClientCAs = nil
	suite.expectedConfig.HTTP.TLS.ClientAuth = ""
	suite.expectedConfig.Redis = Redis{}
	suite.expectedConfig.Validation.Manifests.Indexes.Platforms = ""

	// Note: this also tests that REGISTRY_STORAGE and
	// REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY can be used together
	suite.T().Setenv("REGISTRY_STORAGE", "filesystem")
	suite.T().Setenv("REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", "/tmp/testroot")
	suite.T().Setenv("REGISTRY_AUTH", "silly")
	suite.T().Setenv("REGISTRY_AUTH_SILLY_REALM", "silly")

	config, err := Parse(bytes.NewReader([]byte(incompleteConfigYaml)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseWithSameEnvStorage validates that providing environment variables
// that match the given storage type will only include environment-defined
// parameters and remove yaml-defined parameters
func (suite *ConfigSuite) TestParseWithSameEnvStorage() {
	suite.expectedConfig.Storage = Storage{"somedriver": Parameters{"region": "us-east-1"}}

	suite.T().Setenv("REGISTRY_STORAGE", "somedriver")
	suite.T().Setenv("REGISTRY_STORAGE_SOMEDRIVER_REGION", "us-east-1")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseWithDifferentEnvStorageParams validates that providing environment variables that change
// and add to the given storage parameters will change and add parameters to the parsed
// Configuration struct
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageParams() {
	suite.expectedConfig.Storage.setParameter("string1", "us-west-1")
	suite.expectedConfig.Storage.setParameter("bool1", true)
	suite.expectedConfig.Storage.setParameter("newparam", "some Value")

	suite.T().Setenv("REGISTRY_STORAGE_SOMEDRIVER_STRING1", "us-west-1")
	suite.T().Setenv("REGISTRY_STORAGE_SOMEDRIVER_BOOL1", "true")
	suite.T().Setenv("REGISTRY_STORAGE_SOMEDRIVER_NEWPARAM", "some Value")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseWithDifferentEnvStorageType validates that providing an environment variable that
// changes the storage type will be reflected in the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageType() {
	suite.expectedConfig.Storage = Storage{"inmemory": Parameters{}}

	suite.T().Setenv("REGISTRY_STORAGE", "inmemory")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseWithDifferentEnvStorageTypeAndParams validates that providing an environment variable
// that changes the storage type will be reflected in the parsed Configuration struct and that
// environment storage parameters will also be included
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageTypeAndParams() {
	suite.expectedConfig.Storage = Storage{"filesystem": Parameters{}}
	suite.expectedConfig.Storage.setParameter("rootdirectory", "/tmp/testroot")

	suite.T().Setenv("REGISTRY_STORAGE", "filesystem")
	suite.T().Setenv("REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", "/tmp/testroot")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseWithSameEnvLoglevel validates that providing an environment variable defining the log
// level to the same as the one provided in the yaml will not change the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithSameEnvLoglevel() {
	suite.T().Setenv("REGISTRY_LOGLEVEL", "info")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseWithDifferentEnvLoglevel validates that providing an environment variable defining the
// log level will override the value provided in the yaml document
func (suite *ConfigSuite) TestParseWithDifferentEnvLoglevel() {
	suite.expectedConfig.Log.Level = "error"

	suite.T().Setenv("REGISTRY_LOG_LEVEL", "error")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseInvalidLoglevel validates that the parser will fail to parse a
// configuration if the loglevel is malformed
func (suite *ConfigSuite) TestParseInvalidLoglevel() {
	invalidConfigYaml := "version: 0.1\nloglevel: derp\nstorage: inmemory"
	_, err := Parse(bytes.NewReader([]byte(invalidConfigYaml)))
	suite.Require().Error(err)

	suite.T().Setenv("REGISTRY_LOGLEVEL", "derp")

	_, err = Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().Error(err)
}

// TestParseInvalidVersion validates that the parser will fail to parse a newer configuration
// version than the CurrentVersion
func (suite *ConfigSuite) TestParseInvalidVersion() {
	suite.expectedConfig.Version = MajorMinorVersion(CurrentVersion.Major(), CurrentVersion.Minor()+1)
	configBytes, err := yaml.Marshal(suite.expectedConfig)
	suite.Require().NoError(err)
	_, err = Parse(bytes.NewReader(configBytes))
	suite.Require().Error(err)
}

// TestParseExtraneousVars validates that environment variables referring to
// nonexistent variables don't cause side effects.
func (suite *ConfigSuite) TestParseExtraneousVars() {
	// Environment variables which shouldn't set config items
	suite.T().Setenv("REGISTRY_DUCKS", "quack")
	suite.T().Setenv("REGISTRY_REPORTING_ASDF", "ghjk")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseEnvVarImplicitMaps validates that environment variables can set
// values in maps that don't already exist.
func (suite *ConfigSuite) TestParseEnvVarImplicitMaps() {
	readonly := make(map[string]interface{})
	readonly["enabled"] = true

	maintenance := make(map[string]interface{})
	maintenance["readonly"] = readonly

	suite.expectedConfig.Storage["maintenance"] = maintenance

	suite.T().Setenv("REGISTRY_STORAGE_MAINTENANCE_READONLY_ENABLED", "true")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

// TestParseEnvWrongTypeMap validates that incorrectly attempting to unmarshal a
// string over existing map fails.
func (suite *ConfigSuite) TestParseEnvWrongTypeMap() {
	suite.T().Setenv("REGISTRY_STORAGE_SOMEDRIVER", "somestring")

	_, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().Error(err)
}

// TestParseEnvWrongTypeStruct validates that incorrectly attempting to
// unmarshal a string into a struct fails.
func (suite *ConfigSuite) TestParseEnvWrongTypeStruct() {
	suite.T().Setenv("REGISTRY_STORAGE_LOG", "somestring")

	_, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().Error(err)
}

// TestParseEnvWrongTypeSlice validates that incorrectly attempting to
// unmarshal a string into a slice fails.
func (suite *ConfigSuite) TestParseEnvWrongTypeSlice() {
	suite.T().Setenv("REGISTRY_LOG_HOOKS", "somestring")

	_, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().Error(err)
}

// TestParseEnvMany tests several environment variable overrides.
// The result is not checked - the goal of this test is to detect panics
// from misuse of reflection.
func (suite *ConfigSuite) TestParseEnvMany() {
	suite.T().Setenv("REGISTRY_VERSION", "0.1")
	suite.T().Setenv("REGISTRY_LOG_LEVEL", "debug")
	suite.T().Setenv("REGISTRY_LOG_FORMATTER", "json")
	suite.T().Setenv("REGISTRY_LOG_HOOKS", "json")
	suite.T().Setenv("REGISTRY_LOG_FIELDS", "abc: xyz")
	suite.T().Setenv("REGISTRY_LOG_HOOKS", "- type: asdf")
	suite.T().Setenv("REGISTRY_LOGLEVEL", "debug")
	suite.T().Setenv("REGISTRY_STORAGE", "somedriver")
	suite.T().Setenv("REGISTRY_AUTH_PARAMS", "param1: value1")
	suite.T().Setenv("REGISTRY_AUTH_PARAMS_VALUE2", "value2")
	suite.T().Setenv("REGISTRY_AUTH_PARAMS_VALUE2", "value2")

	_, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
}

// TestParseEnvInlinedStruct tests whether environment variables are properly matched to fields in inlined structs.
func (suite *ConfigSuite) TestParseEnvInlinedStruct() {
	suite.expectedConfig.Redis.Options.Username = "bob"
	suite.expectedConfig.Redis.Options.Password = "password123"

	// Test without inlined struct name in the env variable name
	suite.T().Setenv("REGISTRY_REDIS_USERNAME", "bob")
	// Test with the inlined struct name in the env variable name, for backward compatibility
	suite.T().Setenv("REGISTRY_REDIS_OPTIONS_PASSWORD", "password123")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	suite.Require().NoError(err)
	suite.Require().Equal(suite.expectedConfig, config)
}

func checkStructs(tt *testing.T, t reflect.Type, structsChecked map[string]struct{}) {
	tt.Helper()

	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Map || t.Kind() == reflect.Slice {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return
	}
	if _, present := structsChecked[t.String()]; present {
		// Already checked this type
		return
	}

	structsChecked[t.String()] = struct{}{}

	byUpperCase := make(map[string]int)
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)

		// Check that the yaml tag does not contain an _.
		yamlTag := sf.Tag.Get("yaml")
		if strings.Contains(yamlTag, "_") {
			tt.Fatalf("yaml field name includes _ character: %s", yamlTag)
		}
		upper := strings.ToUpper(sf.Name)
		if _, present := byUpperCase[upper]; present {
			tt.Fatalf("field name collision in configuration object: %s", sf.Name)
		}
		byUpperCase[upper] = i

		checkStructs(tt, sf.Type, structsChecked)
	}
}

// TestValidateConfigStruct makes sure that the config struct has no members
// with yaml tags that would be ambiguous to the environment variable parser.
func (suite *ConfigSuite) TestValidateConfigStruct() {
	structsChecked := make(map[string]struct{})
	checkStructs(suite.T(), reflect.TypeOf(Configuration{}), structsChecked)
}

func copyConfig(config Configuration) *Configuration {
	configCopy := new(Configuration)

	configCopy.Version = MajorMinorVersion(config.Version.Major(), config.Version.Minor())
	configCopy.Loglevel = config.Loglevel
	configCopy.Log = config.Log
	configCopy.Catalog = config.Catalog
	configCopy.Log.Fields = make(map[string]interface{}, len(config.Log.Fields))
	for k, v := range config.Log.Fields {
		configCopy.Log.Fields[k] = v
	}

	configCopy.Storage = Storage{config.Storage.Type(): Parameters{}}
	for k, v := range config.Storage.Parameters() {
		configCopy.Storage.setParameter(k, v)
	}
	for k, v := range config.Storage.TagParameters() {
		configCopy.Storage.setTagParameter(k, v)
	}

	configCopy.Auth = Auth{config.Auth.Type(): Parameters{}}
	for k, v := range config.Auth.Parameters() {
		configCopy.Auth.setParameter(k, v)
	}

	configCopy.Notifications = Notifications{Endpoints: []Endpoint{}}
	configCopy.Notifications.Endpoints = append(configCopy.Notifications.Endpoints, config.Notifications.Endpoints...)

	configCopy.HTTP.Headers = make(http.Header)
	for k, v := range config.HTTP.Headers {
		configCopy.HTTP.Headers[k] = v
	}
	configCopy.HTTP.TLS.ClientCAs = make([]string, 0, len(config.HTTP.TLS.ClientCAs))
	configCopy.HTTP.TLS.ClientCAs = append(configCopy.HTTP.TLS.ClientCAs, config.HTTP.TLS.ClientCAs...)
	configCopy.HTTP.TLS.ClientAuth = config.HTTP.TLS.ClientAuth

	configCopy.Redis = config.Redis
	configCopy.Redis.TLS.Certificate = config.Redis.TLS.Certificate
	configCopy.Redis.TLS.Key = config.Redis.TLS.Key
	configCopy.Redis.TLS.ClientCAs = make([]string, 0, len(config.Redis.TLS.ClientCAs))
	configCopy.Redis.TLS.ClientCAs = append(configCopy.Redis.TLS.ClientCAs, config.Redis.TLS.ClientCAs...)

	configCopy.Validation = Validation{
		Enabled:   config.Validation.Enabled,
		Disabled:  config.Validation.Disabled,
		Manifests: config.Validation.Manifests,
	}

	return configCopy
}
