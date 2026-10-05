package service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jonboulle/clockwork"
)

type scriptedEventWatcher struct {
	events chan fsnotify.Event
	errors chan error
	added  chan string
	once   sync.Once
}

func newScriptedEventWatcher() *scriptedEventWatcher {
	return &scriptedEventWatcher{
		events: make(chan fsnotify.Event, 4),
		errors: make(chan error, 1),
		added:  make(chan string, 8),
	}
}

func (w *scriptedEventWatcher) Add(path string) error {
	w.added <- path
	return nil
}
func (w *scriptedEventWatcher) Close() error {
	w.once.Do(func() {
		close(w.events)
		close(w.errors)
	})
	return nil
}
func (w *scriptedEventWatcher) Events() <-chan fsnotify.Event { return w.events }
func (w *scriptedEventWatcher) Errors() <-chan error          { return w.errors }

func TestFileWatcher_InjectedEventProcessesAfterDirectoryRegistration(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "injected.md")
	content := []byte("deterministic input")
	submitter := &recordingSubmitter{submitted: make(chan struct{}, 1)}
	watcher := newScriptedEventWatcher()
	clock := clockwork.NewFakeClock()
	registeredClock := &registeringDebounceClock{Clock: clock, registered: make(chan struct{}, 1)}
	fw := newDebouncedTestWatcher(dir, submitter, registeredClock, watcher)
	cancel, done := startDebouncedWatch(t, fw, watcher)
	t.Cleanup(func() {
		cancel()
		waitForWatchDone(t, done)
	})

	// Empty discovery is complete, so only this injected event can register
	// the timer acknowledged before advancing the debounce window.
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	publishDebounceEvent(t, watcher, registeredClock, fsnotify.Event{Name: path, Op: fsnotify.Create})
	advanceDebounce(t, clock)
	select {
	case <-submitter.submitted:
	case <-time.After(time.Second):
		t.Fatal("injected file event was not submitted")
	}
	requests := submitter.getWorkRequests()
	if got := string(requests[0].Works[0].Payload.([]byte)); got != string(content) {
		t.Fatalf("payload = %q, want %q", got, content)
	}
}

func waitForRegisteredDirectory(t *testing.T, added <-chan string, want string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case got := <-added:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("watcher did not register %q", want)
		}
	}
}

func TestFileWatcher_DiscoversFilesPublishedBeforeDirectoryRegistration(t *testing.T) {
	for _, scenario := range []string{"startup_after_preseed", "new_channel", "new_work_type_and_channel"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			dir := setupWatchDir(t)
			submitter := &recordingSubmitter{submitted: make(chan struct{}, 2)}
			events := newScriptedEventWatcher()
			clock := clockwork.NewFakeClock()
			fw := newDebouncedTestWatcher(dir, submitter, clock, events)
			if err := fw.PreseedInputs(context.Background()); err != nil {
				t.Fatal(err)
			}
			workType := "request"
			if scenario == "new_work_type_and_channel" {
				workType = "new-type"
			}
			channel := filepath.Join(dir, workType, "execution-owned")
			path := filepath.Join(channel, "input.md")
			var cancel context.CancelFunc
			var done <-chan error
			if scenario != "startup_after_preseed" {
				cancel, done = startDebouncedWatch(t, fw, events)
				defer cancel()
				waitForRegisteredDirectory(t, events.added, filepath.Join(dir, "request", "default"))
			}
			if err := os.MkdirAll(channel, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("owned input"), 0o644); err != nil {
				t.Fatal(err)
			}
			if scenario == "startup_after_preseed" {
				cancel, done = startDebouncedWatch(t, fw, events)
				defer cancel()
			} else {
				created := channel
				if scenario == "new_work_type_and_channel" {
					created = filepath.Dir(channel)
				}
				// Only the ancestor creation is observed. The file was published
				// before Add, so no file event can rescue a missing discovery scan.
				events.events <- fsnotify.Event{Name: created, Op: fsnotify.Create}
			}
			waitForRegisteredDirectory(t, events.added, channel)
			advanceDebounce(t, clock)
			waitForSubmitCount(t, submitter, 1)
			requests := submitter.getWorkRequests()
			item := requests[0].Works[0]
			if item.WorkTypeID != workType || item.ExecutionID != "execution-owned" || string(item.Payload.([]byte)) != "owned input" {
				t.Fatalf("discovered Work = %#v, want exact type, correlation and payload", item)
			}
			cancel()
			waitForWatchDone(t, done)
		})
	}
}
