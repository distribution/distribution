package gorelic

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	metrics "github.com/yvasiyarov/go-metrics"
	"github.com/yvasiyarov/newrelic_platform_go"
)

const (
	// DefaultNewRelicPollInterval - how often we will report metrics to NewRelic.
	// Recommended values is 60 seconds
	DefaultNewRelicPollInterval = 60

	// DefaultGcPollIntervalInSeconds - how often we will get garbage collector run statistic
	// Default value is - every 10 seconds
	// During GC stat pooling - mheap will be locked, so be carefull changing this value
	DefaultGcPollIntervalInSeconds = 10

	// DefaultMemoryAllocatorPollIntervalInSeconds - how often we will get memory allocator statistic.
	// Default value is - every 60 seconds
	// During this process stoptheword() is called, so be carefull changing this value
	DefaultMemoryAllocatorPollIntervalInSeconds = 60

	//DefaultAgentGuid is plugin ID in NewRelic.
	//You should not change it unless you want to create your own plugin.
	DefaultAgentGuid = "com.github.yvasiyarov.GoRelic"

	//CurrentAgentVersion is plugin version
	CurrentAgentVersion = "0.0.6"

	//DefaultAgentName in NewRelic GUI. You can change it.
	DefaultAgentName = "Go daemon"
)

//Agent - is NewRelic agent implementation.
//Agent start separate go routine which will report data to NewRelic
type Agent struct {
	NewrelicName                string
	NewrelicLicense             string
	NewrelicPollInterval        int
	Verbose                     bool
	CollectGcStat               bool
	CollectMemoryStat           bool
	CollectHTTPStat             bool
	CollectHTTPStatuses         bool
	GCPollInterval              int
	MemoryAllocatorPollInterval int
	AgentGUID                   string
	AgentVersion                string
	plugin                      *newrelic_platform_go.NewrelicPlugin
	HTTPTimer                   metrics.Timer
	HTTPStatusCounters          map[int]metrics.Counter
	Tracer                      *Tracer
	CustomMetrics               []newrelic_platform_go.IMetrica

	// All HTTP requests will be done using this client. Change it if you need
	// to use a proxy.
	Client http.Client
}

// NewAgent builds new Agent objects.
func NewAgent() *Agent {
	agent := &Agent{
		NewrelicName:                DefaultAgentName,
		NewrelicPollInterval:        DefaultNewRelicPollInterval,
		Verbose:                     false,
		CollectGcStat:               true,
		CollectMemoryStat:           true,
		GCPollInterval:              DefaultGcPollIntervalInSeconds,
		MemoryAllocatorPollInterval: DefaultMemoryAllocatorPollIntervalInSeconds,
		AgentGUID:                   DefaultAgentGuid,
		AgentVersion:                CurrentAgentVersion,
		Tracer:                      nil,
		CustomMetrics:               make([]newrelic_platform_go.IMetrica, 0),
	}
	return agent
}

// our custom component
type resettableComponent struct {
	newrelic_platform_go.IComponent
	counters map[int]metrics.Counter
}

// newrelic_platform_go.IComponent interface implementation
func (c resettableComponent) ClearSentData() {
	c.IComponent.ClearSentData()
	for _, counter := range c.counters {
		counter.Clear()
	}
}

//WrapHTTPHandlerFunc  instrument HTTP handler functions to collect HTTP metrics
func (agent *Agent) WrapHTTPHandlerFunc(h tHTTPHandlerFunc) tHTTPHandlerFunc {
	agent.CollectHTTPStat = true
	agent.initTimer()
	return func(w http.ResponseWriter, req *http.Request) {
		proxy := newHTTPHandlerFunc(h)
		proxy.timer = agent.HTTPTimer
		//set the http status counters before serving request.
		proxy.httpStatusCounters = agent.HTTPStatusCounters
		proxy.ServeHTTP(w, req)
	}
}

//WrapHTTPHandler  instrument HTTP handler object to collect HTTP metrics
func (agent *Agent) WrapHTTPHandler(h http.Handler) http.Handler {
	agent.CollectHTTPStat = true
	agent.initTimer()

	proxy := newHTTPHandler(h)
	proxy.timer = agent.HTTPTimer
	return proxy
}

//AddCustomMetric adds metric to be collected periodically with NewrelicPollInterval interval
func (agent *Agent) AddCustomMetric(metric newrelic_platform_go.IMetrica) {
	agent.CustomMetrics = append(agent.CustomMetrics, metric)
}

//Run initialize Agent instance and start harvest go routine
func (agent *Agent) Run() error {
	if agent.NewrelicLicense == "" {
		return errors.New("please, pass a valid newrelic license key")
	}

	var component newrelic_platform_go.IComponent
	component = newrelic_platform_go.NewPluginComponent(agent.NewrelicName, agent.AgentGUID, agent.Verbose)

	// Add default metrics and tracer.
	addRuntimeMericsToComponent(component)
	agent.Tracer = newTracer(component)

	// Check agent flags and add relevant metrics.
	if agent.CollectGcStat {
		addGCMetricsToComponent(component, agent.GCPollInterval)
		agent.debug(fmt.Sprintf("Init GC metrics collection. Poll interval %d seconds.", agent.GCPollInterval))
	}

	if agent.CollectMemoryStat {
		addMemoryMericsToComponent(component, agent.MemoryAllocatorPollInterval)
		agent.debug(fmt.Sprintf("Init memory allocator metrics collection. Poll interval %d seconds.", agent.MemoryAllocatorPollInterval))
	}

	if agent.CollectHTTPStat {
		agent.initTimer()
		addHTTPMericsToComponent(component, agent.HTTPTimer)
		agent.debug(fmt.Sprintf("Init HTTP metrics collection."))
	}

	for _, metric := range agent.CustomMetrics {
		component.AddMetrica(metric)
		agent.debug(fmt.Sprintf("Init %s metric collection.", metric.GetName()))
	}

	if agent.CollectHTTPStatuses {
		agent.initStatusCounters()
		component = &resettableComponent{component, agent.HTTPStatusCounters}
		addHTTPStatusMetricsToComponent(component, agent.HTTPStatusCounters)
		agent.debug(fmt.Sprintf("Init HTTP status metrics collection."))
	}

	// Init newrelic reporting plugin.
	agent.plugin = newrelic_platform_go.NewNewrelicPlugin(agent.AgentVersion, agent.NewrelicLicense, agent.NewrelicPollInterval)
	agent.plugin.Client = agent.Client
	agent.plugin.Verbose = agent.Verbose

	// Add our metrics component to the plugin.
	agent.plugin.AddComponent(component)

	// Start reporting!
	go agent.plugin.Run()
	return nil
}

//Initialize global metrics.Timer object, used to collect HTTP metrics
func (agent *Agent) initTimer() {
	if agent.HTTPTimer == nil {
		agent.HTTPTimer = metrics.NewTimer()
	}
}

//Initialize metrics.Counters objects, used to collect HTTP statuses
func (agent *Agent) initStatusCounters() {
	httpStatuses := []int{
		http.StatusContinue, http.StatusSwitchingProtocols,

		http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNonAuthoritativeInfo,
		http.StatusNoContent, http.StatusResetContent, http.StatusPartialContent,

		http.StatusMultipleChoices, http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusNotModified, http.StatusUseProxy, http.StatusTemporaryRedirect,

		http.StatusBadRequest, http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden,
		http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotAcceptable, http.StatusProxyAuthRequired,
		http.StatusRequestTimeout, http.StatusConflict, http.StatusGone, http.StatusLengthRequired,
		http.StatusPreconditionFailed, http.StatusRequestEntityTooLarge, http.StatusRequestURITooLong, http.StatusUnsupportedMediaType,
		http.StatusRequestedRangeNotSatisfiable, http.StatusExpectationFailed, http.StatusTeapot,

		http.StatusInternalServerError, http.StatusNotImplemented, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusHTTPVersionNotSupported,
	}

	agent.HTTPStatusCounters = make(map[int]metrics.Counter, len(httpStatuses))
	for _, statusCode := range httpStatuses {
		agent.HTTPStatusCounters[statusCode] = metrics.NewCounter()
	}
}

//Print debug messages
func (agent *Agent) debug(msg string) {
	if agent.Verbose {
		log.Println(msg)
	}
}
