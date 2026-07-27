package service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
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
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitter := &recordingSubmitter{submitted: make(chan struct{}, 1)}
	watcher := newScriptedEventWatcher()
	fw := newTestWatcher(dir, submitter, zap.NewNop(), nil, nil, localInputFiles{}, filepath.WalkDir)
	fw.newWatcher = func() (fileEventWatcher, error) { return watcher, nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- fw.Watch(ctx) }()

	waitForRegisteredDirectory(t, watcher.added, dir)
	watcher.events <- fsnotify.Event{Name: path, Op: fsnotify.Create}
	select {
	case <-submitter.submitted:
	case <-time.After(time.Second):
		t.Fatal("injected file event was not submitted")
	}
	requests := submitter.getWorkRequests()
	if got := string(requests[0].Works[0].Payload.([]byte)); got != string(content) {
		t.Fatalf("payload = %q, want %q", got, content)
	}

	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("Watch returned %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Watch did not stop after cancellation")
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
