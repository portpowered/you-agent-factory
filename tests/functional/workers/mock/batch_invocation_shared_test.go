package mock

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The migrated rows use the same root-built Process as the shared host. The
// host is stopped once before this local lane begins; each invocation then has
// its own Current Factory, HOME, input file, runtime-log directory, streams,
// and mock-worker configuration.
func testSharedBatchDefaultSuccess(t *testing.T, fixture *sharedWorkersMockFixture) {
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchCurrentFactory,
		[]batchWorkSpec{{Name: "single default batch Work", WorkTypeID: stdinRunWorkTypeName}},
		writeBatchMockWorkers(t, workers.MockWorkerRunTypeAccept), nil,
	)
	if err != nil || !strings.Contains(inputs.Stdout(), "Batch completed successfully.") {
		t.Fatalf("shared default batch success: %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
}

func testSharedBatchVerboseSuccess(t *testing.T, fixture *sharedWorkersMockFixture) {
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchCurrentFactory,
		[]batchWorkSpec{{Name: "single verbose batch Work", WorkTypeID: stdinRunWorkTypeName}},
		writeBatchMockWorkers(t, workers.MockWorkerRunTypeAccept), nil, "--verbose",
	)
	if err != nil || !strings.Contains(inputs.Stdout(), "Batch completed successfully.") {
		t.Fatalf("shared verbose batch success: %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
}

func testSharedBatchAllFailedHuman(t *testing.T, fixture *sharedWorkersMockFixture) {
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchCurrentFactory,
		[]batchWorkSpec{
			{Name: "all failed first Work", WorkTypeID: stdinRunWorkTypeName},
			{Name: "all failed second Work", WorkTypeID: stdinRunWorkTypeName},
		},
		writeBatchMockWorkers(t, workers.MockWorkerRunTypeReject), nil, "--quiet",
	)
	if err == nil {
		t.Fatalf("shared all-failed batch succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	for _, want := range []string{`Work "all failed first Work"`, `Work "all failed second Work"`, "prompt-task:failed"} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("shared all-failed stdout missing %q:\n%s", want, inputs.Stdout())
		}
	}

	// Reuse the same root after the failure with fresh invocation-owned state.
	recovery, recoveryErr := executeSharedBatch(
		t, fixture, writeBatchCurrentFactory,
		[]batchWorkSpec{{Name: "post-failure recovery Work", WorkTypeID: stdinRunWorkTypeName}},
		writeBatchMockWorkers(t, workers.MockWorkerRunTypeAccept), nil, "--quiet",
	)
	if recoveryErr != nil || recovery.Stdout() != "Batch completed successfully.\n" || recovery.Stderr() != "" {
		t.Fatalf("shared post-failure recovery: %v; stdout=%q stderr=%q", recoveryErr, recovery.Stdout(), recovery.Stderr())
	}
}

func testSharedBatchMixedJSON(t *testing.T, fixture *sharedWorkersMockFixture) {
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchMixedFactory,
		[]batchWorkSpec{
			{Name: "mixed successful Work", WorkTypeID: "successful-task"},
			{Name: "mixed failed Work", WorkTypeID: "failed-task"},
		},
		writeBatchMixedMockWorkers(t), []string{"--json"},
	)
	if err == nil {
		t.Fatalf("shared mixed batch succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	report := decodeBatchProcessReport(t, inputs.Stdout())
	if report.Status != "FAILED" || len(report.Failures) != 1 {
		t.Fatalf("shared mixed JSON report = %#v, want one failed Work", report)
	}
	failure := report.Failures[0]
	if failure.WorkName != "mixed failed Work" || failure.WorkState != "failed-task:failed" {
		t.Fatalf("shared mixed failure = %#v, want only the failed Work", failure)
	}
}

func testSharedBatchCircuitBreakerHuman(t *testing.T, fixture *sharedWorkersMockFixture) {
	const breakerReason = "consecutive failures 1 for transition retry-work exceeds max 1"
	runner := testutil.NewProviderCommandRunner(sharedScriptFailureResult(), sharedScriptFailureResult())
	fixture.useCommandRunners(nil, runner)
	defer fixture.useCommandRunners(nil, nil)
	mockWorkersPath := writeBatchScriptMockWorkers(t, "retry-worker", "retry-work", "mock-circuit-breaker")
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchRetryFactory,
		[]batchWorkSpec{{Name: "circuit breaker Work", WorkTypeID: "retry-task"}},
		mockWorkersPath, nil, "--quiet",
	)
	if err == nil {
		t.Fatalf("shared circuit-breaker batch succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	for _, want := range []string{`Work "circuit breaker Work"`, "retry-task:failed", breakerReason} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("shared circuit-breaker stdout missing %q:\n%s", want, inputs.Stdout())
		}
	}
	if runner.CallCount() != 1 {
		t.Fatalf("shared circuit-breaker script calls = %d, want one attempt before the breaker", runner.CallCount())
	}
}

func testSharedBatchCircuitBreakerJSON(t *testing.T, fixture *sharedWorkersMockFixture) {
	const breakerReason = "consecutive failures 1 for transition retry-work exceeds max 1"
	runner := testutil.NewProviderCommandRunner(sharedScriptFailureResult(), sharedScriptFailureResult())
	fixture.useCommandRunners(nil, runner)
	defer fixture.useCommandRunners(nil, nil)
	mockWorkersPath := writeBatchScriptMockWorkers(t, "retry-worker", "retry-work", "mock-circuit-breaker")
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchRetryFactory,
		[]batchWorkSpec{{Name: "circuit breaker Work", WorkTypeID: "retry-task"}},
		mockWorkersPath, []string{"--json"},
	)
	if err == nil {
		t.Fatalf("shared circuit-breaker JSON batch succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	report := decodeBatchProcessReport(t, inputs.Stdout())
	if report.Status != "FAILED" || len(report.Failures) != 1 {
		t.Fatalf("shared circuit-breaker JSON report = %#v, want one failure", report)
	}
	failure := report.Failures[0]
	if failure.WorkName != "circuit breaker Work" || failure.WorkState != "retry-task:failed" || !strings.Contains(failure.Reason, breakerReason) {
		t.Fatalf("shared circuit-breaker JSON failure = %#v, want Work, state, and breaker reason", failure)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("shared circuit-breaker JSON script calls = %d, want one attempt before the breaker", runner.CallCount())
	}
}

func testSharedBatchScriptFailureHuman(t *testing.T, fixture *sharedWorkersMockFixture) {
	runner := testutil.NewProviderCommandRunner(sharedScriptFailureResult())
	fixture.useCommandRunners(nil, runner)
	defer fixture.useCommandRunners(nil, nil)
	mockWorkersPath := writeBatchScriptMockWorkers(t, "script-worker", "script-work", "mock-script-failure")
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchScriptFactory,
		[]batchWorkSpec{{Name: "script failure Work", WorkTypeID: "script-task"}},
		mockWorkersPath, nil, "--quiet",
	)
	if err == nil {
		t.Fatalf("shared script-failure batch succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	for _, want := range []string{`Work "script failure Work"`, "script-task:failed", "script worker exited non-zero"} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("shared script-failure stdout missing %q:\n%s", want, inputs.Stdout())
		}
	}
	if runner.CallCount() != 1 {
		t.Fatalf("shared script-failure script calls = %d, want one", runner.CallCount())
	}
}

func testSharedBatchScriptFailureJSON(t *testing.T, fixture *sharedWorkersMockFixture) {
	runner := testutil.NewProviderCommandRunner(sharedScriptFailureResult())
	fixture.useCommandRunners(nil, runner)
	defer fixture.useCommandRunners(nil, nil)
	mockWorkersPath := writeBatchScriptMockWorkers(t, "script-worker", "script-work", "mock-script-failure")
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchScriptFactory,
		[]batchWorkSpec{{Name: "script failure Work", WorkTypeID: "script-task"}},
		mockWorkersPath, []string{"--json"},
	)
	if err == nil {
		t.Fatalf("shared script-failure JSON batch succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	report := decodeBatchProcessReport(t, inputs.Stdout())
	if report.Status != "FAILED" || len(report.Failures) != 1 {
		t.Fatalf("shared script-failure JSON report = %#v, want one failure", report)
	}
	failure := report.Failures[0]
	if failure.WorkName != "script failure Work" || failure.WorkState != "script-task:failed" || !strings.Contains(failure.Reason, "script worker exited non-zero") {
		t.Fatalf("shared script-failure JSON failure = %#v, want Work, state, and script reason", failure)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("shared script-failure JSON script calls = %d, want one", runner.CallCount())
	}
}

func testSharedNamedHumanFailure(t *testing.T, fixture *sharedWorkersMockFixture) {
	fixture.prepareLocalActivation(t)
	workingDirectory := t.TempDir()
	mockWorkersPath := writeRejectingGoalMockWorkers(t)
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--runtime-log-dir", filepath.Join(workingDirectory, "runtime-logs"),
		"run",
		"--named", characterizedNamedFactory,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--output", "response-stream",
		"shared named human failure",
	})
	inputs.Input.WorkingDirectory = workingDirectory
	inputs.Input.Env = sharedWorkersMockEnvironment(t, writeSharedWorkersMockOperatorHome(t))
	if err := fixture.executeLocal(t, inputs.Input); err == nil {
		t.Fatalf("shared named human failure succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	for _, want := range []string{"status: FAILED", "workName: ", "workState: goal:failed"} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("shared named human failure stdout missing %q:\n%s", want, inputs.Stdout())
		}
	}
	if !sharedHasNonEmptyLabeledValue(inputs.Stdout(), "workName: ") {
		t.Fatalf("shared named human failure has an empty Work name:\n%s", inputs.Stdout())
	}
}

func executeSharedBatch(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
	writeFactory func(testing.TB, string),
	specs []batchWorkSpec,
	mockWorkersPath string,
	globalArgs []string,
	runArgs ...string,
) (*support.CapturedInputs, error) {
	t.Helper()
	fixture.prepareLocalActivation(t)
	workingDirectory := t.TempDir()
	writeFactory(t, workingDirectory)
	workFile := writeBatchWorksWithTypes(t, specs...)
	args := append([]string{"you"}, globalArgs...)
	args = append(args,
		"--runtime-log-dir", filepath.Join(workingDirectory, "runtime-logs"),
		"run",
		"--dir", filepath.Join(workingDirectory, "factory"),
		"--work", workFile,
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
	)
	args = append(args, runArgs...)
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = workingDirectory
	inputs.Input.Env = sharedWorkersMockEnvironment(t, writeSharedWorkersMockOperatorHome(t))
	return inputs, fixture.executeLocal(t, inputs.Input)
}

func writeBatchScriptMockWorkers(t testing.TB, workerName, workstationName, command string) string {
	t.Helper()
	return writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: workerName, WorkstationName: workstationName, RunType: workers.MockWorkerRunTypeScript,
		ScriptConfig: &workers.MockWorkerScriptConfig{Command: command},
	}}})
}

func sharedScriptFailureResult() platformprocess.CommandResult {
	return platformprocess.CommandResult{
		ExitCode: 7,
		Stderr:   []byte("script worker exited non-zero"),
	}
}

func sharedHasNonEmptyLabeledValue(output, label string) bool {
	for _, line := range strings.Split(output, "\n") {
		if value, ok := strings.CutPrefix(line, label); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}
