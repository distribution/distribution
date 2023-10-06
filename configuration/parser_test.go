package configuration

import (
	"os"
	"reflect"

	"gopkg.in/check.v1"
)

type localConfiguration struct {
	Version       Version `yaml:"version"`
	Log           *Log    `yaml:"log"`
	Notifications []Notif `yaml:"notifications,omitempty"`
}

type Log struct {
	Formatter string `yaml:"formatter,omitempty"`
}

type Notif struct {
	Name string `yaml:"name"`
}

var expectedConfig = localConfiguration{
	Version: "0.1",
	Log: &Log{
		Formatter: "json",
	},
	Notifications: []Notif{
		{Name: "foo"},
		{Name: "bar"},
		{Name: "car"},
	},
}

const testConfig = `version: "0.1"
log:
  formatter: "text"
notifications:
  - name: "foo"
  - name: "bar"
  - name: "car"`

type ParserSuite struct{}

var _ = check.Suite(new(ParserSuite))

func (suite *ParserSuite) TestParserOverwriteIninitializedPoiner(c *check.C) {
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

	err := p.Parse([]byte(testConfig), &config)
	c.Assert(err, check.IsNil)
	c.Assert(config, check.DeepEquals, expectedConfig)
}

const testConfig2 = `version: "0.1"
log:
  formatter: "text"
notifications:
  - name: "val1"
  - name: "val2"
  - name: "car"`

func (suite *ParserSuite) TestParseOverwriteUnininitializedPoiner(c *check.C) {
	config := localConfiguration{}

	os.Setenv("REGISTRY_LOG_FORMATTER", "json")
	defer os.Unsetenv("REGISTRY_LOG_FORMATTER")

	// override only first two notificationsvalues
	// in the tetConfig: leave the last value unchanged.
	os.Setenv("REGISTRY_NOTIFICATIONS_0_NAME", "foo")
	defer os.Unsetenv("REGISTRY_NOTIFICATIONS_0_NAME")
	os.Setenv("REGISTRY_NOTIFICATIONS_1_NAME", "bar")
	defer os.Unsetenv("REGISTRY_NOTIFICATIONS_1_NAME")

	p := NewParser("registry", []VersionedParseInfo{
		{
			Version: "0.1",
			ParseAs: reflect.TypeOf(config),
			ConversionFunc: func(c interface{}) (interface{}, error) {
				return c, nil
			},
		},
	})

	err := p.Parse([]byte(testConfig2), &config)
	c.Assert(err, check.IsNil)
	c.Assert(config, check.DeepEquals, expectedConfig)
}
