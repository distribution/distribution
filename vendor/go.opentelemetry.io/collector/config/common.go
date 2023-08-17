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

package config // import "go.opentelemetry.io/collector/config"
import (
	"go.opentelemetry.io/collector/confmap"
)

// Type is the component type as it is used in the config.
type Type string

// validatable defines the interface for the configuration validation.
type validatable interface {
	// Validate validates the configuration and returns an error if invalid.
	Validate() error
}

// Unmarshallable defines an optional interface for custom configuration unmarshalling.
// A configuration struct can implement this interface to override the default unmarshalling.
type Unmarshallable interface {
	// Unmarshal is a function that unmarshalls a confmap.Conf into the unmarshable struct in a custom way.
	// The confmap.Conf for this specific component may be nil or empty if no config available.
	Unmarshal(component *confmap.Conf) error
}

// DataType is a special Type that represents the data types supported by the collector. We currently support
// collecting metrics, traces and logs, this can expand in the future.
type DataType = Type

// Currently supported data types. Add new data types here when new types are supported in the future.
const (
	// TracesDataType is the data type tag for traces.
	TracesDataType DataType = "traces"

	// MetricsDataType is the data type tag for metrics.
	MetricsDataType DataType = "metrics"

	// LogsDataType is the data type tag for logs.
	LogsDataType DataType = "logs"
)

func unmarshal(componentSection *confmap.Conf, intoCfg interface{}) error {
	if cu, ok := intoCfg.(Unmarshallable); ok {
		return cu.Unmarshal(componentSection)
	}

	return componentSection.UnmarshalExact(intoCfg)
}
