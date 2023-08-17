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

// Keep the original Uber license.

// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

//go:build linux
// +build linux

package cgroups // import "go.opentelemetry.io/collector/internal/cgroups"
import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// _cgroupFSType is the Linux CGroup file system type used in
	// `/proc/$PID/mountinfo`.
	_cgroupFSType = "cgroup"
	// _cgroupSubsysCPU is the CPU CGroup subsystem.
	_cgroupSubsysCPU = "cpu"
	// _cgroupSubsysCPUAcct is the CPU accounting CGroup subsystem.
	_cgroupSubsysCPUAcct = "cpuacct"
	// _cgroupSubsysCPUSet is the CPUSet CGroup subsystem.
	_cgroupSubsysCPUSet = "cpuset"
	// _cgroupSubsysMemory is the Memory CGroup subsystem.
	_cgroupSubsysMemory = "memory"

	_cgroupMemoryLimitBytes = "memory.limit_in_bytes"

	// _cgroupv2MemoryMax is the file name for the CGroup-V2 Memory max
	// parameter.
	_cgroupv2MemoryMax = "memory.max"
	// _cgroupFSType is the Linux CGroup-V2 file system type used in
	// `/proc/$PID/mountinfo`.
	_cgroupv2FSType = "cgroup2"
)

const (
	_procPathCGroup     = "/proc/self/cgroup"
	_procPathMountInfo  = "/proc/self/mountinfo"
	_cgroupv2MountPoint = "/sys/fs/cgroup"
)

// CGroups is a map that associates each CGroup with its subsystem name.
type CGroups map[string]*CGroup

// NewCGroups returns a new *CGroups from given `mountinfo` and `cgroup` files
// under for some process under `/proc` file system (see also proc(5) for more
// information).
func NewCGroups(procPathMountInfo, procPathCGroup string) (CGroups, error) {
	cgroupSubsystems, err := parseCGroupSubsystems(procPathCGroup)
	if err != nil {
		return nil, err
	}

	cgroups := make(CGroups)
	newMountPoint := func(mp *MountPoint) error {
		if mp.FSType != _cgroupFSType {
			return nil
		}

		for _, opt := range mp.SuperOptions {
			subsys, exists := cgroupSubsystems[opt]
			if !exists {
				continue
			}

			cgroupPath, err := mp.Translate(subsys.Name)
			if err != nil {
				return err
			}
			cgroups[opt] = NewCGroup(cgroupPath)
		}

		return nil
	}

	if err := parseMountInfo(procPathMountInfo, newMountPoint); err != nil {
		return nil, err
	}
	return cgroups, nil
}

// NewCGroupsForCurrentProcess returns a new *CGroups instance for the current
// process.
func NewCGroupsForCurrentProcess() (CGroups, error) {
	return NewCGroups(_procPathMountInfo, _procPathCGroup)
}

// MemoryQuota returns the total memory limit of the process
// It is a result of `memory.limit_in_bytes`. If the value of
// `memory.limit_in_bytes` was not set (-1) or (9223372036854771712), the method returns `(-1, false, nil)`.
func (cg CGroups) MemoryQuota() (int64, bool, error) {
	memCGroup, exists := cg[_cgroupSubsysMemory]
	if !exists {
		return -1, false, nil
	}

	memLimitBytes, err := memCGroup.readInt(_cgroupMemoryLimitBytes)
	if defined := memLimitBytes > 0; err != nil || !defined {
		return -1, defined, err
	}
	return int64(memLimitBytes), true, nil
}

// IsCGroupV2 returns true if the system supports and uses cgroup2.
// It gets the required information for deciding from mountinfo file.
func IsCGroupV2() (bool, error) {
	return isCGroupV2(_procPathMountInfo)
}

func isCGroupV2(procPathMountInfo string) (bool, error) {
	isV2 := false
	newMountPoint := func(mp *MountPoint) error {
		if mp.FSType == _cgroupv2FSType && mp.MountPoint == _cgroupv2MountPoint {
			isV2 = true
		}
		return nil
	}
	if err := parseMountInfo(procPathMountInfo, newMountPoint); err != nil {
		return false, err
	}
	return isV2, nil
}

// MemoryQuotaV2 returns the total memory limit of the process
// It is a result of cgroupv2 `memory.max`. If the value of
// `memory.max` was not set (max), the method returns `(-1, false, nil)`.
func MemoryQuotaV2() (int64, bool, error) {
	return memoryQuotaV2(_cgroupv2MountPoint, _cgroupv2MemoryMax)
}

func memoryQuotaV2(cgroupv2MountPoint, cgroupv2MemoryMax string) (int64, bool, error) {
	memoryMaxParams, err := os.Open(filepath.Clean(filepath.Join(cgroupv2MountPoint, cgroupv2MemoryMax)))
	if err != nil {
		if os.IsNotExist(err) {
			return -1, false, nil
		}
		return -1, false, err
	}
	scanner := bufio.NewScanner(memoryMaxParams)
	if scanner.Scan() {
		value := strings.TrimSpace(scanner.Text())
		if value == "max" {
			return -1, false, nil
		}
		max, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return -1, false, err
		}
		return max, true, nil
	}
	if err := scanner.Err(); err != nil {
		return -1, false, err
	}
	return -1, false, io.ErrUnexpectedEOF
}
