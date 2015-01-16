package configuration

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

// Version is a major/minor version pair of the form Major.Minor
// Major version upgrades indicate structure or type changes
// Minor version upgrades should be strictly additive
type Version string

// MajorMinorVersion constructs a Version from its Major and Minor components
func MajorMinorVersion(major, minor uint) Version {
	return Version(fmt.Sprintf("%d.%d", major, minor))
}

func (version Version) major() (uint, error) {
	majorPart := strings.Split(string(version), ".")[0]
	major, err := strconv.ParseUint(majorPart, 10, 0)
	return uint(major), err
}

// Major returns the major version portion of a Version
func (version Version) Major() uint {
	major, _ := version.major()
	return major
}

func (version Version) minor() (uint, error) {
	minorPart := strings.Split(string(version), ".")[1]
	minor, err := strconv.ParseUint(minorPart, 10, 0)
	return uint(minor), err
}

// Minor returns the minor version portion of a Version
func (version Version) Minor() uint {
	minor, _ := version.minor()
	return minor
}

// VersionedParseInfo defines how a specific version of a configuration should
// be parsed into the current version
type VersionedParseInfo struct {
	// Version is the version which this parsing information relates to
	Version Version
	// ParseAs defines the type which a configuration file of this version
	// should be parsed into
	ParseAs reflect.Type
	// ConversionFunc defines a method for converting the parsed configuration
	// (of type ParseAs) into the current configuration version
	// Note: this method signature is very unclear with the absence of generics
	ConversionFunc func(interface{}) (interface{}, error)
}

// Parser can be used to parse a configuration file and environment of a defined
// version into a unified output structure
type Parser struct {
	prefix  string
	mapping map[Version]VersionedParseInfo
	env     map[string]string
}

// NewParser returns a *Parser with the given environment prefix which handles
// versioned configurations which match the given parseInfos
func NewParser(prefix string, parseInfos []VersionedParseInfo) *Parser {
	p := Parser{prefix: prefix, mapping: make(map[Version]VersionedParseInfo), env: make(map[string]string)}

	for _, parseInfo := range parseInfos {
		p.mapping[parseInfo.Version] = parseInfo
	}

	for _, env := range os.Environ() {
		envParts := strings.SplitN(env, "=", 2)
		p.env[envParts[0]] = envParts[1]
	}

	return &p
}

// Parse reads in the given []byte and environment and writes the resulting
// configuration into the input v
//
// Environment variables may be used to override configuration parameters other
// than version, following the scheme below:
// v.Abc may be replaced by the value of PREFIX_ABC,
// v.Abc.Xyz may be replaced by the value of PREFIX_ABC_XYZ, and so forth
func (p *Parser) Parse(in []byte, v interface{}) error {
	var versionedStruct struct {
		Version Version
	}

	if err := yaml.Unmarshal(in, &versionedStruct); err != nil {
		return err
	}

	parseInfo, ok := p.mapping[versionedStruct.Version]
	if !ok {
		return fmt.Errorf("Unsupported version: %q", versionedStruct.Version)
	}

	parseAs := reflect.New(parseInfo.ParseAs)
	err := yaml.Unmarshal(in, parseAs.Interface())
	if err != nil {
		return err
	}

	err = p.overwriteFields(parseAs, p.prefix)
	if err != nil {
		return err
	}

	c, err := parseInfo.ConversionFunc(parseAs.Interface())
	if err != nil {
		return err
	}
	reflect.ValueOf(v).Elem().Set(reflect.Indirect(reflect.ValueOf(c)))
	return nil
}

func (p *Parser) overwriteFields(v reflect.Value, prefix string) error {
	for v.Kind() == reflect.Ptr {
		v = reflect.Indirect(v)
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			sf := v.Type().Field(i)
			fieldPrefix := strings.ToUpper(prefix + "_" + sf.Name)
			if e, ok := p.env[fieldPrefix]; ok {
				fieldVal := reflect.New(sf.Type)
				err := yaml.Unmarshal([]byte(e), fieldVal.Interface())
				if err != nil {
					return err
				}
				v.Field(i).Set(reflect.Indirect(fieldVal))
			}
			err := p.overwriteFields(v.Field(i), fieldPrefix)
			if err != nil {
				return err
			}
		}
	case reflect.Map:
		p.overwriteMap(v, prefix)
	}
	return nil
}

func (p *Parser) overwriteMap(m reflect.Value, prefix string) error {
	switch m.Type().Elem().Kind() {
	case reflect.Struct:
		for _, k := range m.MapKeys() {
			err := p.overwriteFields(m.MapIndex(k), strings.ToUpper(fmt.Sprintf("%s_%s", prefix, k)))
			if err != nil {
				return err
			}
		}
		envMapRegexp, err := regexp.Compile(fmt.Sprintf("^%s_([A-Z0-9]+)$", strings.ToUpper(prefix)))
		if err != nil {
			return err
		}
		for key, val := range p.env {
			if submatches := envMapRegexp.FindStringSubmatch(key); submatches != nil {
				mapValue := reflect.New(m.Type().Elem())
				err := yaml.Unmarshal([]byte(val), mapValue.Interface())
				if err != nil {
					return err
				}
				m.SetMapIndex(reflect.ValueOf(strings.ToLower(submatches[1])), reflect.Indirect(mapValue))
			}
		}
	case reflect.Map:
		for _, k := range m.MapKeys() {
			err := p.overwriteMap(m.MapIndex(k), strings.ToUpper(fmt.Sprintf("%s_%s", prefix, k)))
			if err != nil {
				return err
			}
		}
	default:
		envMapRegexp, err := regexp.Compile(fmt.Sprintf("^%s_([A-Z0-9]+)$", strings.ToUpper(prefix)))
		if err != nil {
			return err
		}

		for key, val := range p.env {
			if submatches := envMapRegexp.FindStringSubmatch(key); submatches != nil {
				mapValue := reflect.New(m.Type().Elem())
				err := yaml.Unmarshal([]byte(val), mapValue.Interface())
				if err != nil {
					return err
				}
				m.SetMapIndex(reflect.ValueOf(strings.ToLower(submatches[1])), reflect.Indirect(mapValue))
			}
		}
	}
	return nil
}
