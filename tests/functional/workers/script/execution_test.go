package script_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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
	agentsMD := "---\ntype: MODEL_WORKSTATION\nlimits:\n  maxExecutionTime: 10ms\n---\nExecute the script.\n"
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
			commandKind:            scriptSharedScriptCommand,
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
			commandKind:            scriptSharedScriptCommand,
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
			expectedOutcome:         factoryapi.WorkOutcomeFailed,
			commandKind:             scriptSharedScriptCommand,
			expectedCommand:         "echo",
			expectedArgs:            []string{"default-output"},
			allowMultipleDispatches: true,
			runner:                  newScriptSharedCommandRunner(&blockingCancellationCommandRunner{}),
			assertResult:            assertScriptSharedCancellation,
		},
	}
}

func failedScriptWorkID(t *testing.T, listed factoryapi.ListWorkResponse) string {
	t.Helper()
	for _, item := range listed.Results {
		if item.State == nil || item.State.Name != "failed" {
			continue
		}
		if id := support.StringPointerValue(item.WorkId); id != "" {
			return id
		}
	}
	t.Fatalf("failed script Work missing from listing: %#v", listed.Results)
	return ""
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
	assertScriptCancellationDispatchFailure(t, events)
}

func detachedScriptExecuteRequest() workerexecution.ExecuteRequest {
	return workerexecution.ExecuteRequest{
		Correlation: workerexecution.ExecutionCorrelation{
			FactorySessionID: "detached-session",
			RuntimeID:        "detached-runtime",
			GenerationID:     "detached-generation",
			DispatchID:       "detached-dispatch",
			AttemptID:        "detached-attempt",
			RequestID:        "detached-request",
			TraceID:          "detached-trace",
		},
		Target: workerexecution.ExecutionTarget{
			WorkerName:      "detached-script-worker",
			WorkerType:      "SCRIPT_WORKER",
			WorkstationName: "detached-workstation",
			RunnerID:        "script",
			Command:         "echo",
			Args:            []string{"detached-output"},
			Environment: workerexecution.EnvironmentPolicy{
				Vars: map[string]string{"DETACHED_MODE": "functional"},
			},
		},
		Input: workerexecution.ExecutionInput{
			Dispatch: work.WorkDispatch{
				DispatchID:  "detached-dispatch",
				WorkerType:  "SCRIPT_WORKER",
				ProjectID:   "detached-project",
				InputTokens: []any{"detached-token"},
				Execution: work.ExecutionMetadata{
					RequestID: "detached-request",
					TraceID:   "detached-trace",
				},
			},
		},
		Attempt: workerexecution.AttemptContext{Number: 1},
	}
}

func executeOutputText(output workerexecution.ProposedOutput) string {
	var builder strings.Builder
	for _, part := range output.Primary {
		builder.WriteString(part.Text)
	}
	return builder.String()
}

type blockingCancellationCommandRunner struct {
	mu              sync.Mutex
	calls           int
	started         bool
	contextCanceled bool
	terminated      bool
}

func (r *blockingCancellationCommandRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.started = true
	r.mu.Unlock()

	<-ctx.Done()

	r.mu.Lock()
	r.contextCanceled = true
	r.terminated = true
	r.mu.Unlock()

	return platformprocess.CommandResult{}, ctx.Err()
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

func assertScriptCancellationDispatchFailure(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) == 0 {
		t.Fatal("factory events missing dispatch observations")
	}

	for _, want := range []string{
		"execution cancelled: context canceled",
		"execution timeout",
	} {
		for _, dispatch := range dispatches {
			response := dispatch.Response
			if response == nil || response.Outcome != factoryapi.WorkOutcomeFailed {
				continue
			}
			if response.Output != nil {
				continue
			}
			if response.Error != nil && strings.Contains(*response.Error, want) {
				return
			}
		}
	}

	t.Fatalf(
		"factory events missing failed cancellation dispatch: %#v",
		dispatches,
	)
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
