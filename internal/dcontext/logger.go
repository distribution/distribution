package dcontext

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Additional log levels mirroring the levels logrus provided beyond the
// standard log/slog levels.
const (
	// LevelTrace is more verbose than slog.LevelDebug.
	LevelTrace = slog.Level(-8)
	// LevelFatal is used for messages logged right before os.Exit(1).
	LevelFatal = slog.Level(12)
	// LevelPanic is used for messages logged right before panicking.
	LevelPanic = slog.Level(16)
)

// ParseLevel converts a level name as used in the configuration ("trace",
// "debug", "info", "warn", "warning", "error", "fatal", "panic") into a
// slog.Level.
func ParseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	case "fatal":
		return LevelFatal, nil
	case "panic":
		return LevelPanic, nil
	}
	return slog.LevelInfo, fmt.Errorf("not a valid log level: %q", level)
}

// LevelName returns the lower-case name of the level, resolving the custom
// trace, fatal and panic levels defined by this package.
func LevelName(level slog.Level) string {
	switch {
	case level <= LevelTrace:
		return "trace"
	case level >= LevelPanic:
		return "panic"
	case level >= LevelFatal:
		return "fatal"
	default:
		return strings.ToLower(level.String())
	}
}

// Logger provides a leveled-logging interface compatible with the one
// formerly backed by logrus, implemented on top of log/slog. The embedded
// *slog.Logger is available for structured logging.
type Logger struct {
	*slog.Logger
}

// NewLogger wraps an *slog.Logger in a Logger.
func NewLogger(l *slog.Logger) Logger {
	return Logger{Logger: l}
}

// WithError adds the error as a field on the returned logger.
func (l Logger) WithError(err error) Logger {
	return Logger{Logger: l.Logger.With("error", err)}
}

// WithField adds the field to the returned logger.
func (l Logger) WithField(key string, value any) Logger {
	return l.with(key, value)
}

// log emits a record at the given level, attributing the source location to
// the caller of the exported Logger method (three frames up the stack).
func (l Logger) log(level slog.Level, msg string) {
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip Callers, log, and the exported method
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	_ = l.Handler().Handle(context.Background(), r)
}

func (l Logger) enabled(level slog.Level) bool {
	return l.Handler().Enabled(context.Background(), level)
}

func (l Logger) Trace(args ...any) {
	if l.enabled(LevelTrace) {
		l.log(LevelTrace, fmt.Sprint(args...))
	}
}

func (l Logger) Tracef(format string, args ...any) {
	if l.enabled(LevelTrace) {
		l.log(LevelTrace, fmt.Sprintf(format, args...))
	}
}

func (l Logger) Debug(args ...any) {
	if l.enabled(slog.LevelDebug) {
		l.log(slog.LevelDebug, fmt.Sprint(args...))
	}
}

func (l Logger) Debugf(format string, args ...any) {
	if l.enabled(slog.LevelDebug) {
		l.log(slog.LevelDebug, fmt.Sprintf(format, args...))
	}
}

func (l Logger) Debugln(args ...any) {
	if l.enabled(slog.LevelDebug) {
		l.log(slog.LevelDebug, sprintlnn(args...))
	}
}

func (l Logger) Info(args ...any) {
	if l.enabled(slog.LevelInfo) {
		l.log(slog.LevelInfo, fmt.Sprint(args...))
	}
}

func (l Logger) Infof(format string, args ...any) {
	if l.enabled(slog.LevelInfo) {
		l.log(slog.LevelInfo, fmt.Sprintf(format, args...))
	}
}

func (l Logger) Infoln(args ...any) {
	if l.enabled(slog.LevelInfo) {
		l.log(slog.LevelInfo, sprintlnn(args...))
	}
}

func (l Logger) Warn(args ...any) {
	if l.enabled(slog.LevelWarn) {
		l.log(slog.LevelWarn, fmt.Sprint(args...))
	}
}

func (l Logger) Warnf(format string, args ...any) {
	if l.enabled(slog.LevelWarn) {
		l.log(slog.LevelWarn, fmt.Sprintf(format, args...))
	}
}

func (l Logger) Warnln(args ...any) {
	if l.enabled(slog.LevelWarn) {
		l.log(slog.LevelWarn, sprintlnn(args...))
	}
}

func (l Logger) Error(args ...any) {
	if l.enabled(slog.LevelError) {
		l.log(slog.LevelError, fmt.Sprint(args...))
	}
}

func (l Logger) Errorf(format string, args ...any) {
	if l.enabled(slog.LevelError) {
		l.log(slog.LevelError, fmt.Sprintf(format, args...))
	}
}

func (l Logger) Errorln(args ...any) {
	if l.enabled(slog.LevelError) {
		l.log(slog.LevelError, sprintlnn(args...))
	}
}

func (l Logger) Print(args ...any) {
	if l.enabled(slog.LevelInfo) {
		l.log(slog.LevelInfo, fmt.Sprint(args...))
	}
}

func (l Logger) Printf(format string, args ...any) {
	if l.enabled(slog.LevelInfo) {
		l.log(slog.LevelInfo, fmt.Sprintf(format, args...))
	}
}

func (l Logger) Println(args ...any) {
	if l.enabled(slog.LevelInfo) {
		l.log(slog.LevelInfo, sprintlnn(args...))
	}
}

func (l Logger) Fatal(args ...any) {
	l.log(LevelFatal, fmt.Sprint(args...))
	os.Exit(1)
}

func (l Logger) Fatalf(format string, args ...any) {
	l.log(LevelFatal, fmt.Sprintf(format, args...))
	os.Exit(1)
}

func (l Logger) Fatalln(args ...any) {
	l.log(LevelFatal, sprintlnn(args...))
	os.Exit(1)
}

func (l Logger) Panic(args ...any) {
	msg := fmt.Sprint(args...)
	l.log(LevelPanic, msg)
	panic(msg)
}

func (l Logger) Panicf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.log(LevelPanic, msg)
	panic(msg)
}

func (l Logger) Panicln(args ...any) {
	msg := sprintlnn(args...)
	l.log(LevelPanic, msg)
	panic(msg)
}

// sprintlnn formats the args like fmt.Sprintln but without the trailing
// newline, matching the behavior logrus had for the *ln methods.
func sprintlnn(args ...any) string {
	msg := fmt.Sprintln(args...)
	return msg[:len(msg)-1]
}

var (
	defaultLogger   Logger
	defaultLoggerMu sync.RWMutex
)

type loggerKey struct{}

// WithLogger creates a new context with provided logger.
func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// GetLoggerWithField returns a logger instance with the specified field key
// and value without affecting the context. Extra specified keys will be
// resolved from the context.
func GetLoggerWithField(ctx context.Context, key, value any, keys ...any) Logger {
	return getSlogLogger(ctx, keys...).with(fmt.Sprint(key), value)
}

// GetLoggerWithFields returns a logger instance with the specified fields
// without affecting the context. Extra specified keys will be resolved from
// the context.
func GetLoggerWithFields(ctx context.Context, fields map[any]any, keys ...any) Logger {
	logger := getSlogLogger(ctx, keys...)
	for key, value := range fields {
		logger = logger.with(fmt.Sprint(key), value)
	}
	return logger
}

// GetLogger returns the logger from the current context, if present. If one
// or more keys are provided, they will be resolved on the context and
// included in the logger. While context.Value takes an interface, any key
// argument passed to GetLogger will be passed to fmt.Sprint when expanded as
// a logging key field. If context keys are integer constants, for example,
// its recommended that a String method is implemented.
func GetLogger(ctx context.Context, keys ...any) Logger {
	return getSlogLogger(ctx, keys...)
}

// SetDefaultLogger sets the default logger upon which to base new loggers.
func SetDefaultLogger(logger Logger) {
	defaultLoggerMu.Lock()
	defaultLogger = logger
	defaultLoggerMu.Unlock()
}

func (l Logger) with(key string, value any) Logger {
	return Logger{Logger: l.Logger.With(key, value)}
}

// getSlogLogger returns the slog-backed logger for the context. If one or
// more keys are provided, they will be resolved on the context and included
// in the logger.
func getSlogLogger(ctx context.Context, keys ...any) Logger {
	var logger Logger

	// Get a logger, if it is present.
	if lgr, ok := ctx.Value(loggerKey{}).(Logger); ok {
		logger = lgr
	}

	if logger.Logger == nil {
		defaultLoggerMu.RLock()
		logger = defaultLogger
		defaultLoggerMu.RUnlock()

		if logger.Logger == nil {
			logger = Logger{Logger: slog.Default().With("go.version", runtime.Version())}
		}

		// Fill in the instance id, if we have it.
		if instanceID := ctx.Value("instance.id"); instanceID != nil {
			logger = logger.with("instance.id", instanceID)
		}
	}

	for _, key := range keys {
		if v := ctx.Value(key); v != nil {
			logger = logger.with(fmt.Sprint(key), v)
		}
	}

	return logger
}
