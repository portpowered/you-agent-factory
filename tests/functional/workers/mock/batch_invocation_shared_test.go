package mock

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The migrated rows use the same root-built Process as the shared host. The
// host is stopped once before this local lane begins; each invocation then has
// its own Current Factory, HOME, input file, runtime-log directory, streams,
// and mock-worker configuration.
func testSharedBatchAllFailedHuman(t *testing.T, fixture *sharedWorkersMockFixture) {
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchCurrentFactory,
		[]batchWorkSpec{
			{Name: "all failed first Work", WorkTypeID: stdinRunWorkTypeName},
			{Name: "all failed second Work", WorkTypeID: stdinRunWorkTypeName},
		},
		writeBatchMockWorkers(t, workers.MockWorkerRunTypeReject), nil, nil, "--quiet",
	)
	if err == nil {
		t.Fatalf("shared all-failed batch succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	for _, want := range []string{`Work "all failed first Work"`, `Work "all failed second Work"`, "prompt-task:failed"} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("shared all-failed stdout missing %q (error=%v, stderr=%q):\n%s", want, err, inputs.Stderr(), inputs.Stdout())
		}
	}

	// Reuse the same root after the failure with fresh invocation-owned state.
	recovery, recoveryErr := executeSharedBatch(
		t, fixture, writeBatchCurrentFactory,
		[]batchWorkSpec{{Name: "post-failure recovery Work", WorkTypeID: stdinRunWorkTypeName}},
		writeBatchMockWorkers(t, workers.MockWorkerRunTypeAccept), nil, nil, "--quiet",
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
		writeBatchMixedMockWorkers(t), []string{"--json"}, nil,
	)
	if err == nil {
		t.Fatalf("shared mixed batch succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	if strings.TrimSpace(inputs.Stdout()) == "" {
		t.Fatalf("shared mixed batch produced no report: error=%v stderr=%q", err, inputs.Stderr())
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
	mockWorkersPath := writeBatchScriptMockWorkers(t, "retry-worker", "retry-work", "mock-circuit-breaker")
	inputs, err := executeSharedBatch(
		t, fixture, writeBatchRetryFactory,
		[]batchWorkSpec{{Name: "circuit breaker Work", WorkTypeID: "retry-task"}},
		mockWorkersPath, nil, runner, "--quiet",
	)
	if err == nil {
		t.Fatalf("shared circuit-breaker batch succeeded: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
	for _, want := range []string{`Work "circuit breaker Work"`, "retry-task:failed", breakerReason} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("shared circuit-breaker stdout missing %q (error=%v, stderr=%q):\n%s", want, err, inputs.Stderr(), inputs.Stdout())
		}
	}
	if runner.CallCount() != 1 {
		t.Fatalf("shared circuit-breaker script calls = %d, want one attempt before the breaker", runner.CallCount())
	}
}

func executeSharedBatch(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
	writeFactory func(testing.TB, string),
	specs []batchWorkSpec,
	mockWorkersPath string,
	globalArgs []string,
	scriptRunner platformprocess.CommandRunner,
	runArgs ...string,
) (*support.CapturedInputs, error) {
	t.Helper()
	workingDirectory := t.TempDir()
	writeFactory(t, workingDirectory)
	if scriptRunner != nil {
		fixture.useCommandRunnersFor(t, workingDirectory, nil, scriptRunner)
	}
	workFile := writeBatchWorksWithTypes(t, specs...)
	args := append([]string{"you"}, globalArgs...)
	args = append(args,
		"run",
		"--runtime-log-dir", filepath.Join(workingDirectory, "runtime-logs"),
		"--session="+uuid.NewString(),
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
