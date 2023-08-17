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

package runtimeinfo // import "go.opentelemetry.io/collector/service/internal/runtimeinfo"

import (
	"runtime"
	"time"
)

var (
	// InfoVar is a singleton instance of the Info struct.
	runtimeInfoVar [][2]string
)

func init() {
	runtimeInfoVar = [][2]string{
		{"StartTimestamp", time.Now().String()},
		{"Go", runtime.Version()},
		{"OS", runtime.GOOS},
		{"Arch", runtime.GOARCH},
		// Add other valuable runtime information here.
	}
}

// Info returns the runtime information like uptime, os, arch, etc.
func Info() [][2]string {
	return runtimeInfoVar
}
