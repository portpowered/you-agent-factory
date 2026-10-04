package service

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jonboulle/clockwork"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	"go.uber.org/zap"
)

const testDebounceWindow = 50 * time.Millisecond

// Each registration acknowledges the event that replaced the debounce timer;
// a waiter count alone can still describe the previous event's timer.
type registeringDebounceClock struct {
	clockwork.Clock
	registered chan struct{}
}

func (c *registeringDebounceClock) AfterFunc(d time.Duration, fn func()) clockwork.Timer {
	timer := c.Clock.AfterFunc(d, fn)
	c.registered <- struct{}{}
	return timer
}

func publishDebounceEvent(t *testing.T, events *scriptedEventWatcher, clock *registeringDebounceClock, event fsnotify.Event) {
	t.Helper()
	events.events <- event
	select {
	case <-clock.registered:
	case <-time.After(time.Second): //nolint:testsleep // Failure ceiling only; timer registration is the readiness signal.
		t.Fatal("event did not register its debounce timer")
	}
}

func TestDebounceCancelJoinsRunningCallbackAndRetiresReplacement(t *testing.T) {
	t.Parallel()
	clock := clockwork.NewFakeClock()
	scheduler := newDebounceScheduler(clock, testDebounceWindow)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var replacementCalls atomic.Int32
	scheduler.schedule("same", func() { close(started); <-release; close(finished) })
	clock.Advance(testDebounceWindow)
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		t.Fatal("callback did not start")
	}
	scheduler.schedule("same", func() { replacementCalls.Add(1) })
	done := make(chan struct{})
	go func() { scheduler.cancelAll(); close(done) }()
	select {
	case <-done:
		t.Fatal("cancel returned before running callback joined")
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("cancel did not join")
	}
	select {
	case <-finished:
	default:
		t.Fatal("running callback not finished")
	}
	clock.Advance(testDebounceWindow)
	scheduler.schedule("later", func() { replacementCalls.Add(1) })
	clock.Advance(testDebounceWindow)
	if replacementCalls.Load() != 0 {
		t.Fatal("retired scheduler admitted another callback")
	}
}

func newDebouncedTestWatcher(
	dir string,
	submitter *recordingSubmitter,
	clock clockwork.Clock,
	eventWatcher *scriptedEventWatcher,
) *watcher {
	fw := newWatcher(filesystemwatchers.Config{
		Dir:            dir,
		Logger:         zap.NewNop(),
		Files:          localInputFiles{},
		WalkDirectory:  filepath.WalkDir,
		WorkRequestIDs: testWorkRequestIDGenerator,
		Submitter:      submitter.Submit,
		Clock:          clock,
		DebounceWindow: testDebounceWindow,
	})
	fw.newWatcher = func() (fileEventWatcher, error) { return eventWatcher, nil }
	return fw
}

func startDebouncedWatch(t *testing.T, fw *watcher, eventWatcher *scriptedEventWatcher) (context.CancelFunc, <-chan error) {
	t.Helper()
	discovered := make(chan struct{})
	var discoveryOnce sync.Once
	walkDirectory := fw.walkDirectory
	fw.walkDirectory = func(root string, fn fs.WalkDirFunc) error {
		err := walkDirectory(root, fn)
		discoveryOnce.Do(func() { close(discovered) })
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- fw.Watch(ctx) }()
	waitForRegisteredDirectory(t, eventWatcher.added, fw.dir)
	select {
	case <-discovered:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("initial directory discovery did not complete")
	}
	return cancel, done
}

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func advanceDebounce(t *testing.T, clock *clockwork.FakeClock) {
	t.Helper()
	waitForFakeClockWaiters(t, clock, 1)
	clock.Advance(testDebounceWindow)
}

func TestFileWatcher_DebounceSingleEventSettlesOnce(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "single.md")
	content := []byte("single event")

	clock := clockwork.NewFakeClock()
	submitter := &recordingSubmitter{submitted: make(chan struct{}, 1)}
	eventWatcher := newScriptedEventWatcher()
	fw := newDebouncedTestWatcher(dir, submitter, clock, eventWatcher)
	cancel, done := startDebouncedWatch(t, fw, eventWatcher)
	defer cancel()

	// Complete empty-directory discovery before publishing the single event.
	if err := writeLocalFile(path, content); err != nil {
		t.Fatal(err)
	}
	eventWatcher.events <- fsnotify.Event{Name: path, Op: fsnotify.Create}
	advanceDebounce(t, clock)

	select {
	case <-submitter.submitted:
	case <-time.After(time.Second):
		t.Fatal("expected one submission after debounce settled")
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1", got)
	}

	cancel()
	waitForWatchDone(t, done)
}

func TestFileWatcher_DebounceCoalescesBurstToOneSubmit(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "burst.md")
	finalContent := []byte("settled content")
	clock := clockwork.NewFakeClock()
	registeredClock := &registeringDebounceClock{Clock: clock, registered: make(chan struct{}, 1)}
	submitter := &recordingSubmitter{submitted: make(chan struct{}, 1)}
	eventWatcher := newScriptedEventWatcher()
	fw := newDebouncedTestWatcher(dir, submitter, registeredClock, eventWatcher)
	cancel, done := startDebouncedWatch(t, fw, eventWatcher)
	defer cancel()

	if err := writeLocalFile(path, []byte("draft")); err != nil {
		t.Fatal(err)
	}
	publishDebounceEvent(t, eventWatcher, registeredClock, fsnotify.Event{Name: path, Op: fsnotify.Create})

	if err := writeLocalFile(path, finalContent); err != nil {
		t.Fatal(err)
	}
	publishDebounceEvent(t, eventWatcher, registeredClock, fsnotify.Event{Name: path, Op: fsnotify.Write})
	advanceDebounce(t, clock)

	select {
	case <-submitter.submitted:
	case <-time.After(time.Second):
		t.Fatal("expected coalesced submission after debounce settled")
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1", got)
	}
	requests := submitter.getWorkRequests()
	if got := string(requests[0].Works[0].Payload.([]byte)); got != string(finalContent) {
		t.Fatalf("payload = %q, want %q", got, finalContent)
	}

	cancel()
	waitForWatchDone(t, done)
}

func TestFileWatcher_DebounceIndependentPathsSubmitSeparately(t *testing.T) {
	dir := setupWatchDir(t)
	pathA := filepath.Join(dir, "request", "default", "a.md")
	pathB := filepath.Join(dir, "request", "default", "b.md")
	if err := writeLocalFile(pathA, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalFile(pathB, []byte("b")); err != nil {
		t.Fatal(err)
	}

	clock := clockwork.NewFakeClock()
	submitter := &recordingSubmitter{submitted: make(chan struct{}, 2)}
	eventWatcher := newScriptedEventWatcher()
	fw := newDebouncedTestWatcher(dir, submitter, clock, eventWatcher)
	cancel, done := startDebouncedWatch(t, fw, eventWatcher)
	defer cancel()

	// Discovery schedules both existing paths. Sending duplicate events here
	// can replace a timer after advancement, so observe their actual registration.
	waitForFakeClockWaiters(t, clock, 2)

	clock.Advance(testDebounceWindow)
	waitForSubmitCount(t, submitter, 2)

	cancel()
	waitForWatchDone(t, done)
}

func TestFileWatcher_DebounceCancelDuringWindowSkipsSubmit(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "cancel.md")
	if err := writeLocalFile(path, []byte("pending")); err != nil {
		t.Fatal(err)
	}

	clock := clockwork.NewFakeClock()
	submitter := &recordingSubmitter{}
	eventWatcher := newScriptedEventWatcher()
	fw := newDebouncedTestWatcher(dir, submitter, clock, eventWatcher)
	cancel, done := startDebouncedWatch(t, fw, eventWatcher)

	eventWatcher.events <- fsnotify.Event{Name: path, Op: fsnotify.Create}
	waitForFakeClockWaiters(t, clock, 1)

	cancel()
	waitForWatchDone(t, done)

	clock.Advance(testDebounceWindow)
	if got := submitter.submitCallCount(); got != 0 {
		t.Fatalf("submit call count = %d, want 0 after cancellation", got)
	}
}

func TestFileWatcher_StoppingOneWatcherPreservesPeerPendingAdmission(t *testing.T) {
	clock := clockwork.NewFakeClock()
	dirA, dirB := setupWatchDir(t), setupWatchDir(t)
	pathA := filepath.Join(dirA, "request", "default", "a.md")
	pathB := filepath.Join(dirB, "request", "default", "b.md")
	for path, content := range map[string]string{pathA: "stopped", pathB: "peer"} {
		if err := writeLocalFile(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	submitA := &recordingSubmitter{}
	submitB := &recordingSubmitter{submitted: make(chan struct{}, 1)}
	eventsA, eventsB := newScriptedEventWatcher(), newScriptedEventWatcher()
	watchA := newDebouncedTestWatcher(dirA, submitA, clock, eventsA)
	watchB := newDebouncedTestWatcher(dirB, submitB, clock, eventsB)
	cancelA, doneA := startDebouncedWatch(t, watchA, eventsA)
	t.Cleanup(cancelA)
	cancelB, doneB := startDebouncedWatch(t, watchB, eventsB)
	t.Cleanup(func() {
		cancelB()
		waitForWatchDone(t, doneB)
	})
	eventsA.events <- fsnotify.Event{Name: pathA, Op: fsnotify.Create}
	waitForFakeClockWaiters(t, clock, 1)
	eventsB.events <- fsnotify.Event{Name: pathB, Op: fsnotify.Create}
	waitForFakeClockWaiters(t, clock, 2)

	cancelA()
	waitForWatchDone(t, doneA)
	clock.Advance(testDebounceWindow)
	waitForSubmitCount(t, submitB, 1)
	if got := submitA.submitCallCount(); got != 0 {
		t.Fatalf("stopped watcher admitted %d Work Requests, want 0", got)
	}
	requests := submitB.getWorkRequests()
	if len(requests) != 1 || string(requests[0].Works[0].Payload.([]byte)) != "peer" {
		t.Fatalf("peer Work Requests = %#v, want one with peer payload", requests)
	}
}

func TestFileWatcher_DebounceEquivalentSequencesProduceSameSubmitCount(t *testing.T) {
	runSequence := func() int {
		dir := setupWatchDir(t)
		path := filepath.Join(dir, "request", "default", "repeat.md")
		clock := clockwork.NewFakeClock()
		registeredClock := &registeringDebounceClock{Clock: clock, registered: make(chan struct{}, 1)}
		submitter := &recordingSubmitter{submitted: make(chan struct{}, 1)}
		eventWatcher := newScriptedEventWatcher()
		fw := newDebouncedTestWatcher(dir, submitter, registeredClock, eventWatcher)
		cancel, done := startDebouncedWatch(t, fw, eventWatcher)
		defer cancel()

		if err := writeLocalFile(path, []byte("repeat")); err != nil {
			t.Fatal(err)
		}
		publishDebounceEvent(t, eventWatcher, registeredClock, fsnotify.Event{Name: path, Op: fsnotify.Create})
		publishDebounceEvent(t, eventWatcher, registeredClock, fsnotify.Event{Name: path, Op: fsnotify.Write})
		advanceDebounce(t, clock)

		select {
		case <-submitter.submitted:
		case <-time.After(time.Second):
			t.Fatal("expected submission")
		}
		count := submitter.submitCallCount()
		cancel()
		waitForWatchDone(t, done)
		return count
	}

	if first, second := runSequence(), runSequence(); first != second || first != 1 {
		t.Fatalf("submit counts = %d and %d, want both 1", first, second)
	}
}

func TestIsWatchedFileEvent(t *testing.T) {
	tests := []struct {
		name string
		op   fsnotify.Op
		want bool
	}{
		{"create", fsnotify.Create, true},
		{"write", fsnotify.Write, true},
		{"create and write", fsnotify.Create | fsnotify.Write, true},
		{"chmod", fsnotify.Chmod, false},
		{"remove", fsnotify.Remove, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWatchedFileEvent(tt.op); got != tt.want {
				t.Fatalf("isWatchedFileEvent(%v) = %v, want %v", tt.op, got, tt.want)
			}
		})
	}
}

func writeLocalFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0o644)
}

func waitForSubmitCount(t *testing.T, submitter *recordingSubmitter, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if submitter.submitCallCount() >= want {
			// The count is authoritative. Clear notifications for submissions
			// already included in the count so callers cannot mistake an old
			// edge-triggered signal for new work.
			submitter.drainSubmitted()
			return
		}
		select {
		case <-submitter.submitted:
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			t.Fatalf("submit call count = %d, want %d", submitter.submitCallCount(), want)
		}
	}
}

func waitForWatchDone(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("Watch returned %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Watch did not stop")
	}
}
