package configuration

import (
	"os"
	"reflect"

	. "gopkg.in/check.v1"
)

type localConfiguration struct {
	Version Version `yaml:"version"`
	Log     *Log    `yaml:"log"`
}

type Log struct {
	Formatter string `yaml:"formatter,omitempty"`
}

var expectedConfig = localConfiguration{
	Version: "0.1",
	Log: &Log{
		Formatter: "json",
	},
}

type ParserSuite struct{}

var _ = Suite(new(ParserSuite))

func (suite *ParserSuite) TestParserOverwriteIninitializedPoiner(c *C) {
	config := localConfiguration{}

	os.Setenv("REGISTRY_LOG_FORMATTER", "json")
	defer os.Unsetenv("REGISTRY_LOG_FORMATTER")

	p := NewParser("registry", []VersionedParseInfo{
		{
			Version: "0.1",
			ParseAs: reflect.TypeOf(config),
			ConversionFunc: func(c interface{}) (interface{}, error) {
				return c, nil
			},
		},
	})

	err := p.Parse([]byte(`{version: "0.1", log: {formatter: "text"}}`), &config)
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, expectedConfig)
}

func (suite *ParserSuite) TestParseOverwriteUnininitializedPoiner(c *C) {
	config := localConfiguration{}

	os.Setenv("REGISTRY_LOG_FORMATTER", "json")
	defer os.Unsetenv("REGISTRY_LOG_FORMATTER")

	p := NewParser("registry", []VersionedParseInfo{
		{
			Version: "0.1",
			ParseAs: reflect.TypeOf(config),
			ConversionFunc: func(c interface{}) (interface{}, error) {
				return c, nil
			},
		},
	})

	err := p.Parse([]byte(`{version: "0.1"}`), &config)
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, expectedConfig)
}
