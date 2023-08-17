// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal // import "go.opentelemetry.io/collector/confmap/provider/internal"

import (
	"gopkg.in/yaml.v3"

	"go.opentelemetry.io/collector/confmap"
)

// NewRetrievedFromYAML returns a new Retrieved instance that contains the deserialized data from the yaml bytes.
// * yamlBytes the yaml bytes that will be deserialized.
// * opts specifies options associated with this Retrieved value, such as CloseFunc.
func NewRetrievedFromYAML(yamlBytes []byte, opts ...confmap.RetrievedOption) (*confmap.Retrieved, error) {
	var rawConf interface{}
	if err := yaml.Unmarshal(yamlBytes, &rawConf); err != nil {
		return nil, err
	}
	return confmap.NewRetrieved(rawConf, opts...)
}
