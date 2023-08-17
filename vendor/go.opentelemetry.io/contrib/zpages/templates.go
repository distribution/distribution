// Copyright The OpenTelemetry Authors
// Copyright 2017, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package zpages // import "go.opentelemetry.io/contrib/zpages"

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"

	"go.opentelemetry.io/contrib/zpages/internal"
)

var (
	templateFunctions = template.FuncMap{
		"even":    even,
		"spanRow": spanRowFormatter,
	}
	headerTemplate       = parseTemplate("header")
	summaryTableTemplate = parseTemplate("summary")
	tracesTableTemplate  = parseTemplate("traces")
	footerTemplate       = parseTemplate("footer")
)

// headerData contains data for the header template.
type headerData struct {
	Title string
}

func parseTemplate(name string) *template.Template {
	f, err := internal.FS(false).Open("/templates/" + name + ".html")
	if err != nil {
		log.Panicf("%v: %v", name, err) // nolint: revive  // Called during initialization.
	}
	defer func() {
		if err = f.Close(); err != nil {
			log.Panicf("%v: %v", name, err) // nolint: revive  // Called during initialization.
		}
	}()
	text, err := ioutil.ReadAll(f)
	if err != nil {
		log.Panicf("%v: %v", name, err) // nolint: revive  // Called during initialization.
	}
	return template.Must(template.New(name).Funcs(templateFunctions).Parse(string(text)))
}

func spanRowFormatter(r spanRow) template.HTML {
	if !r.SpanContext.IsValid() {
		return ""
	}
	col := "black"
	if r.SpanContext.IsSampled() {
		col = "blue"
	}
	if r.ParentSpanContext.IsValid() {
		return template.HTML(fmt.Sprintf(`trace_id: <b style="color:%s">%s</b> span_id: %s parent_span_id: %s`, col, r.SpanContext.TraceID(), r.SpanContext.SpanID(), r.ParentSpanContext.SpanID()))
	}
	return template.HTML(fmt.Sprintf(`trace_id: <b style="color:%s">%s</b> span_id: %s`, col, r.SpanContext.TraceID(), r.SpanContext.SpanID()))
}

func even(x int) bool {
	return x%2 == 0
}
