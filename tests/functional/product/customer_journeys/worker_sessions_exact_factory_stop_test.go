package customer_journeys_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workercli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// One reusable production graph and explicit Factory Session exercise cancel
// then terminate while a direct sibling stays live. Only provider execution is
// controlled; HTTP/Process.Execute, Runtime and capture files are real. Ordered
// admissions assign immutable gates. Runtime requeues canceled Work, so each
// next dispatch is observed independently rather than assuming stop finishes Work.
func TestExactStopFactoryJoinsAndPreservesSibling(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	finalSlot := &fleetCharacterizationSlot{started: make(chan struct{}), canceled: make(chan struct{}), returned: make(chan struct{}), gate: make(chan struct{})}
	finalSlot.release()
	runner.slots = append(runner.slots, finalSlot)
	sessionID := uuid.NewString()
	server := startExactFactoryStopServer(t, runner, sessionID)
	start := postDirectWorkerSession(t, t.Context(), server.URL(), "sibling-request", "exact-sibling", "sibling-dispatch")
	_ = start.Body.Close()
	if start.StatusCode != http.StatusAccepted {
		t.Fatalf("sibling admission status=%d", start.StatusCode)
	}
	waitFleetCharacterizationSignal(t, runner.slots[0].started, "sibling admission")
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), sessionID))
	name := "exact-factory-work"
	submitted := support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": name},
	})
	workID := support.StringPointerValue(submitted.WorkId)
	var previous *routeCharacterizationDispatch
	for index, action := range []string{"cancel", "terminate"} {
		waitFleetCharacterizationSignal(t, runner.slots[index+1].started, "Factory admission")
		target := waitForRouteCharacterizationAssociation(t, stream, workID)
		if previous != nil {
			assertExactFactoryReplacementUnaffected(t, server, runner.slots[index+1], *previous, target)
		}
		live := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+target.workerSessionID)
		if live.TerminalCause != nil || live.ProviderSession != nil {
			t.Fatalf("live no-reference Worker = %#v", live)
		}
		assertFactoryInterruptRefusesBeforeStop(t, server, runner, target, index+2)
		joinedExactFactoryStop(t, server, runner.slots[index+1], target, action, index == 1)
		state := "CANCELED"
		if action == "terminate" {
			state = "TERMINATED"
		}
		assertFleetCharacterizationSnapshot(t, server, target.workerSessionID, target.dispatchID, state)
		assertCapturedStopCause(t, server.URL(), target.workerSessionID, "OPERATOR_"+strings.ToUpper(action))
		assertExactFactoryStopConsequence(t, server, stream, sessionID, target)
		// A mixed terminal repeat preserves the first winner and publishes no
		// second Factory result. The direct sibling remains uncanceled.
		repeat, err := executeExactFactoryStop(t, server, t.Context(), target.workerSessionID, "terminate", false)
		if err != nil || string(repeat.Outcome) != "NOOP" || string(repeat.State) != state {
			t.Fatalf("terminal repeat=%#v error=%v", repeat, err)
		}
		assertCapturedStopCause(t, server.URL(), target.workerSessionID, "OPERATOR_"+strings.ToUpper(action))
		assertExactFactoryStopEventCount(t, server, sessionID, target.dispatchID)
		select {
		case <-runner.slots[0].canceled:
			t.Fatal("stop canceled unrelated direct sibling")
		default:
		}
		assertFleetCharacterizationSnapshot(t, server, "exact-sibling", "sibling-dispatch", "RUNNING")
		previous = &target
	}
	waitFleetCharacterizationSignal(t, finalSlot.started, "Runtime capacity reuse")
	final := waitForRouteCharacterizationAssociation(t, stream, workID)
	completeFleetCharacterizationSibling(t, server, finalSlot, stream, final)
	assertCapturedStopCause(t, server.URL(), final.workerSessionID, "COMPLETED")
	runner.slots[0].release()
	waitFleetCharacterizationSignal(t, runner.slots[0].returned, "sibling natural completion")
	assertFleetCharacterizationTerminal(t, server, sessionID, "exact-sibling", "sibling-dispatch", "COMPLETED")
	assertCapturedStopCause(t, server.URL(), "exact-sibling", "COMPLETED")
	if runner.callCount() != 4 {
		t.Fatalf("provider calls=%d, want direct sibling and three Runtime attempts", runner.callCount())
	}
}

// Extend the live Factory journey without another process graph. Both public
// transports must refuse replacement while Runtime and the direct sibling
// retain their original execution; the following joined stop proves recovery.
func assertFactoryInterruptRefusesBeforeStop(t *testing.T, server *fleetCharacterizationServer, runner *fleetCharacterizationRunner, target routeCharacterizationDispatch, calls int) {
	t.Helper()
	successor := "refused-" + target.workerSessionID
	requestID := "factory-interrupt-" + target.workerSessionID
	payload, err := json.Marshal(factoryapi.WorkerSessionInterruptRequest{
		RequestId: requestID, SuccessorWorkerSessionId: successor, ReplacementMessage: "replacement",
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL()+"/worker-sessions/"+target.workerSessionID+"/interrupt", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var refusal factoryapi.WorkerSessionInterruptError
	if err := json.NewDecoder(response.Body).Decode(&refusal); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict || refusal.Code != "UNSUPPORTED" || string(refusal.Phase) != "VALIDATION" {
		t.Fatalf("Factory interrupt HTTP=%d/%#v", response.StatusCode, refusal)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "--remote", "--server", server.URL(), "worker-sessions", "interrupt", target.workerSessionID,
		"--request-id", requestID, "--successor-worker-session-id", successor, "--replacement-message", "replacement", "--async", "--output", "json"})
	err = server.Execute(t, inputs.Input)
	var diagnostic *workercli.CLIError
	if !errors.As(err, &diagnostic) || diagnostic.Code != "UNSUPPORTED" || diagnostic.Phase != "VALIDATION" {
		t.Fatalf("Factory interrupt CLI=%#v err=%v", diagnostic, err)
	}
	assertFleetCharacterizationSnapshot(t, server, target.workerSessionID, target.dispatchID, "RUNNING")
	assertFleetCharacterizationSnapshot(t, server, "exact-sibling", "sibling-dispatch", "RUNNING")
	if runner.callCount() != calls {
		t.Fatalf("Factory refusal provider calls=%d want=%d", runner.callCount(), calls)
	}
	absent, err := http.Get(server.URL() + "/worker-sessions/" + successor)
	if err != nil {
		t.Fatal(err)
	}
	defer absent.Body.Close()
	if absent.StatusCode != http.StatusNotFound {
		t.Fatalf("refused successor status=%d", absent.StatusCode)
	}
}

func assertExactFactoryReplacementUnaffected(t *testing.T, server *fleetCharacterizationServer, slot *fleetCharacterizationSlot, previous, target routeCharacterizationDispatch) {
	t.Helper()
	if target.workerSessionID == previous.workerSessionID || target.dispatchID == previous.dispatchID {
		t.Fatal("Runtime replacement reused stopped execution identity")
	}
	repeat, err := executeExactFactoryStop(t, server, t.Context(), previous.workerSessionID, "terminate", false)
	if err != nil || string(repeat.Outcome) != "NOOP" {
		t.Fatalf("old attempt repeat=%#v error=%v", repeat, err)
	}
	select {
	case <-slot.canceled:
		t.Fatal("old attempt stop canceled Runtime replacement")
	default:
	}
}

func startExactFactoryStopServer(t *testing.T, runner *fleetCharacterizationRunner, sessionID string) *fleetCharacterizationServer {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "exact-factory-stop")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	home := t.TempDir()
	server := &fleetCharacterizationServer{env: []string{"HOME=" + home, "USERPROFILE=" + home, "HOMEDRIVE=" + filepath.VolumeName(home), "HOMEPATH=" + home[len(filepath.VolumeName(home)):]}, dir: dir}
	server.FunctionalAPIServer = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Args: []string{"--session", sessionID}, Env: server.env,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	})
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(runner.releaseAll)
	return server
}

func executeExactFactoryStop(t *testing.T, server *fleetCharacterizationServer, ctx context.Context, id, action string, cli bool) (factoryapi.WorkerSessionControlResponse, error) {
	var result factoryapi.WorkerSessionControlResponse
	if cli {
		inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", server.URL(), "worker-sessions", action, id, "--output", "json"})
		if err := server.Execute(t, inputs.Input); err != nil {
			return result, fmt.Errorf("CLI %s: %w; %s", action, err, inputs.Stderr())
		}
		return result, json.Unmarshal([]byte(inputs.Stdout()), &result)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL()+"/worker-sessions/"+id+"/"+action, nil)
	if err != nil {
		return result, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("HTTP %s status=%d", action, response.StatusCode)
	}
	return result, json.NewDecoder(response.Body).Decode(&result)
}

func joinedExactFactoryStop(t *testing.T, server *fleetCharacterizationServer, slot *fleetCharacterizationSlot, target routeCharacterizationDispatch, action string, cli bool) {
	t.Helper()
	type outcome struct {
		result   factoryapi.WorkerSessionControlResponse
		err      error
		returned bool
	}
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	t.Cleanup(cancel)
	results := make(chan outcome, 1)
	done := make(chan struct{})
	t.Cleanup(func() {
		slot.release()
		select {
		case <-done:
		case <-time.After(functionalWorkerSignalTimeout):
			t.Error("owned stop did not join cleanup")
		}
	})
	go func() {
		defer close(done)
		result, err := executeExactFactoryStop(t, server, ctx, target.workerSessionID, action, cli)
		returned := false
		select {
		case <-slot.returned:
			returned = true
		default:
		}
		results <- outcome{result, err, returned}
	}()
	select {
	case <-slot.canceled:
	case got := <-results:
		t.Fatalf("Factory stop before cancellation acknowledgement: %#v error=%v", got.result, got.err)
	case <-ctx.Done():
		t.Fatal("missing Factory cancellation acknowledgement")
	}
	select {
	case got := <-results:
		t.Fatalf("stop returned before callback release: %#v", got)
	default:
	}
	slot.release()
	select {
	case got := <-results:
		if got.err != nil || !got.returned {
			t.Fatalf("joined %s=%#v", action, got)
		}
		assertExactJoinedStopResponse(t, got.result, target, action)
	case <-ctx.Done():
		t.Fatal("Factory stop did not join")
	}
}

func assertExactJoinedStopResponse(t *testing.T, result factoryapi.WorkerSessionControlResponse, target routeCharacterizationDispatch, action string) {
	t.Helper()
	state := "CANCELED"
	if action == "terminate" {
		state = "TERMINATED"
	}
	if result.WorkerSessionId != target.workerSessionID || result.DispatchId != target.dispatchID ||
		string(result.Action) != strings.ToUpper(action) || string(result.Outcome) != "APPLIED" || string(result.State) != state {
		t.Fatalf("joined %s=%#v", action, result)
	}
}

func assertExactFactoryStopConsequence(t *testing.T, server *fleetCharacterizationServer, stream *support.FactoryEventStream, sessionID string, target routeCharacterizationDispatch) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	defer cancel()
	for {
		event := stream.NextEventContext(ctx)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || support.StringPointerValue(event.Context.DispatchId) != target.dispatchID {
			continue
		}
		result, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil || result.Cancellation == nil {
			t.Fatalf("stopped Factory dispatch=%#v error=%v", result, err)
		}
		break
	}
	assertExactFactoryStopEventCount(t, server, sessionID, target.dispatchID)
}

func assertExactFactoryStopEventCount(t *testing.T, server *fleetCharacterizationServer, sessionID, dispatchID string) {
	t.Helper()
	count := 0
	for _, event := range support.GetFactoryEventsForSessionAt(t, server.URL(), sessionID) {
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse && support.StringPointerValue(event.Context.DispatchId) == dispatchID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Factory terminal consequences=%d, want one", count)
	}
}
