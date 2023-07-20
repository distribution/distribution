// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/newrelic/go-agent/v3/internal"
	"github.com/newrelic/go-agent/v3/internal/logger"
	"github.com/newrelic/go-agent/v3/internal/sysinfo"
	"github.com/newrelic/go-agent/v3/internal/utilization"
)

// Config contains Application and Transaction behavior settings.
type Config struct {
	// AppName is used by New Relic to link data across servers.
	//
	// https://docs.newrelic.com/docs/apm/new-relic-apm/installation-configuration/naming-your-application
	AppName string

	// License is your New Relic license key.
	//
	// https://docs.newrelic.com/docs/accounts/install-new-relic/account-setup/license-key
	License string

	// Logger controls Go Agent logging.
	//
	// See https://github.com/newrelic/go-agent/blob/master/GUIDE.md#logging
	// for more examples and logging integrations.
	Logger Logger

	// Enabled controls whether the agent will communicate with the New Relic
	// servers and spawn goroutines.  Setting this to be false is useful in
	// testing and staging situations.
	Enabled bool

	// Labels are key value pairs used to roll up applications into specific
	// categories.
	//
	// https://docs.newrelic.com/docs/using-new-relic/user-interface-functions/organize-your-data/labels-categories-organize-apps-monitors
	Labels map[string]string

	// HighSecurity guarantees that certain agent settings can not be made
	// more permissive.  This setting must match the corresponding account
	// setting in the New Relic UI.
	//
	// https://docs.newrelic.com/docs/agents/manage-apm-agents/configuration/high-security-mode
	HighSecurity bool

	// SecurityPoliciesToken enables security policies if set to a non-empty
	// string.  Only set this if security policies have been enabled on your
	// account.  This cannot be used in conjunction with HighSecurity.
	//
	// https://docs.newrelic.com/docs/agents/manage-apm-agents/configuration/enable-configurable-security-policies
	SecurityPoliciesToken string

	// CustomInsightsEvents controls the behavior of
	// Application.RecordCustomEvent.
	//
	// https://docs.newrelic.com/docs/insights/new-relic-insights/adding-querying-data/inserting-custom-events-new-relic-apm-agents
	CustomInsightsEvents struct {
		// Enabled controls whether RecordCustomEvent will collect
		// custom analytics events.  High security mode overrides this
		// setting.
		Enabled bool
		// MaxSamplesStored sets the desired maximum custom event samples stored
		MaxSamplesStored int
	}

	// TransactionEvents controls the behavior of transaction analytics
	// events.
	TransactionEvents struct {
		// Enabled controls whether transaction events are captured.
		Enabled bool
		// Attributes controls the attributes included with transaction
		// events.
		Attributes AttributeDestinationConfig
		// MaxSamplesStored allows you to limit the number of Transaction
		// Events stored/reported in a given 60-second period
		MaxSamplesStored int
	}

	// ErrorCollector controls the capture of errors.
	ErrorCollector struct {
		// Enabled controls whether errors are captured.  This setting
		// affects both traced errors and error analytics events.
		Enabled bool
		// CaptureEvents controls whether error analytics events are
		// captured.
		CaptureEvents bool
		// IgnoreStatusCodes controls which http response codes are
		// automatically turned into errors.  By default, response codes
		// greater than or equal to 400 or less than 100 -- with the exception
		// of 0, 5, and 404 -- are turned into errors.
		IgnoreStatusCodes []int
		// ExpectStatusCodes controls which http response codes should
		// impact your error metrics, apdex score and alerts. Expected errors will
		// be silently captured without impacting any of those. Note that setting an error
		// code as Ignored will prevent it from being collected, even if its expected.
		ExpectStatusCodes []int
		// Attributes controls the attributes included with errors.
		Attributes AttributeDestinationConfig
		// RecordPanics controls whether or not a deferred
		// Transaction.End will attempt to recover panics, record them
		// as errors, and then re-panic them.  By default, this is
		// set to false.
		RecordPanics bool
		// ErrorGroupCallback is a user defined callback function that takes an error as an input
		// and returns a string that will be applied to an error to put it in an error group.
		//
		// If no error group is identified for a given error, this function should return an empty string.
		// If an ErrorGroupCallbeck is defined, it will be executed against every error the go agent notices that
		// is not ignored.
		//
		// example function:
		//
		// func ErrorGroupCallback(errorInfo newrelic.ErrorInfo) string {
		//		if errorInfo.Class == "403" && errorInfo.GetUserId() == "example user" {
		//			return "customer X payment issue"
		// 		}
		//
		//		// returning empty string causes default error grouping behavior
		//		return ""
		// }
		ErrorGroupCallback `json:"-"`
	}

	// TransactionTracer controls the capture of transaction traces.
	TransactionTracer struct {
		// Enabled controls whether transaction traces are captured.
		Enabled bool
		// Threshold controls whether a transaction trace will be
		// considered for capture.  Of the traces exceeding the
		// threshold, the slowest trace every minute is captured.
		Threshold struct {
			// If IsApdexFailing is true then the trace threshold is
			// four times the apdex threshold.
			IsApdexFailing bool
			// If IsApdexFailing is false then this field is the
			// threshold, otherwise it is ignored.
			Duration time.Duration
		}
		// Attributes controls the attributes included with transaction
		// traces.
		Attributes AttributeDestinationConfig
		// Segments contains fields which control the behavior of
		// transaction trace segments.
		Segments struct {
			// StackTraceThreshold is the threshold at which
			// segments will be given a stack trace in the
			// transaction trace.  Lowering this setting will
			// increase overhead.
			StackTraceThreshold time.Duration
			// Threshold is the threshold at which segments will be
			// added to the trace.  Lowering this setting may
			// increase overhead.  Decrease this duration if your
			// transaction traces are missing segments.
			Threshold time.Duration
			// Attributes controls the attributes included with each
			// trace segment.
			Attributes AttributeDestinationConfig
		}
	}

	// BrowserMonitoring contains settings which control the behavior of
	// Transaction.BrowserTimingHeader.
	BrowserMonitoring struct {
		// Enabled controls whether or not the Browser monitoring feature is
		// enabled.
		Enabled bool
		// Attributes controls the attributes included with Browser monitoring.
		// BrowserMonitoring.Attributes.Enabled is false by default, to include
		// attributes in the Browser timing Javascript:
		//
		//	cfg.BrowserMonitoring.Attributes.Enabled = true
		Attributes AttributeDestinationConfig
	}

	// HostDisplayName gives this server a recognizable name in the New
	// Relic UI.  This is an optional setting.
	HostDisplayName string

	// Transport customizes communication with the New Relic servers.  This may
	// be used to configure a proxy.
	Transport http.RoundTripper

	// Utilization controls the detection and gathering of system
	// information.
	Utilization struct {
		// DetectAWS controls whether the Application attempts to detect
		// AWS.
		DetectAWS bool
		// DetectAzure controls whether the Application attempts to detect
		// Azure.
		DetectAzure bool
		// DetectPCF controls whether the Application attempts to detect
		// PCF.
		DetectPCF bool
		// DetectGCP controls whether the Application attempts to detect
		// GCP.
		DetectGCP bool
		// DetectDocker controls whether the Application attempts to
		// detect Docker.
		DetectDocker bool
		// DetectKubernetes controls whether the Application attempts to
		// detect Kubernetes.
		DetectKubernetes bool

		// These settings provide system information when custom values
		// are required.
		LogicalProcessors int
		TotalRAMMIB       int
		BillingHostname   string
	}

	// Heroku controls the behavior of Heroku specific features.
	Heroku struct {
		// UseDynoNames controls if Heroku dyno names are reported as the
		// hostname.  Default is true.
		UseDynoNames bool
		// DynoNamePrefixesToShorten allows you to shorten and combine some
		// Heroku dyno names into a single value.  Ordinarily the agent reports
		// dyno names with a trailing dot and process ID (for example,
		// worker.3). You can remove this trailing data by specifying the
		// prefixes you want to report without trailing data (for example,
		// worker.*).  Defaults to shortening "scheduler" and "run" dyno names.
		DynoNamePrefixesToShorten []string
	}

	// CrossApplicationTracer controls behavior relating to cross application
	// tracing (CAT).  In the case where CrossApplicationTracer and
	// DistributedTracer are both enabled, DistributedTracer takes precedence.
	//
	// https://docs.newrelic.com/docs/apm/transactions/cross-application-traces/introduction-cross-application-traces
	CrossApplicationTracer struct {
		Enabled bool
	}

	// DistributedTracer controls behavior relating to Distributed Tracing.  In
	// the case where CrossApplicationTracer and DistributedTracer are both
	// enabled, DistributedTracer takes precedence.
	//
	// https://docs.newrelic.com/docs/apm/distributed-tracing/getting-started/introduction-distributed-tracing
	DistributedTracer struct {
		Enabled bool
		// ExcludeNewRelicHeader allows you to choose whether to insert the New
		// Relic Distributed Tracing header on outbound requests, which by
		// default is emitted along with the W3C trace context headers.  Set
		// this value to true if you do not want to include the New Relic
		// distributed tracing header in your outbound requests.
		//
		// Disabling the New Relic header here does not prevent the agent from
		// accepting *inbound* New Relic headers.
		ExcludeNewRelicHeader bool
		// ReservoirLimit sets the desired maximum span event reservoir limit
		// for collecting span event data. The collector MAY override this value.
		ReservoirLimit int
	}

	// SpanEvents controls behavior relating to Span Events.  Span Events
	// require that DistributedTracer is enabled.
	SpanEvents struct {
		Enabled bool
		// Attributes controls the attributes included on Spans.
		Attributes AttributeDestinationConfig
	}

	// InfiniteTracing controls behavior related to Infinite Tracing tail based
	// sampling.  InfiniteTracing requires that both DistributedTracer and
	// SpanEvents are enabled.
	//
	// https://docs.newrelic.com/docs/understand-dependencies/distributed-tracing/enable-configure/enable-distributed-tracing
	InfiniteTracing struct {
		// TraceObserver controls behavior of connecting to the Trace Observer.
		TraceObserver struct {
			// Host is the Trace Observer host to connect to and tells the
			// Application to enable Infinite Tracing support. When this field
			// is set to an empty string, which is the default, Infinite
			// Tracing support is disabled.
			Host string
			// Port is the Trace Observer port to connect to. The default is
			// 443.
			Port int
		}
		// SpanEvents controls the behavior of the span events sent to the
		// Trace Observer.
		SpanEvents struct {
			// QueueSize is the maximum number of span events that may be held
			// in memory as they wait to be serialized and sent to the Trace
			// Observer.  Default value is 10,000. Any span event created when
			// the QueueSize limit is reached will be discarded.
			QueueSize int
		}
	}

	// DatastoreTracer controls behavior relating to datastore segments.
	DatastoreTracer struct {
		// InstanceReporting controls whether the host and port are collected
		// for datastore segments.
		InstanceReporting struct {
			Enabled bool
		}
		// DatabaseNameReporting controls whether the database name is
		// collected for datastore segments.
		DatabaseNameReporting struct {
			Enabled bool
		}
		QueryParameters struct {
			Enabled bool
		}
		RawQuery struct {
			Enabled bool
		}

		// SlowQuery controls the capture of slow query traces.  Slow
		// query traces show you instances of your slowest datastore
		// segments.
		SlowQuery struct {
			Enabled   bool
			Threshold time.Duration
		}
	}

	// Config Settings for Logs in Context features
	ApplicationLogging ApplicationLogging

	// Attributes controls which attributes are enabled and disabled globally.
	// This setting affects all attribute destinations: Transaction Events,
	// Error Events, Transaction Traces and segments, Traced Errors, Span
	// Events, and Browser timing header.
	Attributes AttributeDestinationConfig

	// RuntimeSampler controls the collection of runtime statistics like
	// CPU/Memory usage, goroutine count, and GC pauses.
	RuntimeSampler struct {
		// Enabled controls whether runtime statistics are captured.
		Enabled bool
	}

	// ServerlessMode contains fields which control behavior when running in
	// AWS Lambda.
	//
	// https://docs.newrelic.com/docs/serverless-function-monitoring/aws-lambda-monitoring/get-started/introduction-new-relic-monitoring-aws-lambda
	ServerlessMode struct {
		// Enabling ServerlessMode will print each transaction's data to
		// stdout.  No agent goroutines will be spawned in serverless mode, and
		// no data will be sent directly to the New Relic backend.
		// nrlambda.NewConfig sets Enabled to true.
		Enabled bool
		// ApdexThreshold sets the Apdex threshold when in ServerlessMode.  The
		// default is 500 milliseconds.  nrlambda.NewConfig populates this
		// field using the NEW_RELIC_APDEX_T environment variable.
		//
		// https://docs.newrelic.com/docs/apm/new-relic-apm/apdex/apdex-measure-user-satisfaction
		ApdexThreshold time.Duration
		// AccountID, TrustedAccountKey, and PrimaryAppID are used for
		// distributed tracing in ServerlessMode.  AccountID and
		// TrustedAccountKey must be populated for distributed tracing to be
		// enabled. nrlambda.NewConfig populates these fields using the
		// NEW_RELIC_ACCOUNT_ID, NEW_RELIC_TRUSTED_ACCOUNT_KEY, and
		// NEW_RELIC_PRIMARY_APPLICATION_ID environment variables.
		AccountID         string
		TrustedAccountKey string
		PrimaryAppID      string
	}

	// Host can be used to override the New Relic endpoint.
	Host string

	// Error may be populated by the ConfigOptions provided to NewApplication
	// to indicate that setup has failed.  NewApplication will return this
	// error if it is set.
	Error error

	// CodeLevelMetrics contains fields which control the collection and reporting
	// of source code context information associated with telemetry data.
	CodeLevelMetrics struct {
		// Enabling CodeLevelMetrics will include source code context information
		// as attributes. If this is disabled, no such metrics will be collected
		// or reported.
		Enabled bool
		// RedactPathPrefixes, if true, will redact a non-nil list of PathPrefixes
		// from the configuration data transmitted by the agent.
		RedactPathPrefixes bool
		// RedactIgnoredPrefixes, if true, will redact a non-nil list of IgnoredPrefixes
		// from the configuration data transmitted by the agent.
		RedactIgnoredPrefixes bool
		// Scope is a combination of CodeLevelMetricsScope values OR-ed together
		// to indicate which specific kinds of events will carry CodeLevelMetrics
		// data. This allows the agent to spend resources on discovering the source
		// code context data only where actually needed.
		Scope CodeLevelMetricsScope
		// PathPrefixes specifies a slice of filename patterns that describe the start of
		// the project area. Any text before any of these patterns is ignored. Thus, if
		// PathPrefixes is set to ["myproject/src", "otherproject/src"], then a function located in a file
		// called "/usr/local/src/myproject/src/foo.go" will be reported with the
		// pathname "myproject/src/foo.go". If this value is nil, the full path
		// will be reported (e.g., "/usr/local/src/myproject/src/foo.go").
		// The first string in the slice which is found in a file pathname will be the one
		// used to truncate that filename; if none of the strings in PathPrefixes are found
		// anywhere in a file's pathname, the full path will be reported.
		PathPrefixes []string
		// PathPrefix specifies the filename pattern that describes the start of
		// the project area. Any text before this pattern is ignored. Thus, if
		// PathPrefix is set to "myproject/src", then a function located in a file
		// called "/usr/local/src/myproject/src/foo.go" will be reported with the
		// pathname "myproject/src/foo.go". If this value is empty, the full path
		// will be reported (e.g., "/usr/local/src/myproject/src/foo.go").
		//
		// Deprecated: new code should use PathPrefixes instead (or better yet,
		// use the ConfigCodeLevelMetricsPathPrefix option, which accepts any number
		// of string parameters for backwards compatibility).
		PathPrefix string
		// IgnoredPrefix holds a single module path prefix to ignore when searching
		// to find the calling function to be reported.
		//
		// Deprecated: new code should use IgnoredPrefixes instead (or better yet,
		// use the ConfigCodeLevelMetricsIgnoredPrefix option, which accepts any number
		// of string parameters for backwards compatibility).
		IgnoredPrefix string
		// IgnoredPrefixes specifies a slice of initial patterns to look for in fully-qualified
		// function names to determine which functions to ignore while searching up
		// through the call stack to find the application function to associate
		// with telemetry data. The agent will look for the innermost caller whose name
		// does not begin with one of these prefixes. If empty, it will ignore functions whose
		// names look like they are internal to the agent itself.
		IgnoredPrefixes []string
	}

	// ModuleDependencyMetrics controls reporting of the packages used to build the instrumented
	// application, to help manage project dependencies.
	ModuleDependencyMetrics struct {
		// Enabled controls whether the module dependencies are collected and reported.
		Enabled bool
		// RedactIgnoredPrefixes, if true, redacts a non-nil list of IgnoredPrefixes from
		// the configuration data transmitted by the agent.
		RedactIgnoredPrefixes bool
		// IgnoredPrefixes is a list of module path prefixes. Any module whose import pathname
		// begins with one of these prefixes is excluded from the dependency reporting.
		// This list of ignored prefixes itself is not reported outside the agent.
		IgnoredPrefixes []string
	}
}

// CodeLevelMetricsScope is a bit-encoded value. Each such value describes
// a trace type for which code-level metrics are to be collected and
// reported.
type CodeLevelMetricsScope uint32

// These constants specify the types of telemetry data to which we will
// attach code level metric data.
//
// Currently, this includes
//
//	TransactionCLM            any kind of transaction
//	AllCLM                    all kinds of telemetry data for which CLM is implemented (the default)
//
// The zero value of CodeLevelMetricsScope means "all types" as a convenience so that
// new variables of this type provide the default expected behavior
// rather than, say, turning off all code level metrics as a 0 bit value would otherwise imply.
// Otherwise the numeric values of these constants are not to be relied
// upon and are subject to change. Only use the named constant identifiers in
// your code. We do not recommend saving the raw numeric value of these constants
// to use later.
const (
	TransactionCLM CodeLevelMetricsScope = 1 << iota
	AllCLM         CodeLevelMetricsScope = 0
)

// CodeLevelMetricsScopeLabelToValue accepts a number of string values representing
// the possible scope restrictions available for the agent, returning the
// CodeLevelMetricsScope value which represents the combination of all of the given
// labels. This value is suitable to be presented to ConfigCodeLevelMetricsScope.
//
// It also returns a boolean flag; if true, it was able to understand all of the
// provided labels; otherwise, one or more of the values were not recognized and
// thus the returned CodeLevelMetricsScope value may be incomplete (although it
// will represent any valid label strings passed, if any).
//
// Currently, this function recognizes the following labels:
//
//	for AllCLM: "all" (if this value appears anywhere in the list of strings, AllCLM will be returned)
//	for TransactionCLM: "transaction", "transactions", "txn"
func CodeLevelMetricsScopeLabelToValue(labels ...string) (CodeLevelMetricsScope, bool) {
	var scope CodeLevelMetricsScope
	ok := true

	for _, label := range labels {
		switch label {
		case "":

		case "all":
			return AllCLM, true

		case "transaction", "transactions", "txn":
			scope |= TransactionCLM

		default:
			ok = false
		}
	}
	return scope, ok
}

// UnmarshalText allows for a CodeLevelMetricsScope value to be read from a JSON
// string (or other text encodings) whose value is a comma-separated list of scope labels.
func (s *CodeLevelMetricsScope) UnmarshalText(b []byte) error {
	var ok bool

	if *s, ok = CodeLevelMetricsScopeLabelListToValue(string(b)); !ok {
		return fmt.Errorf("invalid code level metrics scope label value")
	}

	return nil
}

// MarshalText allows for a CodeLevelMetrics value to be encoded into JSON strings and other
// text encodings.
func (s CodeLevelMetricsScope) MarshalText() ([]byte, error) {
	if s == 0 || s == AllCLM {
		return []byte("all"), nil
	}

	if (s & TransactionCLM) != 0 {
		return []byte("transaction"), nil
	}

	return nil, fmt.Errorf("unrecognized bit pattern in CodeLevelMetricsScope value")
}

// CodeLevelMetricsScopeLabelListToValue is a convenience function which
// is like CodeLevelMetricsScopeLabeltoValue except that it takes a single
// string which contains comma-separated values instead of an already-broken-out
// set of individual label strings.
func CodeLevelMetricsScopeLabelListToValue(labels string) (CodeLevelMetricsScope, bool) {
	return CodeLevelMetricsScopeLabelToValue(strings.Split(labels, ",")...)
}

// ApplicationLogging contains settings which control the capture and sending
// of log event data
type ApplicationLogging struct {
	// If this is disabled, all sub-features are disabled;
	// if it is enabled, the individual sub-feature configurations take effect.
	// MAY accomplish this by not installing instrumentation, or by early-return/no-op as necessary for an agent.
	Enabled bool
	// Forwarding controls log forwarding to New Relic One
	Forwarding struct {
		// Toggles whether the agent gathers log records for sending to New Relic.
		Enabled bool
		// Number of log records to send per minute to New Relic.
		// Controls the overall memory consumption when using log forwarding.
		// SHOULD be sent as part of the harvest_limits on Connect.
		MaxSamplesStored int
	}
	Metrics struct {
		// Toggles whether the agent gathers the the user facing Logging/lines and Logging/lines/{SEVERITY}
		// Logging Metrics used in the Logs chart on the APM Summary page.
		Enabled bool
	}
	LocalDecorating struct {
		// Toggles whether the agent enriches local logs printed to console so they can be sent to new relic for ingestion
		Enabled bool
	}
}

// AttributeDestinationConfig controls the attributes sent to each destination.
// For more information, see:
// https://docs.newrelic.com/docs/agents/manage-apm-agents/agent-data/agent-attributes
type AttributeDestinationConfig struct {
	// Enabled controls whether or not this destination will get any
	// attributes at all.  For example, to prevent any attributes from being
	// added to errors, set:
	//
	//	cfg.ErrorCollector.Attributes.Enabled = false
	//
	Enabled bool
	Include []string
	// Exclude allows you to prevent the capture of certain attributes.  For
	// example, to prevent the capture of the request URL attribute
	// "request.uri", set:
	//
	//	cfg.Attributes.Exclude = append(cfg.Attributes.Exclude, newrelic.AttributeRequestURI)
	//
	// The '*' character acts as a wildcard.  For example, to prevent the
	// capture of all request related attributes, set:
	//
	//	cfg.Attributes.Exclude = append(cfg.Attributes.Exclude, "request.*")
	//
	Exclude []string
}

// defaultConfig creates a Config populated with default settings.
func defaultConfig() Config {
	c := Config{}

	c.Enabled = true
	c.Labels = make(map[string]string)
	c.CustomInsightsEvents.Enabled = true
	c.CustomInsightsEvents.MaxSamplesStored = internal.MaxCustomEvents
	c.TransactionEvents.Enabled = true
	c.TransactionEvents.Attributes.Enabled = true
	c.TransactionEvents.MaxSamplesStored = internal.MaxTxnEvents
	c.HighSecurity = false
	c.ErrorCollector.Enabled = true
	c.ErrorCollector.CaptureEvents = true
	c.ErrorCollector.IgnoreStatusCodes = []int{
		// https://github.com/grpc/grpc/blob/master/doc/statuscodes.md
		0,                   // gRPC OK
		5,                   // gRPC NOT_FOUND
		http.StatusNotFound, // 404
	}
	c.ErrorCollector.Attributes.Enabled = true
	c.Utilization.DetectAWS = true
	c.Utilization.DetectAzure = true
	c.Utilization.DetectPCF = true
	c.Utilization.DetectGCP = true
	c.Utilization.DetectDocker = true
	c.Utilization.DetectKubernetes = true
	c.Attributes.Enabled = true
	c.RuntimeSampler.Enabled = true

	c.TransactionTracer.Enabled = true
	c.TransactionTracer.Threshold.IsApdexFailing = true
	c.TransactionTracer.Threshold.Duration = 500 * time.Millisecond
	c.TransactionTracer.Segments.Threshold = 2 * time.Millisecond
	c.TransactionTracer.Segments.StackTraceThreshold = 500 * time.Millisecond
	c.TransactionTracer.Attributes.Enabled = true
	c.TransactionTracer.Segments.Attributes.Enabled = true

	// Application Logging Settings
	c.ApplicationLogging.Enabled = true
	c.ApplicationLogging.Forwarding.Enabled = true
	c.ApplicationLogging.Forwarding.MaxSamplesStored = internal.MaxLogEvents
	c.ApplicationLogging.Metrics.Enabled = true
	c.ApplicationLogging.LocalDecorating.Enabled = false

	c.BrowserMonitoring.Enabled = true
	// browser monitoring attributes are disabled by default
	c.BrowserMonitoring.Attributes.Enabled = false

	c.CrossApplicationTracer.Enabled = false
	c.DistributedTracer.Enabled = true
	c.DistributedTracer.ReservoirLimit = internal.MaxSpanEvents
	c.SpanEvents.Enabled = true
	c.SpanEvents.Attributes.Enabled = true

	c.DatastoreTracer.InstanceReporting.Enabled = true
	c.DatastoreTracer.DatabaseNameReporting.Enabled = true
	c.DatastoreTracer.QueryParameters.Enabled = true
	c.DatastoreTracer.SlowQuery.Enabled = true
	c.DatastoreTracer.SlowQuery.Threshold = 10 * time.Millisecond
	c.DatastoreTracer.RawQuery.Enabled = false

	c.ServerlessMode.ApdexThreshold = 500 * time.Millisecond
	c.ServerlessMode.Enabled = false

	c.Heroku.UseDynoNames = true
	c.Heroku.DynoNamePrefixesToShorten = []string{"scheduler", "run"}

	c.InfiniteTracing.TraceObserver.Port = 443
	c.InfiniteTracing.SpanEvents.QueueSize = 10000

	// Code Level Metrics
	c.CodeLevelMetrics.Enabled = false
	c.CodeLevelMetrics.RedactPathPrefixes = true
	c.CodeLevelMetrics.RedactIgnoredPrefixes = true
	c.CodeLevelMetrics.Scope = AllCLM

	// Module Dependency Metrics
	c.ModuleDependencyMetrics.Enabled = true
	c.ModuleDependencyMetrics.RedactIgnoredPrefixes = true
	return c
}

const (
	licenseLength = 40
	appNameLimit  = 3
)

// The following errors will be returned if your Config fails to validate.
var (
	errLicenseLen                       = fmt.Errorf("license length is not %d", licenseLength)
	errAppNameMissing                   = errors.New("string AppName required")
	errAppNameLimit                     = fmt.Errorf("max of %d rollup application names", appNameLimit)
	errHighSecurityWithSecurityPolicies = errors.New("SecurityPoliciesToken and HighSecurity are incompatible; please ensure HighSecurity is set to false if SecurityPoliciesToken is a non-empty string and a security policy has been set for your account")
	errInfTracingServerless             = errors.New("ServerlessMode cannot be used with Infinite Tracing")
)

// validate checks the config for improper fields.  If the config is invalid,
// newrelic.NewApplication returns an error.
func (c Config) validate() error {
	if c.Enabled && !c.ServerlessMode.Enabled {
		if len(c.License) != licenseLength {
			return errLicenseLen
		}
	} else {
		// The License may be empty when the agent is not enabled.
		if len(c.License) != licenseLength && len(c.License) != 0 {
			return errLicenseLen
		}
	}
	if c.AppName == "" && c.Enabled && !c.ServerlessMode.Enabled {
		return errAppNameMissing
	}
	if c.HighSecurity && c.SecurityPoliciesToken != "" {
		return errHighSecurityWithSecurityPolicies
	}
	if strings.Count(c.AppName, ";") >= appNameLimit {
		return errAppNameLimit
	}
	if c.InfiniteTracing.TraceObserver.Host != "" && c.ServerlessMode.Enabled {
		return errInfTracingServerless
	}

	return nil
}

func (c Config) validateTraceObserverConfig() (*observerURL, error) {
	configHost := c.InfiniteTracing.TraceObserver.Host
	if configHost == "" {
		// This is the only instance from which we can return nil, nil.
		// If the user requests use of a trace observer, we must either provide
		// them with a valid observerURL _or_ alert them to the failure to do so.
		return nil, nil
	}
	if !versionSupports8T {
		return nil, errUnsupportedVersion
	}
	if !c.DistributedTracer.Enabled || !c.SpanEvents.Enabled {
		return nil, errSpanOrDTDisabled
	}
	return &observerURL{
		host:   fmt.Sprintf("%s:%d", configHost, c.InfiniteTracing.TraceObserver.Port),
		secure: configHost != localTestingHost,
	}, nil
}

// maxTxnEvents returns the configured maximum number of Transaction Events if it has been configured
// and is less than the default maximum; otherwise it returns the default max.
func (c Config) maxTxnEvents() int {
	configured := c.TransactionEvents.MaxSamplesStored
	if configured < 0 || configured > internal.MaxTxnEvents {
		return internal.MaxTxnEvents
	}
	return configured
}

// maxCustomEvents returns the configured maximum number of Custom Events if it has been configured
// and is less than the default maximum; otherwise it returns the default max.
func (c Config) maxCustomEvents() int {
	configured := c.CustomInsightsEvents.MaxSamplesStored
	if configured < 0 || configured > internal.MaxCustomEvents {
		return internal.MaxCustomEvents
	}
	return configured
}

// maxLogEvents returns the configured maximum number of Log Events if it has been configured
// and is less than the default maximum; otherwise it returns the default max.
func (c Config) maxLogEvents() int {
	configured := c.ApplicationLogging.Forwarding.MaxSamplesStored
	if configured < 0 || configured > internal.MaxLogEvents {
		return internal.MaxLogEvents
	}
	return configured
}

func copyDestConfig(c AttributeDestinationConfig) AttributeDestinationConfig {
	cp := c
	if nil != c.Include {
		cp.Include = make([]string, len(c.Include))
		copy(cp.Include, c.Include)
	}
	if nil != c.Exclude {
		cp.Exclude = make([]string, len(c.Exclude))
		copy(cp.Exclude, c.Exclude)
	}
	return cp
}

func copyConfigReferenceFields(cfg Config) Config {
	cp := cfg
	if nil != cfg.Labels {
		cp.Labels = make(map[string]string, len(cfg.Labels))
		for key, val := range cfg.Labels {
			cp.Labels[key] = val
		}
	}
	if cfg.ErrorCollector.IgnoreStatusCodes != nil {
		ignored := make([]int, len(cfg.ErrorCollector.IgnoreStatusCodes))
		copy(ignored, cfg.ErrorCollector.IgnoreStatusCodes)
		cp.ErrorCollector.IgnoreStatusCodes = ignored
	}

	cp.Attributes = copyDestConfig(cfg.Attributes)
	cp.ErrorCollector.Attributes = copyDestConfig(cfg.ErrorCollector.Attributes)
	cp.TransactionEvents.Attributes = copyDestConfig(cfg.TransactionEvents.Attributes)
	cp.TransactionTracer.Attributes = copyDestConfig(cfg.TransactionTracer.Attributes)
	cp.BrowserMonitoring.Attributes = copyDestConfig(cfg.BrowserMonitoring.Attributes)
	cp.SpanEvents.Attributes = copyDestConfig(cfg.SpanEvents.Attributes)
	cp.TransactionTracer.Segments.Attributes = copyDestConfig(cfg.TransactionTracer.Segments.Attributes)

	return cp
}

func transportSetting(t http.RoundTripper) interface{} {
	if nil == t {
		return nil
	}
	return fmt.Sprintf("%T", t)
}

func loggerSetting(lg Logger) interface{} {
	if nil == lg {
		return nil
	}
	if _, ok := lg.(logger.ShimLogger); ok {
		return nil
	}
	return fmt.Sprintf("%T", lg)
}

const (
	// https://source.datanerd.us/agents/agent-specs/blob/master/Custom-Host-Names.md
	hostByteLimit = 255
)

type settings Config

func (s settings) MarshalJSON() ([]byte, error) {
	c := Config(s)
	transport := c.Transport
	c.Transport = nil
	l := c.Logger
	c.Logger = nil

	js, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]interface{})
	err = json.Unmarshal(js, &fields)
	if err != nil {
		return nil, err
	}
	// The License field is not simply ignored by adding the `json:"-"` tag
	// to it since we want to allow consumers to populate Config from JSON.
	delete(fields, `License`)
	fields[`Transport`] = transportSetting(transport)
	fields[`Logger`] = loggerSetting(l)

	// Browser monitoring support.
	if c.BrowserMonitoring.Enabled {
		fields[`browser_monitoring.loader`] = "rum"
	}

	// Protect privacy for restricted fields
	if clmConfig, ok := fields["CodeLevelMetrics"]; ok {
		if clmMap, ok := clmConfig.(map[string]interface{}); ok {
			if c.CodeLevelMetrics.RedactIgnoredPrefixes && c.CodeLevelMetrics.IgnoredPrefixes != nil {
				delete(clmMap, "IgnoredPrefixes")
				delete(clmMap, "IgnoredPrefix")
			}
			if c.CodeLevelMetrics.RedactPathPrefixes && c.CodeLevelMetrics.PathPrefixes != nil {
				delete(clmMap, "PathPrefixes")
				delete(clmMap, "PathPrefix")
			}
		}
	}

	if mdmConfig, ok := fields["ModuleDependencyMetrics"]; ok {
		if mdmMap, ok := mdmConfig.(map[string]interface{}); ok {
			if c.ModuleDependencyMetrics.RedactIgnoredPrefixes && c.ModuleDependencyMetrics.IgnoredPrefixes != nil {
				delete(mdmMap, "IgnoredPrefixes")
			}
		}
	}

	return json.Marshal(fields)
}

// labels is used for connect JSON formatting.
type labels map[string]string

func (l labels) MarshalJSON() ([]byte, error) {
	ls := make([]struct {
		Key   string `json:"label_type"`
		Value string `json:"label_value"`
	}, len(l))

	i := 0
	for key, val := range l {
		ls[i].Key = key
		ls[i].Value = val
		i++
	}

	return json.Marshal(ls)
}

func configConnectJSONInternal(c Config, pid int, util *utilization.Data, e environment, version string, securityPolicies *internal.SecurityPolicies, metadata map[string]string) ([]byte, error) {
	return json.Marshal([]interface{}{struct {
		Pid              int                         `json:"pid"`
		Language         string                      `json:"language"`
		Version          string                      `json:"agent_version"`
		Host             string                      `json:"host"`
		HostDisplayName  string                      `json:"display_host,omitempty"`
		Settings         interface{}                 `json:"settings"`
		AppName          []string                    `json:"app_name"`
		HighSecurity     bool                        `json:"high_security"`
		Labels           labels                      `json:"labels,omitempty"`
		Environment      environment                 `json:"environment"`
		Identifier       string                      `json:"identifier"`
		Util             *utilization.Data           `json:"utilization"`
		SecurityPolicies *internal.SecurityPolicies  `json:"security_policies,omitempty"`
		Metadata         map[string]string           `json:"metadata"`
		EventData        internal.EventHarvestConfig `json:"event_harvest_config"`
	}{
		Pid:             pid,
		Language:        agentLanguage,
		Version:         version,
		Host:            stringLengthByteLimit(util.Hostname, hostByteLimit),
		HostDisplayName: stringLengthByteLimit(c.HostDisplayName, hostByteLimit),
		Settings:        (settings)(c),
		AppName:         strings.Split(c.AppName, ";"),
		HighSecurity:    c.HighSecurity,
		Labels:          c.Labels,
		Environment:     e,
		// This identifier field is provided to avoid:
		// https://newrelic.atlassian.net/browse/DSCORE-778
		//
		// This identifier is used by the collector to look up the real
		// agent. If an identifier isn't provided, the collector will
		// create its own based on the first appname, which prevents a
		// single daemon from connecting "a;b" and "a;c" at the same
		// time.
		//
		// Providing the identifier below works around this issue and
		// allows users more flexibility in using application rollups.
		Identifier:       c.AppName,
		Util:             util,
		SecurityPolicies: securityPolicies,
		Metadata:         metadata,
		EventData:        internal.DefaultEventHarvestConfigWithDT(c.TransactionEvents.MaxSamplesStored, c.ApplicationLogging.Forwarding.MaxSamplesStored, c.CustomInsightsEvents.MaxSamplesStored, c.DistributedTracer.ReservoirLimit, c.DistributedTracer.Enabled),
	}})
}

const (
	// https://source.datanerd.us/agents/agent-specs/blob/master/Connect-LEGACY.md#metadata-hash
	metadataPrefix = "NEW_RELIC_METADATA_"
)

func gatherMetadata(env []string) map[string]string {
	metadata := make(map[string]string)
	for _, pair := range env {
		if strings.HasPrefix(pair, metadataPrefix) {
			idx := strings.Index(pair, "=")
			if idx >= 0 {
				metadata[pair[0:idx]] = pair[idx+1:]
			}
		}
	}
	return metadata
}

// config exists to avoid adding private fields to Config.
type config struct {
	Config
	// These fields based on environment variables are located here, rather
	// than in appRun, to ensure that they are calculated during
	// NewApplication (instead of at each connect) because some customers
	// may unset environment variables after startup:
	// https://github.com/newrelic/go-agent/issues/127
	metadata         map[string]string
	hostname         string
	traceObserverURL *observerURL
}

func (c Config) computeDynoHostname(getenv func(string) string) string {
	if !c.Heroku.UseDynoNames {
		return ""
	}
	dyno := getenv("DYNO")
	if dyno == "" {
		return ""
	}
	for _, prefix := range c.Heroku.DynoNamePrefixesToShorten {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(dyno, prefix+".") {
			dyno = prefix + ".*"
			break
		}
	}
	return dyno
}

func newInternalConfig(cfg Config, getenv func(string) string, environ []string) (config, error) {
	// Copy maps and slices to prevent race conditions if a consumer changes
	// them after calling NewApplication.
	cfg = copyConfigReferenceFields(cfg)
	if err := cfg.validate(); nil != err {
		return config{}, err
	}
	obsURL, err := cfg.validateTraceObserverConfig()
	if err != nil {
		return config{}, err
	}
	// Ensure that Logger is always set to avoid nil checks.
	if nil == cfg.Logger {
		cfg.Logger = logger.ShimLogger{}
	}
	var hostname string
	if host := cfg.computeDynoHostname(getenv); host != "" {
		hostname = host
	} else if host, err := sysinfo.Hostname(); err == nil {
		hostname = host
	} else {
		hostname = "unknown"
	}
	return config{
		Config:           cfg,
		metadata:         gatherMetadata(environ),
		hostname:         hostname,
		traceObserverURL: obsURL,
	}, nil
}

func (c config) createConnectJSON(securityPolicies *internal.SecurityPolicies) ([]byte, error) {
	env := newEnvironment(&c)
	util := utilization.Gather(utilization.Config{
		DetectAWS:         c.Utilization.DetectAWS,
		DetectAzure:       c.Utilization.DetectAzure,
		DetectPCF:         c.Utilization.DetectPCF,
		DetectGCP:         c.Utilization.DetectGCP,
		DetectDocker:      c.Utilization.DetectDocker,
		DetectKubernetes:  c.Utilization.DetectKubernetes,
		LogicalProcessors: c.Utilization.LogicalProcessors,
		TotalRAMMIB:       c.Utilization.TotalRAMMIB,
		BillingHostname:   c.Utilization.BillingHostname,
		Hostname:          c.hostname,
	}, c.Logger)
	return configConnectJSONInternal(c.Config, os.Getpid(), util, env, Version, securityPolicies, c.metadata)
}

var (
	preconnectHostDefault        = "collector.newrelic.com"
	preconnectRegionLicenseRegex = regexp.MustCompile(`(^.+?)x`)
)

func (c config) preconnectHost() string {
	if c.Host != "" {
		return c.Host
	}
	m := preconnectRegionLicenseRegex.FindStringSubmatch(c.License)
	if len(m) > 1 {
		return "collector." + m[1] + ".nr-data.net"
	}
	return preconnectHostDefault
}
