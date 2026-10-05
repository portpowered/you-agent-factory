package customer_journeys_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Only read latency is controlled; writes and subsequent reads use the real
// journal. Cancel after the request reaches storage, rather than before sending.
type capturedReadGate struct {
	recordings.WorkerRecordingStore
	blockNext atomic.Bool
	started   chan struct{}
	detached  chan struct{}
}

func (store *capturedReadGate) ReadWorkerCapturedActivity(ctx context.Context, req recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	if store.blockNext.CompareAndSwap(true, false) {
		store.started <- struct{}{}
		<-ctx.Done()
		store.detached <- struct{}{}
		return recordings.WorkerCapturedActivityPage{}, ctx.Err()
	}
	return store.WorkerRecordingStore.ReadWorkerCapturedActivity(ctx, req)
}

func TestWorkerSessionCapturedLogsObserverCancellation(t *testing.T) {
	t.Parallel()
	server, runner, store, finish := capturedReadCancellationServer(t)
	response := postDirectWorkerSession(t, t.Context(), server.URL(), "observer-request", "observer-worker", "observer-dispatch")
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("start status=%d", response.StatusCode)
	}
	runner.waitStarted(t)
	prefix := readCapturedContinuation(t, server, "observer-worker", "")
	for _, observer := range []string{"CLI", "HTTP"} {
		cancelCapturedRead(t, server, store, observer)
		retry := readCapturedContinuation(t, server, "observer-worker", "")
		if !reflect.DeepEqual(retry, prefix) || runner.wasCanceled() || runner.callCount() != 1 {
			t.Fatalf("%s observer cancellation changed capture or execution", observer)
		}
		shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/observer-worker")
		if shown.State != factoryapi.WorkerSessionObservationStateRunning {
			t.Fatalf("%s cancellation changed Worker state: %s", observer, shown.State)
		}
	}
	close(finish)
	runner.waitCompleted(t)
	ended := waitCapturedTerminal(t, server.URL(), "observer-worker")
	if runner.wasCanceled() || ended.CommittedPosition <= prefix.CommittedPosition || !reflect.DeepEqual(ended.Events[:len(prefix.Events)], prefix.Events) {
		t.Fatal("detached observers changed completed capture")
	}
	assertCapturedLogsCLIHTTPParity(t, server, "observer-worker")
	// The same process still owns the captured history; selecting a stopped
	// host must fail rather than silently switching to that local recording.
	server.Close(t)
	assertCapturedFollowFailure(t, server, "observer-worker", "", "FACTORY_UNREACHABLE", nil)
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", "observer-worker", "--view", "logs", "--server", server.URL(), "--output", "json"})
	err := server.Execute(t, inputs.Input)
	if err == nil || !strings.Contains(inputs.Stderr()+inputs.Stdout(), "FACTORY_UNREACHABLE") || strings.Contains(inputs.Stdout(), "recordingGenerationId") {
		t.Fatalf("selected-host failure used local capture: %v %s %s", err, inputs.Stdout(), inputs.Stderr())
	}
}

func capturedReadCancellationServer(t *testing.T) (*support.FunctionalAPIServer, *functionalWorkerGate, *capturedReadGate, chan struct{}) {
	t.Helper()
	store := &capturedReadGate{started: make(chan struct{}, 1), detached: make(chan struct{}, 1)}
	finish := make(chan struct{})
	runner := newFunctionalWorkerGate(finish)
	dir := support.ScaffoldSingleStepFactory(t, "captured-observer")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	home := t.TempDir()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, WorkerRecordingWriter: store, WorkerRecordingStoreObserver: func(backing recordings.WorkerRecordingStore) { store.WorkerRecordingStore = backing }, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	})
	return server, runner, store, finish
}

func cancelCapturedRead(t *testing.T, server *support.FunctionalAPIServer, store *capturedReadGate, observer string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store.blockNext.Store(true)
	done := make(chan error, 1)
	inputs := support.FakeInputs(ctx, []string{"you", "worker-sessions", "read", "--worker-session-id", "observer-worker", "--view", "logs", "--server", server.URL(), "--output", "json"})
	go func() {
		if observer == "CLI" {
			done <- server.Execute(t, inputs.Input)
			return
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+"/worker-sessions/observer-worker/logs", nil)
		if err == nil {
			var response *http.Response
			response, err = http.DefaultClient.Do(request)
			if response != nil {
				_ = response.Body.Close()
			}
		}
		done <- err
	}()
	waitCapturedReadSignal(t, store.started, "read admission")
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%s canceled read error=%v", observer, err)
		}
		if observer == "CLI" && !strings.Contains(inputs.Stderr()+inputs.Stdout(), "WORKER_SESSION_LOGS_INTERRUPTED") {
			t.Fatalf("CLI cancellation diagnostics: %s %s", inputs.Stderr(), inputs.Stdout())
		}
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("observer did not detach")
	}
	waitCapturedReadSignal(t, store.detached, "storage read cancellation")
}

func waitCapturedReadSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatalf("timed out waiting for %s", name)
	}
}
