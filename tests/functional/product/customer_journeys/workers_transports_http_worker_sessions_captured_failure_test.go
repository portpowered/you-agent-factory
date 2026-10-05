package customer_journeys_test

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
}

func (store *capturedFaultStore) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if uint64(record.Record.ID.Position) > store.acceptedThrough {
		store.failureOnce.Do(func() { close(store.failed) })
		return errors.New(capturedStorageFault)
	}
	return store.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}

func capturedFailureServer(t *testing.T, acceptedThrough uint64) (*support.FunctionalAPIServer, *functionalWorkerGate, *capturedFaultStore) {
	t.Helper()
	fault := &capturedFaultStore{acceptedThrough: acceptedThrough, failed: make(chan struct{})}
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
	assertCapturedFollowFailure(t, server, "midrun-worker", "", "WORKER_SESSION_LOGS_GAP", ended.Events)
	assertCapturedFollowFailure(t, server, "midrun-worker", "invalid", "BAD_REQUEST", nil)
	repeated := postWorkerSessionControl(t, server.URL(), "midrun-worker", "cancel")
	_ = repeated.Body.Close()
	if repeated.StatusCode != http.StatusOK {
		t.Fatalf("cancel unavailable after capture loss: %d", repeated.StatusCode)
	}
}
