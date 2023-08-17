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

package time // import "go.opentelemetry.io/otel/sdk/metric/controller/time"

import (
	"time"
)

// Several types below are created to match "github.com/benbjohnson/clock"
// so that it remains a test-only dependency.

// Clock keeps track of time for a metric SDK.
type Clock interface {
	Now() time.Time
	Ticker(duration time.Duration) Ticker
}

// Ticker signals time intervals.
type Ticker interface {
	Stop()
	C() <-chan time.Time
}

// RealClock wraps the time package and uses the system time to tell time.
type RealClock struct {
}

// RealTicker wraps the time package and uses system time to tick time
// intervals.
type RealTicker struct {
	ticker *time.Ticker
}

var _ Clock = RealClock{}
var _ Ticker = RealTicker{}

// Now returns the current time.
func (RealClock) Now() time.Time {
	return time.Now()
}

// Ticker creates a new RealTicker that will tick with period.
func (RealClock) Ticker(period time.Duration) Ticker {
	return RealTicker{time.NewTicker(period)}
}

// Stop turns off the RealTicker.
func (t RealTicker) Stop() {
	t.ticker.Stop()
}

// C returns a channel that receives the current time when RealTicker ticks.
func (t RealTicker) C() <-chan time.Time {
	return t.ticker.C
}
