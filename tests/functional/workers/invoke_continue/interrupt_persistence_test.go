package acceptance

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The public request crosses the real host, input artifacts and journal. Only
// the selected persistence acknowledgement is unavailable; ordinary stopping
// still uses the same production store and exact owned provider execution.
func TestInterruptPersistenceFailureLeavesSourceControllable(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"interrupt-input-write-failure", "interrupt-input-read-failure", "interrupt-input-corrupt", "interrupt-intent-failure", "interrupt-ack-intent-disputed", "interrupt-ack-input-unsynced", "interrupt-ack-input-missing-ref"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			scenario := newS8InterruptScenario(t, ctx, name)
			scenario.ids.interruptRequest = name + "-" + scenario.ids.interruptRequest
			t.Cleanup(scenario.runner.releaseAll)
			ids := scenario.ids
			invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
				requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
				factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
			})
			scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
			status, body, first := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
			if status != http.StatusInternalServerError || string(first.Code) != "INTERNAL_ERROR" || string(first.Phase) != "VALIDATION" || first.Message != "Worker Session interrupt persistence unavailable" || strings.Contains(body, "private-") {
				t.Fatalf("persistence refusal status=%d response=%#v", status, first)
			}
			code, phase, err := executeS8InterruptCLIError(ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
			if err != nil || code != string(first.Code) || phase != string(first.Phase) {
				t.Fatalf("remote CLI refusal code=%s phase=%s err=%v", code, phase, err)
			}
			repeatedStatus, _, repeated := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
			if repeatedStatus != status || !reflect.DeepEqual(first, repeated) || scenario.runner.CallCount() != 1 || scenario.runner.cancellationCount(s8InterruptCallAInitial) != 0 {
				t.Fatal("persistence refusal changed on retry or stopped/admitted an execution")
			}
			assertS8PersistenceRefusalKeepsSource(t, ctx, scenario)
			terminateS8AfterPersistenceRefusal(t, ctx, scenario)
			assertS8WorkNotAdvanced(t, scenario.fixture, scenario.session.id, ids.workA)
			scenario.close(t)
		})
	}
}

func assertS8PersistenceRefusalKeepsSource(t *testing.T, ctx context.Context, scenario s8InterruptScenario) {
	t.Helper()
	listed := listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	if source := findS8Observation(t, listed, scenario.ids.workerA); source.State != "RUNNING" {
		t.Fatalf("refused interruption source=%#v", source)
	}
	for _, observation := range listed {
		if observation.WorkerSessionID == scenario.ids.successor {
			t.Fatal("refused interruption exposed a successor")
		}
	}
}

func terminateS8AfterPersistenceRefusal(t *testing.T, ctx context.Context, scenario s8InterruptScenario) {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "terminate", scenario.ids.workerA})
	inputs.Input.Env = append([]string(nil), scenario.env...)
	inputs.Input.WorkingDirectory = scenario.repositoryA.path
	if err := scenario.manager.Execute(inputs.Input); err != nil {
		t.Fatalf("exact terminate after persistence refusal: %v stderr=%s", err, inputs.Stderr())
	}
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	listed := listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	if source := findS8Observation(t, listed, scenario.ids.workerA); source.State != "TERMINATED" || scenario.runner.CallCount() != 1 || scenario.runner.cancellationCount(s8InterruptCallAInitial) != 1 {
		t.Fatalf("exact termination source=%#v provider calls=%d", source, scenario.runner.CallCount())
	}
}

// A conflict is a definitive refusal even when a controlled store exposes a
// matching synced row. The real host must join the source and stop before
// successor admission; CLI and HTTP retries retain the same partial failure.
func TestInterruptPhaseConflictLeavesJoinedSourceWithoutSuccessor(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "interrupt-phase-conflict")
	scenario.ids.interruptRequest = "interrupt-phase-conflict-" + scenario.ids.interruptRequest
	t.Cleanup(scenario.runner.releaseAll)
	ids := scenario.ids
	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
	status, body, first := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
	if status != http.StatusInternalServerError || string(first.Code) != "INTERNAL_ERROR" || string(first.Phase) != "SUCCESSOR_ADMISSION" || first.Source == nil || string(first.Source.State) != "CANCELED" || first.Successor != nil || strings.Contains(body, "private-") {
		t.Fatalf("phase conflict status=%d response=%#v", status, first)
	}
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	code, phase, err := executeS8InterruptCLIError(ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
	if err != nil || code != string(first.Code) || phase != string(first.Phase) {
		t.Fatalf("CLI conflict replay code=%s phase=%s err=%v", code, phase, err)
	}
	repeatedStatus, _, repeated := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
	if repeatedStatus != status || !reflect.DeepEqual(first, repeated) || scenario.runner.CallCount() != 1 || scenario.runner.cancellationCount(s8InterruptCallAInitial) != 1 {
		t.Fatal("phase conflict changed outcome or admitted a successor")
	}
	assertS8WorkNotAdvanced(t, scenario.fixture, scenario.session.id, ids.workA)
	scenario.close(t)
}
