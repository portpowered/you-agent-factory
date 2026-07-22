package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Hermetic S02 failure-baseline fixtures for one-shot run paths: quiet/CI
// suppression contracts for named @you/goal invocation.

var quietLeakForbiddenMarkers = []string{
	"Factory initiated",
	"Dashboard URL",
	"Runtime log",
	"Opening dashboard",
	"Factory:",
	"Recording saved",
}

func buildFailureBaselineLogger(mode terminalpolicy.Mode, debug bool) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	switch mode {
	case terminalpolicy.ModeQuiet:
		return zap.NewNop(), nil
	case terminalpolicy.ModeNormal:
		level = zapcore.WarnLevel
	case terminalpolicy.ModeVerbose:
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

func assertQuietLeakContractForbidden(t *testing.T, output string) {
	t.Helper()

	for _, forbidden := range quietLeakForbiddenMarkers {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no quiet-leak marker %q", output, forbidden)
		}
	}
}

func TestFailureBaseline_QuietLeak_OneShotBatchQuietSuppressesStartupChatter(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)
	port := unusedTCPPort(t)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                        dir,
		Port:                       port,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		SuppressDashboardRendering: true,
		OpenDashboard:              false,
		DisableDefaultRecording:    true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if output != "" {
		t.Fatalf("stdout = %q, want empty quiet success terminal output", output)
	}
	assertQuietLeakContractForbidden(t, output)
}

func TestFailureBaseline_QuietLeak_OneShotBatchRunSuppressesDashboardMarkers(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                        dir,
		Port:                       0,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		SuppressDashboardRendering: true,
		DisableDefaultRecording:    true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if output != "" {
		t.Fatalf("stdout = %q, want empty dashboard output with quiet suppression", output)
	}
	assertQuietLeakContractForbidden(t, output)
}

func TestFailureBaseline_QuietLeak_OneShotCleanInvocationSuppressesOperatorChatter(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                        dir,
		Port:                       0,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		CleanInvocation:            true,
		SuppressDashboardRendering: true,
		DisableDefaultRecording:    true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if output != "mock worker accepted" {
		t.Fatalf("stdout = %q, want primary clean invocation output", output)
	}
	assertQuietLeakContractForbidden(t, output)
}

func TestFailureBaseline_QuietLeak_OneShotNamedGoalInvocationSuppressesOperatorChatter(t *testing.T) {
	preserveRunGlobals(t)

	text := "quiet-leak baseline goal prompt"
	var output bytes.Buffer
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "req-quiet-leak",
					TraceID:   "trace-quiet-leak",
					Status:    interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "goal quiet baseline completed",
					}},
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                        "/tmp/builtin-goal",
		NamedFactoryName:           packagedGoalFactoryName,
		InvocationPositionalText:   &text,
		StdinIsTTY:                 func() bool { return true },
		SuppressDashboardRendering: true,
		Output:                     &output,
		Port:                       7437,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != "goal quiet baseline completed" {
		t.Fatalf("stdout = %q, want primary invocation result only", got)
	}
	assertQuietLeakContractForbidden(t, output.String())
}

func TestFailureBaseline_QuietLeak_OneShotBatchQuietSuppressesTerminalOnOperationalFailure(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()
	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error {
				return fmt.Errorf("operational failure: mock batch run rejected")
			},
		}, nil
	}

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), RunConfig{
		Dir:                        dir,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		SuppressDashboardRendering: true,
		DisableDefaultRecording:    true,
		TerminalPolicy:             terminalpolicy.Resolve(terminalpolicy.Options{Quiet: true}),
		Logger:                     zap.NewNop(),
		Output:                     &stdout,
	})
	if err == nil {
		t.Fatal("expected operational failure")
	}
	if !strings.Contains(err.Error(), "operational failure") {
		t.Fatalf("error = %q, want operational failure returned to caller", err.Error())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty quiet operational-failure terminal output", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty quiet operational-failure terminal output", stderr.String())
	}
	assertQuietLeakContractForbidden(t, stdout.String()+stderr.String())
}

func TestFailureBaseline_TerminalPolicyNeverLeaksInvocationPromptAcrossModes(t *testing.T) {
	preserveRunGlobals(t)

	secretPrompt := "SECRET_INVOCATION_PROMPT_do-not-log-712407"
	modes := []struct {
		name   string
		policy terminalpolicy.Policy
	}{
		{name: "quiet", policy: terminalpolicy.Resolve(terminalpolicy.Options{Quiet: true})},
		{name: "normal", policy: terminalpolicy.Resolve(terminalpolicy.Options{})},
		{name: "verbose", policy: terminalpolicy.Resolve(terminalpolicy.Options{Verbose: true})},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
				return stubInvocationService{
					run: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
					invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
						return apisurface.FactoryInvocationResult{
							Status: interfaces.InvocationTerminalStatusCompleted,
							PrimaryResult: []work.WorkContentPart{{
								Type: work.WorkContentPartTypeText,
								Text: "policy-safe primary result",
							}},
						}, nil
					},
				}, nil
			}

			logger, err := mode.policy.BuildLogger(buildFailureBaselineLogger)
			if err != nil {
				t.Fatalf("BuildLogger: %v", err)
			}

			var stdout, stderr, diagnostics bytes.Buffer
			err = Run(context.Background(), RunConfig{
				Dir:                        "/tmp/builtin-goal",
				NamedFactoryName:           packagedGoalFactoryName,
				InvocationPositionalText:   &secretPrompt,
				StdinIsTTY:                 func() bool { return true },
				SuppressDashboardRendering: mode.policy.Mode() == terminalpolicy.ModeQuiet,
				TerminalPolicy:             mode.policy,
				Verbose:                    mode.policy.VerboseEnabled(),
				Diagnostics:                mode.policy.DiagnosticsWriter(&diagnostics),
				Output:                     &stdout,
				Port:                       7437,
				Logger:                     logger,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			capture := stdout.String() + stderr.String() + diagnostics.String()
			if strings.Contains(capture, secretPrompt) {
				t.Fatalf("terminal or diagnostics leaked invocation prompt in %s mode:\n%s", mode.name, capture)
			}
			if got := stdout.String(); got != "policy-safe primary result" {
				t.Fatalf("stdout = %q, want primary result only", got)
			}
		})
	}
}

func TestFailureBaseline_NormalModeSuppressesRawStructuredTerminalLogs(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	policy := terminalpolicy.Resolve(terminalpolicy.Options{})
	logger, err := policy.BuildLogger(buildFailureBaselineLogger)
	if err != nil {
		t.Fatalf("BuildLogger: %v", err)
	}

	var startupOut bytes.Buffer
	stdout, stderr, runErr := runWithCapturedTerminal(t, RunConfig{
		Dir:                     dir,
		Port:                    0,
		WorkFile:                workFile,
		MockWorkersEnabled:      true,
		TerminalPolicy:          policy,
		Logger:                  logger,
		StartupOutput:           policy.HumanTerminalWriter(&startupOut),
		DisableDefaultRecording: true,
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	if !strings.Contains(startupOut.String(), "Factory initiated") {
		t.Fatalf("startup output = %q, want human-facing startup lines in normal mode", startupOut.String())
	}
	assertNoRawStructuredTerminalLogs(t, stdout)
	assertNoRawStructuredTerminalLogs(t, stderr)
}

func assertNoRawStructuredTerminalLogs(t *testing.T, output string) {
	t.Helper()

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if looksLikeStructuredLogLine(line) {
			t.Fatalf("terminal output contains raw structured log line %q", line)
		}
	}
}

func looksLikeStructuredLogLine(line string) bool {
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return false
	}
	_, hasLevel := record["level"]
	_, hasMsg := record["msg"]
	_, hasTS := record["ts"]
	_, hasTimestamp := record["timestamp"]
	return hasLevel && hasMsg && (hasTS || hasTimestamp)
}

func runWithCapturedTerminal(t *testing.T, cfg RunConfig) (stdout, stderr string, err error) {
	t.Helper()
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	if cfg.ExecutionBaseDir == "" && cfg.Dir != "" {
		cfg.ExecutionBaseDir = cfg.Dir
	}
	cfg.Output = &stdoutBuffer
	cfg.Diagnostics = &stderrBuffer

	runErr := runWithTestRuntimeRunner(context.Background(), cfg, adaptTestRuntimeRunnerOpener(buildTransportTestRuntime))

	return stdoutBuffer.String(), stderrBuffer.String(), runErr
}

func requireQuietRuntimeLogPath(t *testing.T, logDir, runtimeInstanceID string) string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(logDir, "*", "*", "*", "*-runtime-log-"+runtimeInstanceID+"-*.log"))
	if err != nil {
		t.Fatalf("glob runtime log path: %v", err)
	}
	if len(matches) != 1 {
		logFiles := collectQuietRuntimeLogFiles(t, logDir)
		t.Fatalf("runtime log paths for %q under %s = %v, all log files = %v, want exactly one", runtimeInstanceID, logDir, matches, logFiles)
	}
	return matches[0]
}

func collectQuietRuntimeLogFiles(t *testing.T, dir string) []string {
	t.Helper()

	var logFiles []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".log" {
			logFiles = append(logFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", dir, err)
	}
	return logFiles
}
