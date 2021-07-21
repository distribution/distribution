package trace

import (
	"fmt"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-client-go/log"
	"time"
)

var GlobalTrace opentracing.Tracer

func InitTrace(c *configuration.Configuration) {
	// TODO
	cfg, err := config.FromEnv()
	if err != nil {
		panic(err)
	}
	cfg.ServiceName = ServerName
	cfg.Sampler.Type = "const"
	cfg.Sampler.Param = 1
	cfg.Reporter.LogSpans = true
	time.Sleep(100 * time.Millisecond)

	//reporter := jaeger.NewLoggingReporter(&cmd.TraceLogger)
	logger := config.Logger(log.DebugLogAdapter(jaeger.StdLogger))
	tracer, _, err := cfg.NewTracer(logger)
	if err != nil {
		panic(fmt.Sprintf("ERROR: cannot init Jaeger :%v\n", err))
	}
	GlobalTrace = tracer
}
