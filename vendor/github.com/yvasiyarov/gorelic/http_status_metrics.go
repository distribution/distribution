package gorelic

import (
	"fmt"

	"github.com/yvasiyarov/go-metrics"
	"github.com/yvasiyarov/newrelic_platform_go"
)

// New metrica collector - counter per each http status code.
type counterByStatusMetrica struct {
	counter metrics.Counter
	name    string
	units   string
}

// metrics.IMetrica interface implementation.
func (m *counterByStatusMetrica) GetName() string { return m.name }

func (m *counterByStatusMetrica) GetUnits() string { return m.units }

func (m *counterByStatusMetrica) GetValue() (float64, error) { return float64(m.counter.Count()), nil }

// addHTTPStatusMetricsToComponent initializes counter metrics for all http statuses and adds them to the component.
func addHTTPStatusMetricsToComponent(component newrelic_platform_go.IComponent, statusCounters map[int]metrics.Counter) {
	for statusCode, counter := range statusCounters {
		component.AddMetrica(&counterByStatusMetrica{
			counter: counter,
			name:    fmt.Sprintf("http/status/%d", statusCode),
			units:   "count",
		})
	}
}
