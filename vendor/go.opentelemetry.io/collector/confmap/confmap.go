// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package confmap // import "go.opentelemetry.io/collector/confmap"

import (
	"encoding"
	"fmt"
	"reflect"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/maps"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/mitchellh/mapstructure"
)

const (
	// KeyDelimiter is used as the default key delimiter in the default koanf instance.
	KeyDelimiter = "::"
)

// New creates a new empty confmap.Conf instance.
func New() *Conf {
	return &Conf{k: koanf.New(KeyDelimiter)}
}

// NewFromStringMap creates a confmap.Conf from a map[string]interface{}.
func NewFromStringMap(data map[string]interface{}) *Conf {
	p := New()
	// Cannot return error because the koanf instance is empty.
	_ = p.k.Load(confmap.Provider(data, KeyDelimiter), nil)
	return p
}

// Conf represents the raw configuration map for the OpenTelemetry Collector.
// The confmap.Conf can be unmarshalled into the Collector's config using the "service" package.
type Conf struct {
	k *koanf.Koanf
}

// AllKeys returns all keys holding a value, regardless of where they are set.
// Nested keys are returned with a KeyDelimiter separator.
func (l *Conf) AllKeys() []string {
	return l.k.Keys()
}

// Unmarshal unmarshalls the config into a struct.
// Tags on the fields of the structure must be properly set.
func (l *Conf) Unmarshal(rawVal interface{}) error {
	decoder, err := mapstructure.NewDecoder(decoderConfig(rawVal))
	if err != nil {
		return err
	}
	return decoder.Decode(l.ToStringMap())
}

// UnmarshalExact unmarshalls the config into a struct, erroring if a field is nonexistent.
func (l *Conf) UnmarshalExact(rawVal interface{}) error {
	dc := decoderConfig(rawVal)
	dc.ErrorUnused = true
	decoder, err := mapstructure.NewDecoder(dc)
	if err != nil {
		return err
	}
	return decoder.Decode(l.ToStringMap())
}

// Get can retrieve any value given the key to use.
func (l *Conf) Get(key string) interface{} {
	return l.k.Get(key)
}

// IsSet checks to see if the key has been set in any of the data locations.
// IsSet is case-insensitive for a key.
func (l *Conf) IsSet(key string) bool {
	return l.k.Exists(key)
}

// Merge merges the input given configuration into the existing config.
// Note that the given map may be modified.
func (l *Conf) Merge(in *Conf) error {
	return l.k.Merge(in.k)
}

// Sub returns new Conf instance representing a sub-config of this instance.
// It returns an error is the sub-config is not a map[string]interface{} (use Get()), and an empty Map if none exists.
func (l *Conf) Sub(key string) (*Conf, error) {
	// Code inspired by the koanf "Cut" func, but returns an error instead of empty map for unsupported sub-config type.
	data := l.Get(key)
	if data == nil {
		return New(), nil
	}

	if v, ok := data.(map[string]interface{}); ok {
		return NewFromStringMap(v), nil
	}

	return nil, fmt.Errorf("unexpected sub-config value kind for key:%s value:%v kind:%v)", key, data, reflect.TypeOf(data).Kind())
}

// ToStringMap creates a map[string]interface{} from a Parser.
func (l *Conf) ToStringMap() map[string]interface{} {
	return maps.Unflatten(l.k.All(), KeyDelimiter)
}

// decoderConfig returns a default mapstructure.DecoderConfig capable of parsing time.Duration
// and weakly converting config field values to primitive types.  It also ensures that maps
// whose values are nil pointer structs resolved to the zero value of the target struct (see
// expandNilStructPointers). A decoder created from this mapstructure.DecoderConfig will decode
// its contents to the result argument.
func decoderConfig(result interface{}) *mapstructure.DecoderConfig {
	return &mapstructure.DecoderConfig{
		Result:           result,
		Metadata:         nil,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			expandNilStructPointersHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			mapKeyStringToMapKeyTextUnmarshalerHookFunc(),
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.TextUnmarshallerHookFunc(),
		),
	}
}

// In cases where a config has a mapping of something to a struct pointers
// we want nil values to resolve to a pointer to the zero value of the
// underlying struct just as we want nil values of a mapping of something
// to a struct to resolve to the zero value of that struct.
//
// e.g. given a config type:
// type Config struct { Thing *SomeStruct `mapstructure:"thing"` }
//
// and yaml of:
// config:
//
//	thing:
//
// we want an unmarshaled Config to be equivalent to
// Config{Thing: &SomeStruct{}} instead of Config{Thing: nil}
func expandNilStructPointersHookFunc() mapstructure.DecodeHookFuncValue {
	return func(from reflect.Value, to reflect.Value) (interface{}, error) {
		// ensure we are dealing with map to map comparison
		if from.Kind() == reflect.Map && to.Kind() == reflect.Map {
			toElem := to.Type().Elem()
			// ensure that map values are pointers to a struct
			// (that may be nil and require manual setting w/ zero value)
			if toElem.Kind() == reflect.Ptr && toElem.Elem().Kind() == reflect.Struct {
				fromRange := from.MapRange()
				for fromRange.Next() {
					fromKey := fromRange.Key()
					fromValue := fromRange.Value()
					// ensure that we've run into a nil pointer instance
					if fromValue.IsNil() {
						newFromValue := reflect.New(toElem.Elem())
						from.SetMapIndex(fromKey, newFromValue)
					}
				}
			}
		}
		return from.Interface(), nil
	}
}

// mapKeyStringToMapKeyTextUnmarshalerHookFunc returns a DecodeHookFuncType that checks that a conversion from
// map[string]interface{} to map[encoding.TextUnmarshaler]interface{} does not overwrite keys,
// when UnmarshalText produces equal elements from different strings (e.g. trims whitespaces).
//
// This is needed in combination with ComponentID, which may produce equal IDs for different strings,
// and an error needs to be returned in that case, otherwise the last equivalent ID overwrites the previous one.
func mapKeyStringToMapKeyTextUnmarshalerHookFunc() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.Map || f.Key().Kind() != reflect.String {
			return data, nil
		}

		if t.Kind() != reflect.Map {
			return data, nil
		}

		if _, ok := reflect.New(t.Key()).Interface().(encoding.TextUnmarshaler); !ok {
			return data, nil
		}

		m := reflect.MakeMap(reflect.MapOf(t.Key(), reflect.TypeOf(true)))
		for k := range data.(map[string]interface{}) {
			tKey := reflect.New(t.Key())
			if err := tKey.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(k)); err != nil {
				return nil, err
			}

			if m.MapIndex(reflect.Indirect(tKey)).IsValid() {
				return nil, fmt.Errorf("duplicate name %q after unmarshaling %v", k, tKey)
			}
			m.SetMapIndex(reflect.Indirect(tKey), reflect.ValueOf(true))
		}
		return data, nil
	}
}
