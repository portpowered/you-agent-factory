package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
)

func watchIdentityForDir(dir string) filesystemwatchers.WatchIdentity {
	return filesystemwatchers.WatchIdentity{
		AutomationID: "factory/test",
		WatchRoot:    strings.ReplaceAll(dir, "\\", "/"),
	}
}

func TestHandleFile_CommitsCursorAfterSuccess(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "cursor-commit.md")
	if err := writeLocalFile(path, []byte("cursor commit")); err != nil {
		t.Fatal(err)
	}

	identity := watchIdentityForDir(dir)
	var committed []filesystemwatchers.WatcherFacts
	store, err := testFilesystemWatcherService().newHandledIdentities(
		filesystemwatchers.WatcherFacts{Identity: identity},
		func(facts filesystemwatchers.WatcherFacts) error {
			committed = append(committed, facts)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("NewHandledIdentities: %v", err)
	}

	submitter := &recordingSubmitter{}
	fw := newTestWatcher(dir, submitter, nil, nil, nil, nil, nil)
	fw.handledIdentities = store

	if err := fw.handleFile(context.Background(), path); err != nil {
		t.Fatalf("handleFile: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1", got)
	}
	if len(committed) != 1 {
		t.Fatalf("committed facts = %d, want 1", len(committed))
	}
	cursor, checkpoint := committed[0].CursorProjection()
	if cursor == "" || checkpoint == "" {
		t.Fatalf("cursor projection = %q/%q, want non-empty opaque facts", cursor, checkpoint)
	}
	observationIdentity, err := observationIdentity(dir, path)
	if err != nil {
		t.Fatalf("observationIdentity: %v", err)
	}
	if !store.Contains(observationIdentity) {
		t.Fatalf("handled identities missing %q after commit", observationIdentity)
	}
}

func TestResumeWatcherFacts_CompatibleCursorSeedsHandledIdentities(t *testing.T) {
	svc := testFilesystemWatcherService()
	identity := filesystemwatchers.WatchIdentity{
		AutomationID: "factory/test",
		WatchRoot:    "/inputs",
	}
	authoritative := filesystemwatchers.WatcherFacts{
		Identity:   identity,
		Cursor:     filesystemwatchers.Cursor("2"),
		Checkpoint: `{"handled":["request/default/preseeded.md"]}`,
	}

	resumed, err := svc.ResumeWatcherFacts(identity, nil, &authoritative)
	if err != nil {
		t.Fatalf("ResumeWatcherFacts: %v", err)
	}
	cursor, checkpoint := resumed.CursorProjection()
	if cursor != authoritative.Cursor || checkpoint != authoritative.Checkpoint {
		t.Fatalf("cursor projection = %q/%q, want %q/%q", cursor, checkpoint, authoritative.Cursor, authoritative.Checkpoint)
	}

	store, err := svc.newHandledIdentities(resumed, func(facts filesystemwatchers.WatcherFacts) error {
		return nil
	})
	if err != nil {
		t.Fatalf("NewHandledIdentities: %v", err)
	}
	if !store.Contains(filesystemwatchers.ObservationIdentity("request/default/preseeded.md")) {
		t.Fatal("resumed handled identities missing preseeded path")
	}
}

func TestResumeWatcherFacts_RejectsStaleCursorWithoutMutation(t *testing.T) {
	svc := testFilesystemWatcherService()
	identity := filesystemwatchers.WatchIdentity{
		AutomationID: "factory/test",
		WatchRoot:    "/inputs",
	}
	authoritative := filesystemwatchers.WatcherFacts{
		Identity:   identity,
		Cursor:     filesystemwatchers.Cursor("3"),
		Checkpoint: `{"handled":["request/default/a.md"]}`,
	}

	got, err := svc.ResumeWatcherFacts(identity, nil, &authoritative)
	if err != nil {
		t.Fatalf("initial ResumeWatcherFacts: %v", err)
	}

	stale := authoritative
	stale.Cursor = filesystemwatchers.Cursor("2")
	_, err = svc.ResumeWatcherFacts(identity, &got, &stale)
	if err == nil || !errors.Is(err, filesystemwatchers.ErrStaleResumeFacts) {
		t.Fatalf("stale resume error = %v, want %v", err, filesystemwatchers.ErrStaleResumeFacts)
	}

	preserved, err := svc.ResumeWatcherFacts(identity, &got, nil)
	if err != nil {
		t.Fatalf("authoritative ResumeWatcherFacts: %v", err)
	}
	if preserved != got {
		t.Fatalf("authoritative facts = %+v, want preserved %+v", preserved, got)
	}
}

func TestValidateExpectedCursor_RejectsMismatch(t *testing.T) {
	svc := testFilesystemWatcherService()
	authoritative := filesystemwatchers.WatcherFacts{
		Identity: filesystemwatchers.WatchIdentity{
			AutomationID: "factory/test",
			WatchRoot:    "/inputs",
		},
		Cursor:     filesystemwatchers.Cursor("4"),
		Checkpoint: `{"handled":["request/default/a.md"]}`,
	}

	err := svc.ValidateExpectedCursor(authoritative, filesystemwatchers.Cursor("3"))
	if err == nil || !errors.Is(err, filesystemwatchers.ErrStaleResumeFacts) {
		t.Fatalf("ValidateExpectedCursor error = %v, want %v", err, filesystemwatchers.ErrStaleResumeFacts)
	}
}

func TestHandleFile_NoSubmitWhenResumeRejectedBeforeWatch(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "blocked.md")
	if err := writeLocalFile(path, []byte("blocked")); err != nil {
		t.Fatal(err)
	}

	svc := testFilesystemWatcherService()
	identity := watchIdentityForDir(dir)
	authoritative := filesystemwatchers.WatcherFacts{
		Identity:   identity,
		Cursor:     filesystemwatchers.Cursor("1"),
		Checkpoint: `{"handled":[]}`,
	}
	got, err := svc.ResumeWatcherFacts(identity, nil, &authoritative)
	if err != nil {
		t.Fatalf("ResumeWatcherFacts: %v", err)
	}

	stale := authoritative
	stale.Cursor = filesystemwatchers.Cursor("stale")
	_, err = svc.ResumeWatcherFacts(identity, &got, &stale)
	if err == nil || !errors.Is(err, filesystemwatchers.ErrStaleResumeFacts) {
		t.Fatalf("stale resume error = %v, want %v", err, filesystemwatchers.ErrStaleResumeFacts)
	}

	submitter := &recordingSubmitter{}
	fw := newTestWatcher(dir, submitter, nil, nil, nil, nil, nil)
	store, err := svc.newHandledIdentities(got, func(facts filesystemwatchers.WatcherFacts) error { return nil })
	if err != nil {
		t.Fatalf("NewHandledIdentities: %v", err)
	}
	fw.handledIdentities = store

	if err := fw.handleFile(context.Background(), path); err != nil {
		t.Fatalf("handleFile: %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1 for pending path after valid authoritative facts", got)
	}
}

func TestRecord_DoesNotReportSuccessWhenCursorPersistFails(t *testing.T) {
	svc := testFilesystemWatcherService()
	identity := watchIdentityForDir("/inputs")
	store, err := svc.newHandledIdentities(
		filesystemwatchers.WatcherFacts{Identity: identity},
		func(filesystemwatchers.WatcherFacts) error {
			return errors.New("disk unavailable")
		},
	)
	if err != nil {
		t.Fatalf("NewHandledIdentities: %v", err)
	}

	err = store.Record(filesystemwatchers.ObservationIdentity("request/default/fail.md"))
	if err == nil || !errors.Is(err, filesystemwatchers.ErrCursorPersistFailed) {
		t.Fatalf("Record error = %v, want %v", err, filesystemwatchers.ErrCursorPersistFailed)
	}
	if store.Contains(filesystemwatchers.ObservationIdentity("request/default/fail.md")) {
		t.Fatal("identity recorded despite persist failure")
	}
}

func TestCursorBackedHandledIdentities_ConcurrentRecordsPreserveFacts(t *testing.T) {
	svc := testFilesystemWatcherService()
	identity := watchIdentityForDir("/inputs")
	var persistMu sync.Mutex
	var committed []filesystemwatchers.WatcherFacts
	store, err := svc.newHandledIdentities(
		filesystemwatchers.WatcherFacts{Identity: identity},
		func(facts filesystemwatchers.WatcherFacts) error {
			persistMu.Lock()
			defer persistMu.Unlock()
			committed = append(committed, facts)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("NewHandledIdentities: %v", err)
	}

	const recordCount = 16
	var wg sync.WaitGroup
	for i := 0; i < recordCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			observation := filesystemwatchers.ObservationIdentity(
				fmt.Sprintf("request/default/concurrent-%d.md", i),
			)
			if err := store.Record(observation); err != nil {
				t.Errorf("Record(%q): %v", observation, err)
			}
		}(i)
	}
	wg.Wait()

	if len(committed) != recordCount {
		t.Fatalf("committed facts = %d, want %d", len(committed), recordCount)
	}
	persisted, err := decodeHandledIdentities(committed[len(committed)-1].Checkpoint)
	if err != nil {
		t.Fatalf("decode final committed checkpoint: %v", err)
	}
	if len(persisted) != recordCount {
		t.Fatalf("persisted handled identities = %d, want %d", len(persisted), recordCount)
	}
	for i := 0; i < recordCount; i++ {
		observation := filesystemwatchers.ObservationIdentity(
			fmt.Sprintf("request/default/concurrent-%d.md", i),
		)
		if !store.Contains(observation) {
			t.Fatalf("handled identities missing %q after concurrent records", observation)
		}
	}
}

func testFilesystemWatcherService() *service {
	return New(nil).(*service)
}
