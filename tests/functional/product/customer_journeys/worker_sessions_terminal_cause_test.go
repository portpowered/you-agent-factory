package customer_journeys_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertCapturedStopCause(t *testing.T, baseURL, id, want string) {
	t.Helper()
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/"+id)
	if shown.TerminalCause == nil || string(*shown.TerminalCause) != want {
		t.Fatalf("terminalCause=%v, want %s", shown.TerminalCause, want)
	}
}

// Production root composition, HTTP and real capture store; only the provider
// command is controlled. Each isolated fixture owns its direct Worker,
// readiness gate, journal, profile and terminal callback.
func TestExactStopTerminateReportsCommittedCause(t *testing.T) {
	t.Parallel()
	runner := newFunctionalWorkerGate(make(chan struct{}))
	server := startDirectWorkerSessionServer(t, runner)
	start := postDirectWorkerSession(t, t.Context(), server.URL(), "cause-request", "cause-worker", "cause-attempt")
	_ = start.Body.Close()
	if start.StatusCode != http.StatusAccepted {
		t.Fatalf("admission status=%d", start.StatusCode)
	}
	runner.waitStarted(t)
	live := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/cause-worker")
	if live.TerminalCause != nil {
		t.Fatal("live Worker has terminalCause")
	}
	stop := postWorkerSessionControl(t, server.URL(), "cause-worker", "terminate")
	_ = stop.Body.Close()
	if stop.StatusCode != http.StatusOK {
		t.Fatalf("terminate status=%d", stop.StatusCode)
	}
	runner.waitCanceled(t)
	assertCapturedStopCause(t, server.URL(), "cause-worker", "OPERATOR_TERMINATE")
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "worker-sessions", "show", "--worker-session-id", "cause-worker", "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI summary: %v %s", err, inputs.Stderr())
	}
	var cli factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil || cli.TerminalCause == nil || string(*cli.TerminalCause) != "OPERATOR_TERMINATE" {
		t.Fatalf("CLI terminal cause = %v, %v", cli.TerminalCause, err)
	}
	repeat := postWorkerSessionControl(t, server.URL(), "cause-worker", "cancel")
	_ = repeat.Body.Close()
	if repeat.StatusCode != http.StatusOK {
		t.Fatalf("terminal repeat status=%d", repeat.StatusCode)
	}
	assertCapturedStopCause(t, server.URL(), "cause-worker", "OPERATOR_TERMINATE")
}

// The Factory Session close owns an ordered stop/join. Provider cancellation
// and completion are separate gates; retained reads use the same root process.
func TestShutdownTerminalCausePublicParity(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	sessionID := uuid.NewString()
	server := startShutdownCauseServer(t, runner, sessionID)
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), sessionID))
	name := "shutdown-cause"
	submitted := support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": name},
	})
	waitFleetCharacterizationSignal(t, runner.slots[0].started, "owned provider admission")
	target := waitForRouteCharacterizationAssociation(t, stream, support.StringPointerValue(submitted.WorkId))
	live := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+target.workerSessionID)
	if live.TerminalCause != nil || live.StartedAt == nil || live.ProviderSession != nil {
		t.Fatalf("live no-reference attempt = %+v", live)
	}
	released := make(chan struct{})
	go func() {
		select {
		case <-runner.slots[0].canceled:
			runner.slots[0].release()
		case <-runner.slots[0].returned:
		}
		close(released)
	}()
	support.CloseFactorySessionAt(t, server.URL(), sessionID)
	waitFleetCharacterizationSignal(t, released, "joined cancellation")
	waitFleetCharacterizationSignal(t, runner.slots[0].canceled, "owned cancellation effect")
	archived := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+target.workerSessionID)
	if archived.State != "CANCELED" || archived.TerminalCause == nil || string(*archived.TerminalCause) != "OPERATOR_CANCEL" ||
		!reflect.DeepEqual(live.StartedAt, archived.StartedAt) || archived.EndedAt == nil || archived.DurationMillis == nil {
		t.Fatalf("shutdown archive facts = %+v", archived)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "worker-sessions", "show", "--worker-session-id", target.workerSessionID, "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI shutdown summary: %v %s", err, inputs.Stderr())
	}
	var shown factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(inputs.Stdout()), &shown); err != nil || !reflect.DeepEqual(shown, archived) {
		t.Fatalf("CLI/HTTP shutdown summary disagree: %s (%v)", inputs.Stdout(), err)
	}
	if runner.callCount() != 1 {
		t.Fatalf("closed Runtime admitted %d executions, want one", runner.callCount())
	}
}

func startShutdownCauseServer(t *testing.T, runner *fleetCharacterizationRunner, sessionID string) *fleetCharacterizationServer {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "shutdown-cause")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	home := t.TempDir()
	server := &fleetCharacterizationServer{env: []string{"HOME=" + home, "USERPROFILE=" + home}, dir: dir}
	server.FunctionalAPIServer = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Env: server.env,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
		BeforeStart: func(tb testing.TB, process support.Process, _ root.Input) {
			result, err := process.(support.ApplicationProcess).FactorySessions().Start(tb.Context(), factorysessions.SessionStartRequest{
				SessionID: sessionID, Mode: factorysessions.SessionOperationModeLive, FolderPath: dir, ActivationOnly: true,
				RuntimeSelection: &factorysessions.SessionRuntimeSelection{
					Mode: factorysessions.SessionRuntimeModeService, SystemConfigHome: home,
					DefinitionSourcePath: filepath.Join(dir, "factory.json"), ExecutionBaseDir: dir, RuntimeInstanceID: uuid.NewString(),
					Recording: factorysessions.SessionRecordingSelection{RecordPath: filepath.Join(dir, "shutdown.json")},
				},
			})
			if err != nil || result.SessionID != sessionID {
				tb.Fatalf("activate shutdown Factory Session: %+v %v", result, err)
			}
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(runner.releaseAll)
	return server
}
