package configuration

import (
	"os"
	"testing"

	"gopkg.in/yaml.v2"

	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner
func Test(t *testing.T) { TestingT(t) }

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

func (suite *ConfigSuite) TestMarshalRoundtrip(c *C) {
	configBytes, err := yaml.Marshal(suite.expectedConfig)
	c.Assert(err, IsNil)
	config, err := Parse(configBytes)
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

func (suite *ConfigSuite) TestParseSimple(c *C) {
	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

func (suite *ConfigSuite) TestParseWithSameEnvStorage(c *C) {
	os.Setenv("REGISTRY_STORAGE", "s3")
	os.Setenv("REGISTRY_STORAGE_S3_REGION", "us-east-1")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

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

func (suite *ConfigSuite) TestParseWithDifferentEnvStorageType(c *C) {
	suite.expectedConfig.Registry.Storage = Storage{Type: "inmemory", Parameters: map[string]string{}}

	os.Setenv("REGISTRY_STORAGE", "inmemory")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

func (suite *ConfigSuite) TestParseWithDifferentEnvStorageTypeAndParams(c *C) {
	suite.expectedConfig.Registry.Storage = Storage{Type: "filesystem", Parameters: map[string]string{}}
	suite.expectedConfig.Registry.Storage.Parameters["rootdirectory"] = "/tmp/testroot"

	os.Setenv("REGISTRY_STORAGE", "filesystem")
	os.Setenv("REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", "/tmp/testroot")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

func (suite *ConfigSuite) TestParseWithSameEnvLoglevel(c *C) {
	os.Setenv("REGISTRY_LOGLEVEL", "info")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

func (suite *ConfigSuite) TestParseWithDifferentEnvLoglevel(c *C) {
	suite.expectedConfig.Registry.LogLevel = "error"

	os.Setenv("REGISTRY_LOGLEVEL", "error")

	config, err := Parse([]byte(configYamlV_0_1))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

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
