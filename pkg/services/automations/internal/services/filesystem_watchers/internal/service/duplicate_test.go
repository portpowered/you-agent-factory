package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jonboulle/clockwork"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type recordingHandledIdentities struct {
	mu             sync.Mutex
	contains       map[filesystemwatchers.ObservationIdentity]bool
	recorded       []filesystemwatchers.ObservationIdentity
	recordErr      error
	recordAttempts int
}

func (s *recordingHandledIdentities) Contains(identity filesystemwatchers.ObservationIdentity) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contains[identity]
}

func (s *recordingHandledIdentities) Record(identity filesystemwatchers.ObservationIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordAttempts++
	if s.recordErr != nil {
		return s.recordErr
	}
	s.contains[identity] = true
	s.recorded = append(s.recorded, identity)
	return nil
}

func (s *recordingHandledIdentities) setRecordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordErr = err
}

type overlappingSubmitter struct {
	firstEntered  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
	firstOnce     sync.Once
	secondOnce    sync.Once
	releaseOnce   sync.Once
	mu            sync.Mutex
	calls         int
}

type failOnSubmitter struct {
	mu        sync.Mutex
	failOn    int
	err       error
	calls     int
	successes int
}

func (s *failOnSubmitter) Submit(context.Context, work.WorkRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.failOn > 0 && s.calls == s.failOn {
		return s.err
	}
	s.successes++
	return nil
}

func (s *failOnSubmitter) setFailOn(call int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failOn = call
}

func (s *failOnSubmitter) successCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.successes
}

func (s *overlappingSubmitter) Submit(context.Context, work.WorkRequest) error {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()

	switch call {
	case 1:
		s.firstOnce.Do(func() { close(s.firstEntered) })
		<-s.releaseFirst
	case 2:
		s.secondOnce.Do(func() { close(s.secondEntered) })
	}
	return nil
}

func (s *overlappingSubmitter) release() {
	s.releaseOnce.Do(func() { close(s.releaseFirst) })
}

func (s *overlappingSubmitter) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
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

func TestHandleFile_SubmitFailureRemainsRetryable(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "submit-failure.md")
	if err := writeLocalFile(path, []byte("submit failure")); err != nil {
		t.Fatal(err)
	}

	submitErr := errors.New("submit failed")
	submitter := &recordingSubmitter{err: submitErr}
	fw := newTestWatcher(dir, submitter, nil, nil, nil, nil, nil)
	if err := fw.handleFile(context.Background(), path); !errors.Is(err, submitErr) {
		t.Fatalf("first handleFile() error = %v, want %v", err, submitErr)
	}

	submitter.mu.Lock()
	submitter.err = nil
	submitter.mu.Unlock()
	if err := fw.handleFile(context.Background(), path); err != nil {
		t.Fatalf("retry handleFile() error = %v", err)
	}
	if got := submitter.submitCallCount(); got != 2 {
		t.Fatalf("submit call count = %d, want 2 after retryable submit failure", got)
	}
}

func TestHandleFile_RecordFailureDoesNotResubmit(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "record-failure.md")
	if err := writeLocalFile(path, []byte("record failure")); err != nil {
		t.Fatal(err)
	}

	recordErr := errors.New("record failed")
	store := &recordingHandledIdentities{
		contains:  make(map[filesystemwatchers.ObservationIdentity]bool),
		recordErr: recordErr,
	}
	submitter := &recordingSubmitter{}
	fw := newTestWatcher(dir, submitter, nil, nil, nil, nil, nil)
	fw.handledIdentities = store

	if err := fw.handleFile(context.Background(), path); !errors.Is(err, recordErr) {
		t.Fatalf("first handleFile() error = %v, want %v", err, recordErr)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count after record failure = %d, want 1", got)
	}

	store.setRecordError(nil)
	if err := fw.handleFile(context.Background(), path); err != nil {
		t.Fatalf("retry handleFile() error = %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count after record retry = %d, want 1", got)
	}
	store.mu.Lock()
	recordAttempts := store.recordAttempts
	store.mu.Unlock()
	if recordAttempts != 2 {
		t.Fatalf("record attempts = %d, want 2", recordAttempts)
	}
}

func TestWatcher_ObservationAdmissionSerializesIdentity(t *testing.T) {
	fw := &watcher{}
	identity := filesystemwatchers.ObservationIdentity("request/default/file.md")
	unlock := fw.lockObservation(identity)

	fw.admissionMu.Lock()
	admission := fw.admissions[identity]
	fw.admissionMu.Unlock()
	if admission == nil {
		unlock()
		t.Fatal("observation admission was not registered")
	}
	if admission.mu.TryLock() {
		admission.mu.Unlock()
		unlock()
		t.Fatal("same observation identity was not held exclusively")
	}

	unlock()
	if !admission.mu.TryLock() {
		t.Fatal("observation admission remained locked after release")
	}
	admission.mu.Unlock()
}

func TestHandleFile_ConcurrentDuplicateAdmissionSubmitsOnce(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "overlap.md")
	if err := writeLocalFile(path, []byte("overlap")); err != nil {
		t.Fatal(err)
	}

	store := &recordingHandledIdentities{contains: make(map[filesystemwatchers.ObservationIdentity]bool)}
	submitter := &overlappingSubmitter{
		firstEntered:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	fw := newTestWatcher(dir, &recordingSubmitter{}, nil, nil, nil, nil, nil)
	fw.submit = submitter.Submit
	fw.handledIdentities = store

	results := make(chan error, 2)
	start := make(chan struct{})
	secondStarted := make(chan struct{})
	var wg sync.WaitGroup
	t.Cleanup(func() {
		submitter.release()
		wg.Wait()
	})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i == 1 {
				close(secondStarted)
			}
			<-start
			results <- fw.handleFile(context.Background(), path)
		}(i)
	}
	close(start)
	<-secondStarted

	select {
	case <-submitter.firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first duplicate admission did not reach submitter")
	}

	submitter.release()
	wg.Wait()
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("handleFile() error = %v", err)
		}
	}
	if got := submitter.callCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1 for concurrent duplicate admission", got)
	}
}

func TestHandleFile_DistinctAdmissionsProceedConcurrently(t *testing.T) {
	dir := setupWatchDir(t)
	firstPath := filepath.Join(dir, "request", "default", "first-concurrent.md")
	secondPath := filepath.Join(dir, "request", "default", "second-concurrent.md")
	for _, path := range []string{firstPath, secondPath} {
		if err := writeLocalFile(path, []byte("concurrent")); err != nil {
			t.Fatal(err)
		}
	}

	submitter := &overlappingSubmitter{
		firstEntered:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	fw := newTestWatcher(dir, &recordingSubmitter{}, nil, nil, nil, nil, nil)
	fw.submit = submitter.Submit

	firstDone := make(chan error, 1)
	go func() { firstDone <- fw.handleFile(context.Background(), firstPath) }()
	select {
	case <-submitter.firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first distinct admission did not reach submitter")
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- fw.handleFile(context.Background(), secondPath) }()
	select {
	case <-submitter.secondEntered:
	case <-time.After(time.Second):
		t.Fatal("distinct admission was blocked by another identity")
	}

	submitter.release()
	if err := <-firstDone; err != nil {
		t.Fatalf("first handleFile() error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second handleFile() error = %v", err)
	}
	if got := submitter.callCount(); got != 2 {
		t.Fatalf("submit call count = %d, want 2 for distinct identities", got)
	}
}

func TestPreseedInputs_ConcurrentLiveDuplicateSubmitsOnce(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "preseed-live.md")
	if err := writeLocalFile(path, []byte("preseed and live")); err != nil {
		t.Fatal(err)
	}

	submitter := &overlappingSubmitter{
		firstEntered:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	fw := newTestWatcher(dir, &recordingSubmitter{}, nil, []string{"request"}, nil, nil, nil)
	fw.submit = submitter.Submit

	preseedDone := make(chan error, 1)
	go func() { preseedDone <- fw.PreseedInputs(context.Background()) }()
	select {
	case <-submitter.firstEntered:
	case <-time.After(time.Second):
		t.Fatal("preseed admission did not reach submitter")
	}

	liveDone := make(chan error, 1)
	go func() { liveDone <- fw.handleFile(context.Background(), path) }()

	submitter.release()
	if err := <-preseedDone; err != nil {
		t.Fatalf("PreseedInputs() error = %v", err)
	}
	if err := <-liveDone; err != nil {
		t.Fatalf("live handleFile() error = %v", err)
	}
	if got := submitter.callCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1 for preseed/live duplicate", got)
	}
}

func TestPreseedInputs_RecordFailureDoesNotResubmit(t *testing.T) {
	dir := setupWatchDir(t)
	path := filepath.Join(dir, "request", "default", "preseed-record-failure.md")
	if err := writeLocalFile(path, []byte("preseed record failure")); err != nil {
		t.Fatal(err)
	}

	recordErr := errors.New("preseed record failed")
	store := &recordingHandledIdentities{
		contains:  make(map[filesystemwatchers.ObservationIdentity]bool),
		recordErr: recordErr,
	}
	submitter := &recordingSubmitter{}
	fw := newTestWatcher(dir, submitter, nil, []string{"request"}, nil, nil, nil)
	fw.handledIdentities = store

	if err := fw.PreseedInputs(context.Background()); !errors.Is(err, recordErr) {
		t.Fatalf("first PreseedInputs() error = %v, want %v", err, recordErr)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count after preseed record failure = %d, want 1", got)
	}

	store.setRecordError(nil)
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("retry PreseedInputs() error = %v", err)
	}
	if got := submitter.submitCallCount(); got != 1 {
		t.Fatalf("submit call count after preseed record retry = %d, want 1", got)
	}
}

func TestPreseedInputs_PartialSubmitFailureDoesNotResubmitSuccessfulPlan(t *testing.T) {
	dir := setupWatchDir(t)
	firstPath := filepath.Join(dir, "request", "default", "first.json")
	secondPath := filepath.Join(dir, "request", "default", "second.json")
	content := []byte(`{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"work","workTypeName":"request","payload":{"step":"one"}}]}`)
	for _, path := range []string{firstPath, secondPath} {
		if err := writeLocalFile(path, content); err != nil {
			t.Fatal(err)
		}
	}

	submitErr := errors.New("second preseed request failed")
	submitter := &failOnSubmitter{failOn: 2, err: submitErr}
	fw := newTestWatcher(dir, &recordingSubmitter{}, nil, []string{"request"}, nil, nil, nil)
	fw.submit = submitter.Submit

	if err := fw.PreseedInputs(context.Background()); !errors.Is(err, submitErr) {
		t.Fatalf("first PreseedInputs() error = %v, want %v", err, submitErr)
	}
	if got := submitter.successCount(); got != 1 {
		t.Fatalf("successful submissions after partial failure = %d, want 1", got)
	}

	submitter.setFailOn(0)
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("retry PreseedInputs() error = %v", err)
	}
	if got := submitter.successCount(); got != 2 {
		t.Fatalf("successful submissions after retry = %d, want 2", got)
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
