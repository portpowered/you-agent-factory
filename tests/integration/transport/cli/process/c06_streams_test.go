package process_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const successStdoutPrimaryResult = "mock worker accepted"

// TestCLISuccessWritesPrimaryResultOnlyToStdout proves a successful built CLI
// run writes the primary invocation result to stdout only.
func TestCLISuccessWritesPrimaryResultOnlyToStdout(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, mustIntegrationRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	session := harness.NewSession(t).WithNoExternalServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	configureGoalSession(t, ctx, binaryPath, session, "success-stdout-purity-config")
	args := appendGoalRunArgs(session, writeAcceptingGoalMockWorkers(t), "success-stdout-purity", "--quiet")
	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
	if err != nil {
		t.Fatalf("successful stdout-purity run failed: %v\nstdout:\n%s\nstderr:\n%s", err, result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 || result.Stdout != successStdoutPrimaryResult {
		t.Fatalf("success result = %#v, want exit 0 and primary stdout %q", result, successStdoutPrimaryResult)
	}
	for _, forbidden := range []string{"Factory initiated:", "Dashboard URL:", "Dashboard server disabled", "error:", "Error:", "traceId:", "requestId:"} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf("stdout mixed diagnostic or lifecycle noise %q into primary result:\n%s", forbidden, result.Stdout)
		}
	}
	if strings.TrimSpace(result.Stderr) != "" {
		t.Fatalf("stderr = %q, want empty diagnostics", result.Stderr)
	}
}

// TestCLIFailureWritesDiagnosticToStderr proves a terminal worker failure in
// the built CLI writes a coded diagnostic to stderr and no false result to stdout.
func TestCLIFailureWritesDiagnosticToStderr(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, mustIntegrationRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	session := harness.NewSession(t).WithNoExternalServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	configureGoalSession(t, ctx, binaryPath, session, "failure-stderr-config")
	args := appendGoalRunArgs(session, writeRejectingGoalMockWorkers(t), "failure-stderr", "--quiet")
	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
	if err == nil || result.ExitCode == 0 {
		t.Fatalf("terminal worker failure result = %#v error=%v; want non-zero process failure", result, err)
	}
	if strings.TrimSpace(result.Stderr) == "" {
		t.Fatal("terminal worker failure stderr was empty; want actionable diagnostic")
	}
	diagnostic := result.Stderr + "\n" + err.Error()
	if !strings.Contains(diagnostic, "INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("terminal worker failure diagnostic missing INVOCATION_RUNTIME_FAILURE:\n%s", diagnostic)
	}
	if strings.Contains(result.Stderr, "Factory initiated:") || strings.Contains(result.Stderr, "Dashboard URL:") {
		t.Fatalf("stderr mixed lifecycle chatter into diagnostic stream:\n%s", result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) != "" || strings.Contains(result.Stdout, successStdoutPrimaryResult) {
		t.Fatalf("stdout = %q, want no false primary-result payload", result.Stdout)
	}
}

const quietPrimaryResultSeparator = "--- primary result ---"

// TestCLIQuietModeSuppressesNonResultNoise proves --quiet keeps built-CLI
// streams script-safe while the response-stream and verbose runs retain their
// expected presentation contrasts.
func TestCLIQuietModeSuppressesNonResultNoise(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, mustIntegrationRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	t.Run("success suppresses stdout lifecycle presentation", func(t *testing.T) {
		streamSession := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-stream-config")
		streamArgs := appendGoalRunArgs(streamSession, writeAcceptingGoalMockWorkers(t), "quiet-mode-stream-baseline", "--output", "response-stream")
		streamResult, err := runBuiltYouBinary(ctx, binaryPath, streamSession, streamArgs...)
		if err != nil || streamResult.ExitCode != 0 {
			t.Fatalf("response-stream success result=%#v error=%v", streamResult, err)
		}
		if !strings.Contains(streamResult.Stdout, quietPrimaryResultSeparator) || !containsHumanLifecycleNoise(streamResult.Stdout) || streamResult.Stdout == successStdoutPrimaryResult {
			t.Fatalf("response-stream stdout = %q, want lifecycle presentation", streamResult.Stdout)
		}
		quietSession := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-success-config")
		quietArgs := appendGoalRunArgs(quietSession, writeAcceptingGoalMockWorkers(t), "quiet-mode-success", "--quiet")
		quietResult, err := runBuiltYouBinary(ctx, binaryPath, quietSession, quietArgs...)
		assertQuietSuccess(t, quietResult, err)
		for _, forbidden := range []string{quietPrimaryResultSeparator, "factory started", "work accepted"} {
			if strings.Contains(quietResult.Stdout, forbidden) {
				t.Fatalf("quiet stdout leaked non-result noise %q:\n%s", forbidden, quietResult.Stdout)
			}
		}
	})
	t.Run("success suppresses verbose stderr operator logs", func(t *testing.T) {
		verboseSession := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-verbose-config")
		verboseArgs := appendGoalRunArgs(verboseSession, writeAcceptingGoalMockWorkers(t), "quiet-mode-verbose-baseline", "--verbose")
		verboseResult, err := runBuiltYouBinary(ctx, binaryPath, verboseSession, verboseArgs...)
		if err != nil || verboseResult.ExitCode != 0 || !strings.HasSuffix(verboseResult.Stdout, successStdoutPrimaryResult) {
			t.Fatalf("verbose result=%#v error=%v, want successful primary-result suffix", verboseResult, err)
		}
		quietSession := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-verbose-contrast-config")
		quietArgs := appendGoalRunArgs(quietSession, writeAcceptingGoalMockWorkers(t), "quiet-mode-verbose-contrast", "--quiet")
		quietResult, err := runBuiltYouBinary(ctx, binaryPath, quietSession, quietArgs...)
		assertQuietSuccess(t, quietResult, err)
	})
	t.Run("failure keeps quiet stdout script-safe", func(t *testing.T) {
		session := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-failure-config")
		args := appendGoalRunArgs(session, writeRejectingGoalMockWorkers(t), "quiet-mode-failure", "--quiet")
		result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
		if err == nil || result.ExitCode == 0 || strings.TrimSpace(result.Stdout) != "" || strings.TrimSpace(result.Stderr) == "" {
			t.Fatalf("quiet failure result=%#v error=%v, want non-zero, empty stdout, diagnostic stderr", result, err)
		}
	})
}

func assertQuietSuccess(t testing.TB, result builtcliacceptance.RunResult, err error) {
	t.Helper()
	if err != nil || result.ExitCode != 0 || result.Stdout != successStdoutPrimaryResult || strings.TrimSpace(result.Stderr) != "" {
		t.Fatalf("quiet success result=%#v error=%v, want exact primary stdout and empty stderr", result, err)
	}
}

func newConfiguredGoalSession(t testing.TB, ctx context.Context, binaryPath string, harness *builtcliacceptance.Harness, scenario string) *builtcliacceptance.Session {
	t.Helper()
	session := harness.NewSession(t).WithNoExternalServer(t)
	configureGoalSession(t, ctx, binaryPath, session, scenario)
	return session
}

func configureGoalSession(t testing.TB, ctx context.Context, binaryPath string, session *builtcliacceptance.Session, scenario string) {
	t.Helper()
	configPath := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	missingFactory := filepath.Join(session.WorkDir, "missing-initialization-factory.json")
	result, err := runBuiltYouBinary(ctx, binaryPath, session, "run", "--factory", missingFactory)
	if err == nil {
		t.Fatalf("%s: missing Factory unexpectedly succeeded: %#v", scenario, result)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("%s: initializer-owned config missing at %s: %v", scenario, configPath, err)
	}
	configBody := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"gpt-5-codex"}}`)
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatalf("write operator config %q: %v", configPath, err)
	}
}

func appendGoalRunArgs(session *builtcliacceptance.Session, mockWorkersPath, prompt string, extraArgs ...string) []string {
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args, "run", "--named", "@you/goal", "--with-mock-workers="+mockWorkersPath, "--no-record")
	args = append(args, extraArgs...)
	return append(args, prompt)
}

func writeAcceptingGoalMockWorkers(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal(workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{WorkerName: "goal-planner", WorkstationName: "plan-goal", RunType: workers.MockWorkerRunTypeAccept},
			{WorkerName: "goal-executor", WorkstationName: "execute-goal", RunType: workers.MockWorkerRunTypeAccept},
			{WorkerName: "goal-checker", WorkstationName: "check-goal", RunType: workers.MockWorkerRunTypeAccept},
			{WorkerName: "goal-reviewer", WorkstationName: "review-goal", RunType: workers.MockWorkerRunTypeAccept},
		},
	})
	return writeIntegrationMockWorkers(t, "accepting-mock-workers.json", data, err)
}

func writeRejectingGoalMockWorkers(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal(workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{
		{WorkerName: "goal-planner", WorkstationName: "plan-goal", RunType: workers.MockWorkerRunTypeReject},
		{WorkerName: "goal-executor", WorkstationName: "execute-goal", RunType: workers.MockWorkerRunTypeReject},
		{WorkerName: "goal-checker", WorkstationName: "check-goal", RunType: workers.MockWorkerRunTypeReject},
		{WorkerName: "goal-reviewer", WorkstationName: "review-goal", RunType: workers.MockWorkerRunTypeReject},
	}})
	return writeIntegrationMockWorkers(t, "rejecting-mock-workers.json", data, err)
}

func writeIntegrationMockWorkers(t *testing.T, name string, data []byte, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func containsHumanLifecycleNoise(stdout string) bool {
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == quietPrimaryResultSeparator || line == successStdoutPrimaryResult {
			continue
		}
		closingBracket := strings.Index(line, "] ")
		if !strings.HasPrefix(line, "[") || closingBracket < 2 {
			continue
		}
		message := line[closingBracket+2:]
		for _, prefix := range []string{"work accepted", "work moved", "factory started", "factory completed", "workstation queued", "workstation started", "workstation completed", "final output updated"} {
			if strings.HasPrefix(message, prefix) {
				return true
			}
		}
	}
	return false
}

func mustIntegrationRepoRoot(t testing.TB) string {
	t.Helper()
	return testutil.MustRepoRoot(t)
}
