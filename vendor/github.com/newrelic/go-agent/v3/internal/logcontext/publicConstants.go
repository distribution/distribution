package logcontext

// Exported Constants for log decorators
const (
	// LogSeverityFieldName is the name of the log level field in New Relic logging JSON
	LogSeverityFieldName = "level"

	// LogMessageFieldName is the name of the log message field in New Relic logging JSON
	LogMessageFieldName = "message"

	// LogTimestampFieldName is the name of the timestamp field in New Relic logging JSON
	LogTimestampFieldName = "timestamp"

	// LogSpanIDFieldName is the name of the span ID field in the New Relic logging JSON
	LogSpanIDFieldName = "span.id"

	// LogTraceIDFieldName is the name of the trace ID field in the New Relic logging JSON
	LogTraceIDFieldName = "trace.id"

	// LogSeverityUnknown is the value the log severity should be set to if no log severity is known
	LogSeverityUnknown = "UNKNOWN"

	// number of bytes expected to be needed for the average log message
	AverageLogSizeEstimate = 400
)
