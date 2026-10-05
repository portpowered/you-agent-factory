package http_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Public CLI cancel and HTTP retries use the same production host, real journal
// and two independent no-reference executions. The controlled runner delays its
// callback after cancellation, proving APPLIED joins rather than merely signals.
func TestExactStopDirectCancelJoinsWithoutAProviderReference(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	sessionID := uuid.NewString()
	server := startExactFactoryStopServer(t, runner, sessionID)
	for index, id := range []string{"exact-target", "exact-sibling"} {
		response := postDirectWorkerSession(t, t.Context(), server.URL(), id+"-request", id, id+"-dispatch")
		_ = response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("admit %s status=%d", id, response.StatusCode)
		}
		waitFleetCharacterizationSignal(t, runner.slots[index].started, "direct admission")
		live := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+id)
		if live.ProviderSession != nil || live.TerminalCause != nil || string(live.State) != "RUNNING" {
			t.Fatalf("live no-reference Worker=%#v", live)
		}
	}
	assertFleetCharacterizationMissing(t, server)
	target := routeCharacterizationDispatch{workerSessionID: "exact-target", dispatchID: "exact-target-dispatch"}
	joinedExactFactoryStop(t, server, runner.slots[0], target, "cancel", true)
	assertFleetCharacterizationTerminal(t, server, sessionID, target.workerSessionID, target.dispatchID, "CANCELED")
	assertCapturedStopCause(t, server.URL(), target.workerSessionID, "OPERATOR_CANCEL")
	for _, action := range []string{"cancel", "terminate"} {
		repeat, err := executeExactFactoryStop(t, server, t.Context(), target.workerSessionID, action, false)
		if err != nil || string(repeat.Outcome) != "NOOP" || string(repeat.State) != "CANCELED" || repeat.DispatchId != target.dispatchID {
			t.Fatalf("terminal %s=%#v error=%v", action, repeat, err)
		}
		assertCapturedStopCause(t, server.URL(), target.workerSessionID, "OPERATOR_CANCEL")
	}
	select {
	case <-runner.slots[1].canceled:
		t.Fatal("cancel affected sibling")
	default:
	}
	assertFleetCharacterizationSnapshot(t, server, "exact-sibling", "exact-sibling-dispatch", "RUNNING")
	runner.slots[1].release()
	waitFleetCharacterizationSignal(t, runner.slots[1].returned, "sibling natural completion")
	assertFleetCharacterizationTerminal(t, server, sessionID, "exact-sibling", "exact-sibling-dispatch", "COMPLETED")
	assertCapturedStopCause(t, server.URL(), "exact-sibling", "COMPLETED")
	if runner.callCount() != 2 {
		t.Fatalf("provider calls=%d, want only original target and sibling", runner.callCount())
	}
}
