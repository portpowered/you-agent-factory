package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jonboulle/clockwork"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
)

type recordingHandledIdentities struct {
	mu        sync.Mutex
	contains  map[filesystemwatchers.ObservationIdentity]bool
	recorded  []filesystemwatchers.ObservationIdentity
	committed chan filesystemwatchers.ObservationIdentity
}

func (s *recordingHandledIdentities) Contains(identity filesystemwatchers.ObservationIdentity) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contains[identity]
}

func (s *recordingHandledIdentities) Record(identity filesystemwatchers.ObservationIdentity) error {
	s.mu.Lock()
	s.contains[identity] = true
	s.recorded = append(s.recorded, identity)
	s.mu.Unlock()
	if s.committed != nil {
		s.committed <- identity
	}
	return nil
}

func TestObservationIdentityForPath(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "input.md")

	identity, err := observationIdentity(dir, path)
	if err != nil {
		t.Fatalf("observationIdentity: %v", err)
	}
	if got, want := identity, filesystemwatchers.ObservationIdentity("request/default/input.md"); got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

func TestHandleFile_FirstSubmitRecordsIdentity(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "first.md")
	content := []byte("first submit")
	if err := writeLocalFile(path, content); err != nil {
		t.Fatal(err)
	}

	store := &recordingHandledIdentities{contains: make(map[filesystemwatchers.ObservationIdentity]bool)}
	submitter := &recordingSubmitter{}
	fw := newTestWatcher(dir, submitter, nil, nil, nil, nil, nil)
	fw.handledIdentities = store

	if err := fw.handleFile(context.Background(), path); err != nil {
		t.Fatalf("handleFile: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1", got)
	}
	identity, err := observationIdentity(dir, path)
	if err != nil {
		t.Fatalf("observationIdentity: %v", err)
	}
	if !store.Contains(identity) {
		t.Fatalf("handled identities did not record %q", identity)
	}
}

func TestHandleFile_DuplicateObservationSkipsSubmit(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "duplicate.md")
	if err := writeLocalFile(path, []byte("duplicate")); err != nil {
		t.Fatal(err)
	}

	identity, err := observationIdentity(dir, path)
	if err != nil {
		t.Fatalf("observationIdentity: %v", err)
	}
	store := &recordingHandledIdentities{
		contains: map[filesystemwatchers.ObservationIdentity]bool{identity: true},
	}
	submitter := &recordingSubmitter{}
	fw := newTestWatcher(dir, submitter, nil, nil, nil, nil, nil)
	fw.handledIdentities = store

	if err := fw.handleFile(context.Background(), path); err != nil {
		t.Fatalf("handleFile: %v", err)
	}
	if got := submitter.submitCallCount(); got != 0 {
		t.Fatalf("submit call count = %d, want 0 for duplicate observation", got)
	}
}

func TestHandleFile_DistinctPathSubmitsAfterDuplicate(t *testing.T) {
	dir := setupWatchDir(t)
	firstPath := filepath.Join(dir, "request", "default", "handled.md")
	secondPath := filepath.Join(dir, "request", "default", "fresh.md")
	if err := writeLocalFile(firstPath, []byte("handled")); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalFile(secondPath, []byte("fresh")); err != nil {
		t.Fatal(err)
	}

	firstIdentity, err := observationIdentity(dir, firstPath)
	if err != nil {
		t.Fatalf("observationIdentity(first): %v", err)
	}
	store := &recordingHandledIdentities{
		contains: map[filesystemwatchers.ObservationIdentity]bool{firstIdentity: true},
	}
	submitter := &recordingSubmitter{}
	fw := newTestWatcher(dir, submitter, nil, nil, nil, nil, nil)
	fw.handledIdentities = store

	if err := fw.handleFile(context.Background(), secondPath); err != nil {
		t.Fatalf("handleFile: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1 for distinct path", got)
	}
}

func TestFileWatcher_DuplicateLiveObservationSubmitsOnce(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "live-dup.md")
	clock := clockwork.NewFakeClock()
	registeredClock := &registeringDebounceClock{
		Clock: clock, registered: make(chan struct{}, 1), completed: make(chan struct{}, 2),
	}
	store := &recordingHandledIdentities{
		contains:  make(map[filesystemwatchers.ObservationIdentity]bool),
		committed: make(chan filesystemwatchers.ObservationIdentity, 2),
	}
	submitter := &recordingSubmitter{submitted: make(chan struct{}, 2)}
	eventWatcher := newScriptedEventWatcher()
	fw := newDebouncedTestWatcher(dir, submitter, registeredClock, eventWatcher)
	fw.handledIdentities = store
	cancel, done := startDebouncedWatch(t, fw, eventWatcher)
	t.Cleanup(func() {
		cancel()
		waitForWatchDone(t, done)
	})

	// Complete empty discovery before publishing. Submission precedes the
	// handled-identity commit, so only Record acknowledges duplicate readiness.
	if err := writeLocalFile(path, []byte("live duplicate")); err != nil {
		t.Fatal(err)
	}
	publishDebounceEvent(t, eventWatcher, registeredClock, fsnotify.Event{Name: path, Op: fsnotify.Create})
	advanceDebounce(t, clock)
	select {
	case identity := <-store.committed:
		if want := filesystemwatchers.ObservationIdentity("request/default/live-dup.md"); identity != want {
			t.Fatalf("committed identity = %q, want %q", identity, want)
		}
	case <-time.After(time.Second): //nolint:testsleep // Failure ceiling only; Record acknowledges the committed identity.
		t.Fatal("first observation did not commit its handled identity")
	}
	waitForDebounceCompletion(t, registeredClock.completed)

	publishDebounceEvent(t, eventWatcher, registeredClock, fsnotify.Event{Name: path, Op: fsnotify.Write})
	advanceDebounce(t, clock)
	waitForDebounceCompletion(t, registeredClock.completed)
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1 after duplicate live observation", got)
	}
	item := submitter.getWorkRequests()[0].Works[0]
	if item.WorkTypeID != "request" || string(item.Payload.([]byte)) != "live duplicate" {
		t.Fatalf("submitted Work = %#v, want request with exact live duplicate payload", item)
	}
}

func waitForDebounceCompletion(t *testing.T, completed <-chan struct{}) {
	t.Helper()
	select {
	case <-completed:
	case <-time.After(time.Second): //nolint:testsleep // Failure ceiling only; callback completion is the readiness signal.
		t.Fatal("debounce callback did not complete")
	}
}

func TestPreseedInputs_RecordsHandledIdentitiesWithoutResubmitOnLiveDuplicate(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "preseeded.md")
	if err := writeLocalFile(path, []byte("preseeded")); err != nil {
		t.Fatal(err)
	}

	store := newMemoryHandledIdentities()
	submitter := &recordingSubmitter{}
	fw := newTestWatcher(dir, submitter, nil, nil, nil, nil, nil)
	fw.handledIdentities = store

	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("PreseedInputs: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("preseed submit call count = %d, want 1", got)
	}
	identity, err := observationIdentity(dir, path)
	if err != nil {
		t.Fatalf("observationIdentity: %v", err)
	}
	if !store.Contains(identity) {
		t.Fatalf("preseed did not record handled identity %q", identity)
	}

	if err := fw.handleFile(context.Background(), path); err != nil {
		t.Fatalf("handleFile after preseed: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count after duplicate = %d, want 1", got)
	}
}
