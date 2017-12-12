package gorelic

import (
	metrics "github.com/yvasiyarov/go-metrics"
	"github.com/yvasiyarov/newrelic_platform_go"
	"net/http"
	"time"
)

type tHTTPHandlerFunc func(http.ResponseWriter, *http.Request)
type tHTTPHandler struct {
	originalHandler     http.Handler
	originalHandlerFunc tHTTPHandlerFunc
	isFunc              bool
	timer               metrics.Timer
	httpStatusCounters  map[int]metrics.Counter
}

//A wrapper over a http.ResponseWriter object
type responseWriterWrapper struct {
	originalWriter http.ResponseWriter
	statusCode     int
}

var httpTimer metrics.Timer

func (reponseWriterWrapper *responseWriterWrapper) Header() http.Header {
	return reponseWriterWrapper.originalWriter.Header()
}

func (reponseWriterWrapper *responseWriterWrapper) Write(bytes []byte) (int, error) {
	return reponseWriterWrapper.originalWriter.Write(bytes)
}

func (reponseWriterWrapper *responseWriterWrapper) WriteHeader(code int) {
	reponseWriterWrapper.originalWriter.WriteHeader(code)
	reponseWriterWrapper.statusCode = code
}

func newHTTPHandlerFunc(h tHTTPHandlerFunc) *tHTTPHandler {
	return &tHTTPHandler{
		isFunc:              true,
		originalHandlerFunc: h,
	}
}
func newHTTPHandler(h http.Handler) *tHTTPHandler {
	return &tHTTPHandler{
		isFunc:          false,
		originalHandler: h,
	}
}

func (handler *tHTTPHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	startTime := time.Now()
	defer handler.timer.UpdateSince(startTime)

	responseWriterWrapper := responseWriterWrapper{originalWriter: w, statusCode: http.StatusOK}

	if handler.isFunc {
		handler.originalHandlerFunc(&responseWriterWrapper, req)
	} else {
		handler.originalHandler.ServeHTTP(&responseWriterWrapper, req)
	}

	if statusCounter := handler.httpStatusCounters[responseWriterWrapper.statusCode]; statusCounter != nil {
		statusCounter.Inc(1)
	}
}

func addHTTPMericsToComponent(component newrelic_platform_go.IComponent, timer metrics.Timer) {
	rate1 := &timerRate1Metrica{
		baseTimerMetrica: &baseTimerMetrica{
			name:       "http/throughput/1minute",
			units:      "rps",
			dataSource: timer,
		},
	}
	component.AddMetrica(rate1)

	rateMean := &timerRateMeanMetrica{
		baseTimerMetrica: &baseTimerMetrica{
			name:       "http/throughput/rateMean",
			units:      "rps",
			dataSource: timer,
		},
	}
	component.AddMetrica(rateMean)

	responseTimeMean := &timerMeanMetrica{
		baseTimerMetrica: &baseTimerMetrica{
			name:       "http/responseTime/mean",
			units:      "ms",
			dataSource: timer,
		},
	}
	component.AddMetrica(responseTimeMean)

	responseTimeMax := &timerMaxMetrica{
		baseTimerMetrica: &baseTimerMetrica{
			name:       "http/responseTime/max",
			units:      "ms",
			dataSource: timer,
		},
	}
	component.AddMetrica(responseTimeMax)

	responseTimeMin := &timerMinMetrica{
		baseTimerMetrica: &baseTimerMetrica{
			name:       "http/responseTime/min",
			units:      "ms",
			dataSource: timer,
		},
	}
	component.AddMetrica(responseTimeMin)

	responseTimePercentile75 := &timerPercentile75Metrica{
		baseTimerMetrica: &baseTimerMetrica{
			name:       "http/responseTime/percentile75",
			units:      "ms",
			dataSource: timer,
		},
	}
	component.AddMetrica(responseTimePercentile75)

	responseTimePercentile90 := &timerPercentile90Metrica{
		baseTimerMetrica: &baseTimerMetrica{
			name:       "http/responseTime/percentile90",
			units:      "ms",
			dataSource: timer,
		},
	}
	component.AddMetrica(responseTimePercentile90)

	responseTimePercentile95 := &timerPercentile95Metrica{
		baseTimerMetrica: &baseTimerMetrica{
			name:       "http/responseTime/percentile95",
			units:      "ms",
			dataSource: timer,
		},
	}
	component.AddMetrica(responseTimePercentile95)
}
