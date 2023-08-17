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

package ballastextension // import "go.opentelemetry.io/collector/extension/ballastextension"

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/internal/iruntime"
)

const (
	// The value of extension "type" in configuration.
	typeStr = "memory_ballast"
)

// memHandler returns the total memory of the target host/vm
var memHandler = iruntime.TotalMemory

// NewFactory creates a factory for FluentBit extension.
func NewFactory() component.ExtensionFactory {
	return component.NewExtensionFactoryWithStabilityLevel(typeStr, createDefaultConfig, createExtension, component.StabilityLevelBeta)
}

func createDefaultConfig() config.Extension {
	return &Config{
		ExtensionSettings: config.NewExtensionSettings(config.NewComponentID(typeStr)),
	}
}

func createExtension(_ context.Context, set component.ExtensionCreateSettings, cfg config.Extension) (component.Extension, error) {
	return newMemoryBallast(cfg.(*Config), set.Logger, memHandler), nil
}
