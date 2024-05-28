package tracing

import "github.com/distribution/distribution/v3/internal/dcontext"

// loggerWriter is a custom writer that implements the io.Writer interface.
// It is designed to redirect log messages to the Logger interface, specifically
// for use with OpenTelemetry's stdouttrace exporter.
type loggerWriter struct {
	logger dcontext.Logger // Use the Logger interface
}

// Write logs the data using the Debug level of the provided logger.
func (lw *loggerWriter) Write(p []byte) (n int, err error) {
	lw.logger.Debug(string(p))
	return len(p), nil
}

// Handle logs the error using the Error level of the provided logger.
func (lw *loggerWriter) Handle(err error) {
	lw.logger.Error(err)
}
