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

package zpagesextension // import "go.opentelemetry.io/collector/extension/zpagesextension"

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/confignet"
)

const (
	// The value of extension "type" in configuration.
	typeStr = "zpages"

	defaultEndpoint = "localhost:55679"
)

// NewFactory creates a factory for Z-Pages extension.
func NewFactory() component.ExtensionFactory {
	return component.NewExtensionFactoryWithStabilityLevel(typeStr, createDefaultConfig, createExtension, component.StabilityLevelBeta)
}

func createDefaultConfig() config.Extension {
	return &Config{
		ExtensionSettings: config.NewExtensionSettings(config.NewComponentID(typeStr)),
		TCPAddr: confignet.TCPAddr{
			Endpoint: defaultEndpoint,
		},
	}
}

// createExtension creates the extension based on this config.
func createExtension(_ context.Context, set component.ExtensionCreateSettings, cfg config.Extension) (component.Extension, error) {
	return newServer(cfg.(*Config), set.TelemetrySettings), nil
}
