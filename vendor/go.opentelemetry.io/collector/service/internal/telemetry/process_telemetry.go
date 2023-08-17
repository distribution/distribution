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

package telemetry // import "go.opentelemetry.io/collector/service/internal/telemetry"

import (
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"go.opencensus.io/metric"
	"go.opencensus.io/stats"
)

// processMetrics is a struct that contains views related to process metrics (cpu, mem, etc)
type processMetrics struct {
	startTimeUnixNano int64
	ballastSizeBytes  uint64
	proc              *process.Process

	processUptime *metric.Float64DerivedCumulative
	allocMem      *metric.Int64DerivedGauge
	totalAllocMem *metric.Int64DerivedCumulative
	sysMem        *metric.Int64DerivedGauge
	cpuSeconds    *metric.Float64DerivedCumulative
	rssMemory     *metric.Int64DerivedGauge

	// mu protects everything bellow.
	mu         sync.Mutex
	lastMsRead time.Time
	ms         *runtime.MemStats
}

// RegisterProcessMetrics creates a new set of processMetrics (mem, cpu) that can be used to measure
// basic information about this process.
func RegisterProcessMetrics(registry *metric.Registry, ballastSizeBytes uint64) error {
	pm := &processMetrics{
		startTimeUnixNano: time.Now().UnixNano(),
		ballastSizeBytes:  ballastSizeBytes,
		ms:                &runtime.MemStats{},
	}
	var err error
	pm.proc, err = process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return err
	}

	pm.processUptime, err = registry.AddFloat64DerivedCumulative(
		"process/uptime",
		metric.WithDescription("Uptime of the process"),
		metric.WithUnit(stats.UnitSeconds))
	if err != nil {
		return err
	}
	if err = pm.processUptime.UpsertEntry(pm.updateProcessUptime); err != nil {
		return err
	}

	pm.allocMem, err = registry.AddInt64DerivedGauge(
		"process/runtime/heap_alloc_bytes",
		metric.WithDescription("Bytes of allocated heap objects (see 'go doc runtime.MemStats.HeapAlloc')"),
		metric.WithUnit(stats.UnitBytes))
	if err != nil {
		return err
	}
	if err = pm.allocMem.UpsertEntry(pm.updateAllocMem); err != nil {
		return err
	}

	pm.totalAllocMem, err = registry.AddInt64DerivedCumulative(
		"process/runtime/total_alloc_bytes",
		metric.WithDescription("Cumulative bytes allocated for heap objects (see 'go doc runtime.MemStats.TotalAlloc')"),
		metric.WithUnit(stats.UnitBytes))
	if err != nil {
		return err
	}
	if err = pm.totalAllocMem.UpsertEntry(pm.updateTotalAllocMem); err != nil {
		return err
	}

	pm.sysMem, err = registry.AddInt64DerivedGauge(
		"process/runtime/total_sys_memory_bytes",
		metric.WithDescription("Total bytes of memory obtained from the OS (see 'go doc runtime.MemStats.Sys')"),
		metric.WithUnit(stats.UnitBytes))
	if err != nil {
		return err
	}
	if err = pm.sysMem.UpsertEntry(pm.updateSysMem); err != nil {
		return err
	}

	pm.cpuSeconds, err = registry.AddFloat64DerivedCumulative(
		"process/cpu_seconds",
		metric.WithDescription("Total CPU user and system time in seconds"),
		metric.WithUnit(stats.UnitSeconds))
	if err != nil {
		return err
	}
	if err = pm.cpuSeconds.UpsertEntry(pm.updateCPUSeconds); err != nil {
		return err
	}

	pm.rssMemory, err = registry.AddInt64DerivedGauge(
		"process/memory/rss",
		metric.WithDescription("Total physical memory (resident set size)"),
		metric.WithUnit(stats.UnitBytes))
	if err != nil {
		return err
	}
	if err = pm.rssMemory.UpsertEntry(pm.updateRSSMemory); err != nil {
		return err
	}

	return nil
}

func (pm *processMetrics) updateProcessUptime() float64 {
	now := time.Now().UnixNano()
	return float64(now-pm.startTimeUnixNano) / 1e9
}

func (pm *processMetrics) updateAllocMem() int64 {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.readMemStatsIfNeeded()
	return int64(pm.ms.Alloc)
}

func (pm *processMetrics) updateTotalAllocMem() int64 {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.readMemStatsIfNeeded()
	return int64(pm.ms.TotalAlloc)
}

func (pm *processMetrics) updateSysMem() int64 {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.readMemStatsIfNeeded()
	return int64(pm.ms.Sys)
}

func (pm *processMetrics) updateCPUSeconds() float64 {
	times, err := pm.proc.Times()
	if err != nil {
		return 0
	}

	return times.Total()
}

func (pm *processMetrics) updateRSSMemory() int64 {
	mem, err := pm.proc.MemoryInfo()
	if err != nil {
		return 0
	}
	return int64(mem.RSS)
}

func (pm *processMetrics) readMemStatsIfNeeded() {
	now := time.Now()
	// If last time we read was less than one second ago just reuse the values
	if now.Sub(pm.lastMsRead) < time.Second {
		return
	}
	pm.lastMsRead = now
	runtime.ReadMemStats(pm.ms)
	if pm.ballastSizeBytes > 0 {
		pm.ms.Alloc -= pm.ballastSizeBytes
		pm.ms.HeapAlloc -= pm.ballastSizeBytes
		pm.ms.HeapSys -= pm.ballastSizeBytes
		pm.ms.HeapInuse -= pm.ballastSizeBytes
	}
}
