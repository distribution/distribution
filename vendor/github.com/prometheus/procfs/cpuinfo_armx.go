<<<<<<< HEAD
<<<<<<< HEAD:vendor/github.com/prometheus/procfs/cpuinfo_armx.go
// Copyright 2020 The Prometheus Authors
=======
// Copyright 2021 The Prometheus Authors
>>>>>>> main:vendor/github.com/prometheus/procfs/cmdline.go
=======
// Copyright 2020 The Prometheus Authors
>>>>>>> main
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

<<<<<<< HEAD
<<<<<<< HEAD:vendor/github.com/prometheus/procfs/cpuinfo_armx.go
=======
>>>>>>> main
// +build linux
// +build arm arm64

package procfs

var parseCPUInfo = parseCPUInfoARM
<<<<<<< HEAD
=======
package procfs

import (
	"strings"

	"github.com/prometheus/procfs/internal/util"
)

// CmdLine returns the command line of the kernel.
func (fs FS) CmdLine() ([]string, error) {
	data, err := util.ReadFileNoStat(fs.proc.Path("cmdline"))
	if err != nil {
		return nil, err
	}

	return strings.Fields(string(data)), nil
}
>>>>>>> main:vendor/github.com/prometheus/procfs/cmdline.go
=======
>>>>>>> main
