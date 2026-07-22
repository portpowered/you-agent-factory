package terminalpolicy

import (
	"bytes"
	"io"
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func buildTestLogger(mode Mode, debug bool) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	switch mode {
	case ModeQuiet:
		return zap.NewNop(), nil
	case ModeNormal:
		level = zapcore.WarnLevel
	case ModeVerbose:
		if debug {
			level = zapcore.DebugLevel
		}
	}
	return zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(io.Discard),
		level,
	)), nil
}

func TestResolve_QuietWinsOverVerboseAndDebug(t *testing.T) {
	policy := Resolve(Options{Quiet: true, Verbose: true, Debug: true})
	if policy.Mode() != ModeQuiet {
		t.Fatalf("mode = %q, want %q", policy.Mode(), ModeQuiet)
	}
	if policy.VerboseEnabled() {
		t.Fatal("expected quiet policy to disable verbose runtime diagnostics")
	}
	if policy.AllowsCommandDiagnostics() {
		t.Fatal("expected quiet policy to suppress command diagnostics")
	}
	if policy.AllowsHumanTerminalOutput() {
		t.Fatal("expected quiet policy to suppress human terminal output")
	}
}

func TestResolve_DebugImpliesVerboseDiagnostics(t *testing.T) {
	policy := Resolve(Options{Debug: true})
	if policy.Mode() != ModeVerbose {
		t.Fatalf("mode = %q, want %q", policy.Mode(), ModeVerbose)
	}
	if !policy.VerboseEnabled() {
		t.Fatal("expected debug policy to enable verbose runtime diagnostics")
	}
	if !policy.AllowsCommandDiagnostics() {
		t.Fatal("expected debug policy to allow command diagnostics")
	}
	if !policy.AllowsStructuredLogTerminal() {
		t.Fatal("expected debug policy to allow structured terminal logs")
	}
}

func TestResolve_NormalSuppressesDiagnosticsAndStructuredLogs(t *testing.T) {
	policy := Resolve(Options{})
	if policy.Mode() != ModeNormal {
		t.Fatalf("mode = %q, want %q", policy.Mode(), ModeNormal)
	}
	if policy.AllowsCommandDiagnostics() {
		t.Fatal("expected normal policy to suppress command diagnostics")
	}
	if policy.AllowsStructuredLogTerminal() {
		t.Fatal("expected normal policy to suppress structured terminal logs")
	}
	if !policy.AllowsHumanTerminalOutput() {
		t.Fatal("expected normal policy to allow human terminal output")
	}
}

func TestPolicy_WritersFollowResolvedMode(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	quiet := Resolve(Options{Quiet: true})
	if quiet.HumanTerminalWriter(stdout) != nil {
		t.Fatal("expected quiet human terminal writer to be nil")
	}
	if quiet.DiagnosticsWriter(stderr) != nil {
		t.Fatal("expected quiet diagnostics writer to be nil")
	}

	normal := Resolve(Options{})
	if normal.HumanTerminalWriter(stdout) == nil {
		t.Fatal("expected normal human terminal writer")
	}
	if normal.DiagnosticsWriter(stderr) != nil {
		t.Fatal("expected normal diagnostics writer to be nil")
	}

	verbose := Resolve(Options{Verbose: true})
	if verbose.HumanTerminalWriter(stdout) == nil {
		t.Fatal("expected verbose human terminal writer")
	}
	if verbose.DiagnosticsWriter(stderr) == nil {
		t.Fatal("expected verbose diagnostics writer")
	}
}

func TestDiagnosticsEnabled_PrefersResolvedPolicy(t *testing.T) {
	quiet := Resolve(Options{Quiet: true})
	if DiagnosticsEnabled(quiet, true) {
		t.Fatal("expected resolved quiet policy to override legacy verbose=true")
	}
	if !DiagnosticsEnabled(Policy{}, true) {
		t.Fatal("expected unresolved policy to fall back to legacy verbose=true")
	}
	if DiagnosticsEnabled(Policy{}, false) {
		t.Fatal("expected unresolved policy to honor legacy verbose=false")
	}
}

func TestBuildLogger_FollowsResolvedMode(t *testing.T) {
	quietLogger, err := Resolve(Options{Quiet: true}).BuildLogger(buildTestLogger)
	if err != nil {
		t.Fatalf("BuildLogger quiet: %v", err)
	}
	if quietLogger.Core().Enabled(zapcore.InfoLevel) {
		t.Fatal("expected quiet logger to discard info level output")
	}

	normalLogger, err := Resolve(Options{}).BuildLogger(buildTestLogger)
	if err != nil {
		t.Fatalf("BuildLogger normal: %v", err)
	}
	if normalLogger.Core().Enabled(zapcore.InfoLevel) {
		t.Fatal("expected normal logger to disable info level")
	}
	if !normalLogger.Core().Enabled(zapcore.WarnLevel) {
		t.Fatal("expected normal logger to enable warn level")
	}

	verboseLogger, err := Resolve(Options{Verbose: true}).BuildLogger(buildTestLogger)
	if err != nil {
		t.Fatalf("BuildLogger verbose: %v", err)
	}
	if !verboseLogger.Core().Enabled(zapcore.InfoLevel) {
		t.Fatal("expected verbose logger to enable info level")
	}
}

func TestBuildLogger_DelegatesEveryResolvedModeToInjectedBuilder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  Options
		mode  Mode
		debug bool
	}{
		{name: "quiet", opts: Options{Quiet: true, Debug: true}, mode: ModeQuiet},
		{name: "normal", opts: Options{}, mode: ModeNormal},
		{name: "verbose", opts: Options{Verbose: true}, mode: ModeVerbose},
		{name: "debug", opts: Options{Debug: true}, mode: ModeVerbose, debug: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotMode Mode
			var gotDebug bool
			logger, err := Resolve(tc.opts).BuildLogger(func(mode Mode, debug bool) (*zap.Logger, error) {
				gotMode, gotDebug = mode, debug
				return zap.NewNop(), nil
			})
			if err != nil || logger == nil {
				t.Fatalf("BuildLogger = %#v, %v", logger, err)
			}
			if gotMode != tc.mode || gotDebug != tc.debug {
				t.Fatalf("builder request = (%q, %t), want (%q, %t)", gotMode, gotDebug, tc.mode, tc.debug)
			}
		})
	}
}

func TestBuildLogger_RejectsMissingVerboseBuilder(t *testing.T) {
	logger, err := Resolve(Options{Verbose: true}).BuildLogger(nil)
	if err == nil || logger != nil {
		t.Fatalf("BuildLogger(nil) = %#v, %v; want explicit error", logger, err)
	}
}

func TestBuildLogger_NormalModeDoesNotWriteStructuredLogsToStderr(t *testing.T) {
	oldStderr := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = writePipe

	logger, err := Resolve(Options{}).BuildLogger(buildTestLogger)
	if err != nil {
		t.Fatalf("BuildLogger normal: %v", err)
	}
	logger.Warn("normal mode structured leak probe", zap.String("probe", "value"))

	if err := writePipe.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	os.Stderr = oldStderr

	captured, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	if len(captured) != 0 {
		t.Fatalf("stderr = %q, want no structured terminal output in normal mode", captured)
	}
}
