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

package extensions // import "go.opentelemetry.io/collector/service/extensions"

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"go.uber.org/multierr"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/service/internal/components"
	"go.opentelemetry.io/collector/service/internal/zpages"
)

const zExtensionName = "zextensionname"

// Extensions is a map of extensions created from extension configs.
type Extensions struct {
	telemetry component.TelemetrySettings
	extMap    map[config.ComponentID]component.Extension
}

// Start starts all extensions.
func (bes *Extensions) Start(ctx context.Context, host component.Host) error {
	bes.telemetry.Logger.Info("Starting extensions...")
	for extID, ext := range bes.extMap {
		extLogger := extensionLogger(bes.telemetry.Logger, extID)
		extLogger.Info("Extension is starting...")
		if err := ext.Start(ctx, components.NewHostWrapper(host, extLogger)); err != nil {
			return err
		}
		extLogger.Info("Extension started.")
	}
	return nil
}

// Shutdown stops all extensions.
func (bes *Extensions) Shutdown(ctx context.Context) error {
	bes.telemetry.Logger.Info("Stopping extensions...")
	var errs error
	for _, ext := range bes.extMap {
		errs = multierr.Append(errs, ext.Shutdown(ctx))
	}

	return errs
}

func (bes *Extensions) NotifyPipelineReady() error {
	for extID, ext := range bes.extMap {
		if pw, ok := ext.(component.PipelineWatcher); ok {
			if err := pw.Ready(); err != nil {
				return fmt.Errorf("failed to notify extension %q: %w", extID, err)
			}
		}
	}
	return nil
}

func (bes *Extensions) NotifyPipelineNotReady() error {
	// Notify extensions in reverse order.
	var errs error
	for _, ext := range bes.extMap {
		if pw, ok := ext.(component.PipelineWatcher); ok {
			errs = multierr.Append(errs, pw.NotReady())
		}
	}
	return errs
}

func (bes *Extensions) GetExtensions() map[config.ComponentID]component.Extension {
	result := make(map[config.ComponentID]component.Extension, len(bes.extMap))
	for extID, v := range bes.extMap {
		result[extID] = v
	}
	return result
}

func (bes *Extensions) HandleZPages(w http.ResponseWriter, r *http.Request) {
	extensionName := r.URL.Query().Get(zExtensionName)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	zpages.WriteHTMLPageHeader(w, zpages.HeaderData{Title: "Extensions"})
	data := zpages.SummaryExtensionsTableData{}

	data.Rows = make([]zpages.SummaryExtensionsTableRowData, 0, len(bes.extMap))
	for id := range bes.extMap {
		row := zpages.SummaryExtensionsTableRowData{FullName: id.String()}
		data.Rows = append(data.Rows, row)
	}

	sort.Slice(data.Rows, func(i, j int) bool {
		return data.Rows[i].FullName < data.Rows[j].FullName
	})
	zpages.WriteHTMLExtensionsSummaryTable(w, data)
	if extensionName != "" {
		zpages.WriteHTMLComponentHeader(w, zpages.ComponentHeaderData{
			Name: extensionName,
		})
		// TODO: Add config + status info.
	}
	zpages.WriteHTMLPageFooter(w)
}

// Settings holds configuration for building Extensions.
type Settings struct {
	Telemetry component.TelemetrySettings
	BuildInfo component.BuildInfo

	// Configs is a map of config.ComponentID to config.Extension.
	Configs map[config.ComponentID]config.Extension

	// Factories maps extension type names in the config to the respective component.ExtensionFactory.
	Factories map[config.Type]component.ExtensionFactory
}

// New creates a new Extensions from Config.
func New(ctx context.Context, set Settings, cfg Config) (*Extensions, error) {
	exts := &Extensions{
		telemetry: set.Telemetry,
		extMap:    make(map[config.ComponentID]component.Extension),
	}
	for _, extID := range cfg {
		extCfg, existsCfg := set.Configs[extID]
		if !existsCfg {
			return nil, fmt.Errorf("extension %q is not configured", extID)
		}

		factory, existsFactory := set.Factories[extID.Type()]
		if !existsFactory {
			return nil, fmt.Errorf("extension factory for type %q is not configured", extID.Type())
		}

		extSet := component.ExtensionCreateSettings{
			TelemetrySettings: set.Telemetry,
			BuildInfo:         set.BuildInfo,
		}
		extSet.TelemetrySettings.Logger = extensionLogger(set.Telemetry.Logger, extID)

		ext, err := factory.CreateExtension(ctx, extSet, extCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create extension %q: %w", extID, err)
		}

		// Check if the factory really created the extension.
		if ext == nil {
			return nil, fmt.Errorf("factory for %q produced a nil extension", extID)
		}

		exts.extMap[extID] = ext
	}

	return exts, nil
}

func extensionLogger(logger *zap.Logger, id config.ComponentID) *zap.Logger {
	return logger.With(
		zap.String(components.ZapKindKey, components.ZapKindExtension),
		zap.String(components.ZapNameKey, id.String()))
}
