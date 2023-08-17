// Copyright  The OpenTelemetry Authors
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

package iruntime // import "go.opentelemetry.io/collector/internal/iruntime"

import (
	"github.com/shirou/gopsutil/v3/mem"
)

// readMemInfo returns the total memory
// supports in linux, darwin and windows
func readMemInfo() (uint64, error) {
	vmStat, err := mem.VirtualMemory()
	return vmStat.Total, err
}
