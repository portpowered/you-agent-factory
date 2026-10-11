package lifecycle_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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

// Claims and provider gates are path/Factory-owned. One immutable process hosts
// every parallel fault cell, each with two explicit Sessions and private homes.
func TestRecordingSelectedClaimLifetime(t *testing.T) {
	runner := &ownershipRunner{gates: make(map[string]*ownershipGate)}
	claims := &ownershipClaims{routes: make(map[string]*ownershipRoute)}
	var service recordings.Service
	var writes atomic.Int32
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		RecordingWriteFile:    func(path string, data []byte) error { writes.Add(1); return os.WriteFile(path, data, 0o600) },
		ProviderCommandRunner: runner, RecordingTargetClaim: claims.acquire,
		RecordingsRootObserver: func(root recordings.Service) { service = root },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := process.Close(context.Background()); err != nil {
			t.Error(err)
		}
		claims.mu.Lock()
		defer claims.mu.Unlock()
		for path, route := range claims.routes {
			if route.closes > 0 && route.held {
				t.Errorf("close retry retained target %q", path)
			}
		}
	})
	if claims.calls.Load() != 0 || runner.calls.Load() != 0 || writes.Load() != 0 {
		t.Fatal("construction performed recording/provider effects")
	}
	for index, mode := range []string{"success", "busy", "acquisition-failure", "release-failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			runSelectedClaimLifetime(t, index, mode, process, service, runner, claims)
		})
	}
}

func runSelectedClaimLifetime(t *testing.T, index int, mode string, process support.Process, service recordings.Service, runner *ownershipRunner, claims *ownershipClaims) {
	t.Helper()
	id := fmt.Sprintf("019a07c0-0000-7000-8000-%012d", 100+index*3)
	peerID := fmt.Sprintf("019a07c0-0000-7000-8000-%012d", 101+index*3)
	pattern := filepath.Join(t.TempDir(), "__factory_session_id__.json")
	path := strings.ReplaceAll(pattern, "__factory_session_id__", id)
	peerPath := strings.ReplaceAll(pattern, "__factory_session_id__", peerID)
	selected := selectedClaimRoute(t, path, mode)
	t.Cleanup(func() { claims.mu.Lock(); selected.releaseErr = nil; claims.mu.Unlock() })
	peer := &ownershipRoute{}
	claims.mu.Lock()
	claims.routes[path], claims.routes[peerPath] = selected, peer
	claims.mu.Unlock()
	peerInputs := cancellationRecordingInputs(t, pattern)
	peerInputs.WorkingDirectory = peerInputs.Args[3]
	peerInputs.Args = append(peerInputs.Args, "--session", peerID)
	gate := &ownershipGate{started: make(chan struct{}), release: make(chan struct{})}
	runner.mu.Lock()
	runner.gates[peerInputs.Args[3]] = gate
	runner.mu.Unlock()
	var once sync.Once
	release := func() { once.Do(func() { close(gate.release) }) }
	peerDone := executeGatedRecordingCommand(t, process, peerInputs, release)
	select {
	case <-gate.started:
	case err := <-peerDone:
		t.Fatalf("peer failed before readiness: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("peer not ready")
	}
	inputs := cancellationRecordingInputs(t, pattern)
	inputs.Args = append(inputs.Args, "--session", id)
	err := process.Execute(inputs.Input)
	assertSelectedClaimOutcome(t, service, id, path, mode, inputs, selected.cause, err)
	claims.mu.Lock()
	if selected.calls != 1 || peer.calls != 1 || !peer.held || peer.closes != 0 {
		t.Error("selected failure/stop released peer or acquired repeatedly")
	}
	if selected.held != (mode == "busy" || mode == "release-failure") {
		t.Error("incorrect selected ownership after close")
	}
	selected.releaseErr = nil // Permit the existing public close retry at cleanup.
	claims.mu.Unlock()
	release()
	if err := <-peerDone; err != nil {
		t.Fatal(err)
	}
	assertOwnershipHistory(t, service, peerID, peerPath)
	if mode == "success" {
		assertLaterClaimAndDisabledRecording(t, process, claims, selected, pattern, id)
	}
}

func selectedClaimRoute(t *testing.T, path, mode string) *ownershipRoute {
	t.Helper()
	selected := &ownershipRoute{cause: errors.New("PRIVATE_CLAIM_CAUSE")}
	if mode == "acquisition-failure" {
		selected.acquireErr = selected.cause
	}
	if mode == "release-failure" {
		selected.releaseErr = selected.cause
	}
	if mode == "busy" {
		if err := os.WriteFile(path, []byte("existing external owner"), 0o600); err != nil {
			t.Fatal(err)
		}
		selected.held = true
	}
	return selected
}

func assertLaterClaimAndDisabledRecording(t *testing.T, process support.Process, claims *ownershipClaims, selected *ownershipRoute, pattern, id string) {
	t.Helper()
	again := cancellationRecordingInputs(t, pattern)
	again.Args = append(again.Args, "--session", id)
	if err := process.Execute(again.Input); err != nil {
		t.Fatal(err)
	}
	claims.mu.Lock()
	calls, held := selected.calls, selected.held
	claims.mu.Unlock()
	if calls != 2 || held {
		t.Fatalf("later ownership calls=%d held=%t", calls, held)
	}
	disabled := cancellationRecordingInputs(t, pattern)
	disabled.Args = append(disabled.Args[:len(disabled.Args)-2], "--no-record", "--session", "019a07c0-0000-7000-8000-000000000199")
	if err := process.Execute(disabled.Input); err != nil {
		t.Fatal(err)
	}
	claims.mu.Lock()
	calls = selected.calls
	claims.mu.Unlock()
	if calls != 2 {
		t.Fatal("disabled recording acquired ownership")
	}
}

func assertSelectedClaimOutcome(t *testing.T, service recordings.Service, id, path, mode string, inputs *support.CapturedInputs, cause, err error) {
	t.Helper()
	if mode == "success" {
		if err != nil {
			t.Fatal(err)
		}
		assertOwnershipHistory(t, service, id, path)
	} else if mode == "release-failure" {
		if !errors.Is(err, cause) {
			t.Fatalf("release cause lost: %v", err)
		}
	} else {
		var coded interface{ CLIErrorCode() string }
		if !errors.Is(err, recordings.ErrRecordingBindingConflict) || !errors.As(err, &coded) || coded.CLIErrorCode() != "RECORDING_TARGET_CONFLICT" {
			t.Fatalf("claim refusal lost typed conflict: %v", err)
		}
		if mode == "acquisition-failure" && !errors.Is(err, cause) {
			t.Fatalf("acquisition cause lost: %v", err)
		}
		if strings.Contains(inputs.Stderr(), cause.Error()) {
			t.Fatal("private acquisition cause exposed")
		}
		if mode == "busy" {
			if data, err := os.ReadFile(path); err != nil || string(data) != "existing external owner" {
				t.Fatalf("busy target bytes changed: %q, %v", data, err)
			}
		} else if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed acquisition published artifact: %v", err)
		}
	}
}

type ownershipGate struct{ started, release chan struct{} }
type ownershipRunner struct {
	mu    sync.Mutex
	gates map[string]*ownershipGate
	calls atomic.Int32
}

func (runner *ownershipRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	runner.mu.Lock()
	gate := runner.gates[request.WorkDir]
	runner.mu.Unlock()
	if gate != nil {
		close(gate.started)
		select {
		case <-gate.release:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return support.NewStaticSuccessCommandRunner("ownership COMPLETE").Run(ctx, request)
}

type ownershipRoute struct {
	cause, acquireErr, releaseErr error
	held                          bool
	calls, closes                 int
}
type ownershipClaims struct {
	mu     sync.Mutex
	routes map[string]*ownershipRoute
	calls  atomic.Int32
}

func (claims *ownershipClaims) acquire(ctx context.Context, path, marker string) (io.Closer, error) {
	claims.calls.Add(1)
	key, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	if marker != key+".recording.lock" || ctx.Err() != nil {
		return nil, fmt.Errorf("incorrect claim arguments")
	}
	claims.mu.Lock()
	defer claims.mu.Unlock()
	route := claims.routes[path]
	if route == nil {
		return nil, fmt.Errorf("unexpected recording destination %q", path)
	}
	route.calls++
	if route.acquireErr != nil {
		return nil, route.acquireErr
	}
	if route.held {
		return nil, recordings.ErrRecordingBindingConflict
	}
	route.held = true
	return &ownershipLease{claims: claims, route: route}, nil
}

type ownershipLease struct {
	claims *ownershipClaims
	route  *ownershipRoute
}

func (lease *ownershipLease) Close() error {
	lease.claims.mu.Lock()
	defer lease.claims.mu.Unlock()
	lease.route.closes++
	if lease.route.releaseErr != nil {
		return lease.route.releaseErr
	}
	lease.route.held = false
	return nil
}
func assertOwnershipHistory(t *testing.T, service recordings.Service, id, path string) {
	t.Helper()
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: recordings.HistoricalRecordingIdentity{
		RecordingID: recordings.RecordingID(id), Artifact: recordings.RecordingArtifactReference(path), Scope: recordings.CanonicalEventScope{FactorySessionID: id},
	}})
	if err != nil || len(history.Dispatches) != 1 || history.Dispatches[0].Status != recordings.FactoryDispatchStatusCompleted {
		t.Fatalf("owned history = %#v, %v", history, err)
	}
}
