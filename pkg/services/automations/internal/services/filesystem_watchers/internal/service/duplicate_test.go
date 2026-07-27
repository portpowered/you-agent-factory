package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jonboulle/clockwork"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
)

type recordingHandledIdentities struct {
	contains map[filesystemwatchers.ObservationIdentity]bool
	recorded []filesystemwatchers.ObservationIdentity
}

func (s *recordingHandledIdentities) Contains(identity filesystemwatchers.ObservationIdentity) bool {
	return s.contains[identity]
}

func (s *recordingHandledIdentities) Record(identity filesystemwatchers.ObservationIdentity) error {
	s.contains[identity] = true
	s.recorded = append(s.recorded, identity)
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
	if err := writeLocalFile(path, []byte("live duplicate")); err != nil {
		t.Fatal(err)
	}

	clock := clockwork.NewFakeClock()
	submitter := &recordingSubmitter{submitted: make(chan struct{}, 2)}
	eventWatcher := newScriptedEventWatcher()
	fw := newDebouncedTestWatcher(dir, submitter, clock, eventWatcher)
	cancel, done := startDebouncedWatch(t, fw, eventWatcher)
	defer cancel()

	eventWatcher.events <- fsnotify.Event{Name: path, Op: fsnotify.Create}
	advanceDebounce(t, clock)
	waitForSubmitCount(t, submitter, 1)

	eventWatcher.events <- fsnotify.Event{Name: path, Op: fsnotify.Write}
	advanceDebounce(t, clock)

	deadline := time.After(200 * time.Millisecond)
	for {
		if submitter.submitCallCount() > 1 {
			t.Fatalf("submit call count = %d, want 1 after duplicate live observation", submitter.submitCallCount())
		}
		select {
		case <-submitter.submitted:
			t.Fatal("unexpected second submission for duplicate observation")
		case <-deadline:
			cancel()
			waitForWatchDone(t, done)
			return
		case <-time.After(10 * time.Millisecond):
		}
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
