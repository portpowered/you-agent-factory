package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the logging interface accepted by the factory.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// verboseLogger is an optional extension for loggers that can emit records
// only when the caller explicitly enabled verbose runtime diagnostics.
type verboseLogger interface {
	Logger
	Verbose(msg string, keysAndValues ...any)
}

// NoopLogger is a Logger that discards all log output.
// Used as the default when no logger is provided.
type NoopLogger struct{}

func (NoopLogger) Debug(_ string, _ ...any) {}
func (NoopLogger) Info(_ string, _ ...any)  {}
func (NoopLogger) Warn(_ string, _ ...any)  {}
func (NoopLogger) Error(_ string, _ ...any) {}
func (NoopLogger) Verbose(_ string, _ ...any) {
}

// EnsureLogger returns l if non-nil, otherwise returns a NoopLogger.
func EnsureLogger(l Logger) Logger {
	if l == nil {
		return NoopLogger{}
	}
	return l
}

// Verbose emits a verbose-only log record when the logger supports that
// optional mode. Plain Logger implementations ignore verbose records.
func Verbose(l Logger, msg string, keysAndValues ...any) {
	if logger, ok := l.(verboseLogger); ok {
		logger.Verbose(msg, keysAndValues...)
	}
}

// BuildLogger creates a zap.Logger with the appropriate verbosity level.
//   - debug=true: Debug+ (implies verbose; development-style output)
//   - verbose=true: Info+ (development-style output)
//   - default: Warn+ (production-like)
func BuildLogger(verbose, debug bool) (*zap.Logger, error) {
	if debug {
		return zap.NewDevelopment()
	}
	if verbose {
		cfg := zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		return cfg.Build()
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	return cfg.Build()
}
