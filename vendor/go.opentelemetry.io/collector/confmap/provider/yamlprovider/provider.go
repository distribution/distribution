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

package yamlprovider // import "go.opentelemetry.io/collector/confmap/provider/yamlprovider"

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/internal"
)

const schemeName = "yaml"

type provider struct{}

// New returns a new confmap.Provider that allows to provide yaml bytes.
//
// This Provider supports "yaml" scheme, and can be called with a "uri" that follows:
//
//	bytes-uri = "yaml:" yaml-bytes
//
// Examples:
// `yaml:processors::batch::timeout: 2s`
// `yaml:processors::batch/foo::timeout: 3s`
func New() confmap.Provider {
	return &provider{}
}

func (s *provider) Retrieve(_ context.Context, uri string, _ confmap.WatcherFunc) (*confmap.Retrieved, error) {
	if !strings.HasPrefix(uri, schemeName+":") {
		return nil, fmt.Errorf("%q uri is not supported by %q provider", uri, schemeName)
	}

	return internal.NewRetrievedFromYAML([]byte(uri[len(schemeName)+1:]))
}

func (*provider) Scheme() string {
	return schemeName
}

func (s *provider) Shutdown(context.Context) error {
	return nil
}
