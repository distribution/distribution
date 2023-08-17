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

package memorylimiterprocessor // import "go.opentelemetry.io/collector/processor/memorylimiterprocessor"

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

const (
	// The value of "type" Attribute Key in configuration.
	typeStr = "memory_limiter"
)

var processorCapabilities = consumer.Capabilities{MutatesData: false}

type factory struct {
	// memoryLimiters stores memoryLimiter instances with unique configs that multiple processors can reuse.
	// This avoids running multiple memory checks (ie: GC) for every processor using the same processor config.
	memoryLimiters map[config.Processor]*memoryLimiter
	lock           sync.Mutex
}

// NewFactory returns a new factory for the Memory Limiter processor.
func NewFactory() component.ProcessorFactory {
	f := &factory{
		memoryLimiters: map[config.Processor]*memoryLimiter{},
	}
	return component.NewProcessorFactory(
		typeStr,
		createDefaultConfig,
		component.WithTracesProcessor(f.createTracesProcessor, component.StabilityLevelBeta),
		component.WithMetricsProcessor(f.createMetricsProcessor, component.StabilityLevelBeta),
		component.WithLogsProcessor(f.createLogsProcessor, component.StabilityLevelBeta))
}

// CreateDefaultConfig creates the default configuration for processor. Notice
// that the default configuration is expected to fail for this processor.
func createDefaultConfig() config.Processor {
	return &Config{
		ProcessorSettings: config.NewProcessorSettings(config.NewComponentID(typeStr)),
	}
}

func (f *factory) createTracesProcessor(
	ctx context.Context,
	set component.ProcessorCreateSettings,
	cfg config.Processor,
	nextConsumer consumer.Traces,
) (component.TracesProcessor, error) {
	memLimiter, err := f.getMemoryLimiter(set, cfg)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewTracesProcessorWithCreateSettings(ctx, set, cfg, nextConsumer,
		memLimiter.processTraces,
		processorhelper.WithCapabilities(processorCapabilities),
		processorhelper.WithStart(memLimiter.start),
		processorhelper.WithShutdown(memLimiter.shutdown))
}

func (f *factory) createMetricsProcessor(
	ctx context.Context,
	set component.ProcessorCreateSettings,
	cfg config.Processor,
	nextConsumer consumer.Metrics,
) (component.MetricsProcessor, error) {
	memLimiter, err := f.getMemoryLimiter(set, cfg)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewMetricsProcessorWithCreateSettings(ctx, set, cfg, nextConsumer,
		memLimiter.processMetrics,
		processorhelper.WithCapabilities(processorCapabilities),
		processorhelper.WithStart(memLimiter.start),
		processorhelper.WithShutdown(memLimiter.shutdown))
}

func (f *factory) createLogsProcessor(
	ctx context.Context,
	set component.ProcessorCreateSettings,
	cfg config.Processor,
	nextConsumer consumer.Logs,
) (component.LogsProcessor, error) {
	memLimiter, err := f.getMemoryLimiter(set, cfg)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewLogsProcessorWithCreateSettings(ctx, set, cfg, nextConsumer,
		memLimiter.processLogs,
		processorhelper.WithCapabilities(processorCapabilities),
		processorhelper.WithStart(memLimiter.start),
		processorhelper.WithShutdown(memLimiter.shutdown))
}

// getMemoryLimiter checks if we have a cached memoryLimiter with a specific config,
// otherwise initialize and add one to the store.
func (f *factory) getMemoryLimiter(set component.ProcessorCreateSettings, cfg config.Processor) (*memoryLimiter, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if memLimiter, ok := f.memoryLimiters[cfg]; ok {
		return memLimiter, nil
	}

	memLimiter, err := newMemoryLimiter(set, cfg.(*Config))
	if err != nil {
		return nil, err
	}

	f.memoryLimiters[cfg] = memLimiter
	return memLimiter, nil
}
