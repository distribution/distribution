package configuration

import (
	"os"
	"testing"

	"gopkg.in/yaml.v2"

	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner
func Test(t *testing.T) { TestingT(t) }

// configStruct is a canonical example configuration, which should map to configYamlV_0_1
var configStruct = Configuration{
	Version: Version{
		Major: 0,
		Minor: 1,
	},
	Registry: Registry{
		LogLevel: "info",
		Storage: Storage{
			Type: "s3",
			Parameters: map[string]string{
				"region":    "us-east-1",
				"bucket":    "my-bucket",
				"rootpath":  "/registry",
				"encrypt":   "true",
				"secure":    "false",
				"accesskey": "SAMPLEACCESSKEY",
				"secretkey": "SUPERSECRET",
				"host":      "",
				"port":      "",
			},
		},
	},
}

// configYamlV_0_1 is a Version{0, 1} yaml document representing configStruct
var configYamlV_0_1 = `
version: 0.1

registry:
  loglevel: info
  storage:
    s3:
      region: us-east-1
      bucket: my-bucket
      rootpath: /registry
      encrypt: true
      secure: false
      accesskey: SAMPLEACCESSKEY
      secretkey: SUPERSECRET
      host: ~
      port: ~
`

type ConfigSuite struct {
	expectedConfig *Configuration
}

var _ = Suite(new(ConfigSuite))

func (suite *ConfigSuite) SetUpTest(c *C) {
	os.Clearenv()
	suite.expectedConfig = copyConfig(configStruct)
}

// TestMarshalRoundtrip validates that configStruct can be marshaled and unmarshaled without
// changing any parameters
func (suite *ConfigSuite) TestMarshalRoundtrip(c *C) {
	configBytes, err := yaml.Marshal(suite.expectedConfig)
	c.Assert(err, IsNil)
	config, err := Parse(configBytes)
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseSimple validates that configYamlV_0_1 can be parsed into a struct matching configStruct
func (suite *ConfigSuite) TestParseSimple(c *C) {
	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithSameEnvStorage validates that providing environment variables that match the given
// storage type and parameters will not alter the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithSameEnvStorage(c *C) {
	os.Setenv("REGISTRY_STORAGE", "s3")
	os.Setenv("REGISTRY_STORAGE_S3_REGION", "us-east-1")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvStorageParams validates that providing environment variables that change
// and add to the given storage parameters will change and add parameters to the parsed
// Configuration struct
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageParams(c *C) {
	suite.expectedConfig.Registry.Storage.Parameters["region"] = "us-west-1"
	suite.expectedConfig.Registry.Storage.Parameters["secure"] = "true"
	suite.expectedConfig.Registry.Storage.Parameters["newparam"] = "some Value"

	os.Setenv("REGISTRY_STORAGE_S3_REGION", "us-west-1")
	os.Setenv("REGISTRY_STORAGE_S3_SECURE", "true")
	os.Setenv("REGISTRY_STORAGE_S3_NEWPARAM", "some Value")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvStorageType validates that providing an environment variable that
// changes the storage type will be reflected in the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageType(c *C) {
	suite.expectedConfig.Registry.Storage = Storage{Type: "inmemory", Parameters: map[string]string{}}

	os.Setenv("REGISTRY_STORAGE", "inmemory")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvStorageTypeAndParams validates that providing an environment variable
// that changes the storage type will be reflected in the parsed Configuration struct and that
// environment storage parameters will also be included
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageTypeAndParams(c *C) {
	suite.expectedConfig.Registry.Storage = Storage{Type: "filesystem", Parameters: map[string]string{}}
	suite.expectedConfig.Registry.Storage.Parameters["rootdirectory"] = "/tmp/testroot"

	os.Setenv("REGISTRY_STORAGE", "filesystem")
	os.Setenv("REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", "/tmp/testroot")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithSameEnvLoglevel validates that providing an environment variable defining the log
// level to the same as the one provided in the yaml will not change the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithSameEnvLoglevel(c *C) {
	os.Setenv("REGISTRY_LOGLEVEL", "info")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvLoglevel validates that providing an environment variable defining the
// log level will override the value provided in the yaml document
func (suite *ConfigSuite) TestParseWithDifferentEnvLoglevel(c *C) {
	suite.expectedConfig.Registry.LogLevel = "error"

	os.Setenv("REGISTRY_LOGLEVEL", "error")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseInvalidVersion validates that the parser will fail to parse a newer configuration
// version than the CurrentVersion
func (suite *ConfigSuite) TestParseInvalidVersion(c *C) {
	suite.expectedConfig.Version = Version{Major: CurrentVersion.Major, Minor: CurrentVersion.Minor + 1}
	configBytes, err := yaml.Marshal(suite.expectedConfig)
	c.Assert(err, IsNil)
	_, err = Parse(configBytes)
	c.Assert(err, NotNil)
}

func copyConfig(config Configuration) *Configuration {
	configCopy := new(Configuration)

	configCopy.Version = *new(Version)
	configCopy.Version.Major = config.Version.Major
	configCopy.Version.Minor = config.Version.Minor

	configCopy.Registry = *new(Registry)
	configCopy.Registry.LogLevel = config.Registry.LogLevel

	configCopy.Registry.Storage = *new(Storage)
	configCopy.Registry.Storage.Type = config.Registry.Storage.Type
	configCopy.Registry.Storage.Parameters = make(map[string]string)
	for k, v := range config.Registry.Storage.Parameters {
		configCopy.Registry.Storage.Parameters[k] = v
	}

	return configCopy
}
