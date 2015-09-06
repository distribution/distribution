package context

import (
	"fmt"

	"github.com/Sirupsen/logrus"
)

// Logger provides a leveled-logging interface.
type Logger interface {
	// standard logger methods
	Print(args ...interface{})
	Printf(format string, args ...interface{})
	Println(args ...interface{})

	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Fatalln(args ...interface{})

	Panic(args ...interface{})
	Panicf(format string, args ...interface{})
	Panicln(args ...interface{})

	// Leveled methods, from logrus
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Debugln(args ...interface{})

	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Errorln(args ...interface{})

	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Infoln(args ...interface{})

	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Warnln(args ...interface{})
}

// WithLogger creates a new context with provided logger.
func WithLogger(ctx Context, logger Logger) Context {
	return WithValue(ctx, "logger", logger)
}

// GetLoggerWithField returns a logger instance with the specified field key
// and value without affecting the context. Extra specified keys will be
// resolved from the context.
func GetLoggerWithField(ctx Context, key, value interface{}, keys ...interface{}) Logger {
	return &entry{getLogrusLogger(ctx, keys...).WithField(fmt.Sprint(key), value)}
}

// GetLoggerWithFields returns a logger instance with the specified fields
// without affecting the context. Extra specified keys will be resolved from
// the context.
func GetLoggerWithFields(ctx Context, fields map[string]interface{}, keys ...interface{}) Logger {
	return &entry{getLogrusLogger(ctx, keys...).WithFields(logrus.Fields(fields))}
}

// GetLogger returns the logger from the current context, if present. If one
// or more keys are provided, they will be resolved on the context and
// included in the logger. While context.Value takes an interface, any key
// argument passed to GetLogger will be passed to fmt.Sprint when expanded as
// a logging key field. If context keys are integer constants, for example,
// its recommended that a String method is implemented.
func GetLogger(ctx Context, keys ...interface{}) Logger {
	return &entry{getLogrusLogger(ctx, keys...)}
}

// GetLogrusLogger returns the logrus logger for the context. If one more keys
// are provided, they will be resolved on the context and included in the
// logger. Only use this function if specific logrus functionality is
// required.
func getLogrusLogger(ctx Context, keys ...interface{}) *logrus.Entry {
	var logger *logrus.Entry

	// Get a logger, if it is present.
	loggerInterface := ctx.Value("logger")
	if loggerInterface != nil {
		if lgr, ok := loggerInterface.(*logrus.Entry); ok {
			logger = lgr
		}
	}

	if logger == nil {
		// If no logger is found, just return the standard logger.
		logger = logrus.NewEntry(logrus.StandardLogger())
	}

	fields := logrus.Fields{}

	for _, key := range keys {
		v := ctx.Value(key)
		if v != nil {
			fields[fmt.Sprint(key)] = v
		}
	}

	return logger.WithFields(fields)
}

var _ Logger = new(entry)

type entry struct {
	*logrus.Entry
}

func (e *entry) Print(args ...interface{})                 { e.Entry.Print(args...) }
func (e *entry) Printf(format string, args ...interface{}) { e.Entry.Printf(format, args...) }
func (e *entry) Println(args ...interface{})               { e.Entry.Println(args...) }
func (e *entry) Fatal(args ...interface{})                 { e.Entry.Fatal(args...) }
func (e *entry) Fatalf(format string, args ...interface{}) { e.Entry.Fatalf(format, args...) }
func (e *entry) Fatalln(args ...interface{})               { e.Entry.Fatalln(args...) }
func (e *entry) Panic(args ...interface{})                 { e.Entry.Panic(args...) }
func (e *entry) Panicf(format string, args ...interface{}) { e.Entry.Panicf(format, args...) }
func (e *entry) Panicln(args ...interface{})               { e.Entry.Panicln(args...) }
func (e *entry) Debug(args ...interface{})                 { e.Entry.Debug(args...) }
func (e *entry) Debugf(format string, args ...interface{}) { e.Entry.Debugf(format, args...) }
func (e *entry) Debugln(args ...interface{})               { e.Entry.Debugln(args...) }
func (e *entry) Error(args ...interface{})                 { e.Entry.Error(args...) }
func (e *entry) Errorf(format string, args ...interface{}) { e.Entry.Errorf(format, args...) }
func (e *entry) Errorln(args ...interface{})               { e.Entry.Errorln(args...) }
func (e *entry) Info(args ...interface{})                  { e.Entry.Info(args...) }
func (e *entry) Infof(format string, args ...interface{})  { e.Entry.Infof(format, args...) }
func (e *entry) Infoln(args ...interface{})                { e.Entry.Infoln(args...) }
func (e *entry) Warn(args ...interface{})                  { e.Entry.Warn(args...) }
func (e *entry) Warnf(format string, args ...interface{})  { e.Entry.Warnf(format, args...) }
func (e *entry) Warnln(args ...interface{})                { e.Entry.Warnln(args...) }
