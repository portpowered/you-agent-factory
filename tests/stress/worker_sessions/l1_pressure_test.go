package workersessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// This dedicated pressure cell holds one actual capture write in flight while
// a controlled command emits 12MiB. The real Events subscription, journal,
// reducers and public HTTP logs/control boundaries remain wired by root.
// Each parallel cell owns its profile/host/Direct attempt; no executable build
// or provider files. Count capacity (128) cannot explain the 24-record loss.
func TestL1CaptureBytePressure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	runner := newPressureScript(24, 512<<10)
	writer := &pressureWriter{entered: make(chan struct{}), release: make(chan struct{})}
	endpoint, store := startL1Host(t, ctx, prepareEvictionFactory(t), runner, writer)
	t.Cleanup(func() {
		select {
		case <-writer.release:
		default:
			close(writer.release)
		}
	})
	writer.WorkerRecordingStore = store
	id := admitPressureScript(t, ctx, endpoint, runner)
	prefix := readPressurePage(t, ctx, endpoint, id)
	close(runner.release)
	waitEvictionSignal(t, ctx, writer.entered)
	waitEvictionSignal(t, ctx, runner.emitted)
	blocked := readPressurePage(t, ctx, endpoint, id)
	if !reflect.DeepEqual(blocked.Events, prefix.Events) || blocked.CommittedPosition != 2 {
		t.Fatal("in-flight unsynced output advanced public prefix")
	}
	close(writer.release)
	page := waitPressureHealth(t, ctx, endpoint, id, factoryapi.INCOMPLETE, "BACKPRESSURE")
	if page.CommittedPosition <= 2 || page.CommittedPosition >= 26 {
		t.Fatalf("pressure did not drain a bounded admitted prefix: %d", page.CommittedPosition)
	}
	var admittedBytes int
	for index, frame := range page.Events {
		if frame.Event.Position != int64(index+1) {
			t.Fatal("pressure prefix is not contiguous")
		}
		if index >= 2 {
			payload, err := json.Marshal(frame.Event.Payload)
			if err != nil {
				t.Fatal(err)
			}
			admittedBytes += len(payload)
		}
	}
	if admittedBytes > 8<<20 {
		t.Fatalf("admitted in-flight/pending payload exceeded 8MiB: %d", admittedBytes)
	}
	t.Logf("pressure emitted=24 payloadBytes=%d admittedRecords=%d admittedPayloadBytes=%d budget=%d", 512<<10, page.CommittedPosition-2, admittedBytes, 8<<20)
	assertPressureCancel(t, ctx, endpoint, id, runner)
	ended := waitPressureHealth(t, ctx, endpoint, id, factoryapi.DEGRADED, "BACKPRESSURE")
	if !reflect.DeepEqual(ended.Events, page.Events) || ended.CommittedPosition != page.CommittedPosition {
		t.Fatalf("terminal loss changed acknowledged pressure prefix: before position=%d events=%d after position=%d events=%d", page.CommittedPosition, len(page.Events), ended.CommittedPosition, len(ended.Events))
	}
}

// Oversized output crosses the actual script observation/capture/read path,
// rather than a seeded recovery representation. One 3MiB output fits the byte
// admission bound and must be durably available through the artifact selector.
func TestL1CapturedOversizedOutput(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	runner := newPressureScript(1, 3<<20)
	endpoint, _ := startL1Host(t, ctx, prepareEvictionFactory(t), runner)
	id := admitPressureScript(t, ctx, endpoint, runner)
	close(runner.release)
	waitEvictionSignal(t, ctx, runner.emitted)
	waitEvictionCapture(t, ctx, endpoint, id, 3)
	page := readPressurePage(t, ctx, endpoint, id)
	event := page.Events[2].Event
	if event.Truncated == nil || !*event.Truncated || event.ArtifactRef == nil || event.OriginalBytes == nil || event.ReturnedBytes == nil {
		t.Fatal("oversized runtime output lacks artifact/size metadata")
	}
	frame, err := json.Marshal(event)
	if err != nil || len(frame) > 1<<20 {
		t.Fatal("returned event exceeds 1MiB")
	}
	exact := fleetProfileHTTP(t, ctx, http.MethodGet, endpoint+"/worker-sessions/"+id+"/logs?artifactRef="+url.QueryEscape(*event.ArtifactRef), nil)
	if int64(len(exact)) != *event.OriginalBytes {
		t.Fatal("artifact original byte count changed")
	}
	var draft workers.Draft
	if err := json.Unmarshal(exact, &draft); err != nil {
		t.Fatal(err)
	}
	var progress workers.ProgressPayload
	if err := json.Unmarshal(draft.Payload, &progress); err != nil {
		t.Fatal(err)
	}
	if progress.Message != strings.Repeat("x", 3<<20) {
		t.Fatal("artifact changed observed script bytes")
	}
	t.Logf("oversized originalBytes=%d returnedFrameBytes=%d artifact roundtrip exact", len(exact), len(frame))
	assertPressureCancel(t, ctx, endpoint, id, runner)
	waitPressureHealth(t, ctx, endpoint, id, factoryapi.COMPLETE, "")
}

type pressureWriter struct {
	recordings.WorkerRecordingStore
	entered, release chan struct{}
}

func (writer *pressureWriter) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if record.Record.ID.Position == 3 {
		close(writer.entered)
		select {
		case <-writer.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return writer.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}

type pressureScript struct {
	started, release, emitted, canceled chan struct{}
	count, size                         int
}

func newPressureScript(count, size int) *pressureScript {
	return &pressureScript{started: make(chan struct{}), release: make(chan struct{}), emitted: make(chan struct{}), canceled: make(chan struct{}), count: count, size: size}
}

func (runner *pressureScript) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner *pressureScript) RunStreaming(ctx context.Context, _ platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	if observe == nil {
		return platformprocess.CommandResult{}, errors.New("pressure requires streaming")
	}
	observe(platformprocess.OutputStreamStdout, []byte("initial committed progress\n"))
	close(runner.started)
	select {
	case <-runner.release:
		for range runner.count {
			observe(platformprocess.OutputStreamStdout, []byte(strings.Repeat("x", runner.size)))
		}
		close(runner.emitted)
	case <-ctx.Done():
	}
	<-ctx.Done()
	close(runner.canceled)
	return platformprocess.CommandResult{}, ctx.Err()
}

func admitPressureScript(t *testing.T, ctx context.Context, endpoint string, runner *pressureScript) string {
	t.Helper()
	id := "l1-pressure"
	worker := "worker"
	fleetProfileHTTP(t, ctx, http.MethodPost, endpoint+"/worker-sessions", factoryapi.WorkerSessionStartRequest{
		RequestId: id, WorkerSessionId: id, Execution: factoryapi.WorkerSessionResolvedExecution{WorkstationName: "process", WorkerType: &worker,
			Dispatch: factoryapi.WorkerSessionResolvedDispatch{DispatchId: id, WorkstationName: "process", WorkerType: &worker}}})
	waitEvictionSignal(t, ctx, runner.started)
	waitEvictionCapture(t, ctx, endpoint, id, 2)
	return id
}

func readPressurePage(t *testing.T, ctx context.Context, endpoint, id string) factoryapi.WorkerSessionLogPage {
	t.Helper()
	data := fleetProfileHTTP(t, ctx, http.MethodGet, endpoint+"/worker-sessions/"+id+"/logs?limit=100", nil)
	var page factoryapi.WorkerSessionLogPage
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func waitPressureHealth(t *testing.T, ctx context.Context, endpoint, id string, health factoryapi.WorkerSessionLogPageHealth, reason string) factoryapi.WorkerSessionLogPage {
	t.Helper()
	// No public capture-health subscription exists; poll the durable projection,
	// with the context as a failure ceiling rather than a readiness delay.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		var observation factoryapi.WorkerSessionObservation
		data := fleetProfileHTTP(t, ctx, http.MethodGet, endpoint+"/worker-sessions/"+id, nil)
		if err := json.Unmarshal(data, &observation); err != nil {
			t.Fatal(err)
		}
		// Observe failure before sampling its durable prefix: ordinary active
		// pages can already be INCOMPLETE while admitted writes still drain.
		page := readPressurePage(t, ctx, endpoint, id)
		if page.Health == health && (reason == "" || observation.RecordingHealthReason != nil && *observation.RecordingHealthReason == reason) {
			return page
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("health=%s reason=%v, want %s/%s", page.Health, observation.RecordingHealthReason, health, reason)
		}
	}
}

func assertPressureCancel(t *testing.T, ctx context.Context, endpoint, id string, runner *pressureScript) {
	t.Helper()
	fleetProfileHTTP(t, ctx, http.MethodPost, endpoint+"/worker-sessions/"+id+"/cancel", map[string]any{})
	waitEvictionSignal(t, ctx, runner.canceled)
}
