package http_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const capturedStorageFault = "storage fault secret=captured-storage-sentinel"

// The store remains a real journal. Only the selected external write effect
// fails; reads, reduction, transport mapping and execution use production code.
type capturedFaultStore struct {
	recordings.WorkerRecordingStore
	acceptedThrough uint64
	failureOnce     sync.Once
	failed          chan struct{}
	controlFailure  string
}

func (store *capturedFaultStore) BeginWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	if store.controlFailure == "intent" {
		return recordings.WorkerControlOperationRecord{}, false, errors.New(capturedStorageFault)
	}
	return store.WorkerRecordingStore.BeginWorkerControlOperation(ctx, record)
}

func (store *capturedFaultStore) AdvanceWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord, expected uint64) (recordings.WorkerControlOperationRecord, error) {
	if store.controlFailure == "result" {
		return recordings.WorkerControlOperationRecord{}, errors.New(capturedStorageFault)
	}
	return store.WorkerRecordingStore.AdvanceWorkerControlOperation(ctx, record, expected)
}

func (store *capturedFaultStore) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if uint64(record.Record.ID.Position) > store.acceptedThrough {
		store.failureOnce.Do(func() { close(store.failed) })
		return errors.New(capturedStorageFault)
	}
	return store.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}

func capturedFailureServer(t *testing.T, acceptedThrough uint64, controlFailure ...string) (*support.FunctionalAPIServer, *functionalWorkerGate, *capturedFaultStore) {
	t.Helper()
	fault := &capturedFaultStore{acceptedThrough: acceptedThrough, failed: make(chan struct{})}
	if len(controlFailure) > 0 {
		fault.controlFailure = controlFailure[0]
	}
	runner := newFunctionalWorkerGate(make(chan struct{}))
	dir := support.ScaffoldSingleStepFactory(t, "captured-failure")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	home := t.TempDir()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, WorkerRecordingWriter: fault, WorkerRecordingStoreObserver: func(store recordings.WorkerRecordingStore) { fault.WorkerRecordingStore = store }, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	})
	return server, runner, fault
}

// HTTP owns the stop/error contract; the remote CLI observes the same durable
// captured health through its customer read command. Production composition
// and the journal remain real, with only the selected storage effect failing.
func TestExactStopControlPersistenceLossKeepsLiveStopAvailable(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"intent", "result"} {
		for _, action := range []string{"cancel", "terminate"} {
			t.Run(phase+"/"+action, func(t *testing.T) {
				t.Parallel()
				server, runner, _ := capturedFailureServer(t, ^uint64(0), phase)
				response := postDirectWorkerSession(t, t.Context(), server.URL(), "stop-request", "stop-worker", "stop-dispatch")
				_ = response.Body.Close()
				if response.StatusCode != http.StatusAccepted {
					t.Fatalf("admission status=%d", response.StatusCode)
				}
				runner.waitStarted(t)
				prefix := readCapturedContinuation(t, server, "stop-worker", "")
				stopped := postWorkerSessionControl(t, server.URL(), "stop-worker", action)
				assertExactStopDurableLossResponse(t, stopped)
				runner.waitCanceled(t)
				shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/stop-worker")
				want := factoryapi.WorkerSessionObservationStateCanceled
				if action == "terminate" {
					want = factoryapi.WorkerSessionObservationStateTerminated
				}
				if shown.State != want || runner.callCount() != 1 {
					t.Fatalf("joined stop state=%s calls=%d", shown.State, runner.callCount())
				}
				if shown.TerminalCause != nil {
					t.Fatalf("uncommitted stop claimed terminal cause %s", *shown.TerminalCause)
				}
				logs := support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/stop-worker/logs")
				if logs.Health != factoryapi.DEGRADED || logs.CommittedPosition < prefix.CommittedPosition || len(logs.Events) < len(prefix.Events) || !reflect.DeepEqual(logs.Events[:len(prefix.Events)], prefix.Events) {
					t.Fatalf("degraded stopped capture=%+v", logs)
				}
				cli := readCapturedContinuation(t, server, "stop-worker", "")
				if !reflect.DeepEqual(cli, logs) {
					t.Fatal("CLI/HTTP durable-loss logs differ")
				}
			})
		}
	}
}

func assertExactStopDurableLossResponse(t *testing.T, response *http.Response) {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "WORKER_SESSION_CONTROL_FAILED") || strings.Contains(string(body), "captured-storage-sentinel") {
		t.Fatalf("durable-loss status=%d body=%s", response.StatusCode, body)
	}
}

func TestWorkerSessionCapturedLogsOpeningFailure(t *testing.T) {
	t.Parallel()
	server, runner, _ := capturedFailureServer(t, 0)
	response := postDirectWorkerSession(t, t.Context(), server.URL(), "opening-request", "opening-worker", "opening-dispatch")
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusServiceUnavailable || strings.Contains(string(body), "captured-storage-sentinel") || runner.callCount() != 0 {
		t.Fatalf("opening failure status=%d calls=%d body=%s", response.StatusCode, runner.callCount(), body)
	}
	logs, err := http.Get(server.URL() + "/worker-sessions/opening-worker/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Body.Close()
	if logs.StatusCode != http.StatusNotFound {
		t.Fatalf("uncommitted opening logs status=%d", logs.StatusCode)
	}
}

func TestWorkerSessionCapturedLogsTerminalWriteFailure(t *testing.T) {
	t.Parallel()
	server, runner, fault := capturedFailureServer(t, 1)
	response := postDirectWorkerSession(t, t.Context(), server.URL(), "midrun-request", "midrun-worker", "midrun-dispatch")
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("start status=%d", response.StatusCode)
	}
	runner.waitStarted(t)
	prefix := readCapturedContinuation(t, server, "midrun-worker", "")
	if prefix.CommittedPosition != 1 || len(prefix.Events) != 1 || prefix.Events[0].Event.CapturedAt == nil {
		t.Fatalf("failed write escaped committed prefix: %+v", prefix)
	}
	cancel := postWorkerSessionControl(t, server.URL(), "midrun-worker", "cancel")
	_ = cancel.Body.Close()
	if cancel.StatusCode != http.StatusOK {
		t.Fatalf("live cancel unavailable after capture failure: %d", cancel.StatusCode)
	}
	runner.waitCanceled(t)
	select {
	case <-fault.failed:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("terminal did not reach injected write failure")
	}
	ended, err := support.WaitForObservation(functionalWorkerSignalTimeout, func() (factoryapi.WorkerSessionLogPage, error) {
		return support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/midrun-worker/logs"), nil
	}, func(page factoryapi.WorkerSessionLogPage) bool {
		return page.Health == factoryapi.DEGRADED
	})
	if err != nil {
		t.Fatal(err)
	}
	if ended.CommittedPosition != prefix.CommittedPosition || !reflect.DeepEqual(ended.Events, prefix.Events) {
		t.Fatal("terminal loss changed accepted records or watermark")
	}
	cli := readCapturedContinuation(t, server, "midrun-worker", "")
	if !reflect.DeepEqual(cli, ended) {
		t.Fatal("CLI/HTTP degraded prefixes differ")
	}
	repeated := postWorkerSessionControl(t, server.URL(), "midrun-worker", "cancel")
	_ = repeated.Body.Close()
	if repeated.StatusCode != http.StatusOK {
		t.Fatalf("cancel unavailable after capture loss: %d", repeated.StatusCode)
	}
}
