package logging

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

// spyLogger records calls to all four logging methods.
type spyLogger struct {
	debugCalls []string
	infoCalls  []string
	warnCalls  []string
	errorCalls []string
}

func (s *spyLogger) Debug(msg string, _ ...any) { s.debugCalls = append(s.debugCalls, msg) }
func (s *spyLogger) Info(msg string, _ ...any)  { s.infoCalls = append(s.infoCalls, msg) }
func (s *spyLogger) Warn(msg string, _ ...any)  { s.warnCalls = append(s.warnCalls, msg) }
func (s *spyLogger) Error(msg string, _ ...any) { s.errorCalls = append(s.errorCalls, msg) }

type verboseCall struct {
	msg           string
	keysAndValues []any
}

type verboseSpyLogger struct {
	spyLogger
	verboseCalls []verboseCall
}

func (s *verboseSpyLogger) Verbose(msg string, keysAndValues ...any) {
	s.verboseCalls = append(s.verboseCalls, verboseCall{
		msg:           msg,
		keysAndValues: append([]any(nil), keysAndValues...),
	})
}

func TestSpyLogger_ImplementsLogger(t *testing.T) {
	var l Logger = &spyLogger{}
	l.Debug("debug msg", "key", "value")
	l.Info("info msg")
	l.Warn("warn msg")
	l.Error("error msg")

	spy := l.(*spyLogger)
	if len(spy.debugCalls) != 1 || spy.debugCalls[0] != "debug msg" {
		t.Errorf("expected 1 debug call with 'debug msg', got %v", spy.debugCalls)
	}
	if len(spy.infoCalls) != 1 {
		t.Errorf("expected 1 info call, got %d", len(spy.infoCalls))
	}
	if len(spy.warnCalls) != 1 {
		t.Errorf("expected 1 warn call, got %d", len(spy.warnCalls))
	}
	if len(spy.errorCalls) != 1 {
		t.Errorf("expected 1 error call, got %d", len(spy.errorCalls))
	}
}

func TestNoopLogger_DebugDoesNotPanic(t *testing.T) {
	var l Logger = NoopLogger{}
	l.Debug("should not panic", "key", "value")
}

func TestEnsureLogger_NilReturnsNoop(t *testing.T) {
	l := EnsureLogger(nil)
	if _, ok := l.(NoopLogger); !ok {
		t.Errorf("expected NoopLogger, got %T", l)
	}
}

func TestEnsureLogger_NonNilReturnsSame(t *testing.T) {
	spy := &spyLogger{}
	l := EnsureLogger(spy)
	if l != spy {
		t.Error("expected EnsureLogger to return the same logger when non-nil")
	}
}

func TestVerbose_ForwardsToVerboseCapableLogger(t *testing.T) {
	logger := &verboseSpyLogger{}

	Verbose(logger, "verbose message", "worker", "alpha", "attempt", 2)

	if len(logger.verboseCalls) != 1 {
		t.Fatalf("expected 1 verbose call, got %d", len(logger.verboseCalls))
	}
	if logger.verboseCalls[0].msg != "verbose message" {
		t.Fatalf("expected verbose message to be forwarded, got %q", logger.verboseCalls[0].msg)
	}
	expectedFields := []any{"worker", "alpha", "attempt", 2}
	if len(logger.verboseCalls[0].keysAndValues) != len(expectedFields) {
		t.Fatalf("expected %d verbose fields, got %d", len(expectedFields), len(logger.verboseCalls[0].keysAndValues))
	}
	for i, field := range expectedFields {
		if logger.verboseCalls[0].keysAndValues[i] != field {
			t.Fatalf("expected verbose field %d to be %#v, got %#v", i, field, logger.verboseCalls[0].keysAndValues[i])
		}
	}
}

func TestVerbose_IgnoresPlainLogger(t *testing.T) {
	logger := &spyLogger{}

	Verbose(logger, "verbose message", "worker", "alpha")

	if len(logger.debugCalls) != 0 || len(logger.infoCalls) != 0 || len(logger.warnCalls) != 0 || len(logger.errorCalls) != 0 {
		t.Fatalf("expected plain logger to ignore verbose emission, got debug=%v info=%v warn=%v error=%v", logger.debugCalls, logger.infoCalls, logger.warnCalls, logger.errorCalls)
	}
}

func TestBuildLogger_Verbose(t *testing.T) {
	logger, err := BuildLogger(true, false)
	if err != nil {
		t.Fatalf("BuildLogger(true, false): %v", err)
	}
	if !logger.Core().Enabled(zapcore.InfoLevel) {
		t.Error("verbose logger should enable info level")
	}
	if logger.Core().Enabled(zapcore.DebugLevel) {
		t.Error("verbose logger should not enable debug level")
	}
}

func TestBuildLogger_Quiet(t *testing.T) {
	logger, err := BuildLogger(false, false)
	if err != nil {
		t.Fatalf("BuildLogger(false, false): %v", err)
	}
	if logger.Core().Enabled(zapcore.InfoLevel) {
		t.Error("quiet logger should not enable info level")
	}
	if !logger.Core().Enabled(zapcore.WarnLevel) {
		t.Error("quiet logger should enable warn level")
	}
}

func TestBuildLogger_Debug(t *testing.T) {
	logger, err := BuildLogger(false, true)
	if err != nil {
		t.Fatalf("BuildLogger(false, true): %v", err)
	}
	if !logger.Core().Enabled(zapcore.DebugLevel) {
		t.Error("debug logger should enable debug level")
	}
	if !logger.Core().Enabled(zapcore.InfoLevel) {
		t.Error("debug logger should enable info level (debug implies verbose)")
	}
}

func TestBuildLogger_DebugOverridesVerbose(t *testing.T) {
	logger, err := BuildLogger(true, true)
	if err != nil {
		t.Fatalf("BuildLogger(true, true): %v", err)
	}
	if !logger.Core().Enabled(zapcore.DebugLevel) {
		t.Error("debug logger should enable debug level even when verbose is also set")
	}
}
