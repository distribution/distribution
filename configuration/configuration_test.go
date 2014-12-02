package configuration

import (
	"bytes"
	"os"
	"testing"

	. "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

// Hook up gocheck into the "go test" runner
func Test(t *testing.T) { TestingT(t) }

// configStruct is a canonical example configuration, which should map to configYamlV0_1
var configStruct = Configuration{
	Version:  "0.1",
	Loglevel: "info",
	Storage: Storage{
		"s3": Parameters{
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
}

// configYamlV0_1 is a Version 0.1 yaml document representing configStruct
var configYamlV0_1 = `
version: 0.1
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

// inmemoryConfigYamlV0_1 is a Version 0.1 yaml document specifying an inmemory storage driver with
// no parameters
var inmemoryConfigYamlV0_1 = `
version: 0.1
loglevel: info
storage: inmemory
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
	config, err := Parse(bytes.NewReader(configBytes))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseSimple validates that configYamlV0_1 can be parsed into a struct matching configStruct
func (suite *ConfigSuite) TestParseSimple(c *C) {
	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseInmemory validates that configuration yaml with storage provided as a string can be
// parsed into a Configuration struct with no storage parameters
func (suite *ConfigSuite) TestParseInmemory(c *C) {
	suite.expectedConfig.Storage = Storage{"inmemory": Parameters{}}

	config, err := Parse(bytes.NewReader([]byte(inmemoryConfigYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithSameEnvStorage validates that providing environment variables that match the given
// storage type and parameters will not alter the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithSameEnvStorage(c *C) {
	os.Setenv("REGISTRY_STORAGE", "s3")
	os.Setenv("REGISTRY_STORAGE_S3_REGION", "us-east-1")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvStorageParams validates that providing environment variables that change
// and add to the given storage parameters will change and add parameters to the parsed
// Configuration struct
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageParams(c *C) {
	suite.expectedConfig.Storage.setParameter("region", "us-west-1")
	suite.expectedConfig.Storage.setParameter("secure", "true")
	suite.expectedConfig.Storage.setParameter("newparam", "some Value")

	os.Setenv("REGISTRY_STORAGE_S3_REGION", "us-west-1")
	os.Setenv("REGISTRY_STORAGE_S3_SECURE", "true")
	os.Setenv("REGISTRY_STORAGE_S3_NEWPARAM", "some Value")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvStorageType validates that providing an environment variable that
// changes the storage type will be reflected in the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageType(c *C) {
	suite.expectedConfig.Storage = Storage{"inmemory": Parameters{}}

	os.Setenv("REGISTRY_STORAGE", "inmemory")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvStorageTypeAndParams validates that providing an environment variable
// that changes the storage type will be reflected in the parsed Configuration struct and that
// environment storage parameters will also be included
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageTypeAndParams(c *C) {
	suite.expectedConfig.Storage = Storage{"filesystem": Parameters{}}
	suite.expectedConfig.Storage.setParameter("rootdirectory", "/tmp/testroot")

	os.Setenv("REGISTRY_STORAGE", "filesystem")
	os.Setenv("REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", "/tmp/testroot")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithSameEnvLoglevel validates that providing an environment variable defining the log
// level to the same as the one provided in the yaml will not change the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithSameEnvLoglevel(c *C) {
	os.Setenv("REGISTRY_LOGLEVEL", "info")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvLoglevel validates that providing an environment variable defining the
// log level will override the value provided in the yaml document
func (suite *ConfigSuite) TestParseWithDifferentEnvLoglevel(c *C) {
	suite.expectedConfig.Loglevel = "error"

	os.Setenv("REGISTRY_LOGLEVEL", "error")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseInvalidVersion validates that the parser will fail to parse a newer configuration
// version than the CurrentVersion
func (suite *ConfigSuite) TestParseInvalidVersion(c *C) {
	suite.expectedConfig.Version = MajorMinorVersion(CurrentVersion.Major(), CurrentVersion.Minor()+1)
	configBytes, err := yaml.Marshal(suite.expectedConfig)
	c.Assert(err, IsNil)
	_, err = Parse(bytes.NewReader(configBytes))
	c.Assert(err, NotNil)
}

func copyConfig(config Configuration) *Configuration {
	configCopy := new(Configuration)

	configCopy.Version = MajorMinorVersion(config.Version.Major(), config.Version.Minor())
	configCopy.Loglevel = config.Loglevel
	configCopy.Storage = Storage{config.Storage.Type(): Parameters{}}
	for k, v := range config.Storage.Parameters() {
		configCopy.Storage.setParameter(k, v)
	}

	return configCopy
}
