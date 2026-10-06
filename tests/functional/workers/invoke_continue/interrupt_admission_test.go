package acceptance

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// F7-03 crosses real source join and successor opening. The controlled store
// edge rejects only the reserved successor's opening, before provider handoff.
func TestInterruptAdmissionFailureReplaysStoppedSourceWithoutAnotherSuccessor(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "interrupt-admission-failure")
	scenario.ids.successor = "interrupt-admission-failure-" + scenario.ids.successor
	t.Cleanup(scenario.runner.releaseAll)
	ids := scenario.ids
	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
	status, body, first := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
	assertS8StoppedAdmissionFailure(t, status, body, first)
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	code, phase, err := executeS8InterruptCLIError(ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
	if err != nil || code != string(first.Code) || phase != string(first.Phase) {
		t.Fatalf("remote CLI replay code=%s phase=%s err=%v", code, phase, err)
	}
	replayedStatus, _, replayed := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
	if replayedStatus != status || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("admission failure changed on retry: first=%#v replay=%#v", first, replayed)
	}
	conflictStatus, _, conflict := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor+"-other", s8ReplacementMessage)
	if conflictStatus != http.StatusConflict || string(conflict.Phase) != "VALIDATION" || string(conflict.Code) != "WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT" {
		t.Fatalf("changed successor was not refused: status=%d response=%#v", conflictStatus, conflict)
	}
	listed := listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	if source := findS8Observation(t, listed, ids.workerA); source.State != "CANCELED" || scenario.runner.CallCount() != 1 || scenario.runner.cancellationCount(s8InterruptCallAInitial) != 1 {
		t.Fatalf("partial admission repeated effects: source=%#v calls=%d", source, scenario.runner.CallCount())
	}
	assertS8WorkNotAdvanced(t, scenario.fixture, scenario.session.id, ids.workA)
	scenario.close(t)
}

func assertS8StoppedAdmissionFailure(t *testing.T, status int, body string, response factoryapi.WorkerSessionInterruptError) {
	t.Helper()
	if status != http.StatusServiceUnavailable || string(response.Code) != "WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED" || string(response.Phase) != "SUCCESSOR_ADMISSION" || response.Source == nil || string(response.Source.State) != "CANCELED" || strings.Contains(body, "private-") {
		t.Fatalf("admission failure status=%d response=%#v body=%s", status, response, body)
	}
}
