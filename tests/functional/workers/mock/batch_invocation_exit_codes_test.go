package mock

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
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type batchProcessReport struct {
	Status   string                `json:"status"`
	Failures []batchProcessFailure `json:"failures"`
}

type batchProcessFailure struct {
	WorkID    string `json:"workId,omitempty"`
	WorkName  string `json:"workName"`
	WorkState string `json:"workState"`
	Reason    string `json:"reason"`
}

// TestBuiltCLIBatchExitCodesReportSingleWorkOutcome proves the ordinary
// --work path, which is distinct from the characterized one-shot --named
// path, returns process status and stdout from the submitted Work outcome. It
// intentionally builds ./cmd/factory to a test-owned executable and invokes
// that subprocess with exec.CommandContext so captured output and the OS exit
// status remain real process witnesses rather than injected-edge observations.
func TestBuiltCLIBatchExitCodesReportSingleWorkOutcome(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildContext, cancelBuild := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancelBuild()
	binaryPath := buildYouBinary(t, buildContext, testutil.MustRepoRoot(t))

	t.Run("success quiet result exits zero", func(t *testing.T) {
		t.Parallel()
		runCompiledBatchQuietSuccess(t, binaryPath, harness)
	})

	for _, policy := range []struct {
		name  string
		extra string
	}{
		{name: "default"},
		{name: "verbose", extra: "--verbose"},
	} {
		policy := policy
		t.Run("success "+policy.name+" policy keeps result", func(t *testing.T) {
			t.Parallel()
			runCompiledBatchSuccess(t, binaryPath, harness, policy.name, policy.extra)
		})
	}

	t.Run("failed terminal Work exits nonzero with human detail", func(t *testing.T) {
		t.Parallel()
		runCompiledBatchHumanFailure(t, binaryPath, harness)
	})

	t.Run("failed terminal Work JSON is parseable", func(t *testing.T) {
		t.Parallel()
		runCompiledBatchJSONFailure(t, binaryPath, harness)
	})
}

// TestBuiltCLIBatchExitCodesAggregateFailureCauses proves batch CLI exit codes
// aggregate every failure cause. Like the companion batch row, it remains
// process-isolated: ./cmd/factory is built as a test-owned executable and
// invoked with exec.CommandContext so subprocess output and OS exit status are
// part of the retained witness.
func TestBuiltCLIBatchExitCodesAggregateFailureCauses(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildContext, cancelBuild := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancelBuild()
	binaryPath := buildYouBinary(t, buildContext, testutil.MustRepoRoot(t))

	t.Run("all submitted Work failures are reported deterministically", func(t *testing.T) {
		t.Parallel()
		runCompiledBatchAllFailedHuman(t, binaryPath, harness)
	})

	t.Run("all submitted Work failures have a complete JSON collection", func(t *testing.T) {
		t.Parallel()
		runCompiledBatchAllFailedJSON(t, binaryPath, harness)
	})

	t.Run("mixed success and failure does not round to success", func(t *testing.T) {
		t.Parallel()
		runCompiledBatchMixed(t, binaryPath, harness)
	})

	for _, policy := range []struct {
		name string
		json bool
	}{
		{name: "human"},
		{name: "json", json: true},
	} {
		policy := policy
		t.Run("circuit breaker reports reason in "+policy.name+" output", func(t *testing.T) {
			t.Parallel()
			runCompiledBatchCircuitBreaker(t, binaryPath, harness, policy.json)
		})
	}

	for _, policy := range []struct {
		name string
		json bool
	}{
		{name: "human"},
		{name: "json", json: true},
	} {
		policy := policy
		t.Run("script non-zero exit reports reason in "+policy.name+" output", func(t *testing.T) {
			t.Parallel()
			runCompiledBatchScriptFailure(t, binaryPath, harness, policy.json)
		})
	}
}

func runCompiledBatchQuietSuccess(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-success")
	writeBatchCurrentFactory(t, session.WorkDir)
	workFile := writeBatchWork(t, "single successful batch Work")
	mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeAccept)
	result, err := runBuiltYouBinary(ctx, binaryPath, session, batchRunArgs(session, workFile, mockWorkersPath, "--quiet")...)
	if err != nil || result.ExitCode != 0 || result.Stdout != "Batch completed successfully.\n" || result.Stderr != "" {
		t.Fatalf("compiled batch success: %v; exit=%d stdout=%q stderr=%q", err, result.ExitCode, result.Stdout, result.Stderr)
	}
}

func runCompiledBatchSuccess(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness, policy, extra string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-success-"+policy)
	writeBatchCurrentFactory(t, session.WorkDir)
	workFile := writeBatchWork(t, "single "+policy+" batch Work")
	mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeAccept)
	args := batchRunArgs(session, workFile, mockWorkersPath)
	if extra != "" {
		args = append(args, extra)
	}
	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
	if err != nil || result.ExitCode != 0 || !strings.Contains(result.Stdout, "Batch completed successfully.") {
		t.Fatalf("compiled batch %s success: %v; exit=%d stdout=%q stderr=%q", policy, err, result.ExitCode, result.Stdout, result.Stderr)
	}
}

func runCompiledBatchHumanFailure(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-human-failure")
	writeBatchCurrentFactory(t, session.WorkDir)
	workFile := writeBatchWork(t, "single failing batch Work")
	mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeReject)
	result, err := runBuiltYouBinary(ctx, binaryPath, session, batchRunArgs(session, workFile, mockWorkersPath, "--quiet")...)
	if err == nil || result.ExitCode == 0 {
		t.Fatalf("compiled batch failure result = %#v, error = %v; want non-zero", result, err)
	}
	for _, want := range []string{"Batch failed:", `Work "single failing batch Work"`, "prompt-task:failed"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("failure stdout missing %q:\n%s", want, result.Stdout)
		}
	}
	if strings.TrimSpace(result.Stderr) == "" {
		t.Fatal("failure stderr is empty; want the non-zero batch error diagnostic")
	}
}

func runCompiledBatchJSONFailure(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-json-failure")
	writeBatchCurrentFactory(t, session.WorkDir)
	workFile := writeBatchWork(t, "single JSON failing batch Work")
	mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeReject)
	result, err := runBuiltYouBinary(ctx, binaryPath, session, append([]string{"--json"}, batchRunArgs(session, workFile, mockWorkersPath)...)...)
	if err == nil || result.ExitCode == 0 {
		t.Fatalf("compiled batch JSON failure result = %#v, error = %v; want non-zero", result, err)
	}
	report := decodeBatchProcessReport(t, result.Stdout)
	if report.Status != "FAILED" || len(report.Failures) != 1 {
		t.Fatalf("JSON batch report = %#v, want one failure", report)
	}
	failure := report.Failures[0]
	if failure.WorkName != "single JSON failing batch Work" || failure.WorkState != "prompt-task:failed" || strings.TrimSpace(failure.Reason) == "" {
		t.Fatalf("JSON failure = %#v, want Work name, terminal state, and reason", failure)
	}
}

func runCompiledBatchAllFailedHuman(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-all-failed")
	writeBatchCurrentFactory(t, session.WorkDir)
	workFile := writeBatchWorks(t, "all failed first Work", "all failed second Work")
	mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeReject)
	result, err := runBuiltYouBinary(ctx, binaryPath, session, batchRunArgs(session, workFile, mockWorkersPath, "--quiet")...)
	if err == nil || result.ExitCode == 0 {
		t.Fatalf("all-failed batch result = %#v, error = %v; want non-zero", result, err)
	}
	for _, want := range []string{`Work "all failed first Work"`, `Work "all failed second Work"`, "prompt-task:failed"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("all-failed stdout missing %q:\n%s", want, result.Stdout)
		}
	}
}

func runCompiledBatchAllFailedJSON(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-all-failed-json")
	writeBatchCurrentFactory(t, session.WorkDir)
	workFile := writeBatchWorks(t, "all failed first JSON Work", "all failed second JSON Work")
	mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeReject)
	result, err := runBuiltYouBinary(ctx, binaryPath, session, append([]string{"--json"}, batchRunArgs(session, workFile, mockWorkersPath)...)...)
	if err == nil || result.ExitCode == 0 {
		t.Fatalf("all-failed JSON batch result = %#v, error = %v; want non-zero", result, err)
	}
	report := decodeBatchProcessReport(t, result.Stdout)
	if report.Status != "FAILED" || len(report.Failures) != 2 {
		t.Fatalf("all-failed JSON report = %#v, want two failures", report)
	}
	if report.Failures[0].WorkName != "all failed first JSON Work" || report.Failures[1].WorkName != "all failed second JSON Work" {
		t.Fatalf("all-failed JSON failures = %#v, want deterministic Work ordering", report.Failures)
	}
	for _, failure := range report.Failures {
		if failure.WorkState != "prompt-task:failed" || strings.TrimSpace(failure.Reason) == "" {
			t.Fatalf("all-failed JSON failure = %#v, want state and reason", failure)
		}
	}
}

func runCompiledBatchMixed(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-mixed-outcomes")
	writeBatchMixedFactory(t, session.WorkDir)
	workFile := writeBatchWorksWithTypes(t, batchWorkSpec{Name: "mixed successful Work", WorkTypeID: "successful-task"}, batchWorkSpec{Name: "mixed failed Work", WorkTypeID: "failed-task"})
	mockWorkersPath := writeBatchMixedMockWorkers(t)
	result, err := runBuiltYouBinary(ctx, binaryPath, session, append([]string{"--json"}, batchRunArgs(session, workFile, mockWorkersPath)...)...)
	if err == nil || result.ExitCode == 0 {
		t.Fatalf("mixed batch result = %#v, error = %v; want non-zero", result, err)
	}
	report := decodeBatchProcessReport(t, result.Stdout)
	if report.Status != "FAILED" || len(report.Failures) != 1 {
		t.Fatalf("mixed JSON report = %#v, want one failed Work", report)
	}
	failure := report.Failures[0]
	if failure.WorkName != "mixed failed Work" || failure.WorkState != "failed-task:failed" {
		t.Fatalf("mixed failure = %#v, want only the failed Work", failure)
	}
}

func runCompiledBatchCircuitBreaker(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness, jsonOutput bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-circuit-breaker")
	writeBatchRetryFactory(t, session.WorkDir)
	workFile := writeBatchWorksWithTypes(t, batchWorkSpec{Name: "circuit breaker Work", WorkTypeID: "retry-task"})
	programPath := writeBatchFailingScript(t)
	// A normalized terminal provider rejection routes directly to FAILED; use
	// a retryable script failure so this fixture exercises the retry breaker.
	mockWorkersPath := writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: "retry-worker", WorkstationName: "retry-work", RunType: workers.MockWorkerRunTypeScript,
		ScriptConfig: &workers.MockWorkerScriptConfig{
			Command: "go", Args: []string{"run", programPath}, Env: map[string]string{"GO111MODULE": "off"},
		},
	}}})
	args := batchRunArgs(session, workFile, mockWorkersPath)
	if jsonOutput {
		args = append([]string{"--json"}, args...)
	} else {
		args = append(args, "--quiet")
	}
	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
	if err == nil || result.ExitCode == 0 {
		t.Fatalf("circuit-breaker batch result = %#v, error = %v; want non-zero", result, err)
	}
	const breakerReason = "consecutive failures 1 for transition retry-work exceeds max 1"
	if jsonOutput {
		report := decodeBatchProcessReport(t, result.Stdout)
		if report.Status != "FAILED" || len(report.Failures) != 1 || report.Failures[0].WorkName != "circuit breaker Work" || report.Failures[0].WorkState != "retry-task:failed" || !strings.Contains(report.Failures[0].Reason, breakerReason) {
			t.Fatalf("circuit-breaker JSON report = %#v, want Work, state, and breaker reason", report)
		}
		return
	}
	for _, want := range []string{`Work "circuit breaker Work"`, "retry-task:failed", breakerReason} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("circuit-breaker stdout missing %q:\n%s", want, result.Stdout)
		}
	}
}

func runCompiledBatchScriptFailure(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness, jsonOutput bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-script-failure")
	writeBatchScriptFactory(t, session.WorkDir)
	workFile := writeBatchWorksWithTypes(t, batchWorkSpec{Name: "script failure Work", WorkTypeID: "script-task"})
	programPath := writeBatchFailingScript(t)
	mockWorkersPath := writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: "script-worker", WorkstationName: "script-work", RunType: workers.MockWorkerRunTypeScript,
		ScriptConfig: &workers.MockWorkerScriptConfig{Command: "go", Args: []string{"run", programPath}, Env: map[string]string{"GO111MODULE": "off"}},
	}}})
	args := batchRunArgs(session, workFile, mockWorkersPath)
	if jsonOutput {
		args = append([]string{"--json"}, args...)
	} else {
		args = append(args, "--quiet")
	}
	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
	if err == nil || result.ExitCode == 0 {
		t.Fatalf("script-failure batch result = %#v, error = %v; want non-zero", result, err)
	}
	if jsonOutput {
		report := decodeBatchProcessReport(t, result.Stdout)
		if report.Status != "FAILED" || len(report.Failures) != 1 || report.Failures[0].WorkName != "script failure Work" || report.Failures[0].WorkState != "script-task:failed" || !strings.Contains(report.Failures[0].Reason, "script worker exited non-zero") {
			t.Fatalf("script-failure JSON report = %#v, want Work, state, and script reason", report)
		}
		return
	}
	for _, want := range []string{`Work "script failure Work"`, "script-task:failed", "script worker exited non-zero"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("script-failure stdout missing %q:\n%s", want, result.Stdout)
		}
	}
}

type batchWorkSpec struct {
	Name       string
	WorkTypeID string
}

func decodeBatchProcessReport(t testing.TB, stdout string) batchProcessReport {
	t.Helper()
	var report batchProcessReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("batch JSON stdout is not parseable: %v\nstdout:\n%s", err, stdout)
	}
	return report
}

func batchRunArgs(
	session *builtcliacceptance.Session,
	workFile string,
	mockWorkersPath string,
	extra ...string,
) []string {
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--work", workFile,
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
	)
	return append(args, extra...)
}

func writeBatchCurrentFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	sourcePath := writeStdinRunFactory(t, workingDirectory)
	sourceDir := filepath.Dir(sourcePath)
	destinationDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(filepath.Join(destinationDir, "workstations", stdinRunWorkstationName), 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	factoryJSON, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read batch Current Factory fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destinationDir, "factory.json"), factoryJSON, 0o600); err != nil {
		t.Fatalf("write batch Current Factory fixture: %v", err)
	}
	workstationConfig, err := os.ReadFile(filepath.Join(sourceDir, "workstations", stdinRunWorkstationName, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read batch workstation fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destinationDir, "workstations", stdinRunWorkstationName, "AGENTS.md"), workstationConfig, 0o644); err != nil {
		t.Fatalf("write batch workstation fixture: %v", err)
	}
	workerDir := filepath.Join(destinationDir, "workers", stdinRunWorkerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create batch worker directory: %v", err)
	}
	workerConfig := "---\ntype: SCRIPT_WORKER\ncommand: echo\nargs:\n  - batch-exit-fixture\n---\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(workerConfig), 0o644); err != nil {
		t.Fatalf("write batch worker fixture: %v", err)
	}
}

func writeBatchWork(t testing.TB, name string) string {
	t.Helper()
	return writeBatchWorksWithTypes(t, batchWorkSpec{Name: name, WorkTypeID: stdinRunWorkTypeName})
}

func writeBatchWorks(t testing.TB, names ...string) string {
	t.Helper()
	specs := make([]batchWorkSpec, 0, len(names))
	for _, name := range names {
		specs = append(specs, batchWorkSpec{Name: name, WorkTypeID: stdinRunWorkTypeName})
	}
	return writeBatchWorksWithTypes(t, specs...)
}

func writeBatchWorksWithTypes(t testing.TB, specs ...batchWorkSpec) string {
	t.Helper()
	works := make([]work.Work, 0, len(specs))
	for _, spec := range specs {
		workID := strings.ToLower(strings.ReplaceAll(spec.Name, " ", "-"))
		works = append(works, work.Work{
			Name: spec.Name, WorkID: workID, WorkTypeID: spec.WorkTypeID,
			TraceID: workID + "-trace", Payload: "batch exit contract",
		})
	}
	path := filepath.Join(t.TempDir(), "batch-work.json")
	request := work.WorkRequest{
		Type:  work.WorkRequestTypeFactoryRequestBatch,
		Works: works,
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal batch Work: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write batch Work: %v", err)
	}
	return path
}

func writeBatchMockWorkers(t testing.TB, runType workers.MockWorkerRunType) string {
	t.Helper()
	return writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: stdinRunWorkerName, WorkstationName: stdinRunWorkstationName, RunType: runType,
	}}})
}

func writeBatchMockWorkersConfig(t testing.TB, config workers.MockWorkersConfig) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch-mock-workers.json")
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal batch mock workers: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write batch mock workers: %v", err)
	}
	return path
}

func writeBatchMixedMockWorkers(t testing.TB) string {
	t.Helper()
	return writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{
		{WorkerName: "successful-worker", WorkstationName: "successful-work", RunType: workers.MockWorkerRunTypeAccept},
		{WorkerName: "failed-worker", WorkstationName: "failed-work", RunType: workers.MockWorkerRunTypeReject},
	}})
}

func writeBatchMixedFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	writeBatchFactoryFiles(t, workingDirectory, map[string]any{
		"name": "batch-mixed-outcomes",
		"workTypes": []map[string]any{
			batchWorkTypeConfig("successful-task"),
			batchWorkTypeConfig("failed-task"),
		},
		"workers": []map[string]string{
			{"name": "successful-worker"}, {"name": "failed-worker"},
		},
		"workstations": []map[string]any{
			batchWorkstationConfig("successful-work", "successful-worker", "successful-task", "complete", "failed"),
			batchWorkstationConfig("failed-work", "failed-worker", "failed-task", "complete", "failed"),
		},
	}, map[string]string{
		"successful-worker": batchModelWorkerConfig(),
		"failed-worker":     batchModelWorkerConfig(),
	}, map[string]string{
		"successful-work": batchModelWorkstationConfig(),
		"failed-work":     batchModelWorkstationConfig(),
	})
}

func writeBatchRetryFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	writeBatchFactoryFiles(t, workingDirectory, map[string]any{
		"name":      "batch-circuit-breaker",
		"workTypes": []map[string]any{batchWorkTypeConfig("retry-task")},
		"workers":   []map[string]string{{"name": "retry-worker"}},
		"workstations": []map[string]any{batchWorkstationConfigWithLimits(
			"retry-work", "retry-worker", "retry-task", "complete", "init", map[string]any{"maxRetries": 1},
		)},
	}, map[string]string{
		"retry-worker": batchScriptWorkerConfig(),
	}, map[string]string{
		"retry-work": batchModelWorkstationConfig(),
	})
}

func writeBatchScriptFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	writeBatchFactoryFiles(t, workingDirectory, map[string]any{
		"name":      "batch-script-failure",
		"workTypes": []map[string]any{batchWorkTypeConfig("script-task")},
		"workers":   []map[string]string{{"name": "script-worker"}},
		"workstations": []map[string]any{
			batchWorkstationConfig("script-work", "script-worker", "script-task", "complete", "failed"),
		},
	}, map[string]string{
		"script-worker": batchScriptWorkerConfig(),
	}, map[string]string{
		"script-work": batchModelWorkstationConfig(),
	})
}

func writeBatchFactoryFiles(
	t testing.TB,
	workingDirectory string,
	factoryConfig map[string]any,
	workerConfigs map[string]string,
	workstationConfigs map[string]string,
) {
	t.Helper()
	factoryDirectory := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create batch Factory directory: %v", err)
	}
	factoryJSON, err := json.MarshalIndent(factoryConfig, "", "  ")
	if err != nil {
		t.Fatalf("marshal batch Factory fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDirectory, "factory.json"), factoryJSON, 0o600); err != nil {
		t.Fatalf("write batch Factory fixture: %v", err)
	}
	for workerName, config := range workerConfigs {
		path := filepath.Join(factoryDirectory, "workers", workerName, "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create batch worker directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
			t.Fatalf("write batch worker %q: %v", workerName, err)
		}
	}
	for workstationName, config := range workstationConfigs {
		path := filepath.Join(factoryDirectory, "workstations", workstationName, "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create batch workstation directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
			t.Fatalf("write batch workstation %q: %v", workstationName, err)
		}
	}
}

func batchWorkTypeConfig(name string) map[string]any {
	return map[string]any{
		"name": name,
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "complete", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}
}

func batchWorkstationConfig(name, worker, workType, output, failure string) map[string]any {
	return batchWorkstationConfigWithLimits(name, worker, workType, output, failure, nil)
}

func batchWorkstationConfigWithLimits(
	name, worker, workType, output, failure string,
	limits map[string]any,
) map[string]any {
	config := map[string]any{
		"name": name, "worker": worker,
		"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
		"outputs":   []map[string]string{{"workType": workType, "state": output}},
		"onFailure": []map[string]string{{"workType": workType, "state": failure}},
	}
	if limits != nil {
		config["limits"] = limits
	}
	return config
}

func batchModelWorkerConfig() string {
	return "---\ntype: MODEL_WORKSTATION\n---\nProcess the batch Work.\n"
}

func batchScriptWorkerConfig() string {
	return "---\ntype: SCRIPT_WORKER\ncommand: echo\nargs:\n  - batch-script\n---\nRun the batch script.\n"
}

func batchModelWorkstationConfig() string {
	return "---\ntype: MODEL_WORKSTATION\n---\nProcess the batch Work.\n"
}

func writeBatchFailingScript(t testing.TB) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch-script-failure.go")
	const source = `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprint(os.Stderr, "script worker exited non-zero")
	os.Exit(7)
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write failing batch script: %v", err)
	}
	return path
}
