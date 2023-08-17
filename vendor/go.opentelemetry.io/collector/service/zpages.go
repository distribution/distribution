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

package service // import "go.opentelemetry.io/collector/service"

import (
	"net/http"
	"path"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/service/featuregate"
	"go.opentelemetry.io/collector/service/internal/runtimeinfo"
	"go.opentelemetry.io/collector/service/internal/zpages"
)

const (
	servicezPath   = "servicez"
	pipelinezPath  = "pipelinez"
	extensionzPath = "extensionz"
	featurezPath   = "featurez"
)

func (host *serviceHost) RegisterZPages(mux *http.ServeMux, pathPrefix string) {
	mux.HandleFunc(path.Join(pathPrefix, servicezPath), host.zPagesRequest)
	mux.HandleFunc(path.Join(pathPrefix, pipelinezPath), host.pipelines.HandleZPages)
	mux.HandleFunc(path.Join(pathPrefix, extensionzPath), host.extensions.HandleZPages)
	mux.HandleFunc(path.Join(pathPrefix, featurezPath), handleFeaturezRequest)
}

func (host *serviceHost) zPagesRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	zpages.WriteHTMLPageHeader(w, zpages.HeaderData{Title: "Service " + host.buildInfo.Command})
	zpages.WriteHTMLPropertiesTable(w, zpages.PropertiesTableData{Name: "Build Info", Properties: getBuildInfoProperties(host.buildInfo)})
	zpages.WriteHTMLPropertiesTable(w, zpages.PropertiesTableData{Name: "Runtime Info", Properties: runtimeinfo.Info()})
	zpages.WriteHTMLComponentHeader(w, zpages.ComponentHeaderData{
		Name:              "Pipelines",
		ComponentEndpoint: pipelinezPath,
		Link:              true,
	})
	zpages.WriteHTMLComponentHeader(w, zpages.ComponentHeaderData{
		Name:              "Extensions",
		ComponentEndpoint: extensionzPath,
		Link:              true,
	})
	zpages.WriteHTMLComponentHeader(w, zpages.ComponentHeaderData{
		Name:              "Features",
		ComponentEndpoint: featurezPath,
		Link:              true,
	})
	zpages.WriteHTMLPageFooter(w)
}

func handleFeaturezRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	zpages.WriteHTMLPageHeader(w, zpages.HeaderData{Title: "Feature Gates"})
	zpages.WriteHTMLFeaturesTable(w, getFeaturesTableData())
	zpages.WriteHTMLPageFooter(w)
}

func getFeaturesTableData() zpages.FeatureGateTableData {
	data := zpages.FeatureGateTableData{}
	for _, g := range featuregate.GetRegistry().List() {
		data.Rows = append(data.Rows, zpages.FeatureGateTableRowData{
			ID:          g.ID,
			Enabled:     g.Enabled,
			Description: g.Description,
		})
	}

	return data
}

func getBuildInfoProperties(buildInfo component.BuildInfo) [][2]string {
	return [][2]string{
		{"Command", buildInfo.Command},
		{"Description", buildInfo.Description},
		{"Version", buildInfo.Version},
	}
}
