package script_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const scriptNonZeroExitMessage = "script-non-zero-exit-output"

func newScriptSharedExecutionScenarios(t *testing.T) []scriptSharedScenario {
	t.Helper()

	nonZeroDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	const actionableDiagnostic = "repository root is dirty: 2 tracked changes, 1 untracked file; inspect and commit or back up changes"
	failureDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	cancellationDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	workstationAgentsPath := filepath.Join(cancellationDir, "workstations", "run-script", "AGENTS.md")
	agentsMD := "---\ntype: MODEL_WORKSTATION\n---\nExecute the script.\n"
	if err := os.WriteFile(workstationAgentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
	updateScriptFixtureFactory(t, cancellationDir, func(cfg map[string]any) {
		cfg["workstations"] = append(cfg["workstations"].([]any), map[string]any{
			"name":     "cancellation-loop-breaker",
			"behavior": "STANDARD",
			"type":     "LOGICAL_MOVE",
			"inputs":   []map[string]any{{"workType": "task", "state": "init"}},
			"outputs":  []map[string]any{{"workType": "task", "state": "failed"}},
			"guards": []map[string]any{{
				"type":        "VISIT_COUNT",
				"workstation": "run-script",
				"maxVisits":   float64(1),
			}},
		})
	})

	return []scriptSharedScenario{
		{
			name:                   "NonZeroExit",
			factoryDir:             nonZeroDir,
			workName:               "shared-script-non-zero",
			traceID:                "shared-script-non-zero-trace",
			workTypeName:           "task",
			terminalState:          "failed",
			expectedOutcome:        factoryapi.WorkOutcomeFailed,
			expectedCommand:        "echo",
			expectedArgs:           []string{"default-output"},
			expectedFailureMessage: scriptNonZeroExitMessage,
			runner: newScriptSharedCommandRunner(nonZeroExitCommandRunner{
				stderr:   scriptNonZeroExitMessage,
				exitCode: 1,
			}),
			assertResult: assertScriptSharedNonZeroExit,
		},
		{
			name:                   "FailureReachesWorkShow",
			factoryDir:             failureDir,
			workName:               "shared-script-failure-work-show",
			traceID:                "shared-script-failure-work-show-trace",
			workTypeName:           "task",
			terminalState:          "failed",
			expectedOutcome:        factoryapi.WorkOutcomeFailed,
			expectedCommand:        "echo",
			expectedArgs:           []string{"default-output"},
			expectedFailureMessage: actionableDiagnostic,
			runner: newScriptSharedCommandRunner(nonZeroExitCommandRunner{
				stderr:   actionableDiagnostic,
				exitCode: 1,
			}),
			assertResult: assertScriptSharedFailureReachesWorkShow,
		},
		{
			name:                    "Cancellation",
			factoryDir:              cancellationDir,
			workName:                "shared-script-cancellation",
			traceID:                 "shared-script-cancellation-trace",
			workTypeName:            "task",
			terminalState:           "failed",
			expectedOutcome:         factoryapi.WorkOutcomeCanceled,
			expectedCommand:         "echo",
			expectedArgs:            []string{"default-output"},
			cancelAfterCommandStart: true,
			runner:                  newScriptSharedCommandRunner(newBlockingCancellationCommandRunner()),
			assertResult:            assertScriptSharedCancellation,
		},
	}
}

func cancelScriptSharedSessionAfterCommandStart(
	t *testing.T,
	fixture *scriptSharedSpineFixture,
	scenario scriptSharedScenario,
	sessionID string,
) {
	t.Helper()
	runner, ok := scenario.runner.Delegate().(*blockingCancellationCommandRunner)
	if !ok {
		t.Fatalf("%s command delegate = %T, want blocking cancellation runner", scenario.name, scenario.runner.Delegate())
	}
	waitForScriptSharedSignal(t, runner.StartedSignal(), "script command admission")
	control := cancelScriptSharedSessionAt(t, fixture.baseURL, sessionID)
	if control.Operation != factoryapi.FactorySessionLifecycleControlKindCancel {
		t.Fatalf("%s cancellation operation = %q, want CANCEL", scenario.name, control.Operation)
	}
	if control.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("%s cancellation outcome = %q, want ACCEPTED", scenario.name, control.Outcome)
	}
	if control.SessionId != sessionID {
		t.Fatalf("%s cancellation session id = %q, want %q", scenario.name, control.SessionId, sessionID)
	}
	waitForScriptSharedSignal(t, runner.TerminatedSignal(), "script command cancellation and termination")
}

func waitForScriptSharedSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	// The signal is emitted by the controlled external-effect edge. The timer
	// only bounds a missing edge or test cancellation; it does not synchronize
	// the Factory workflow with a guessed duration.
	timer := time.NewTimer(scriptSharedSpineTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-t.Context().Done():
		t.Fatalf("waiting for %s ended with test context: %v", label, t.Context().Err())
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", label)
	}
}

func workByID(t *testing.T, listed factoryapi.ListWorkResponse, workID string) factoryapi.Work {
	t.Helper()
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == workID {
			return item
		}
	}
	t.Fatalf("Work %q missing from listing: %#v", workID, listed.Results)
	return factoryapi.Work{}
}

func assertWorkFailureDetail(t *testing.T, detail *factoryapi.FailureDetail, diagnostic string) {
	t.Helper()
	if detail == nil || detail.Reason != factoryapi.WorkFailureTypeInternalServerError || detail.Message != diagnostic {
		t.Fatalf("Work failure detail = %#v, want internal_server_error/%q", detail, diagnostic)
	}
}

func assertScriptSharedNonZeroExit(
	t *testing.T,
	_ *scriptSharedSpineFixture,
	scenario scriptSharedScenario,
	_ string,
	_ factoryapi.SubmitWorkResponse,
	_ factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	assertScriptNonZeroExitDispatchFailure(t, events, scenario.expectedFailureMessage)
}

func assertScriptSharedFailureReachesWorkShow(
	t *testing.T,
	fixture *scriptSharedSpineFixture,
	scenario scriptSharedScenario,
	sessionID string,
	submitted factoryapi.SubmitWorkResponse,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatal("shared failure Work id is empty")
	}
	assertWorkFailureDetail(t, workByID(t, listed, workID).FailureDetail, scenario.expectedFailureMessage)
	got := getScriptSharedWorkByID(t, fixture.baseURL, sessionID, workID)
	assertWorkFailureDetail(t, got.FailureDetail, scenario.expectedFailureMessage)

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || dispatches[0].Response == nil {
		t.Fatalf("shared failure dispatch observations = %#v, want one failed response", dispatches)
	}
	assertWorkFailureDetail(t, dispatches[0].Response.FailureDetail, scenario.expectedFailureMessage)

	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("shared failure session id is empty")
	}
	human, stderr, err := executeScriptSharedWorkShow(t, fixture, sessionID, workID, false)
	if err != nil {
		t.Fatalf("shared root work show human error = %v\nstdout=%s\nstderr=%s", err, human, stderr)
	}
	if !strings.Contains(human, "Failure reason:\tinternal_server_error") ||
		!strings.Contains(human, "Failure message:\t"+scenario.expectedFailureMessage) {
		t.Fatalf("shared root work show human output = %q, want typed actionable failure", human)
	}
	assertScriptSharedNoUndeclaredValue(t, scenario.name+" human work show", human, stderr)

	jsonOutput, stderr, err := executeScriptSharedWorkShow(t, fixture, sessionID, workID, true)
	if err != nil {
		t.Fatalf("shared root work show JSON error = %v\nstdout=%s\nstderr=%s", err, jsonOutput, stderr)
	}
	var jsonWork factoryapi.Work
	if err := json.Unmarshal([]byte(jsonOutput), &jsonWork); err != nil {
		t.Fatalf("decode shared root work show JSON: %v\nstdout=%s", err, jsonOutput)
	}
	assertWorkFailureDetail(t, jsonWork.FailureDetail, scenario.expectedFailureMessage)
	assertScriptSharedNoUndeclaredValue(t, scenario.name+" JSON work show", jsonOutput, stderr)
}

func assertScriptSharedCancellation(
	t *testing.T,
	_ *scriptSharedSpineFixture,
	scenario scriptSharedScenario,
	_ string,
	_ factoryapi.SubmitWorkResponse,
	_ factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	runner, ok := scenario.runner.Delegate().(*blockingCancellationCommandRunner)
	if !ok {
		t.Fatalf("%s command delegate = %T, want blocking cancellation runner", scenario.name, scenario.runner.Delegate())
	}
	if runner.CallCount() != 1 || !runner.Started() || !runner.ContextCanceled() || !runner.Terminated() {
		t.Fatalf(
			"%s cancellation edge state = calls:%d started:%t canceled:%t terminated:%t, want 1/true/true/true",
			scenario.name, runner.CallCount(), runner.Started(), runner.ContextCanceled(), runner.Terminated(),
		)
	}
}

type blockingCancellationCommandRunner struct {
	mu               sync.Mutex
	calls            int
	started          bool
	contextCanceled  bool
	terminated       bool
	startedSignal    chan struct{}
	terminatedSignal chan struct{}
	startOnce        sync.Once
	terminationOnce  sync.Once
}

func newBlockingCancellationCommandRunner() *blockingCancellationCommandRunner {
	return &blockingCancellationCommandRunner{
		startedSignal:    make(chan struct{}),
		terminatedSignal: make(chan struct{}),
	}
}

func (r *blockingCancellationCommandRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.started = true
	r.startOnce.Do(func() { close(r.startedSignal) })
	r.mu.Unlock()

	<-ctx.Done()

	r.mu.Lock()
	r.contextCanceled = true
	r.terminated = true
	r.terminationOnce.Do(func() { close(r.terminatedSignal) })
	r.mu.Unlock()

	return platformprocess.CommandResult{}, ctx.Err()
}

func (r *blockingCancellationCommandRunner) StartedSignal() <-chan struct{} {
	return r.startedSignal
}

func (r *blockingCancellationCommandRunner) TerminatedSignal() <-chan struct{} {
	return r.terminatedSignal
}

func (r *blockingCancellationCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *blockingCancellationCommandRunner) Started() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started
}

func (r *blockingCancellationCommandRunner) ContextCanceled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.contextCanceled
}

func (r *blockingCancellationCommandRunner) Terminated() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.terminated
}

type nonZeroExitCommandRunner struct {
	stderr   string
	exitCode int
}

func (r nonZeroExitCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{
		Stderr:   []byte(r.stderr),
		ExitCode: r.exitCode,
	}, nil
}

func assertScriptNonZeroExitDispatchFailure(t *testing.T, events []factoryapi.FactoryEvent, wantMessage string) {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) == 0 {
		t.Fatal("factory events missing dispatch observations")
	}
	response := dispatches[len(dispatches)-1].Response
	if response == nil {
		t.Fatal("dispatch response missing for failed script execution")
	}
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want FAILED", response.Outcome)
	}
	if response.Output != nil {
		t.Fatalf("dispatch output = %#v, want no primary result on script failure", response.Output)
	}
	assertDispatchErrorContains(t, events, wantMessage)
}
