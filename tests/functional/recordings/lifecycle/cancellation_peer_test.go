package lifecycle_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func startConcurrentCancellationPeer(t *testing.T, process support.Process, service recordings.Service, runner *cancellationRecordingRunner, inputs *support.CapturedInputs) func() {
	t.Helper()
	const sessionID = "019a07c0-0000-7000-8000-000000000034"
	inputs.Args = append(inputs.Args, "--session", sessionID)
	var once sync.Once
	release := func() { once.Do(func() { close(runner.releasePeer) }) }
	done := executeGatedRecordingCommand(t, process, inputs, release)
	select {
	case <-runner.peerStarted:
	case err := <-done:
		t.Fatalf("concurrent peer ended before provider admission: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent peer provider was never admitted")
	}
	return func() {
		release()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("concurrent invocation affected by selected cancellation: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("concurrent peer did not complete")
		}
		if inputs.Stdout() != "Batch completed successfully.\n" {
			t.Fatalf("concurrent peer output = %q", inputs.Stdout())
		}
		id := recordings.RecordingID("019a07c0-0000-7000-8000-000000000034")
		status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: id})
		if err != nil || status.Status.State != recordings.RecordingFinalized || len(status.Status.Failures) != 0 {
			t.Fatalf("concurrent peer recording = (%#v, %v)", status, err)
		}
		history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: id, Artifact: status.Status.Artifact, Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID},
		}})
		if err != nil || len(history.Dispatches) != 1 || history.Dispatches[0].Status != recordings.FactoryDispatchStatusCompleted {
			t.Fatalf("concurrent peer persisted dispatch = (%#v, %v)", history, err)
		}
	}
}

// The two fact-clock shapes require separate immutable process configurations;
// each process hosts both explicit sessions concurrently.
func TestRecordingSelectedScheduler(t *testing.T) {
	for _, timerFacts := range []bool{false, true} {
		t.Run(fmt.Sprintf("timer-capable-facts=%t", timerFacts), func(t *testing.T) {
			t.Parallel()
			runRecordingSelectedScheduler(t, timerFacts)
		})
	}
}

type recordingNowOnly struct{ source *platformclock.Deterministic }

func (clock recordingNowOnly) Now() time.Time { return clock.source.Now() }

type recordingScheduler struct {
	*platformclock.Deterministic
	armed chan struct{}
	ready chan struct{}
}

func (scheduler *recordingScheduler) NewTimer(d time.Duration) platformclock.Timer {
	if d == 10*time.Millisecond {
		scheduler.ready <- struct{}{}
	}
	timer := scheduler.Deterministic.NewTimer(d)
	if d == recordings.DefaultRecordingFlushInterval {
		scheduler.armed <- struct{}{}
	}
	return timer
}
func recordingSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatal("recording edge did not signal")
	}
}

type selectedRecordingWrites struct {
	selected string
	calls    atomic.Int32
	failNext atomic.Bool
	hold     atomic.Bool
	started  chan struct{}
	release  chan struct{}
	once     sync.Once
	cause    error
}

func (writes *selectedRecordingWrites) write(path string, data []byte) error {
	if path == writes.selected {
		writes.calls.Add(1)
		if writes.failNext.Swap(false) {
			return writes.cause
		}
		if writes.hold.Swap(false) {
			close(writes.started)
			<-writes.release
		}
	}
	return os.WriteFile(path, data, 0o600)
}
func (writes *selectedRecordingWrites) unblock() { writes.once.Do(func() { close(writes.release) }) }

func runRecordingSelectedScheduler(t *testing.T, timerFacts bool) {
	t.Helper()
	at := time.Date(2026, 10, 3, 12, 34, 56, 0, time.UTC)
	facts := platformclock.NewDeterministic(at, time.Second)
	var factSource platformclock.Source = recordingNowOnly{facts}
	if timerFacts {
		factSource = facts
	}
	scheduler := &recordingScheduler{platformclock.NewDeterministic(at.Add(24*time.Hour), recordings.DefaultRecordingFlushInterval), make(chan struct{}, 64), make(chan struct{}, 8)}
	path := filepath.Join(t.TempDir(), "selected.json")
	peerPath := filepath.Join(t.TempDir(), "peer.json")
	selected := cancellationRecordingInputs(t, path)
	peer := cancellationRecordingInputs(t, peerPath)
	peer.WorkingDirectory = peer.Args[3]
	const selectedID = "019a07c0-0000-7000-8000-000000000031"
	const peerID = "019a07c0-0000-7000-8000-000000000034"
	selected.Args = append(selected.Args, "--session", selectedID)
	peer.Args = append(peer.Args, "--session", peerID)
	runner := &cancellationRecordingRunner{started: make(chan struct{}), canceled: make(chan error, 1), peerDirectory: peer.Args[3], peerStarted: make(chan struct{}), releasePeer: make(chan struct{})}
	writes := &selectedRecordingWrites{selected: path, started: make(chan struct{}), release: make(chan struct{}), cause: errors.New("selected periodic storage unavailable")}
	var service recordings.Service
	var nextInstance atomic.Uint32
	process := support.BuildProcess(t, serviceedges.Edges{
		Clock: factSource, ProcessScheduler: scheduler, ProviderCommandRunner: runner,
		FactorySessionRuntimeInstanceIDGenerator: func() string {
			if nextInstance.Add(1) == 1 {
				return selectedID
			}
			return peerID
		},
		RecordingsRootObserver: func(root recordings.Service) { service = root },
		RecordingReadFile:      os.ReadFile, RecordingWriteFile: writes.write,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			return nil
		},
	})
	// Construction and a deferred binding allocate no product timer or artifact.
	deferredPath := filepath.Join(t.TempDir(), "deferred.json")
	bindPublicRecording(t, service, "deferred", "deferred", deferredPath)
	select {
	case <-scheduler.armed:
		t.Fatal("recording cadence activated before Execute")
	default:
	}
	if _, err := os.Stat(deferredPath); !os.IsNotExist(err) {
		t.Fatalf("deferred artifact: %v", err)
	}
	verifySelectedCadence(t, service, process, selected, peer, runner, writes, facts, scheduler, at)
}

func verifySelectedCadence(t *testing.T, service recordings.Service, process support.Process, selected, peer *support.CapturedInputs, runner *cancellationRecordingRunner, writes *selectedRecordingWrites, facts *platformclock.Deterministic, scheduler *recordingScheduler, at time.Time) {
	t.Helper()
	const selectedID = "019a07c0-0000-7000-8000-000000000031"
	const peerID = "019a07c0-0000-7000-8000-000000000034"
	ctx, cancel := context.WithCancel(selected.Context)
	defer cancel()
	selected.Context = ctx
	selectedDone := executeGatedRecordingCommand(t, process, selected, writes.unblock)
	recordingSignal(t, runner.started)

	peerAt := facts.Now()
	finishPeer := startConcurrentCancellationPeer(t, process, service, runner, peer)
	recordingSignal(t, scheduler.ready)
	recordingSignal(t, scheduler.ready)
	scheduler.SetTick(1)
	recordingSignal(t, scheduler.armed)
	recordingSignal(t, scheduler.armed)
	initial, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: selectedID})
	if err != nil {
		t.Fatal(err)
	}
	calls := writes.calls.Load()
	facts.SetTick(3600)
	if writes.calls.Load() != calls {
		t.Fatal("fact clock drove periodic write")
	}
	unchanged, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: selectedID})
	if err != nil || unchanged.Status.FlushedThrough.Sequence != initial.Status.FlushedThrough.Sequence {
		t.Fatalf("fact timers changed watermark: %#v %v", unchanged, err)
	}
	appendSelectedWorkFact(t, service, selectedID, "selected-periodic-work", facts.Now())
	appendSelectedWorkFact(t, service, peerID, "peer-periodic-work", facts.Now())
	writes.failNext.Store(true)
	scheduler.SetTick(2)
	failed := waitSelectedRecordingStatus(t, service, selectedID, func(status recordings.RecordingStatusFacts) bool { return len(status.Failures) > 0 })
	if failed.FlushedThrough.Sequence != initial.Status.FlushedThrough.Sequence || failed.Failures[0].Code != "periodic_flush_failed" {
		t.Fatalf("failed periodic watermark: %#v", failed)
	}
	recordingSignal(t, scheduler.armed)
	recordingSignal(t, scheduler.armed)
	scheduler.SetTick(3)
	durable := waitSelectedRecordingStatus(t, service, selectedID, func(status recordings.RecordingStatusFacts) bool {
		return status.FlushedThrough != nil && status.FlushedThrough.Sequence >= recordings.CanonicalEventSequence(status.AcceptedEvents-1)
	})
	assertSelectedLiveHistory(t, service, durable, at)
	peerStatus := waitSelectedRecordingStatus(t, service, peerID, func(status recordings.RecordingStatusFacts) bool {
		return status.FlushedThrough != nil && status.FlushedThrough.Sequence >= recordings.CanonicalEventSequence(status.AcceptedEvents-1)
	})
	assertSelectedLiveHistory(t, service, peerStatus, peerAt)
	recordingSignal(t, scheduler.armed)
	recordingSignal(t, scheduler.armed)
	assertSelectedCancellation(t, service, scheduler, writes, facts.Now(), selectedID, cancel, selectedDone)
	verifySelectedRecordingPeer(t, service, scheduler, writes, selectedID, peerID, finishPeer)
}

func assertSelectedCancellation(t *testing.T, service recordings.Service, scheduler *recordingScheduler, writes *selectedRecordingWrites, at time.Time, selectedID string, cancel context.CancelFunc, selectedDone <-chan error) {
	t.Helper()
	// Add a public live fact so the next periodic snapshot is dirty.
	appendSelectedWorkFact(t, service, selectedID, "selected-after-retry", at)
	writes.hold.Store(true)
	scheduler.SetTick(4)
	recordingSignal(t, writes.started)
	cancel()
	select {
	case err := <-selectedDone:
		t.Fatalf("Execute failed to join held periodic writer: %v", err)
	default:
	}
	writes.unblock()
	select {
	case err := <-selectedDone:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, writes.cause) {
			t.Fatalf("joined cancellation/periodic error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("selected Execute did not join")
	}

}

func waitSelectedRecordingStatus(t *testing.T, service recordings.Service, id recordings.RecordingID, accept func(recordings.RecordingStatusFacts) bool) recordings.RecordingStatusFacts {
	t.Helper()
	// The write edge completes before the public watermark is published. There
	// is no completion notification for that projection, so observe its public
	// status with the bounded repository helper after deterministic edge signals.
	result, err := support.WaitForObservation(10*time.Second, func() (recordings.RecordingStatusResult, error) {
		return service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: id})
	}, func(result recordings.RecordingStatusResult) bool { return accept(result.Status) })
	if err != nil {
		t.Fatal(err)
	}
	return result.Status
}
func assertSelectedLiveHistory(t *testing.T, service recordings.Service, status recordings.RecordingStatusFacts, startupAt time.Time) {
	t.Helper()
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: recordings.HistoricalRecordingIdentity{RecordingID: status.RecordingID, Artifact: status.Artifact, Scope: status.Scope}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	retained, err := service.SubscribeFrom(ctx, recordings.SubscribeRequest{Scope: status.Scope})
	if err != nil {
		t.Fatal(err)
	}
	live := make(map[recordings.CanonicalEventID]recordings.CanonicalEvent)
	for range retained.RetainedEventCount {
		outcome := retained.Subscription.Next(ctx)
		if outcome.Kind != recordings.SubscriptionEvent {
			t.Fatalf("live subscription: %#v", outcome)
		}
		live[outcome.Event.ID] = outcome.Event
	}
	seen := map[recordings.CanonicalEventKind]bool{}
	for _, event := range history.Events {
		seen[event.Kind] = true
		want, ok := live[event.ID]
		if !ok {
			// Startup and state frames are absent from the scoped runtime
			// subscription. Their persisted timestamps still belong to the fact clock.
			if (event.Sequence >= 2 && event.Kind != "FACTORY_STATE_RESPONSE") || !event.RecordedAt.Equal(startupAt) {
				t.Fatalf("unexpected unscoped history: %#v", event)
			}
			continue
		}
		assertPublicHistoricalFact(t, event, want)
	}
	for _, kind := range []recordings.CanonicalEventKind{"RUN_REQUEST", "WORK_REQUEST", "DISPATCH_REQUEST"} {
		if !seen[kind] {
			t.Fatalf("live history lacks %s", kind)
		}
	}
	if seen["RUN_RESPONSE"] || status.FinalizedAt != nil {
		t.Fatal("periodic witness came from terminal flush")
	}
	if history.Events[len(history.Events)-1].Sequence != status.FlushedThrough.Sequence {
		t.Fatal("durable prefix and watermark differ")
	}
}
func verifySelectedRecordingPeer(t *testing.T, service recordings.Service, scheduler *recordingScheduler, writes *selectedRecordingWrites, selectedID, peerID recordings.RecordingID, finishPeer func()) {
	t.Helper()
	calls := writes.calls.Load()
	before, err := os.ReadFile(writes.selected)
	if err != nil {
		t.Fatal(err)
	}
	fact := appendSelectedWorkFact(t, service, string(peerID), "peer-after-selected-close", scheduler.Now())
	scheduler.SetTick(5)
	waitSelectedRecordingStatus(t, service, peerID, func(status recordings.RecordingStatusFacts) bool {
		return status.FlushedThrough != nil && status.FlushedThrough.Sequence >= fact.Sequence
	})
	finishPeer()
	scheduler.SetTick(100)
	after, err := os.ReadFile(writes.selected)
	if err != nil || writes.calls.Load() != calls || string(before) != string(after) {
		t.Fatalf("selected writes changed after join: calls=%d -> %d, err=%v", calls, writes.calls.Load(), err)
	}
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: selectedID})
	if err != nil || status.Status.FinalizedAt == nil {
		t.Fatalf("selected terminal status: %#v %v", status, err)
	}
}

func appendSelectedWorkFact(t *testing.T, service recordings.Service, sessionID, id string, at time.Time) recordings.CanonicalEvent {
	t.Helper()
	payload, err := json.Marshal(work.WorkRequest{Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{Name: id, WorkTypeID: "task", Payload: map[string]string{"title": "live recording cadence witness"}}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Append(recordings.AppendRecordedEventRequest{Event: recordings.CanonicalEvent{
		ID: recordings.CanonicalEventID(id), Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID},
		RecordedAt: at, Kind: "WORK_REQUEST", Payload: string(payload),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return result.Event
}
