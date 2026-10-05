package http_test

import (
	"context"
	"encoding/json"
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

// The injected edge delays only the acknowledgement of a genuinely synced
// intent. All recording writes, reads and control result persistence are real.
type exactStopIntentGate struct {
	recordings.WorkerRecordingStore
	committed chan struct{}
	proceed   chan struct{}
	once      sync.Once
}

func (store *exactStopIntentGate) release() { store.once.Do(func() { close(store.proceed) }) }

func (store *exactStopIntentGate) BeginWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	accepted, created, err := store.WorkerRecordingStore.BeginWorkerControlOperation(ctx, record)
	if err == nil && created && record.Target.WorkerSessionID == "natural-target" {
		close(store.committed)
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
			WorkerRecordingStoreObserver: func(backing recordings.WorkerRecordingStore) { store.WorkerRecordingStore = backing },
		},
		BeforeStart: func(tb testing.TB, process support.Process, inputs root.Input) {
			support.InitializeCustomerHomeWithProcess(tb, process, inputs.Env, inputs.WorkingDirectory)
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
