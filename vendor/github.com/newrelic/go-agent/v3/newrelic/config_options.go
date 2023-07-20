// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ConfigOption configures the Config when provided to NewApplication.
type ConfigOption func(*Config)

// ConfigEnabled sets the whether or not the agent is enabled.
func ConfigEnabled(enabled bool) ConfigOption {
	return func(cfg *Config) { cfg.Enabled = enabled }
}

// ConfigAppName sets the application name.
func ConfigAppName(appName string) ConfigOption {
	return func(cfg *Config) { cfg.AppName = appName }
}

// ConfigLicense sets the license.
func ConfigLicense(license string) ConfigOption {
	return func(cfg *Config) { cfg.License = license }
}

// ConfigDistributedTracerEnabled populates the Config's
// DistributedTracer.Enabled setting.
func ConfigDistributedTracerEnabled(enabled bool) ConfigOption {
	return func(cfg *Config) { cfg.DistributedTracer.Enabled = enabled }
}

// ConfigCustomInsightsEventsMaxSamplesStored alters the sample size allowing control
// of how many custom events are stored in an agent for a given harvest cycle.
// Alters the CustomInsightsEvents.MaxSamplesStored setting.
// Note: As of Jul 2022, the absolute maximum events that can be sent each minute is 100000.
func ConfigCustomInsightsEventsMaxSamplesStored(limit int) ConfigOption {
	if limit > 100000 {
		return func(cfg *Config) { cfg.CustomInsightsEvents.MaxSamplesStored = 100000 }
	}
	return func(cfg *Config) { cfg.CustomInsightsEvents.MaxSamplesStored = limit }
}

// ConfigCustomInsightsEventsEnabled enables or disables the collection of custom insight events.
func ConfigCustomInsightsEventsEnabled(enabled bool) ConfigOption {
	return func(cfg *Config) { cfg.CustomInsightsEvents.Enabled = enabled }
}

// ConfigDistributedTracerReservoirLimit alters the sample reservoir size (maximum
// number of span events to be collected) for distributed tracing instead of
// using the built-in default.
// Alters the DistributedTracer.ReservoirLimit setting.
func ConfigDistributedTracerReservoirLimit(limit int) ConfigOption {
	return func(cfg *Config) { cfg.DistributedTracer.ReservoirLimit = limit }
}

// ConfigCodeLevelMetricsEnabled turns on or off the collection of code
// level metrics entirely.
func ConfigCodeLevelMetricsEnabled(enabled bool) ConfigOption {
	return func(cfg *Config) {
		cfg.CodeLevelMetrics.Enabled = enabled
	}
}

// ConfigDatastoreRawQuery replaces a parameterized query in datastores
// with the full raw query
func ConfigDatastoreRawQuery(enabled bool) ConfigOption {
	return func(cfg *Config) {
		cfg.DatastoreTracer.RawQuery.Enabled = enabled
	}
}

// ConfigCodeLevelMetricsIgnoredPrefix alters the way the Code Level Metrics
// collection code searches for the right function to report for a given
// telemetry trace. It will find the innermost function whose name does NOT
// begin with any of the strings given here. By default (or if no paramters are given),
// it will ignore functions whose names imply that the function is part of
// the agent itself.
//
// In agent version 3.18.0 (only), this took a single string parameter.
// It now takes a variable number of parameters, preserving the old call semantics
// for backward compatibility while allowing for multiple IgnoredPrefix values now.
//
// Deprecated: New code should use ConfigCodeLevelmetricsIgnoredPrefixes instead,
// so the naming of this function is consistent with other related identifiers and
// the fact that multiple such prefixes are now used.
func ConfigCodeLevelMetricsIgnoredPrefix(prefix ...string) ConfigOption {
	return ConfigCodeLevelMetricsIgnoredPrefixes(prefix...)
}

// ConfigCodeLevelMetricsIgnoredPrefixes alters the way the Code Level Metrics
// collection code searches for the right function to report for a given
// telemetry trace. It will find the innermost function whose name does NOT
// begin with any of the strings given here. By default (or if no paramters are given),
// it will ignore functions whose names imply that the function is part of
// the agent itself.
func ConfigCodeLevelMetricsIgnoredPrefixes(prefix ...string) ConfigOption {
	return func(cfg *Config) {
		cfg.CodeLevelMetrics.IgnoredPrefixes = prefix

		// Correct things if the user populated the old IgnoredPrefix value in the struct
		if cfg.CodeLevelMetrics.IgnoredPrefix != "" {
			cfg.CodeLevelMetrics.IgnoredPrefixes = append(cfg.CodeLevelMetrics.IgnoredPrefixes, cfg.CodeLevelMetrics.IgnoredPrefix)
			cfg.CodeLevelMetrics.IgnoredPrefix = ""
		}
	}
}

// ConfigCodeLevelMetricsRedactIgnoredPrefixes controls whether the names
// of ignored modules should be redacted from the agent configuration data
// reported and visible in the New Relic UI. Since one of the reasons these
// modules may be excluded is to preserve confidentiality of module or
// directory names, the default behavior (if this option is set to true)
// is to redact those names from the configuration data so that the only thing
// reported is that some list of unnamed modules were excluded from reporting.
// If this is set to false, then the names of the ignored modules will be
// listed in the configuration data, although those modules will still be ignored
// by Code Level Metrics.
func ConfigCodeLevelMetricsRedactIgnoredPrefixes(enabled bool) ConfigOption {
	return func(cfg *Config) {
		cfg.CodeLevelMetrics.RedactIgnoredPrefixes = enabled
	}
}

// ConfigCodeLevelMetricsRedactPathPrefixes controls whether the names
// of source code parent directories should be redacted from the agent configuration data
// reported and visible in the New Relic UI. Since one of the reasons these
// path prefixes may be excluded is to preserve confidentiality of
// directory names, the default behavior (if this option is set to true)
// is to redact those names from the configuration data so that the only thing
// reported is that some list of unnamed path prefixes were removed from reported pathnames.
// If this is set to false, then the names of the removed path prefixes will be
// listed in the configuration data, although those strings will still be removed from pathnames
// reported by Code Level Metrics.
func ConfigCodeLevelMetricsRedactPathPrefixes(enabled bool) ConfigOption {
	return func(cfg *Config) {
		cfg.CodeLevelMetrics.RedactPathPrefixes = enabled
	}
}

// ConfigCodeLevelMetricsScope narrows the scope of where code level
// metrics are to be used. By default, if CodeLevelMetrics are enabled,
// they apply everywhere the agent currently supports them. To narrow
// this, supply a list of one or more CodeLevelMetricsScope values
// ORed together to ConfigCodeLevelMetricsScope.
//
// Note that a zero value CodeLevelMetricsScope means to collect all supported
// telemetry data types. If you want to stop collecting any code level metrics,
// then disable collection via ConfigCodeLevelMetricsEnabled.
func ConfigCodeLevelMetricsScope(scope CodeLevelMetricsScope) ConfigOption {
	return func(cfg *Config) {
		cfg.CodeLevelMetrics.Scope = scope
	}
}

// ConfigCodeLevelMetricsPathPrefix specifies the filename pattern(s) that describe(s) the start of
// the project area(s). When reporting a source filename for Code Level Metrics, and any of the
// values in the path prefix list are found in the source filename, anything before that prefix
// is discarded from the file pathname. This will be based on the first value in the prefix list
// that is found in the pathname.
//
// For example, if
// the path prefix list is set to ["myproject/src", "myproject/extra"], then a function located in a file
// called "/usr/local/src/myproject/src/foo.go" will be reported with the
// pathname "myproject/src/foo.go". If this value is empty or none of the prefix strings
// are found in a file's pathname, the full path
// will be reported (e.g., "/usr/local/src/myproject/src/foo.go").
//
// In agent versions 3.18.0 and 3.18.1, this took a single string parameter.
// It now takes a variable number of parameters, preserving the old call semantics
// for backward compatibility while allowing for multiple PathPrefix values now.
//
// Deprecated: New code should use ConfigCodeLevelMetricsPathPrefixes instead,
// so the naming of this function is consistent with other related identifiers
// and the fact that multiple such prefixes are now used.
func ConfigCodeLevelMetricsPathPrefix(prefix ...string) ConfigOption {
	return ConfigCodeLevelMetricsPathPrefixes(prefix...)
}

// ConfigCodeLevelMetricsPathPrefixes specifies the filename pattern(s) that describe(s) the start of
// the project area(s). When reporting a source filename for Code Level Metrics, and any of the
// values in the path prefix list are found in the source filename, anything before that prefix
// is discarded from the file pathname. This will be based on the first value in the prefix list
// that is found in the pathname.
//
// For example, if
// the path prefix list is set to ["myproject/src", "myproject/extra"], then a function located in a file
// called "/usr/local/src/myproject/src/foo.go" will be reported with the
// pathname "myproject/src/foo.go". If this value is empty or none of the prefix strings
// are found in a file's pathname, the full path
// will be reported (e.g., "/usr/local/src/myproject/src/foo.go").
func ConfigCodeLevelMetricsPathPrefixes(prefix ...string) ConfigOption {
	return func(cfg *Config) {
		cfg.CodeLevelMetrics.PathPrefixes = prefix

		// Correct things if the user populated the old PathPrefix value in the struct
		if cfg.CodeLevelMetrics.PathPrefix != "" {
			cfg.CodeLevelMetrics.PathPrefixes = append(cfg.CodeLevelMetrics.PathPrefixes, cfg.CodeLevelMetrics.PathPrefix)
			cfg.CodeLevelMetrics.PathPrefix = ""
		}
	}
}

// ConfigAppLogForwardingEnabled enables or disables the collection
// of logs from a user's application by the agent
// Defaults: enabled=false
func ConfigAppLogForwardingEnabled(enabled bool) ConfigOption {
	return func(cfg *Config) {
		if enabled {
			cfg.ApplicationLogging.Enabled = true
			cfg.ApplicationLogging.Forwarding.Enabled = true
		} else {
			cfg.ApplicationLogging.Forwarding.Enabled = false
			cfg.ApplicationLogging.Forwarding.MaxSamplesStored = 0
		}
	}
}

// ConfigAppLogDecoratingEnabled enables or disables the local decoration
// of logs when using one of our logs in context plugins
// Defaults: enabled=false
func ConfigAppLogDecoratingEnabled(enabled bool) ConfigOption {
	return func(cfg *Config) {
		if enabled {
			cfg.ApplicationLogging.Enabled = true
			cfg.ApplicationLogging.LocalDecorating.Enabled = true
		} else {
			cfg.ApplicationLogging.LocalDecorating.Enabled = false
		}
	}
}

// ConfigAppLogMetricsEnabled enables or disables the collection of metrics
// data for logs seen by an instrumented logging framework
// default: true
func ConfigAppLogMetricsEnabled(enabled bool) ConfigOption {
	return func(cfg *Config) {
		if enabled {
			cfg.ApplicationLogging.Enabled = true
			cfg.ApplicationLogging.Metrics.Enabled = true
		} else {
			cfg.ApplicationLogging.Metrics.Enabled = false
		}
	}
}

// ConfigAppLogEnabled enables or disables all application logging features
// and data collection
func ConfigAppLogEnabled(enabled bool) ConfigOption {
	return func(cfg *Config) {
		if enabled {
			cfg.ApplicationLogging.Enabled = true
		} else {
			cfg.ApplicationLogging.Enabled = false
		}
	}
}

// ConfigAppLogForwardingMaxSamplesStored allows users to set the maximium number of
// log events the agent is allowed to collect and store in a given harvest cycle.
func ConfigAppLogForwardingMaxSamplesStored(maxSamplesStored int) ConfigOption {
	return func(cfg *Config) {
		cfg.ApplicationLogging.Forwarding.MaxSamplesStored = maxSamplesStored
	}
}

// ConfigLogger populates the Config's Logger.
func ConfigLogger(l Logger) ConfigOption {
	return func(cfg *Config) { cfg.Logger = l }
}

// ConfigInfoLogger populates the config with basic Logger at info level.
func ConfigInfoLogger(w io.Writer) ConfigOption {
	return ConfigLogger(NewLogger(w))
}

// ConfigModuleDependencyMetricsEnabled controls whether the agent collects and reports
// the list of modules compiled into the instrumented application.
func ConfigModuleDependencyMetricsEnabled(enabled bool) ConfigOption {
	return func(cfg *Config) {
		cfg.ModuleDependencyMetrics.Enabled = enabled
	}
}

// ConfigModuleDependencyMetricsIgnoredPrefixes sets the list of module path prefix strings
// indicating which modules should be excluded from the dependency report.
func ConfigModuleDependencyMetricsIgnoredPrefixes(prefix ...string) ConfigOption {
	return func(cfg *Config) {
		cfg.ModuleDependencyMetrics.IgnoredPrefixes = prefix
	}
}

// ConfigSetErrorGroupCallbackFunction set a callback function of type ErrorGroupCallback that will
// be invoked against errors at harvest time. This function overrides the default grouping behavior
// of errors into a custom, user defined group when set. Setting this may have performance implications
// for your application depending on the contents of the callback function. Do not set this if you want
// the default error grouping behavior to be executed.
func ConfigSetErrorGroupCallbackFunction(callback ErrorGroupCallback) ConfigOption {
	return func(cfg *Config) {
		cfg.ErrorCollector.ErrorGroupCallback = callback
	}
}

// ConfigModuleDependencyMetricsRedactIgnoredPrefixes controls whether the names
// of ignored module path prefixes should be redacted from the agent configuration data
// reported and visible in the New Relic UI. Since one of the reasons these
// modules may be excluded is to preserve confidentiality of module or
// directory names, the default behavior (if this option is set to true)
// is to redact those names from the configuration data so that the only thing
// reported is that some list of unnamed modules were excluded from reporting.
// If this is set to false, then the names of the ignored modules will be
// listed in the configuration data, although those modules will still be ignored
// by Module Dependency Metrics.
func ConfigModuleDependencyMetricsRedactIgnoredPrefixes(enabled bool) ConfigOption {
	return func(cfg *Config) {
		cfg.ModuleDependencyMetrics.RedactIgnoredPrefixes = enabled
	}
}

// ConfigDebugLogger populates the config with a Logger at debug level.
func ConfigDebugLogger(w io.Writer) ConfigOption {
	return ConfigLogger(NewDebugLogger(w))
}

// ConfigFromEnvironment populates the config based on environment variables:
//
//		NEW_RELIC_APP_NAME                                			sets AppName
//		NEW_RELIC_ATTRIBUTES_EXCLUDE                      			sets Attributes.Exclude using a comma-separated list, eg. "request.headers.host,request.method"
//		NEW_RELIC_ATTRIBUTES_INCLUDE                      			sets Attributes.Include using a comma-separated list
//		NEW_RELIC_MODULE_DEPENDENCY_METRICS_ENABLED          		sets ModuleDependencyMetrics.Enabled
//		NEW_RELIC_MODULE_DEPENDENCY_METRICS_IGNORED_PREFIXES 		sets ModuleDependencyMetrics.IgnoredPrefixes
//		NEW_RELIC_MODULE_DEPENDENCY_METRICS_REDACT_IGNORED_PREFIXES sets ModuleDependencyMetrics.RedactIgnoredPrefixes to a boolean value
//		NEW_RELIC_CODE_LEVEL_METRICS_ENABLED              			sets CodeLevelMetrics.Enabled
//		NEW_RELIC_CODE_LEVEL_METRICS_SCOPE                			sets CodeLevelMetrics.Scope using a comma-separated list, e.g. "transaction"
//		NEW_RELIC_CODE_LEVEL_METRICS_PATH_PREFIX          			sets CodeLevelMetrics.PathPrefixes using a comma-separated list
//		NEW_RELIC_CODE_LEVEL_METRICS_REDACT_PATH_PREFIXES    		sets CodeLevelMetrics.RedactPathPrefixes to a boolean value
//	 	NEW_RELIC_CODE_LEVEL_METRICS_REDACT_IGNORED_PREFIXES 		sets CodeLevelMetrics.RedactIgnoredPrefixes to a boolean value
//		NEW_RELIC_CODE_LEVEL_METRICS_IGNORED_PREFIX       			sets CodeLevelMetrics.IgnoredPrefixes using a comma-separated list
//		NEW_RELIC_DISTRIBUTED_TRACING_ENABLED             			sets DistributedTracer.Enabled using strconv.ParseBool
//		NEW_RELIC_ENABLED                                 			sets Enabled using strconv.ParseBool
//		NEW_RELIC_HIGH_SECURITY                           			sets HighSecurity using strconv.ParseBool
//		NEW_RELIC_HOST                                    			sets Host
//		NEW_RELIC_INFINITE_TRACING_SPAN_EVENTS_QUEUE_SIZE 			sets InfiniteTracing.SpanEvents.QueueSize using strconv.Atoi
//		NEW_RELIC_INFINITE_TRACING_TRACE_OBSERVER_PORT    			sets InfiniteTracing.TraceObserver.Port using strconv.Atoi
//		NEW_RELIC_INFINITE_TRACING_TRACE_OBSERVER_HOST    			sets InfiniteTracing.TraceObserver.Host
//		NEW_RELIC_LABELS                                  			sets Labels using a semi-colon delimited string of colon-separated pairs, eg. "Server:One;DataCenter:Primary"
//		NEW_RELIC_LICENSE_KEY                             			sets License
//		NEW_RELIC_LOG                                     			sets Logger to log to either "stdout" or "stderr" (filenames are not supported)
//		NEW_RELIC_LOG_LEVEL                               			controls the NEW_RELIC_LOG level, must be "debug" for debug, or empty for info
//		NEW_RELIC_PROCESS_HOST_DISPLAY_NAME               			sets HostDisplayName
//		NEW_RELIC_SECURITY_POLICIES_TOKEN                 			sets SecurityPoliciesToken
//		NEW_RELIC_UTILIZATION_BILLING_HOSTNAME            			sets Utilization.BillingHostname
//		NEW_RELIC_UTILIZATION_LOGICAL_PROCESSORS          			sets Utilization.LogicalProcessors using strconv.Atoi
//		NEW_RELIC_UTILIZATION_TOTAL_RAM_MIB               			sets Utilization.TotalRAMMIB using strconv.Atoi
//		NEW_RELIC_APPLICATION_LOGGING_ENABLED						sets ApplicationLogging.Enabled. Set to false to disable all application logging features.
//	 	NEW_RELIC_APPLICATION_LOGGING_FORWARDING_ENABLED  			sets ApplicationLogging.LogForwarding.Enabled. Set to false to disable in agent log forwarding.
//	 	NEW_RELIC_APPLICATION_LOGGING_METRICS_ENABLED		  		sets ApplicationLogging.Metrics.Enabled. Set to false to disable the collection of application log metrics.
//	 	NEW_RELIC_APPLICATION_LOGGING_LOCAL_DECORATING_ENABLED      sets ApplicationLogging.LocalDecoration.Enabled. Set to true to enable local log decoration.
//		NEW_RELIC_APPLICATION_LOGGING_FORWARDING_MAX_SAMPLES_STORED	sets ApplicationLogging.LogForwarding.Limit. Set to 0 to prevent captured logs from being forwarded.
//
// This function is strict and will assign Config.Error if any of the
// environment variables cannot be parsed.
func ConfigFromEnvironment() ConfigOption {
	return configFromEnvironment(os.Getenv)
}

func configFromEnvironment(getenv func(string) string) ConfigOption {
	return func(cfg *Config) {
		// Because fields could have been assigned in a previous
		// ConfigOption, we only want to assign fields using environment
		// variables that have been populated.  This is especially
		// relevant for the string case where no processing occurs.
		assignBool := func(field *bool, name string) {
			if env := getenv(name); env != "" {
				if b, err := strconv.ParseBool(env); nil != err {
					cfg.Error = fmt.Errorf("invalid %s value: %s", name, env)
				} else {
					*field = b
				}
			}
		}
		assignInt := func(field *int, name string) {
			if env := getenv(name); env != "" {
				if i, err := strconv.Atoi(env); nil != err {
					cfg.Error = fmt.Errorf("invalid %s value: %s", name, env)
				} else {
					*field = i
				}
			}
		}
		assignString := func(field *string, name string) {
			if env := getenv(name); env != "" {
				*field = env
			}
		}

		assignString(&cfg.AppName, "NEW_RELIC_APP_NAME")
		assignString(&cfg.License, "NEW_RELIC_LICENSE_KEY")
		assignBool(&cfg.ModuleDependencyMetrics.Enabled, "NEW_RELIC_MODULE_DEPENDENCY_METRICS_ENABLED")
		assignBool(&cfg.ModuleDependencyMetrics.RedactIgnoredPrefixes, "NEW_RELIC_MODULE_DEPENDENCY_METRICS_REDACT_IGNORED_PREFIXES")
		assignBool(&cfg.CodeLevelMetrics.Enabled, "NEW_RELIC_CODE_LEVEL_METRICS_ENABLED")
		assignBool(&cfg.CodeLevelMetrics.RedactPathPrefixes, "NEW_RELIC_CODE_LEVEL_METRICS_REDACT_PATH_PREFIXES")
		assignBool(&cfg.CodeLevelMetrics.RedactIgnoredPrefixes, "NEW_RELIC_CODE_LEVEL_METRICS_REDACT_IGNORED_PREFIXES")
		assignBool(&cfg.DistributedTracer.Enabled, "NEW_RELIC_DISTRIBUTED_TRACING_ENABLED")
		assignBool(&cfg.Enabled, "NEW_RELIC_ENABLED")
		assignBool(&cfg.HighSecurity, "NEW_RELIC_HIGH_SECURITY")
		assignString(&cfg.SecurityPoliciesToken, "NEW_RELIC_SECURITY_POLICIES_TOKEN")
		assignString(&cfg.Host, "NEW_RELIC_HOST")
		assignString(&cfg.HostDisplayName, "NEW_RELIC_PROCESS_HOST_DISPLAY_NAME")
		assignString(&cfg.Utilization.BillingHostname, "NEW_RELIC_UTILIZATION_BILLING_HOSTNAME")
		assignString(&cfg.InfiniteTracing.TraceObserver.Host, "NEW_RELIC_INFINITE_TRACING_TRACE_OBSERVER_HOST")
		assignInt(&cfg.InfiniteTracing.TraceObserver.Port, "NEW_RELIC_INFINITE_TRACING_TRACE_OBSERVER_PORT")
		assignInt(&cfg.Utilization.LogicalProcessors, "NEW_RELIC_UTILIZATION_LOGICAL_PROCESSORS")
		assignInt(&cfg.Utilization.TotalRAMMIB, "NEW_RELIC_UTILIZATION_TOTAL_RAM_MIB")
		assignInt(&cfg.InfiniteTracing.SpanEvents.QueueSize, "NEW_RELIC_INFINITE_TRACING_SPAN_EVENTS_QUEUE_SIZE")

		// Application Logging Env Variables
		assignBool(&cfg.ApplicationLogging.Enabled, "NEW_RELIC_APPLICATION_LOGGING_ENABLED")
		assignBool(&cfg.ApplicationLogging.Forwarding.Enabled, "NEW_RELIC_APPLICATION_LOGGING_FORWARDING_ENABLED")
		assignInt(&cfg.ApplicationLogging.Forwarding.MaxSamplesStored, "NEW_RELIC_APPLICATION_LOGGING_FORWARDING_MAX_SAMPLES_STORED")
		assignBool(&cfg.ApplicationLogging.Metrics.Enabled, "NEW_RELIC_APPLICATION_LOGGING_METRICS_ENABLED")
		assignBool(&cfg.ApplicationLogging.LocalDecorating.Enabled, "NEW_RELIC_APPLICATION_LOGGING_LOCAL_DECORATING_ENABLED")

		if env := getenv("NEW_RELIC_LABELS"); env != "" {
			if labels := getLabels(getenv("NEW_RELIC_LABELS")); len(labels) > 0 {
				cfg.Labels = labels
			} else {
				cfg.Error = fmt.Errorf("invalid NEW_RELIC_LABELS value: %s", env)
			}
		}

		if env := getenv("NEW_RELIC_ATTRIBUTES_INCLUDE"); env != "" {
			cfg.Attributes.Include = strings.Split(env, ",")
		}
		if env := getenv("NEW_RELIC_ATTRIBUTES_EXCLUDE"); env != "" {
			cfg.Attributes.Exclude = strings.Split(env, ",")
		}

		if env := getenv("NEW_RELIC_CODE_LEVEL_METRICS_SCOPE"); env != "" {
			var ok bool
			cfg.CodeLevelMetrics.Scope, ok = CodeLevelMetricsScopeLabelListToValue(env)
			if !ok {
				cfg.Error = fmt.Errorf("invalid NEW_RELIC_CODE_LEVEL_METRICS_SCOPE value")
			}
		}

		if env := getenv("NEW_RELIC_CODE_LEVEL_METRICS_IGNORED_PREFIXES"); env != "" {
			cfg.CodeLevelMetrics.IgnoredPrefixes = strings.Split(env, ",")
		} else if env := getenv("NEW_RELIC_CODE_LEVEL_METRICS_IGNORED_PREFIX"); env != "" {
			cfg.CodeLevelMetrics.IgnoredPrefixes = strings.Split(env, ",")
		}

		if env := getenv("NEW_RELIC_CODE_LEVEL_METRICS_PATH_PREFIXES"); env != "" {
			cfg.CodeLevelMetrics.PathPrefixes = strings.Split(env, ",")
		} else if env := getenv("NEW_RELIC_CODE_LEVEL_METRICS_PATH_PREFIX"); env != "" {
			cfg.CodeLevelMetrics.PathPrefixes = strings.Split(env, ",")
		}

		if env := getenv("NEW_RELIC_MODULE_DEPENDENCY_METRICS_IGNORED_PREFIXES"); env != "" {
			cfg.ModuleDependencyMetrics.IgnoredPrefixes = strings.Split(env, ",")
		}

		if env := getenv("NEW_RELIC_LOG"); env != "" {
			if dest := getLogDest(env); dest != nil {
				if isDebugEnv(getenv("NEW_RELIC_LOG_LEVEL")) {
					cfg.Logger = NewDebugLogger(dest)
				} else {
					cfg.Logger = NewLogger(dest)
				}
			} else {
				cfg.Error = fmt.Errorf("invalid NEW_RELIC_LOG value %s", env)
			}
		}
	}
}

func getLogDest(env string) io.Writer {
	switch env {
	case "stdout", "Stdout", "STDOUT":
		return os.Stdout
	case "stderr", "Stderr", "STDERR":
		return os.Stderr
	default:
		return nil
	}
}

func isDebugEnv(env string) bool {
	switch env {
	case "debug", "Debug", "DEBUG", "d", "D":
		return true
	default:
		return false
	}
}

// getLabels reads Labels from the env string, expressed as a semi-colon
// delimited string of colon-separated pairs (for example, "Server:One;Data
// Center:Primary").  Label keys and values must be 255 characters or less in
// length.  No more than 64 Labels can be set.
func getLabels(env string) map[string]string {
	out := make(map[string]string)
	env = strings.Trim(env, ";\t\n\v\f\r ")
	for _, entry := range strings.Split(env, ";") {
		if entry == "" {
			return nil
		}
		split := strings.Split(entry, ":")
		if len(split) != 2 {
			return nil
		}
		left := strings.TrimSpace(split[0])
		right := strings.TrimSpace(split[1])
		if left == "" || right == "" {
			return nil
		}
		if utf8.RuneCountInString(left) > 255 {
			runes := []rune(left)
			left = string(runes[:255])
		}
		if utf8.RuneCountInString(right) > 255 {
			runes := []rune(right)
			right = string(runes[:255])
		}
		out[left] = right
		if len(out) >= 64 {
			return out
		}
	}
	return out
}
