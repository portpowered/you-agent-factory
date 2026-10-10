package concurrency_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func configureConcurrencyScript(t *testing.T, directory string) {
	t.Helper()
	support.WriteAgentConfig(t, directory, "worker-a", "---\ntype: SCRIPT_WORKER\ncommand: controlled-script\nargs:\n  - '{{ (index .Inputs 0).Payload }}'\n---\n")
	support.WriteWorkstationConfig(t, directory, "process", "---\ntype: SCRIPT_RUN\n---\nRun the selected script.\n")
}

// Work and canonical event readback are selected-session API contracts. The
// shared host enters through Process.Execute, with script effects replaced at
// ScriptCommandRunner and no executable launched by the fixture.
func (fixture *concurrencySharedProcessFixture) runScriptCommandOutcomes(t *testing.T) {
	t.Helper()
	failed := fixture.openCase(t, "B12-script-failure", 1, concurrencyRunnerFailureHold, "cc14-fail", "cc14-fail", 0, true)
	peer := fixture.openCase(t, "B12-script-success", 1, concurrencyRunnerHold, "cc14-peer", "", 0, true)
	ownAdmittedWorkSession(t, failed, false)
	ownAdmittedWorkSession(t, peer, false)
	failure := submitConcurrencyWork(t, failed, failed.marker)
	success := submitConcurrencyWork(t, peer, peer.marker)
	badCall := failed.runner.waitStarted(t, concurrencySharedProcessTimeout)
	goodCall := peer.runner.waitStarted(t, concurrencySharedProcessTimeout)
	if badCall.request.Command != "controlled-script" || !commandRequestContains(badCall.request, failed.marker) ||
		goodCall.request.Command != "controlled-script" || !commandRequestContains(goodCall.request, peer.marker) {
		t.Fatal("script command did not receive its selected expanded input")
	}
	failed.runner.releaseCall(badCall.index)
	waitConcurrencyWorkSettled(t, fixture.baseURL, failed.id, 1)
	assertConcurrencyWorkFailed(t, failed, failure.WorkId, factoryapi.WorkFailureTypeInternalServerError, "selected script rejected input")
	if workContentText(t, concurrencyWorkByID(t, failed, failure.WorkId)) != "" {
		t.Fatal("failed script published successful Work content")
	}
	assertScriptResponse(t, failed, factoryapi.ScriptExecutionOutcomeFailedExitCode, "")
	if peer.runner.activeCallCount() != 1 || peer.runner.canceledCount() != 0 {
		t.Fatal("script failure changed held peer")
	}
	peer.runner.releaseCall(goodCall.index)
	waitConcurrencyWorkSettled(t, fixture.baseURL, peer.id, 1)
	assertConcurrencyWorkCompleted(t, peer, success.WorkId, peer.marker)
	assertScriptResponse(t, peer, factoryapi.ScriptExecutionOutcomeSucceeded, peer.marker+" output COMPLETE")
	assertConcurrencyCounts(t, failed, 1, 1)
	assertConcurrencyCounts(t, peer, 1, 1)
	assertScriptEventIsolation(t, failed, peer.marker)
	assertScriptEventIsolation(t, peer, failed.marker)
	// A later admission proves the healthy owner still executes after its
	// peer's failure, rather than merely retaining its first accepted result.
	later := submitConcurrencyWork(t, peer, "cc14-later")
	call := peer.runner.waitStarted(t, concurrencySharedProcessTimeout)
	peer.runner.releaseCall(call.index)
	waitConcurrencyWorkSettled(t, fixture.baseURL, peer.id, 2)
	assertConcurrencyWorkCompleted(t, peer, later.WorkId, "cc14-later")
}

func assertScriptEventIsolation(t *testing.T, session *concurrencySession, foreign string) {
	t.Helper()
	for _, event := range concurrencySessionEvents(t, session.fixture.baseURL, session.id) {
		if event.Context.SessionId != nil && *event.Context.SessionId != session.id {
			t.Fatal("event attributed to a foreign session")
		}
		body, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), foreign) {
			t.Fatal("event contains a foreign script result")
		}
	}
}

func (fixture *concurrencySharedProcessFixture) runScriptCommandCancellation(t *testing.T) {
	t.Helper()
	selected := fixture.openCase(t, "B12-script-cancel", 1, concurrencyRunnerHold, "cc15-cancel", "", 0, true)
	peer := fixture.openCase(t, "B12-script-survivor", 1, concurrencyRunnerHold, "cc15-peer", "", 0, true)
	responses := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(fixture.baseURL, selected.id))
	ownAdmittedWorkSession(t, selected, false)
	ownAdmittedWorkSession(t, peer, false)
	work := submitConcurrencyWork(t, selected, selected.marker)
	peerWork := submitConcurrencyWork(t, peer, peer.marker)
	selected.runner.waitStarted(t, concurrencySharedProcessTimeout)
	peerCall := peer.runner.waitStarted(t, concurrencySharedProcessTimeout)
	dispatch := admittedWorkDispatch(t, selected, work)
	cancelAdmittedWorkSession(t, selected)
	selected.runner.waitCanceled(t, concurrencySharedProcessTimeout)
	selected.runner.joinCalls(t)
	awaitAdmittedWorkTerminalPublication(t, responses)
	assertAdmittedWorkCanceled(t, selected, work, dispatch)
	assertScriptResponse(t, selected, factoryapi.ScriptExecutionOutcome("CANCELED"), "")
	if peer.runner.activeCallCount() != 1 || peer.runner.canceledCount() != 0 {
		t.Fatal("script cancellation reached peer")
	}
	peer.runner.releaseCall(peerCall.index)
	waitConcurrencyWorkSettled(t, fixture.baseURL, peer.id, 1)
	assertConcurrencyWorkCompleted(t, peer, peerWork.WorkId, peer.marker)
	responses.Close()
	responses.WaitClosed(concurrencySharedProcessTimeout)
}

func assertScriptResponse(t *testing.T, session *concurrencySession, outcome factoryapi.ScriptExecutionOutcome, stdout string) {
	t.Helper()
	responses := 0
	for _, event := range concurrencySessionEvents(t, session.fixture.baseURL, session.id) {
		if event.Type != factoryapi.FactoryEventTypeScriptResponse {
			continue
		}
		payload, err := event.Payload.AsScriptResponseEventPayload()
		if err != nil || payload.Outcome != outcome || payload.Stdout != stdout {
			t.Fatalf("script response = %#v, %v", payload, err)
		}
		responses++
	}
	if responses != 1 {
		t.Fatalf("selected script responses = %d, want one", responses)
	}
}
