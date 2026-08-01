package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	"go.uber.org/zap"
)

type memoryCursorStore struct {
	facts filesystemwatchers.WatcherFacts
}

func (s *memoryCursorStore) persist(facts filesystemwatchers.WatcherFacts) error {
	s.facts = facts
	return nil
}

func (s *memoryCursorStore) current() filesystemwatchers.WatcherFacts {
	return s.facts
}

func restartRequest(
	dir string,
	submitter *recordingSubmitter,
	store *memoryCursorStore,
	authoritative *filesystemwatchers.WatcherFacts,
	resume *filesystemwatchers.WatcherFacts,
) filesystemwatchers.RestartRequest {
	return filesystemwatchers.RestartRequest{
		Config: filesystemwatchers.Config{
			Dir:            dir,
			Logger:         zap.NewNop(),
			Files:          localInputFiles{},
			WalkDirectory:  filepath.WalkDir,
			WorkRequestIDs: testWorkRequestIDGenerator,
			Submitter:      submitter.Submit,
		},
		Identity:      watchIdentityForDir(dir),
		Authoritative: authoritative,
		Resume:        resume,
		Persist:       store.persist,
	}
}

func TestNewWatcherWithResume_PreseedSkipsRecordedIdentity(t *testing.T) {
	dir := setupWatchDir(t)
	handledPath := filepath.Join(dir, "request", "default", "handled.md")
	if err := writeLocalFile(handledPath, []byte("handled")); err != nil {
		t.Fatal(err)
	}

	svc := testFilesystemWatcherService()
	store := &memoryCursorStore{}
	submitter := &recordingSubmitter{}

	first, _, err := svc.NewWatcherWithResume(restartRequest(dir, submitter, store, nil, nil))
	if err != nil {
		t.Fatalf("initial NewWatcherWithResume: %v", err)
	}
	fw := first.(*watcher)
	if err := fw.handleFile(context.Background(), handledPath); err != nil {
		t.Fatalf("initial handleFile: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("initial submit call count = %d, want 1", got)
	}
	committed := store.current()

	pendingPath := filepath.Join(dir, "request", "default", "pending.md")
	if err := writeLocalFile(pendingPath, []byte("pending")); err != nil {
		t.Fatal(err)
	}

	submitter = &recordingSubmitter{}
	restarted, _, err := svc.NewWatcherWithResume(
		restartRequest(dir, submitter, store, nil, &committed),
	)
	if err != nil {
		t.Fatalf("restart NewWatcherWithResume: %v", err)
	}
	if err := restarted.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("restarted PreseedInputs: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("restarted submit call count = %d, want 1 for pending input only", got)
	}

	handledIdentity, err := observationIdentity(dir, handledPath)
	if err != nil {
		t.Fatalf("observationIdentity(handled): %v", err)
	}
	pendingIdentity, err := observationIdentity(dir, pendingPath)
	if err != nil {
		t.Fatalf("observationIdentity(pending): %v", err)
	}
	if store.facts.Checkpoint != "" {
		if !containsHandledIdentity(store.facts.Checkpoint, handledIdentity) {
			t.Fatalf("checkpoint %q missing handled identity %q", store.facts.Checkpoint, handledIdentity)
		}
		if !containsHandledIdentity(store.facts.Checkpoint, pendingIdentity) {
			t.Fatalf("checkpoint %q missing pending identity %q after preseed", store.facts.Checkpoint, pendingIdentity)
		}
	}
}

func containsHandledIdentity(checkpoint string, identity filesystemwatchers.ObservationIdentity) bool {
	store, err := testFilesystemWatcherService().newHandledIdentities(
		filesystemwatchers.WatcherFacts{Checkpoint: checkpoint},
		func(filesystemwatchers.WatcherFacts) error { return nil },
	)
	if err != nil {
		return false
	}
	return store.Contains(identity)
}

func TestNewWatcherWithResume_HandleFileSkipsRecordedAfterRestart(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "restart-dup.md")
	if err := writeLocalFile(path, []byte("restart duplicate")); err != nil {
		t.Fatal(err)
	}

	svc := testFilesystemWatcherService()
	store := &memoryCursorStore{}
	submitter := &recordingSubmitter{}

	first, _, err := svc.NewWatcherWithResume(restartRequest(dir, submitter, store, nil, nil))
	if err != nil {
		t.Fatalf("initial NewWatcherWithResume: %v", err)
	}
	fw := first.(*watcher)
	if err := fw.handleFile(context.Background(), path); err != nil {
		t.Fatalf("initial handleFile: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("initial submit call count = %d, want 1", got)
	}
	committed := store.current()

	submitter = &recordingSubmitter{}
	restarted, _, err := svc.NewWatcherWithResume(
		restartRequest(dir, submitter, store, nil, &committed),
	)
	if err != nil {
		t.Fatalf("restart NewWatcherWithResume: %v", err)
	}
	restartedWatcher := restarted.(*watcher)
	if err := restartedWatcher.handleFile(context.Background(), path); err != nil {
		t.Fatalf("restarted handleFile: %v", err)
	}
	if got := submitter.submitCallCount(); got != 0 {
		t.Fatalf("restarted submit call count = %d, want 0 for recorded identity", got)
	}
}

func TestNewWatcherWithResume_RejectsInvalidResumeWithoutWatcher(t *testing.T) {
	dir := setupWatchDir(t)
	svc := testFilesystemWatcherService()
	identity := watchIdentityForDir(dir)
	store := &memoryCursorStore{}
	submitter := &recordingSubmitter{}

	authoritative := filesystemwatchers.WatcherFacts{
		Identity:   identity,
		Cursor:     filesystemwatchers.Cursor("2"),
		Checkpoint: `{"handled":["request/default/a.md"]}`,
	}
	got, err := svc.ResumeWatcherFacts(identity, nil, &authoritative)
	if err != nil {
		t.Fatalf("ResumeWatcherFacts: %v", err)
	}

	stale := authoritative
	stale.Cursor = filesystemwatchers.Cursor("1")
	_, _, err = svc.NewWatcherWithResume(
		restartRequest(dir, submitter, store, &got, &stale),
	)
	if err == nil || !errors.Is(err, filesystemwatchers.ErrStaleResumeFacts) {
		t.Fatalf("NewWatcherWithResume error = %v, want %v", err, filesystemwatchers.ErrStaleResumeFacts)
	}
	if got := submitter.submitCallCount(); got != 0 {
		t.Fatalf("submit call count = %d, want 0 on invalid resume", got)
	}
}

func TestNewWatcherWithResume_RejectsMalformedAuthoritativeFacts(t *testing.T) {
	dir := setupWatchDir(t)
	svc := testFilesystemWatcherService()
	identity := watchIdentityForDir(dir)

	tests := []struct {
		name  string
		facts filesystemwatchers.WatcherFacts
	}{
		{
			name: "nonnumeric cursor",
			facts: filesystemwatchers.WatcherFacts{
				Identity:   identity,
				Cursor:     filesystemwatchers.Cursor("opaque-cursor"),
				Checkpoint: `{"handled":[]}`,
			},
		},
		{
			name: "cursor without checkpoint",
			facts: filesystemwatchers.WatcherFacts{
				Identity: identity,
				Cursor:   filesystemwatchers.Cursor("2"),
			},
		},
		{
			name: "checkpoint without cursor",
			facts: filesystemwatchers.WatcherFacts{
				Identity:   identity,
				Checkpoint: `{"handled":["request/default/already-handled.md"]}`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryCursorStore{}
			submitter := &recordingSubmitter{}
			_, _, err := svc.NewWatcherWithResume(
				restartRequest(dir, submitter, store, &tc.facts, nil),
			)
			if err == nil || !errors.Is(err, filesystemwatchers.ErrInvalidResumeFacts) {
				t.Fatalf("NewWatcherWithResume error = %v, want %v", err, filesystemwatchers.ErrInvalidResumeFacts)
			}
			if got := submitter.submitCallCount(); got != 0 {
				t.Fatalf("submit call count = %d, want 0 on malformed authoritative facts", got)
			}
		})
	}
}

func TestNewWatcherWithResume_EquivalentPreseedMatchesAfterRestart(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "equivalent.md")
	if err := writeLocalFile(path, []byte("equivalent")); err != nil {
		t.Fatal(err)
	}

	svc := testFilesystemWatcherService()

	directSubmitter := &recordingSubmitter{}
	directStore := &memoryCursorStore{}
	direct, _, err := svc.NewWatcherWithResume(restartRequest(dir, directSubmitter, directStore, nil, nil))
	if err != nil {
		t.Fatalf("direct NewWatcherWithResume: %v", err)
	}
	if err := direct.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("direct PreseedInputs: %v", err)
	}
	directCount := directSubmitter.submitCallCount()
	directFacts := directStore.current()

	restartSubmitter := &recordingSubmitter{}
	restartStore := &memoryCursorStore{}
	restarted, resumedFacts, err := svc.NewWatcherWithResume(
		restartRequest(dir, restartSubmitter, restartStore, nil, &directFacts),
	)
	if err != nil {
		t.Fatalf("restart NewWatcherWithResume: %v", err)
	}
	if err := restarted.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("restarted PreseedInputs: %v", err)
	}
	if got := restartSubmitter.submitCallCount(); got != 0 {
		t.Fatalf("restarted submit call count = %d, want 0 when facts already committed", got)
	}
	if resumedFacts.Cursor != directFacts.Cursor || resumedFacts.Checkpoint != directFacts.Checkpoint {
		t.Fatalf("resumed facts = %+v, want %+v", resumedFacts, directFacts)
	}
	_ = directCount
}

func TestNewWatcherWithResume_PendingInputRemainsEligibleAfterRestart(t *testing.T) {
	dir := setupWatchDir(t)
	recordedPath := filepath.Join(dir, "request", "default", "recorded.md")
	pendingPath := filepath.Join(dir, "request", "default", "pending.md")
	if err := writeLocalFile(recordedPath, []byte("recorded")); err != nil {
		t.Fatal(err)
	}

	svc := testFilesystemWatcherService()
	store := &memoryCursorStore{}
	submitter := &recordingSubmitter{}

	first, _, err := svc.NewWatcherWithResume(restartRequest(dir, submitter, store, nil, nil))
	if err != nil {
		t.Fatalf("initial NewWatcherWithResume: %v", err)
	}
	fw := first.(*watcher)
	if err := fw.handleFile(context.Background(), recordedPath); err != nil {
		t.Fatalf("handle recorded file: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("initial submit call count = %d, want 1", got)
	}
	committed := store.current()

	if err := writeLocalFile(pendingPath, []byte("pending")); err != nil {
		t.Fatal(err)
	}

	submitter = &recordingSubmitter{}
	restarted, _, err := svc.NewWatcherWithResume(
		restartRequest(dir, submitter, store, nil, &committed),
	)
	if err != nil {
		t.Fatalf("restart NewWatcherWithResume: %v", err)
	}
	if err := restarted.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("restarted PreseedInputs: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("restarted submit call count = %d, want 1 for pending input only", got)
	}
}
