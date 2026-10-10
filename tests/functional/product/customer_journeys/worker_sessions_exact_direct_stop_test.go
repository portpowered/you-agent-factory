package customer_journeys_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workercli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F9-01 uses separate roots because the selected profile and command edge are
// immutable process inputs. No real remote account or new authorization policy
// is inferred from this existing selected-host not-found boundary.
func TestExactStopWrongSelectedProfileHasNoEffects(t *testing.T) {
	t.Parallel()
	ownerRunner := newFleetCharacterizationRunner()
	owner := startExactFactoryStopServer(t, ownerRunner, uuid.NewString())
	foreignRunner := newFleetCharacterizationRunner()
	foreign := startExactFactoryStopServer(t, foreignRunner, uuid.NewString())
	for index, id := range []string{"profile-target", "profile-sibling"} {
		response := postDirectWorkerSession(t, t.Context(), owner.URL(), id+"-request", id, id+"-dispatch")
		_ = response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("admit %s status=%d", id, response.StatusCode)
		}
		waitFleetCharacterizationSignal(t, ownerRunner.slots[index].started, "profile owner admission")
	}
	for _, action := range []string{"cancel", "terminate"} {
		assertExactForeignStopRefused(t, foreign, "profile-target", action)
		for index, id := range []string{"profile-target", "profile-sibling"} {
			select {
			case <-ownerRunner.slots[index].canceled:
				t.Fatalf("foreign %s canceled %s", action, id)
			default:
			}
			assertFleetCharacterizationSnapshot(t, owner, id, id+"-dispatch", "RUNNING")
		}
	}
	if ownerRunner.callCount() != 2 || foreignRunner.callCount() != 0 {
		t.Fatalf("owner/foreign calls=%d/%d", ownerRunner.callCount(), foreignRunner.callCount())
	}
}

func assertExactForeignStopRefused(t *testing.T, foreign *fleetCharacterizationServer, id, action string) {
	t.Helper()
	response := postWorkerSessionControl(t, foreign.URL(), id, action)
	defer response.Body.Close()
	var diagnostic struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&diagnostic); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound || diagnostic.Code != "NOT_FOUND" {
		t.Fatalf("foreign HTTP %s status=%d diagnostic=%#v", action, response.StatusCode, diagnostic)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "--remote", "--server", foreign.URL(), "worker-sessions", action, id, "--output", "json"})
	err := foreign.Execute(t, inputs.Input)
	var typed *workercli.CLIError
	if !errors.As(err, &typed) || typed.Code != "NOT_FOUND" || strings.TrimSpace(inputs.Stdout()) != "" {
		t.Fatalf("foreign CLI %s error=%v stdout=%s stderr=%s", action, err, inputs.Stdout(), inputs.Stderr())
	}
	if err := json.Unmarshal([]byte(inputs.Stderr()), &diagnostic); err != nil || diagnostic.Code != "NOT_FOUND" {
		t.Fatalf("foreign CLI diagnostic=%#v error=%v", diagnostic, err)
	}
}

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

// The injected edge delays only the acknowledgement of a genuinely synced
// intent. All recording writes, reads and control result persistence are real.
type exactStopIntentGate struct {
	recordings.WorkerRecordingStore
	committed  chan struct{}
	proceed    chan struct{}
	once       sync.Once
	commitOnce sync.Once
}

// The journal and execution remain real. Only this exact worker's returned
// operation evidence is corrupted at the replaceable recording-store edge.
func (store *exactStopIntentGate) ListWorkerControlOperations(ctx context.Context, target recordings.WorkerControlTarget) ([]recordings.WorkerControlOperationRecord, error) {
	records, err := store.WorkerRecordingStore.ListWorkerControlOperations(ctx, target)
	if target.WorkerSessionID == "corrupt-stop-target" {
		for index := range records {
			if records[index].Operation.Phase == "COMPLETED" {
				records[index].Result = append([]byte(`{"private":"private-stop-detail",`), records[index].Result[1:]...)
			}
		}
	}
	return records, err
}

func TestExactStopMalformedEvidenceCannotClaimOperatorCause(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	store := &exactStopIntentGate{committed: make(chan struct{}), proceed: make(chan struct{})}
	server := startExactNaturalStopServer(t, runner, store, uuid.NewString())
	id := "corrupt-stop-target"
	response := postDirectWorkerSession(t, t.Context(), server.URL(), id+"-request", id, id+"-dispatch")
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("admission status=%d", response.StatusCode)
	}
	waitFleetCharacterizationSignal(t, runner.slots[0].started, "corrupt-evidence admission")
	joinedExactFactoryStop(t, server, runner.slots[0], routeCharacterizationDispatch{workerSessionID: id, dispatchID: id + "-dispatch"}, "cancel", true)
	for _, action := range []string{"cancel", "terminate"} {
		repeat, err := executeExactFactoryStop(t, server, t.Context(), id, action, false)
		if err != nil || string(repeat.Outcome) != "NOOP" || string(repeat.State) != "CANCELED" {
			t.Fatalf("terminal repeat=%#v error=%v", repeat, err)
		}
		shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+id)
		if string(shown.State) != "CANCELED" || shown.TerminalCause != nil {
			t.Fatalf("malformed evidence invented cause or hid state: %#v", shown)
		}
		encoded, err := json.Marshal(shown)
		if err != nil || strings.Contains(string(encoded), "private-stop-detail") {
			t.Fatalf("private evidence leaked: error=%v observation=%s", err, encoded)
		}
	}
	if runner.callCount() != 1 {
		t.Fatalf("provider calls=%d, want original execution only", runner.callCount())
	}
	assertExactStopUnknownCauseCLI(t, server, id)
}

func assertExactStopUnknownCauseCLI(t *testing.T, server *fleetCharacterizationServer, id string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--remote", "--server", server.URL(), "--json", "worker-sessions", "show", "--worker-session-id", id})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI show error=%v stderr=%s", err, inputs.Stderr())
	}
	var shown factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(inputs.Stdout()), &shown); err != nil || string(shown.State) != "CANCELED" || shown.TerminalCause != nil || strings.Contains(inputs.Stdout()+inputs.Stderr(), "private-stop-detail") {
		t.Fatalf("CLI malformed-evidence observation=%#v error=%v stdout=%s stderr=%s", shown, err, inputs.Stdout(), inputs.Stderr())
	}
}

func (store *exactStopIntentGate) release() { store.once.Do(func() { close(store.proceed) }) }

func (store *exactStopIntentGate) BeginWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	accepted, created, err := store.WorkerRecordingStore.BeginWorkerControlOperation(ctx, record)
	if err == nil && created && record.Target.WorkerSessionID == "natural-target" {
		store.commitOnce.Do(func() { close(store.committed) })
		select {
		case <-store.proceed:
		case <-ctx.Done():
			return accepted, created, ctx.Err()
		}
	}
	return accepted, created, err
}

func startExactNaturalStopServer(t *testing.T, runner *fleetCharacterizationRunner, store *exactStopIntentGate, sessionID string) *fleetCharacterizationServer {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "exact-natural-stop")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	home := t.TempDir()
	server := &fleetCharacterizationServer{env: []string{"HOME=" + home, "USERPROFILE=" + home}, dir: dir}
	server.FunctionalAPIServer = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Args: []string{"--session", sessionID}, Env: server.env,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner, WorkerRecordingWriter: store,
			FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir),
			WorkerRecordingStoreObserver:    func(backing recordings.WorkerRecordingStore) { store.WorkerRecordingStore = backing },
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(runner.releaseAll)
	t.Cleanup(store.release)
	return server
}

// Natural completion races a real accepted control while storage acknowledgement
// is paused. Terminal event subscription is the completion barrier; no sleep or
// scheduling assumption determines the winner. HTTP cancel and CLI terminate
// retain one natural terminal event/cause and leave the independent sibling live.
func TestExactStopNaturalCompletionWinsDuringIntentAcknowledgement(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"cancel", "terminate"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			runner := newFleetCharacterizationRunner()
			store := &exactStopIntentGate{committed: make(chan struct{}), proceed: make(chan struct{})}
			sessionID := uuid.NewString()
			server := startExactNaturalStopServer(t, runner, store, sessionID)
			for index, id := range []string{"natural-target", "natural-sibling"} {
				response := postDirectWorkerSession(t, t.Context(), server.URL(), id+"-request", id, id+"-dispatch")
				_ = response.Body.Close()
				if response.StatusCode != http.StatusAccepted {
					t.Fatalf("admit %s status=%d", id, response.StatusCode)
				}
				waitFleetCharacterizationSignal(t, runner.slots[index].started, "natural race admission")
			}
			results := make(chan error, 1)
			go func() {
				result, err := executeExactFactoryStop(t, server, t.Context(), "natural-target", action, action == "terminate")
				if err == nil && (string(result.Outcome) != "NOOP" || string(result.State) != "COMPLETED" || result.DispatchId != "natural-target-dispatch") {
					err = fmt.Errorf("natural racing stop=%#v", result)
				}
				results <- err
			}()
			waitFleetCharacterizationSignal(t, store.committed, "durable intent")
			runner.slots[0].release()
			assertFleetCharacterizationTerminal(t, server, sessionID, "natural-target", "natural-target-dispatch", "COMPLETED")
			store.release()
			select {
			case err := <-results:
				if err != nil {
					t.Fatal(err)
				}
			case <-t.Context().Done():
				t.Fatal("natural racing stop did not return")
			}
			assertCapturedStopCause(t, server.URL(), "natural-target", "COMPLETED")
			assertExactNaturalStopHistory(t, server, sessionID)
			assertFleetCharacterizationSnapshot(t, server, "natural-sibling", "natural-sibling-dispatch", "RUNNING")
			for _, slot := range runner.slots[:2] {
				select {
				case <-slot.canceled:
					t.Fatal("natural racing control canceled an execution")
				default:
				}
			}
			if runner.callCount() != 2 {
				t.Fatalf("provider calls=%d, want two original executions", runner.callCount())
			}
		})
	}
}

func assertExactNaturalStopHistory(t *testing.T, server *fleetCharacterizationServer, sessionID string) {
	t.Helper()
	for _, action := range []string{"cancel", "terminate"} {
		result, err := executeExactFactoryStop(t, server, t.Context(), "natural-target", action, false)
		if err != nil || string(result.Outcome) != "NOOP" || string(result.State) != "COMPLETED" {
			t.Fatalf("natural terminal repeat=%#v error=%v", result, err)
		}
	}
	assertCapturedStopCause(t, server.URL(), "natural-target", "COMPLETED")
	assertFleetCharacterizationTerminal(t, server, sessionID, "natural-target", "natural-target-dispatch", "COMPLETED")
	assertCapturedLogsCLIHTTPParity(t, server.FunctionalAPIServer, "natural-target")
}

// Secrets enter through the actual public resolved execution contract. Public
// stop, show and captured logs must preserve useful facts without that content.
func TestExactStopPublicResultsExcludeConfiguredSecrets(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	server := startExactFactoryStopServer(t, runner, uuid.NewString())
	payload := directWorkerSessionPayload("private-stop-request", "private-stop-worker", "private-stop-dispatch")
	message, prompt := "classified-tool-token", "classified-environment-token"
	payload.Execution.UserMessage, payload.Execution.SystemPrompt = &message, &prompt
	payload.Execution.EnvVars = &map[string]string{"CONTROL_SECRET": message, "PROVIDER_SECRET": prompt}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL()+"/worker-sessions", "application/json", strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("private admission status=%d", response.StatusCode)
	}
	waitFleetCharacterizationSignal(t, runner.slots[0].started, "private admission")
	target := routeCharacterizationDispatch{workerSessionID: "private-stop-worker", dispatchID: "private-stop-dispatch"}
	joinedExactFactoryStop(t, server, runner.slots[0], target, "cancel", true)
	assertCapturedStopCause(t, server.URL(), target.workerSessionID, "OPERATOR_CANCEL")
	for _, action := range []string{"cancel", "terminate"} {
		result, err := executeExactFactoryStop(t, server, t.Context(), target.workerSessionID, action, false)
		if err != nil || string(result.Outcome) != "NOOP" {
			t.Fatalf("private terminal repeat=%#v error=%v", result, err)
		}
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		assertCapturedSecretsAbsent(t, string(data))
	}
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+target.workerSessionID)
	logs := assertCapturedLogsCLIHTTPParity(t, server.FunctionalAPIServer, target.workerSessionID)
	data, err = json.Marshal([]any{shown, logs})
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedSecretsAbsent(t, string(data))
	if runner.callCount() != 1 {
		t.Fatalf("private stop provider calls=%d", runner.callCount())
	}
}

// Preserve the real store's bounded read and activation capabilities while
// keeping this decorator's controlled write/activity faults.
func (store *exactStopIntentGate) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	if id == "unavailable-archive" {
		return recordings.WorkerCapturedSummary{}, errors.New("archive storage unavailable")
	}
	summary, err := store.WorkerRecordingStore.(recordings.WorkerCapturedSummaryReader).LookupWorkerSessionSummary(ctx, id)
	if id == "corrupt-stop-target" {
		for index := range summary.ControlOperations {
			record := &summary.ControlOperations[index]
			if record.Operation.Phase == "COMPLETED" {
				record.Result = append([]byte(`{"private":"private-stop-detail",`), record.Result[1:]...)
			}
		}
	}
	return summary, err
}

func (store *exactStopIntentGate) RecoverWorkerOwners(ctx context.Context) error {
	return store.WorkerRecordingStore.(interface{ RecoverWorkerOwners(context.Context) error }).RecoverWorkerOwners(ctx)
}

// The caller has its own empty registry and unprepared capture. Reuse that
// process for every command while the selected host owns target and peer.
func TestDefaultWorkerSessionCancelSeparateOwner(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	sessionID := uuid.NewString()
	server := startExactFactoryStopServer(t, runner, sessionID)
	caller := support.BuildProcess(t, serviceedges.Edges{})
	for index, id := range []string{"default-target", "default-peer"} {
		response := postDirectWorkerSession(t, t.Context(), server.URL(), id+"-request", id, id+"-dispatch")
		_ = response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("admit %s: %d", id, response.StatusCode)
		}
		waitFleetCharacterizationSignal(t, runner.slots[index].started, "owner admission")
	}
	joinedExactFactoryStop(t, server, runner.slots[0], routeCharacterizationDispatch{workerSessionID: "default-target", dispatchID: "default-target-dispatch"}, "cancel", true, caller)
	assertDefaultOwnerTerminalAndMissing(t, caller, server, runner)
	// Keep the peer live throughout target controls, then join its execution
	// and durable terminal publication before closing the owning process.
	joinedExactFactoryStop(t, server, runner.slots[1], routeCharacterizationDispatch{workerSessionID: "default-peer", dispatchID: "default-peer-dispatch"}, "cancel", false)
	assertFleetCharacterizationTerminal(t, server, sessionID, "default-peer", "default-peer-dispatch", "CANCELED")
	assertCapturedStopCause(t, server.URL(), "default-peer", "OPERATOR_CANCEL")
	// Stop alone joins the daemon invocation; Close also drains resources
	// retained by the reusable root before caller commands reuse this profile.
	server.Close(t)
	assertDefaultOwnerUnavailable(t, caller, server)
}

func assertDefaultOwnerTerminalAndMissing(t *testing.T, caller support.Process, server *fleetCharacterizationServer, runner *fleetCharacterizationRunner) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		result, err := executeDefaultOwnerCancel(t, caller, server, "default-target", remote)
		if err != nil || string(result.Outcome) != "NOOP" || string(result.State) != "CANCELED" || result.DispatchId != "default-target-dispatch" {
			t.Fatalf("terminal remote=%v: %+v %v", remote, result, err)
		}
	}
	result, err := executeExactFactoryStop(t, server, t.Context(), "default-target", "cancel", false)
	if err != nil || string(result.Outcome) != "NOOP" {
		t.Fatalf("HTTP repeat: %+v %v", result, err)
	}
	assertFleetCharacterizationSnapshot(t, server, "default-peer", "default-peer-dispatch", "RUNNING")
	select {
	case <-runner.slots[1].canceled:
		t.Fatal("peer canceled")
	default:
	}
	for _, remote := range []bool{false, true} {
		_, err := executeDefaultOwnerCancel(t, caller, server, "missing-worker", remote)
		var typed *workercli.CLIError
		if !errors.As(err, &typed) || typed.Code != "NOT_FOUND" {
			t.Fatalf("unknown remote=%v: %v", remote, err)
		}
	}
}

func assertDefaultOwnerUnavailable(t *testing.T, caller support.Process, server *fleetCharacterizationServer) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		_, err := executeDefaultOwnerCancel(t, caller, server, "default-target", remote)
		var typed *workercli.CLIError
		code := "NOT_FOUND"
		if remote {
			code = "FACTORY_UNREACHABLE"
		}
		if !errors.As(err, &typed) || typed.Code != code {
			t.Fatalf("unreachable remote=%v: %v", remote, err)
		}
	}
}

func executeDefaultOwnerCancel(t *testing.T, caller support.Process, server *fleetCharacterizationServer, id string, remote bool) (factoryapi.WorkerSessionControlResponse, error) {
	t.Helper()
	args := []string{"you", "--server", server.URL(), "--json", "worker-sessions", "cancel", id}
	if remote {
		args = append(args, "--remote")
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Env, inputs.WorkingDirectory = server.env, server.dir
	var result factoryapi.WorkerSessionControlResponse
	if err := caller.Execute(inputs.Input); err != nil {
		return result, err
	}
	return result, json.Unmarshal([]byte(inputs.Stdout()), &result)
}

// Same-ID executions in separate profiles prove local ownership wins placement.
// Explicit remote failure and stale-attempt requests precede any stop.
// Foreign selected-profile refusal is covered by TestExactStopWrongSelectedProfileHasNoEffects.
func TestDefaultWorkerSessionCancelLocalOwnershipAndFences(t *testing.T) {
	t.Parallel()
	localRunner, foreignRunner := newFleetCharacterizationRunner(), newFleetCharacterizationRunner()
	local := startExactFactoryStopServer(t, localRunner, uuid.NewString())
	foreign := startExactFactoryStopServer(t, foreignRunner, uuid.NewString())
	for _, item := range []struct {
		server *fleetCharacterizationServer
		runner *fleetCharacterizationRunner
	}{{local, localRunner}, {foreign, foreignRunner}} {
		for index, id := range []string{"local-target", "local-peer"} {
			response := postDirectWorkerSession(t, t.Context(), item.server.URL(), id+"-request", id, id+"-dispatch")
			_ = response.Body.Close()
			if response.StatusCode != http.StatusAccepted {
				t.Fatalf("admission: %d", response.StatusCode)
			}
			waitFleetCharacterizationSignal(t, item.runner.slots[index].started, "local ownership admission")
		}
	}
	assertDefaultOwnerRefusals(t, local)
	for _, item := range []struct {
		server *fleetCharacterizationServer
		runner *fleetCharacterizationRunner
	}{{local, localRunner}, {foreign, foreignRunner}} {
		for index, id := range []string{"local-target", "local-peer"} {
			assertFleetCharacterizationSnapshot(t, item.server, id, id+"-dispatch", "RUNNING")
			select {
			case <-item.runner.slots[index].canceled:
				t.Fatal("refusal canceled peer")
			default:
			}
		}
	}
	caller := ownerProcessFunc(func(input root.Input) error { return local.Execute(t, input) })
	joinedExactFactoryStop(t, foreign, localRunner.slots[0], routeCharacterizationDispatch{workerSessionID: "local-target", dispatchID: "local-target-dispatch"}, "cancel", true, caller)
	assertFleetCharacterizationSnapshot(t, foreign, "local-target", "local-target-dispatch", "RUNNING")
	assertFleetCharacterizationSnapshot(t, local, "local-peer", "local-peer-dispatch", "RUNNING")
}

func TestDefaultWorkerSessionCancelArchiveFailureIsAuthoritative(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	store := &exactStopIntentGate{committed: make(chan struct{}), proceed: make(chan struct{})}
	server := startExactNaturalStopServer(t, runner, store, uuid.NewString())
	caller := support.BuildProcess(t, serviceedges.Edges{})
	response := postDirectWorkerSession(t, t.Context(), server.URL(), "archive-peer-request", "archive-peer", "archive-peer-dispatch")
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("admission: %d", response.StatusCode)
	}
	waitFleetCharacterizationSignal(t, runner.slots[0].started, "archive healthy peer")
	for _, remote := range []bool{false, true} {
		_, err := executeDefaultOwnerCancel(t, caller, server, "unavailable-archive", remote)
		var typed *workercli.CLIError
		if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_CONTROL_FAILED" {
			t.Fatalf("archive remote=%v: %v", remote, err)
		}
	}
	assertFleetCharacterizationSnapshot(t, server, "archive-peer", "archive-peer-dispatch", "RUNNING")
	select {
	case <-runner.slots[0].canceled:
		t.Fatal("archive failure canceled peer")
	default:
	}
}

func assertDefaultOwnerRefusals(t *testing.T, local *fleetCharacterizationServer) {
	t.Helper()
	for index, args := range [][]string{
		{"--remote", "--server", "http://127.0.0.1:1", "worker-sessions", "cancel", "local-target"},
		{"--server", local.URL(), "worker-sessions", "terminate", "local-target", "--force", "--request-id", "stale-request", "--expected-attempt-id", "wrong-attempt"},
	} {
		inputs := support.FakeInputs(t.Context(), append([]string{"you", "--json"}, args...))
		err := local.Execute(t, inputs.Input)
		var typed *workercli.CLIError
		code := "FACTORY_UNREACHABLE"
		if index == 1 {
			code = "WORKER_SESSION_CONTROL_CONFLICT"
		}
		if !errors.As(err, &typed) || typed.Code != code {
			t.Fatalf("refusal: %v %s", err, inputs.Stderr())
		}
	}
}

type ownerProcessFunc func(root.Input) error

func (call ownerProcessFunc) Execute(input root.Input) error { return call(input) }
