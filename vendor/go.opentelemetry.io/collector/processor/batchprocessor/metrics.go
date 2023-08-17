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

package batchprocessor // import "go.opentelemetry.io/collector/processor/batchprocessor"

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	"go.opentelemetry.io/collector/internal/obsreportconfig/obsmetrics"
	"go.opentelemetry.io/collector/obsreport"
)

var (
	processorTagKey          = tag.MustNewKey(obsmetrics.ProcessorKey)
	statBatchSizeTriggerSend = stats.Int64("batch_size_trigger_send", "Number of times the batch was sent due to a size trigger", stats.UnitDimensionless)
	statTimeoutTriggerSend   = stats.Int64("timeout_trigger_send", "Number of times the batch was sent due to a timeout trigger", stats.UnitDimensionless)
	statBatchSendSize        = stats.Int64("batch_send_size", "Number of units in the batch", stats.UnitDimensionless)
	statBatchSendSizeBytes   = stats.Int64("batch_send_size_bytes", "Number of bytes in batch that was sent", stats.UnitBytes)
)

// MetricViews returns the metrics views related to batching
func MetricViews() []*view.View {
	processorTagKeys := []tag.Key{processorTagKey}

	countBatchSizeTriggerSendView := &view.View{
		Name:        obsreport.BuildProcessorCustomMetricName(typeStr, statBatchSizeTriggerSend.Name()),
		Measure:     statBatchSizeTriggerSend,
		Description: statBatchSizeTriggerSend.Description(),
		TagKeys:     processorTagKeys,
		Aggregation: view.Sum(),
	}

	countTimeoutTriggerSendView := &view.View{
		Name:        obsreport.BuildProcessorCustomMetricName(typeStr, statTimeoutTriggerSend.Name()),
		Measure:     statTimeoutTriggerSend,
		Description: statTimeoutTriggerSend.Description(),
		TagKeys:     processorTagKeys,
		Aggregation: view.Sum(),
	}

	distributionBatchSendSizeView := &view.View{
		Name:        obsreport.BuildProcessorCustomMetricName(typeStr, statBatchSendSize.Name()),
		Measure:     statBatchSendSize,
		Description: statBatchSendSize.Description(),
		TagKeys:     processorTagKeys,
		Aggregation: view.Distribution(10, 25, 50, 75, 100, 250, 500, 750, 1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000, 10000, 20000, 30000, 50000, 100000),
	}

	distributionBatchSendSizeBytesView := &view.View{
		Name:        obsreport.BuildProcessorCustomMetricName(typeStr, statBatchSendSizeBytes.Name()),
		Measure:     statBatchSendSizeBytes,
		Description: statBatchSendSizeBytes.Description(),
		TagKeys:     processorTagKeys,
		Aggregation: view.Distribution(10, 25, 50, 75, 100, 250, 500, 750, 1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000, 10000, 20000, 30000, 50000,
			100_000, 200_000, 300_000, 400_000, 500_000, 600_000, 700_000, 800_00, 900_000,
			1000_000, 2000_000, 3000_000, 4000_000, 5000_000, 6000_000, 7000_000, 8000_000, 9000_000),
	}

	return []*view.View{
		countBatchSizeTriggerSendView,
		countTimeoutTriggerSendView,
		distributionBatchSendSizeView,
		distributionBatchSendSizeBytesView,
	}
}
