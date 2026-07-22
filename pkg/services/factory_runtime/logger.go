package factory

import "go.uber.org/zap"

// Logger is the Factory Runtime-owned structured logging contract.
type Logger interface {
	Debug(string, ...any)
	Info(string, ...any)
	Warn(string, ...any)
	Error(string, ...any)
	Verbose(string, ...any)
}

type RuntimeLoggerFactory func(*zap.Logger, bool) Logger

type NoopLogger struct{}

func (NoopLogger) Debug(string, ...any)   {}
func (NoopLogger) Info(string, ...any)    {}
func (NoopLogger) Warn(string, ...any)    {}
func (NoopLogger) Error(string, ...any)   {}
func (NoopLogger) Verbose(string, ...any) {}
