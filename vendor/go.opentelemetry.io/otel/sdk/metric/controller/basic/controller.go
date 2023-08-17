// Copyright The OpenTelemetry Authors
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

package basic // import "go.opentelemetry.io/otel/sdk/metric/controller/basic"

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	controllerTime "go.opentelemetry.io/otel/sdk/metric/controller/time"
	"go.opentelemetry.io/otel/sdk/metric/export"
	"go.opentelemetry.io/otel/sdk/metric/registry"
	"go.opentelemetry.io/otel/sdk/metric/sdkapi"
	"go.opentelemetry.io/otel/sdk/resource"
)

// DefaultPeriod is used for:
//
// - the minimum time between calls to Collect()
// - the timeout for Export()
// - the timeout for Collect().
const DefaultPeriod = 10 * time.Second

// ErrControllerStarted indicates that a controller was started more
// than once.
var ErrControllerStarted = fmt.Errorf("controller already started")

// Controller organizes and synchronizes collection of metric data in
// both "pull" and "push" configurations.  This supports two distinct
// modes:
//
// - Push and Pull: Start() must be called to begin calling the exporter;
//   Collect() is called periodically by a background thread after starting
//   the controller.
// - Pull-Only: Start() is optional in this case, to call Collect periodically.
//   If Start() is not called, Collect() can be called manually to initiate
//   collection
//
// The controller supports mixing push and pull access to metric data
// using the export.Reader RWLock interface.  Collection will
// be blocked by a pull request in the basic controller.
type Controller struct {
	// lock synchronizes Start() and Stop().
	lock                sync.Mutex
	scopes              sync.Map
	checkpointerFactory export.CheckpointerFactory

	resource *resource.Resource
	exporter export.Exporter
	wg       sync.WaitGroup
	stopCh   chan struct{}
	clock    controllerTime.Clock
	ticker   controllerTime.Ticker

	collectPeriod  time.Duration
	collectTimeout time.Duration
	pushTimeout    time.Duration

	// collectedTime is used only in configurations with no
	// exporter, when ticker != nil.
	collectedTime time.Time
}

var _ export.InstrumentationLibraryReader = &Controller{}
var _ metric.MeterProvider = &Controller{}

// Meter returns a new Meter defined by instrumentationName and configured
// with opts.
func (c *Controller) Meter(instrumentationName string, opts ...metric.MeterOption) metric.Meter {
	cfg := metric.NewMeterConfig(opts...)
	scope := instrumentation.Scope{
		Name:      instrumentationName,
		Version:   cfg.InstrumentationVersion(),
		SchemaURL: cfg.SchemaURL(),
	}

	m, ok := c.scopes.Load(scope)
	if !ok {
		checkpointer := c.checkpointerFactory.NewCheckpointer()
		m, _ = c.scopes.LoadOrStore(
			scope,
			registry.NewUniqueInstrumentMeterImpl(&accumulatorCheckpointer{
				Accumulator:  sdk.NewAccumulator(checkpointer),
				checkpointer: checkpointer,
				scope:        scope,
			}))
	}
	return sdkapi.WrapMeterImpl(m.(*registry.UniqueInstrumentMeterImpl))
}

type accumulatorCheckpointer struct {
	*sdk.Accumulator
	checkpointer export.Checkpointer
	scope        instrumentation.Scope
}

var _ sdkapi.MeterImpl = &accumulatorCheckpointer{}

// New constructs a Controller using the provided checkpointer factory
// and options (including optional exporter) to configure a metric
// export pipeline.
func New(checkpointerFactory export.CheckpointerFactory, opts ...Option) *Controller {
	c := config{
		CollectPeriod:  DefaultPeriod,
		CollectTimeout: DefaultPeriod,
		PushTimeout:    DefaultPeriod,
	}
	for _, opt := range opts {
		c = opt.apply(c)
	}
	if c.Resource == nil {
		c.Resource = resource.Default()
	} else {
		var err error
		c.Resource, err = resource.Merge(resource.Environment(), c.Resource)
		if err != nil {
			otel.Handle(err)
		}
	}
	return &Controller{
		checkpointerFactory: checkpointerFactory,
		exporter:            c.Exporter,
		resource:            c.Resource,
		stopCh:              nil,
		clock:               controllerTime.RealClock{},

		collectPeriod:  c.CollectPeriod,
		collectTimeout: c.CollectTimeout,
		pushTimeout:    c.PushTimeout,
	}
}

// SetClock supports setting a mock clock for testing.  This must be
// called before Start().
func (c *Controller) SetClock(clock controllerTime.Clock) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.clock = clock
}

// Resource returns the *resource.Resource associated with this
// controller.
func (c *Controller) Resource() *resource.Resource {
	return c.resource
}

// Start begins a ticker that periodically collects and exports
// metrics with the configured interval.  This is required for calling
// a configured Exporter (see WithExporter) and is otherwise optional
// when only pulling metric data.
//
// The passed context is passed to Collect() and subsequently to
// asynchronous instrument callbacks.  Returns an error when the
// controller was already started.
//
// Note that it is not necessary to Start a controller when only
// pulling data; use the Collect() and ForEach() methods directly in
// this case.
func (c *Controller) Start(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.stopCh != nil {
		return ErrControllerStarted
	}

	c.wg.Add(1)
	c.stopCh = make(chan struct{})
	c.ticker = c.clock.Ticker(c.collectPeriod)
	go c.runTicker(ctx, c.stopCh)
	return nil
}

// Stop waits for the background goroutine to return and then collects
// and exports metrics one last time before returning.  The passed
// context is passed to the final Collect() and subsequently to the
// final asynchronous instruments.
//
// Note that Stop() will not cancel an ongoing collection or export.
func (c *Controller) Stop(ctx context.Context) error {
	if lastCollection := func() bool {
		c.lock.Lock()
		defer c.lock.Unlock()

		if c.stopCh == nil {
			return false
		}

		close(c.stopCh)
		c.stopCh = nil
		c.wg.Wait()
		c.ticker.Stop()
		c.ticker = nil
		return true
	}(); !lastCollection {
		return nil
	}
	return c.collect(ctx)
}

// runTicker collection on ticker events until the stop channel is closed.
func (c *Controller) runTicker(ctx context.Context, stopCh chan struct{}) {
	defer c.wg.Done()
	for {
		select {
		case <-stopCh:
			return
		case <-c.ticker.C():
			if err := c.collect(ctx); err != nil {
				otel.Handle(err)
			}
		}
	}
}

// collect computes a checkpoint and optionally exports it.
func (c *Controller) collect(ctx context.Context) error {
	if err := c.checkpoint(ctx); err != nil {
		return err
	}
	if c.exporter == nil {
		return nil
	}

	// Note: this is not subject to collectTimeout.  This blocks the next
	// collection despite collectTimeout because it holds a lock.
	return c.export(ctx)
}

// accumulatorList returns a snapshot of current accumulators
// registered to this controller.  This briefly locks the controller.
func (c *Controller) accumulatorList() []*accumulatorCheckpointer {
	var r []*accumulatorCheckpointer
	c.scopes.Range(func(key, value interface{}) bool {
		acc, ok := value.(*registry.UniqueInstrumentMeterImpl).MeterImpl().(*accumulatorCheckpointer)
		if ok {
			r = append(r, acc)
		}
		return true
	})
	return r
}

// checkpoint calls the Accumulator and Checkpointer interfaces to
// compute the Reader.  This applies the configured collection
// timeout.  Note that this does not try to cancel a Collect or Export
// when Stop() is called.
func (c *Controller) checkpoint(ctx context.Context) error {
	for _, impl := range c.accumulatorList() {
		if err := c.checkpointSingleAccumulator(ctx, impl); err != nil {
			return err
		}
	}
	return nil
}

// checkpointSingleAccumulator checkpoints a single instrumentation
// scope's accumulator, which involves calling
// checkpointer.StartCollection, accumulator.Collect, and
// checkpointer.FinishCollection in sequence.
func (c *Controller) checkpointSingleAccumulator(ctx context.Context, ac *accumulatorCheckpointer) error {
	ckpt := ac.checkpointer.Reader()
	ckpt.Lock()
	defer ckpt.Unlock()

	ac.checkpointer.StartCollection()

	if c.collectTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.collectTimeout)
		defer cancel()
	}

	_ = ac.Accumulator.Collect(ctx)

	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	default:
		// The context wasn't done, ok.
	}

	// Finish the checkpoint whether the accumulator timed out or not.
	if cerr := ac.checkpointer.FinishCollection(); cerr != nil {
		if err == nil {
			err = cerr
		} else {
			err = fmt.Errorf("%s: %w", cerr.Error(), err)
		}
	}

	return err
}

// export calls the exporter with a read lock on the Reader,
// applying the configured export timeout.
func (c *Controller) export(ctx context.Context) error { // nolint:revive  // method name shadows import.
	if c.pushTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.pushTimeout)
		defer cancel()
	}

	return c.exporter.Export(ctx, c.resource, c)
}

// ForEach implements export.InstrumentationLibraryReader.
func (c *Controller) ForEach(readerFunc func(l instrumentation.Library, r export.Reader) error) error {
	for _, acPair := range c.accumulatorList() {
		reader := acPair.checkpointer.Reader()
		// TODO: We should not fail fast; instead accumulate errors.
		if err := func() error {
			reader.RLock()
			defer reader.RUnlock()
			return readerFunc(acPair.scope, reader)
		}(); err != nil {
			return err
		}
	}
	return nil
}

// IsRunning returns true if the controller was started via Start(),
// indicating that the current export.Reader is being kept
// up-to-date.
func (c *Controller) IsRunning() bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.ticker != nil
}

// Collect requests a collection.  The collection will be skipped if
// the last collection is aged less than the configured collection
// period.
func (c *Controller) Collect(ctx context.Context) error {
	if c.IsRunning() {
		// When there's a non-nil ticker, there's a goroutine
		// computing checkpoints with the collection period.
		return ErrControllerStarted
	}
	if !c.shouldCollect() {
		return nil
	}

	return c.checkpoint(ctx)
}

// shouldCollect returns true if the collector should collect now,
// based on the timestamp, the last collection time, and the
// configured period.
func (c *Controller) shouldCollect() bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.collectPeriod == 0 {
		return true
	}
	now := c.clock.Now()
	if now.Sub(c.collectedTime) < c.collectPeriod {
		return false
	}
	c.collectedTime = now
	return true
}
