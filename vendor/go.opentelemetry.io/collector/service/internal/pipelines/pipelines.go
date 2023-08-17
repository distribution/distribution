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

package pipelines // import "go.opentelemetry.io/collector/service/internal/pipelines"

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"go.uber.org/multierr"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/service/internal/components"
	"go.opentelemetry.io/collector/service/internal/fanoutconsumer"
	"go.opentelemetry.io/collector/service/internal/zpages"
)

const (
	zPipelineName  = "zpipelinename"
	zComponentName = "zcomponentname"
	zComponentKind = "zcomponentkind"
)

// baseConsumer redeclared here since not public in consumer package. May consider to make that public.
type baseConsumer interface {
	Capabilities() consumer.Capabilities
}

type builtComponent struct {
	id   config.ComponentID
	comp component.Component
}

type builtPipeline struct {
	lastConsumer baseConsumer

	receivers  []builtComponent
	processors []builtComponent
	exporters  []builtComponent
}

// Pipelines is set of all pipelines created from exporter configs.
type Pipelines struct {
	telemetry component.TelemetrySettings

	allReceivers map[config.DataType]map[config.ComponentID]component.Receiver
	allExporters map[config.DataType]map[config.ComponentID]component.Exporter

	pipelines map[config.ComponentID]*builtPipeline
}

// StartAll starts all pipelines.
//
// Start with exporters, processors (in reverse configured order), then receivers.
// This is important so that components that are earlier in the pipeline and reference components that are
// later in the pipeline do not start sending data to later components which are not yet started.
func (bps *Pipelines) StartAll(ctx context.Context, host component.Host) error {
	bps.telemetry.Logger.Info("Starting exporters...")
	for dt, expByID := range bps.allExporters {
		for expID, exp := range expByID {
			expLogger := exporterLogger(bps.telemetry.Logger, expID, dt)
			expLogger.Info("Exporter is starting...")
			if err := exp.Start(ctx, components.NewHostWrapper(host, expLogger)); err != nil {
				return err
			}
			expLogger.Info("Exporter started.")
		}
	}

	bps.telemetry.Logger.Info("Starting processors...")
	for pipelineID, bp := range bps.pipelines {
		for i := len(bp.processors) - 1; i >= 0; i-- {
			procLogger := processorLogger(bps.telemetry.Logger, bp.processors[i].id, pipelineID)
			procLogger.Info("Processor is starting...")
			if err := bp.processors[i].comp.Start(ctx, components.NewHostWrapper(host, procLogger)); err != nil {
				return err
			}
			procLogger.Info("Processor started.")
		}
	}

	bps.telemetry.Logger.Info("Starting receivers...")
	for dt, recvByID := range bps.allReceivers {
		for recvID, recv := range recvByID {
			recvLogger := receiverLogger(bps.telemetry.Logger, recvID, dt)
			recvLogger.Info("Receiver is starting...")
			if err := recv.Start(ctx, components.NewHostWrapper(host, recvLogger)); err != nil {
				return err
			}
			recvLogger.Info("Receiver started.")
		}
	}
	return nil
}

// ShutdownAll stops all pipelines.
//
// Shutdown order is the reverse of starting: receivers, processors, then exporters.
// This gives senders a chance to send all their data to a not "shutdown" component.
func (bps *Pipelines) ShutdownAll(ctx context.Context) error {
	var errs error
	bps.telemetry.Logger.Info("Stopping receivers...")
	for _, recvByID := range bps.allReceivers {
		for _, recv := range recvByID {
			errs = multierr.Append(errs, recv.Shutdown(ctx))
		}
	}

	bps.telemetry.Logger.Info("Stopping processors...")
	for _, bp := range bps.pipelines {
		for _, p := range bp.processors {
			errs = multierr.Append(errs, p.comp.Shutdown(ctx))
		}
	}

	bps.telemetry.Logger.Info("Stopping exporters...")
	for _, expByID := range bps.allExporters {
		for _, exp := range expByID {
			errs = multierr.Append(errs, exp.Shutdown(ctx))
		}
	}

	return errs
}

func (bps *Pipelines) GetExporters() map[config.DataType]map[config.ComponentID]component.Exporter {
	exportersMap := make(map[config.DataType]map[config.ComponentID]component.Exporter)

	exportersMap[config.TracesDataType] = make(map[config.ComponentID]component.Exporter, len(bps.allExporters[config.TracesDataType]))
	exportersMap[config.MetricsDataType] = make(map[config.ComponentID]component.Exporter, len(bps.allExporters[config.MetricsDataType]))
	exportersMap[config.LogsDataType] = make(map[config.ComponentID]component.Exporter, len(bps.allExporters[config.LogsDataType]))

	for dt, expByID := range bps.allExporters {
		for expID, exp := range expByID {
			exportersMap[dt][expID] = exp
		}
	}

	return exportersMap
}

func (bps *Pipelines) HandleZPages(w http.ResponseWriter, r *http.Request) {
	qValues := r.URL.Query()
	pipelineName := qValues.Get(zPipelineName)
	componentName := qValues.Get(zComponentName)
	componentKind := qValues.Get(zComponentKind)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	zpages.WriteHTMLPageHeader(w, zpages.HeaderData{Title: "Pipelines"})
	zpages.WriteHTMLPipelinesSummaryTable(w, bps.getPipelinesSummaryTableData())
	if pipelineName != "" && componentName != "" && componentKind != "" {
		fullName := componentName
		if componentKind == "processor" {
			fullName = pipelineName + "/" + componentName
		}
		zpages.WriteHTMLComponentHeader(w, zpages.ComponentHeaderData{
			Name: componentKind + ": " + fullName,
		})
		// TODO: Add config + status info.
	}
	zpages.WriteHTMLPageFooter(w)
}

// Settings holds configuration for building Pipelines.
type Settings struct {
	Telemetry component.TelemetrySettings
	BuildInfo component.BuildInfo

	// ReceiverFactories maps receiver type names in the config to the respective component.ReceiverFactory.
	ReceiverFactories map[config.Type]component.ReceiverFactory

	// ReceiverConfigs is a map of config.ComponentID to config.Receiver.
	ReceiverConfigs map[config.ComponentID]config.Receiver

	// ProcessorFactories maps processor type names in the config to the respective component.ProcessorFactory.
	ProcessorFactories map[config.Type]component.ProcessorFactory

	// ProcessorConfigs is a map of config.ComponentID to config.Processor.
	ProcessorConfigs map[config.ComponentID]config.Processor

	// ExporterFactories maps exporter type names in the config to the respective component.ExporterFactory.
	ExporterFactories map[config.Type]component.ExporterFactory

	// ExporterConfigs is a map of config.ComponentID to config.Exporter.
	ExporterConfigs map[config.ComponentID]config.Exporter

	// PipelineConfigs is a map of config.ComponentID to config.Pipeline.
	PipelineConfigs map[config.ComponentID]*config.Pipeline
}

// Build builds all pipelines from config.
func Build(ctx context.Context, set Settings) (*Pipelines, error) {
	exps := &Pipelines{
		telemetry:    set.Telemetry,
		allReceivers: make(map[config.DataType]map[config.ComponentID]component.Receiver),
		allExporters: make(map[config.DataType]map[config.ComponentID]component.Exporter),
		pipelines:    make(map[config.ComponentID]*builtPipeline, len(set.PipelineConfigs)),
	}

	receiversConsumers := make(map[config.DataType]map[config.ComponentID][]baseConsumer)

	// Iterate over all pipelines, and create exporters, then processors.
	// Receivers cannot be created since we need to know all consumers, a.k.a. we need all pipelines build up to the
	// first processor.
	for pipelineID, pipeline := range set.PipelineConfigs {
		// The data type of the pipeline defines what data type each exporter is expected to receive.
		if _, ok := exps.allExporters[pipelineID.Type()]; !ok {
			exps.allExporters[pipelineID.Type()] = make(map[config.ComponentID]component.Exporter)
		}
		expByID := exps.allExporters[pipelineID.Type()]

		bp := &builtPipeline{
			receivers:  make([]builtComponent, len(pipeline.Receivers)),
			processors: make([]builtComponent, len(pipeline.Processors)),
			exporters:  make([]builtComponent, len(pipeline.Exporters)),
		}
		exps.pipelines[pipelineID] = bp

		// Iterate over all Exporters for this pipeline.
		for i, expID := range pipeline.Exporters {
			// If already created an exporter for this [DataType, ComponentID] nothing to do, will reuse this instance.
			if exp, ok := expByID[expID]; ok {
				bp.exporters[i] = builtComponent{id: expID, comp: exp}
				continue
			}

			exp, err := buildExporter(ctx, set.Telemetry, set.BuildInfo, set.ExporterConfigs, set.ExporterFactories, expID, pipelineID)
			if err != nil {
				return nil, err
			}

			bp.exporters[i] = builtComponent{id: expID, comp: exp}
			expByID[expID] = exp
		}

		// Build a fan out consumer to all exporters.
		switch pipelineID.Type() {
		case config.TracesDataType:
			bp.lastConsumer = buildFanOutExportersTracesConsumer(bp.exporters)
		case config.MetricsDataType:
			bp.lastConsumer = buildFanOutExportersMetricsConsumer(bp.exporters)
		case config.LogsDataType:
			bp.lastConsumer = buildFanOutExportersLogsConsumer(bp.exporters)
		default:
			return nil, fmt.Errorf("create fan-out exporter in pipeline %q, data type %q is not supported", pipelineID, pipelineID.Type())
		}

		mutatesConsumedData := bp.lastConsumer.Capabilities().MutatesData
		// Build the processors backwards, starting from the last one.
		// The last processor points to fan out consumer to all Exporters, then the processor itself becomes a
		// consumer for the one that precedes it in the pipeline and so on.
		for i := len(pipeline.Processors) - 1; i >= 0; i-- {
			procID := pipeline.Processors[i]

			proc, err := buildProcessor(ctx, set.Telemetry, set.BuildInfo, set.ProcessorConfigs, set.ProcessorFactories, procID, pipelineID, bp.lastConsumer)
			if err != nil {
				return nil, err
			}

			bp.processors[i] = builtComponent{id: procID, comp: proc}
			bp.lastConsumer = proc.(baseConsumer)
			mutatesConsumedData = mutatesConsumedData || bp.lastConsumer.Capabilities().MutatesData
		}

		// Some consumers may not correctly implement the Capabilities, and ignore the next consumer when calculated the Capabilities.
		// Because of this wrap the first consumer if any consumers in the pipeline mutate the data and the first says that it doesn't.
		switch pipelineID.Type() {
		case config.TracesDataType:
			bp.lastConsumer = capTraces{Traces: bp.lastConsumer.(consumer.Traces), cap: consumer.Capabilities{MutatesData: mutatesConsumedData}}
		case config.MetricsDataType:
			bp.lastConsumer = capMetrics{Metrics: bp.lastConsumer.(consumer.Metrics), cap: consumer.Capabilities{MutatesData: mutatesConsumedData}}
		case config.LogsDataType:
			bp.lastConsumer = capLogs{Logs: bp.lastConsumer.(consumer.Logs), cap: consumer.Capabilities{MutatesData: mutatesConsumedData}}
		default:
			return nil, fmt.Errorf("create cap consumer in pipeline %q, data type %q is not supported", pipelineID, pipelineID.Type())
		}

		// The data type of the pipeline defines what data type each exporter is expected to receive.
		if _, ok := receiversConsumers[pipelineID.Type()]; !ok {
			receiversConsumers[pipelineID.Type()] = make(map[config.ComponentID][]baseConsumer)
		}
		recvConsByID := receiversConsumers[pipelineID.Type()]
		// Iterate over all Receivers for this pipeline and just append the lastConsumer as a consumer for the receiver.
		for _, recvID := range pipeline.Receivers {
			recvConsByID[recvID] = append(recvConsByID[recvID], bp.lastConsumer)
		}
	}

	// Now that we built the `receiversConsumers` map, we can build the receivers as well.
	for pipelineID, pipeline := range set.PipelineConfigs {
		// The data type of the pipeline defines what data type each exporter is expected to receive.
		if _, ok := exps.allReceivers[pipelineID.Type()]; !ok {
			exps.allReceivers[pipelineID.Type()] = make(map[config.ComponentID]component.Receiver)
		}
		recvByID := exps.allReceivers[pipelineID.Type()]
		bp := exps.pipelines[pipelineID]

		// Iterate over all Receivers for this pipeline.
		for i, recvID := range pipeline.Receivers {
			// If already created a receiver for this [DataType, ComponentID] nothing to do.
			if exp, ok := recvByID[recvID]; ok {
				bp.receivers[i] = builtComponent{id: recvID, comp: exp}
				continue
			}

			recv, err := buildReceiver(ctx, set.Telemetry, set.BuildInfo, set.ReceiverConfigs, set.ReceiverFactories, recvID, pipelineID, receiversConsumers[pipelineID.Type()][recvID])
			if err != nil {
				return nil, err
			}

			bp.receivers[i] = builtComponent{id: recvID, comp: recv}
			recvByID[recvID] = recv
		}
	}
	return exps, nil
}

func buildExporter(
	ctx context.Context,
	settings component.TelemetrySettings,
	buildInfo component.BuildInfo,
	cfgs map[config.ComponentID]config.Exporter,
	factories map[config.Type]component.ExporterFactory,
	id config.ComponentID,
	pipelineID config.ComponentID,
) (component.Exporter, error) {
	cfg, existsCfg := cfgs[id]
	if !existsCfg {
		return nil, fmt.Errorf("exporter %q is not configured", id)
	}

	factory, existsFactory := factories[id.Type()]
	if !existsFactory {
		return nil, fmt.Errorf("exporter factory not available for: %q", id)
	}

	set := component.ExporterCreateSettings{
		TelemetrySettings: settings,
		BuildInfo:         buildInfo,
	}
	set.TelemetrySettings.Logger = exporterLogger(settings.Logger, id, pipelineID.Type())
	components.LogStabilityLevel(set.TelemetrySettings.Logger, getExporterStabilityLevel(factory, pipelineID.Type()))

	exp, err := createExporter(ctx, set, cfg, id, pipelineID, factory)
	if err != nil {
		return nil, fmt.Errorf("failed to create %q exporter, in pipeline %q: %w", id, pipelineID, err)
	}

	return exp, nil
}

func createExporter(ctx context.Context, set component.ExporterCreateSettings, cfg config.Exporter, id config.ComponentID, pipelineID config.ComponentID, factory component.ExporterFactory) (component.Exporter, error) {
	switch pipelineID.Type() {
	case config.TracesDataType:
		return factory.CreateTracesExporter(ctx, set, cfg)

	case config.MetricsDataType:
		return factory.CreateMetricsExporter(ctx, set, cfg)

	case config.LogsDataType:
		return factory.CreateLogsExporter(ctx, set, cfg)
	}
	return nil, fmt.Errorf("error creating exporter %q in pipeline %q, data type %q is not supported", id, pipelineID, pipelineID.Type())
}

func buildFanOutExportersTracesConsumer(exporters []builtComponent) consumer.Traces {
	consumers := make([]consumer.Traces, 0, len(exporters))
	for _, exp := range exporters {
		consumers = append(consumers, exp.comp.(consumer.Traces))
	}
	// Create a junction point that fans out to all allExporters.
	return fanoutconsumer.NewTraces(consumers)
}

func buildFanOutExportersMetricsConsumer(exporters []builtComponent) consumer.Metrics {
	consumers := make([]consumer.Metrics, 0, len(exporters))
	for _, exp := range exporters {
		consumers = append(consumers, exp.comp.(consumer.Metrics))
	}
	// Create a junction point that fans out to all allExporters.
	return fanoutconsumer.NewMetrics(consumers)
}

func buildFanOutExportersLogsConsumer(exporters []builtComponent) consumer.Logs {
	consumers := make([]consumer.Logs, 0, len(exporters))
	for _, exp := range exporters {
		consumers = append(consumers, exp.comp.(consumer.Logs))
	}
	// Create a junction point that fans out to all allExporters.
	return fanoutconsumer.NewLogs(consumers)
}

func exporterLogger(logger *zap.Logger, id config.ComponentID, dt config.DataType) *zap.Logger {
	return logger.With(
		zap.String(components.ZapKindKey, components.ZapKindExporter),
		zap.String(components.ZapDataTypeKey, string(dt)),
		zap.String(components.ZapNameKey, id.String()))
}

func getExporterStabilityLevel(factory component.ExporterFactory, dt config.DataType) component.StabilityLevel {
	switch dt {
	case config.TracesDataType:
		return factory.TracesExporterStability()
	case config.MetricsDataType:
		return factory.MetricsExporterStability()
	case config.LogsDataType:
		return factory.LogsExporterStability()
	}
	return component.StabilityLevelUndefined
}

func buildProcessor(ctx context.Context,
	settings component.TelemetrySettings,
	buildInfo component.BuildInfo,
	cfgs map[config.ComponentID]config.Processor,
	factories map[config.Type]component.ProcessorFactory,
	id config.ComponentID,
	pipelineID config.ComponentID,
	next baseConsumer,
) (component.Processor, error) {
	procCfg, existsCfg := cfgs[id]
	if !existsCfg {
		return nil, fmt.Errorf("processor %q is not configured", id)
	}

	factory, existsFactory := factories[id.Type()]
	if !existsFactory {
		return nil, fmt.Errorf("processor factory not available for: %q", id)
	}

	set := component.ProcessorCreateSettings{
		TelemetrySettings: settings,
		BuildInfo:         buildInfo,
	}
	set.TelemetrySettings.Logger = processorLogger(settings.Logger, id, pipelineID)
	components.LogStabilityLevel(set.TelemetrySettings.Logger, getProcessorStabilityLevel(factory, pipelineID.Type()))

	proc, err := createProcessor(ctx, set, procCfg, id, pipelineID, next, factory)
	if err != nil {
		return nil, fmt.Errorf("failed to create %q processor, in pipeline %q: %w", id, pipelineID, err)
	}
	return proc, nil
}

func createProcessor(ctx context.Context, set component.ProcessorCreateSettings, cfg config.Processor, id config.ComponentID, pipelineID config.ComponentID, next baseConsumer, factory component.ProcessorFactory) (component.Processor, error) {
	switch pipelineID.Type() {
	case config.TracesDataType:
		return factory.CreateTracesProcessor(ctx, set, cfg, next.(consumer.Traces))

	case config.MetricsDataType:
		return factory.CreateMetricsProcessor(ctx, set, cfg, next.(consumer.Metrics))

	case config.LogsDataType:
		return factory.CreateLogsProcessor(ctx, set, cfg, next.(consumer.Logs))
	}
	return nil, fmt.Errorf("error creating processor %q in pipeline %q, data type %q is not supported", id, pipelineID, pipelineID.Type())
}

func processorLogger(logger *zap.Logger, procID config.ComponentID, pipelineID config.ComponentID) *zap.Logger {
	return logger.With(
		zap.String(components.ZapKindKey, components.ZapKindProcessor),
		zap.String(components.ZapNameKey, procID.String()),
		zap.String(components.ZapKindPipeline, pipelineID.String()))
}

func getProcessorStabilityLevel(factory component.ProcessorFactory, dt config.DataType) component.StabilityLevel {
	switch dt {
	case config.TracesDataType:
		return factory.TracesProcessorStability()
	case config.MetricsDataType:
		return factory.MetricsProcessorStability()
	case config.LogsDataType:
		return factory.LogsProcessorStability()
	}
	return component.StabilityLevelUndefined
}

func buildReceiver(ctx context.Context,
	settings component.TelemetrySettings,
	buildInfo component.BuildInfo,
	cfgs map[config.ComponentID]config.Receiver,
	factories map[config.Type]component.ReceiverFactory,
	id config.ComponentID,
	pipelineID config.ComponentID,
	nexts []baseConsumer,
) (component.Receiver, error) {
	cfg, existsCfg := cfgs[id]
	if !existsCfg {
		return nil, fmt.Errorf("receiver %q is not configured", id)
	}

	factory, existsFactory := factories[id.Type()]
	if !existsFactory {
		return nil, fmt.Errorf("receiver factory not available for: %q", id)
	}

	set := component.ReceiverCreateSettings{
		TelemetrySettings: settings,
		BuildInfo:         buildInfo,
	}
	set.TelemetrySettings.Logger = receiverLogger(settings.Logger, id, pipelineID.Type())
	components.LogStabilityLevel(set.TelemetrySettings.Logger, getReceiverStabilityLevel(factory, pipelineID.Type()))

	recv, err := createReceiver(ctx, set, cfg, id, pipelineID, nexts, factory)
	if err != nil {
		return nil, fmt.Errorf("failed to create %q receiver, in pipeline %q: %w", id, pipelineID, err)
	}

	return recv, nil
}

func createReceiver(ctx context.Context, set component.ReceiverCreateSettings, cfg config.Receiver, id config.ComponentID, pipelineID config.ComponentID, nexts []baseConsumer, factory component.ReceiverFactory) (component.Receiver, error) {
	switch pipelineID.Type() {
	case config.TracesDataType:
		var consumers []consumer.Traces
		for _, next := range nexts {
			consumers = append(consumers, next.(consumer.Traces))
		}
		return factory.CreateTracesReceiver(ctx, set, cfg, fanoutconsumer.NewTraces(consumers))
	case config.MetricsDataType:
		var consumers []consumer.Metrics
		for _, next := range nexts {
			consumers = append(consumers, next.(consumer.Metrics))
		}
		return factory.CreateMetricsReceiver(ctx, set, cfg, fanoutconsumer.NewMetrics(consumers))
	case config.LogsDataType:
		var consumers []consumer.Logs
		for _, next := range nexts {
			consumers = append(consumers, next.(consumer.Logs))
		}
		return factory.CreateLogsReceiver(ctx, set, cfg, fanoutconsumer.NewLogs(consumers))
	}
	return nil, fmt.Errorf("error creating receiver %q in pipeline %q, data type %q is not supported", id, pipelineID, pipelineID.Type())
}

func receiverLogger(logger *zap.Logger, id config.ComponentID, dt config.DataType) *zap.Logger {
	return logger.With(
		zap.String(components.ZapKindKey, components.ZapKindReceiver),
		zap.String(components.ZapNameKey, id.String()),
		zap.String(components.ZapKindPipeline, string(dt)))
}

func getReceiverStabilityLevel(factory component.ReceiverFactory, dt config.DataType) component.StabilityLevel {
	switch dt {
	case config.TracesDataType:
		return factory.TracesReceiverStability()
	case config.MetricsDataType:
		return factory.MetricsReceiverStability()
	case config.LogsDataType:
		return factory.LogsReceiverStability()
	}
	return component.StabilityLevelUndefined
}

func (bps *Pipelines) getPipelinesSummaryTableData() zpages.SummaryPipelinesTableData {
	sumData := zpages.SummaryPipelinesTableData{}
	sumData.Rows = make([]zpages.SummaryPipelinesTableRowData, 0, len(bps.pipelines))
	for c, p := range bps.pipelines {
		// TODO: Change the template to use ID.
		var recvs []string
		for _, bRecv := range p.receivers {
			recvs = append(recvs, bRecv.id.String())
		}
		var procs []string
		for _, bProc := range p.processors {
			procs = append(procs, bProc.id.String())
		}
		var exps []string
		for _, bExp := range p.exporters {
			exps = append(exps, bExp.id.String())
		}
		row := zpages.SummaryPipelinesTableRowData{
			FullName:    c.String(),
			InputType:   string(c.Type()),
			MutatesData: p.lastConsumer.Capabilities().MutatesData,
			Receivers:   recvs,
			Processors:  procs,
			Exporters:   exps,
		}
		sumData.Rows = append(sumData.Rows, row)
	}

	sort.Slice(sumData.Rows, func(i, j int) bool {
		return sumData.Rows[i].FullName < sumData.Rows[j].FullName
	})
	return sumData
}
