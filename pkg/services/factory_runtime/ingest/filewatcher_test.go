package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

var testWorkRequestIdentity atomic.Uint64

var testWorkRequestIDGenerator work.RequestIDGenerator = func() string {
	return fmt.Sprintf("test-id-%d", testWorkRequestIdentity.Add(1))
}

// mockFactory records SubmitWorkRequest calls for test assertions.
type mockFactory struct {
	mu           sync.Mutex
	workRequests []work.WorkRequest
	submitted    chan struct{}
	result       work.WorkRequestSubmitResult
	err          error
}

func (m *mockFactory) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workRequests = append(m.workRequests, cloneWorkRequest(request))
	if m.submitted != nil {
		select {
		case m.submitted <- struct{}{}:
		default:
		}
	}
	if m.err != nil {
		return work.WorkRequestSubmitResult{}, m.err
	}
	result := m.result
	if result.RequestID == "" && result.TraceID == "" && result.WorkID == "" && result.Name == "" &&
		result.WorkTypeName == "" && !result.Accepted && len(result.Works) == 0 {
		result = work.WorkRequestSubmitResult{RequestID: request.RequestID, Accepted: true}
	}
	return result, nil
}

func (m *mockFactory) Run(_ context.Context) error { return nil }
func (m *mockFactory) SubscribeFactoryEvents(_ context.Context, _ *interfaces.FactoryEventReconnectCursor, _ interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{Events: make(chan interfaces.FactoryEvent)}, nil
}
func (m *mockFactory) Pause(_ context.Context) error  { return nil }
func (m *mockFactory) Resume(_ context.Context) error { return nil }
func (m *mockFactory) Terminate(_ context.Context, _ factory.TerminateRequest) (factory.TerminateResult, error) {
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (m *mockFactory) MoveWork(_ context.Context, _ string, _ string, _ work.WorkStateChangeSource, _ string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, errors.New("MoveWork is not implemented in ingest mockFactory")
}

func (m *mockFactory) GetEngineStateSnapshot(_ context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{}, nil
}
func (m *mockFactory) GetFactoryEvents(_ context.Context) ([]interfaces.FactoryEvent, error) {
	return nil, nil
}
func (m *mockFactory) WaitToComplete() <-chan struct{} {
	return make(chan struct{})
}

func (m *mockFactory) getWorkRequests() []work.WorkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]work.WorkRequest, len(m.workRequests))
	for i := range m.workRequests {
		out[i] = cloneWorkRequest(m.workRequests[i])
	}
	return out
}

func cloneWorkRequest(request work.WorkRequest) work.WorkRequest {
	out := request
	out.Works = make([]work.Work, len(request.Works))
	for i := range request.Works {
		out.Works[i] = request.Works[i]
		if payload, ok := request.Works[i].Payload.([]byte); ok {
			out.Works[i].Payload = append([]byte(nil), payload...)
		}
		if request.Works[i].Tags != nil {
			out.Works[i].Tags = make(map[string]string, len(request.Works[i].Tags))
			for key, value := range request.Works[i].Tags {
				out.Works[i].Tags[key] = value
			}
		}
	}
	out.Relations = append([]work.WorkRelation(nil), request.Relations...)
	return out
}

func requireFileWatcherIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("real filesystem watcher integration")
	}
}

func setupWatchDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Create representative watched input roots.
	if err := os.MkdirAll(filepath.Join(dir, "request", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "BATCH", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// setupMultiChannelDir creates an inputs/ directory with multi-channel layout.
func setupMultiChannelDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Create inputs/<work-type>/default/ structure.
	if err := os.MkdirAll(filepath.Join(dir, "task", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func waitForSubmission(t *testing.T, mf *mockFactory, count int) []work.WorkRequest {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		submitted := mf.getWorkRequests()
		if len(submitted) >= count {
			return submitted
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d submissions, got %d", count, len(submitted))
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func assertNoSubmissionWithin(t *testing.T, mf *mockFactory, duration time.Duration) {
	t.Helper()
	time.Sleep(duration)
	if got := len(mf.getWorkRequests()); got != 0 {
		t.Fatalf("expected no submissions, got %d", got)
	}
}

func TestFileWatcher_MDFile(t *testing.T) {
	requireFileWatcherIntegration(t)
	dir := setupWatchDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger, nil, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	// Give the watcher time to start.
	time.Sleep(200 * time.Millisecond)

	// Drop a .md file into request/default/.
	content := []byte("# My Task\nDo something useful.")
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "task.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	requests := submitted
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if requests[0].Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want FACTORY_REQUEST_BATCH", requests[0].Type)
	}
	if len(requests[0].Works) != 1 {
		t.Fatalf("expected 1 work in request, got %d", len(requests[0].Works))
	}
	if requests[0].Works[0].WorkTypeID != "request" {
		t.Errorf("expected wrapped work_type_name 'request', got %q", requests[0].Works[0].WorkTypeID)
	}
	if submitted[0].Works[0].WorkTypeID != "request" {
		t.Errorf("expected WorkTypeID 'request', got %q", submitted[0].Works[0].WorkTypeID)
	}
	if string(submitted[0].Works[0].Payload.([]byte)) != string(content) {
		t.Errorf("payload mismatch: got %q", string(submitted[0].Works[0].Payload.([]byte)))
	}
}

func TestFileWatcher_JSONNonBatchWrapsRawPayload(t *testing.T) {
	requireFileWatcherIntegration(t)
	dir := setupWatchDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger, nil, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	content := []byte(`{"work_type_name":"chapter","payload":"translate this","tags":{"lang":"ja"}}`)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	requests := submitted
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if requests[0].Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want FACTORY_REQUEST_BATCH", requests[0].Type)
	}
	if got := requests[0].Works[0].WorkTypeID; got != "request" {
		t.Errorf("wrapped work_type_name = %q, want request", got)
	}
	if submitted[0].Works[0].WorkTypeID != "request" {
		t.Errorf("expected WorkTypeID 'request', got %q", submitted[0].Works[0].WorkTypeID)
	}
	if string(submitted[0].Works[0].Payload.([]byte)) != string(content) {
		t.Errorf("payload mismatch: got %q", string(submitted[0].Works[0].Payload.([]byte)))
	}
	if submitted[0].Works[0].Name != "batch" {
		t.Errorf("expected Name 'batch', got %q", submitted[0].Works[0].Name)
	}
}

func TestFileWatcher_JSONFallbackPayload(t *testing.T) {
	requireFileWatcherIntegration(t)
	dir := setupWatchDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger, nil, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	content := []byte(`{"name": "some data", "value": 42}`)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "data.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	requests := submitted
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if got := requests[0].Works[0].WorkTypeID; got != "request" {
		t.Errorf("wrapped work_type_name = %q, want request", got)
	}
	if submitted[0].Works[0].WorkTypeID != "request" {
		t.Errorf("expected WorkTypeID 'request', got %q", submitted[0].Works[0].WorkTypeID)
	}
	if string(submitted[0].Works[0].Payload.([]byte)) != string(content) {
		t.Errorf("payload mismatch: got %q", string(submitted[0].Works[0].Payload.([]byte)))
	}
}

func TestFileWatcher_JSONFactoryRequestBatch(t *testing.T) {
	dir := setupWatchDir(t)
	batch := work.WorkRequest{
		RequestID: "request-batch-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{
				Name:    "first",
				TraceID: "trace-batch",
				Payload: map[string]string{"step": "first"},
			},
			{
				Name:    "second",
				Payload: map[string]string{"step": "second"},
			},
		},
		Relations: []work.WorkRelation{
			{
				Type:           work.WorkRelationDependsOn,
				SourceWorkName: "second",
				TargetWorkName: "first",
				RequiredState:  "complete",
			},
		},
	}
	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	requests := mf.getWorkRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if requests[0].Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want FACTORY_REQUEST_BATCH", requests[0].Type)
	}
	if len(requests[0].Works) != 2 {
		t.Fatalf("expected 2 works in submitted batch, got %d", len(requests[0].Works))
	}
	if requests[0].Works[0].WorkTypeID != "request" || requests[0].Works[1].WorkTypeID != "request" {
		t.Fatalf("expected batch work_type_name fields filled from watched folder, got %#v", requests[0].Works)
	}
	if requests[0].Works[0].TraceID != "trace-batch" {
		t.Errorf("expected input trace ID to be preserved, got %q", requests[0].Works[0].TraceID)
	}
	if len(requests[0].Relations) != 1 || requests[0].Relations[0].TargetWorkName != "first" {
		t.Fatalf("submitted relations = %#v, want dependency targeting first by public Work name", requests[0].Relations)
	}
}

func TestFileWatcher_JSONFactoryRequestBatchMapsWorkTypeName(t *testing.T) {
	dir := setupWatchDir(t)
	data := []byte(`{
		"requestId": "request-batch-work-type-name",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "first", "workTypeName": "request", "state": "queued", "payload": {"step": "first"}},
			{"name": "second", "workTypeName": "request", "payload": {"step": "second"}}
		],
		"relations": [
			{"type": "DEPENDS_ON", "sourceWorkName": "second", "targetWorkName": "first"}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), []string{"request"}, map[string]map[string]bool{"request": {"queued": true, "complete": true}}, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)

	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	requests := mf.getWorkRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if requests[0].Works[0].WorkTypeID != "request" || requests[0].Works[1].WorkTypeID != "request" {
		t.Fatalf("expected work_type_name values mapped to runtime WorkTypeID fields, got %#v", requests[0].Works)
	}
	if requests[0].Works[0].State != "queued" {
		t.Fatalf("expected explicit public state to be preserved, got %#v", requests[0].Works[0])
	}
	if requests[0].Works[0].State != "queued" {
		t.Fatalf("expected public state queued, got %q", requests[0].Works[0].State)
	}
	if len(requests[0].Relations) != 1 {
		t.Fatalf("expected one public dependency relation, got %d", len(requests[0].Relations))
	}
}

func TestFileWatcher_JSONFactoryRequestBatchAcceptsParentChildByWorkName(t *testing.T) {
	dir := setupWatchDir(t)
	data := []byte(`{
		"requestId": "request-batch-parent-child",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "parent", "workTypeName": "request", "traceId": "trace-parent-child", "payload": {"step": "parent"}},
			{"name": "prerequisite", "workTypeName": "request", "payload": {"step": "prerequisite"}},
			{"name": "child", "workTypeName": "request", "payload": {"step": "child"}}
		],
		"relations": [
			{"type": "PARENT_CHILD", "sourceWorkName": "child", "targetWorkName": "parent"},
			{"type": "DEPENDS_ON", "sourceWorkName": "child", "targetWorkName": "prerequisite"}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	requests := mf.getWorkRequests()
	if len(requests) != 1 || len(requests[0].Works) != 3 {
		t.Fatalf("submitted Work Requests = %#v, want one three-Work batch", requests)
	}

	var child work.Work
	for _, item := range requests[0].Works {
		if item.Name == "child" {
			child = item
			break
		}
	}
	if child.Name == "" {
		t.Fatal("expected child submission")
	}
	wantRelations := []work.WorkRelation{
		{Type: work.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
		{Type: work.WorkRelationDependsOn, SourceWorkName: "child", TargetWorkName: "prerequisite"},
	}
	if !reflect.DeepEqual(requests[0].Relations, wantRelations) {
		t.Fatalf("public relations = %#v, want %#v", requests[0].Relations, wantRelations)
	}
}

func TestFileWatcher_JSONFactoryRequestBatchMapsStateAndParentChild(t *testing.T) {
	dir := setupWatchDir(t)
	data := []byte(`{
		"requestId": "request-batch-parent-child",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "story-set", "workTypeName": "request", "state": "waiting"},
			{"name": "story-a", "workTypeName": "story", "payload": {"step": "child"}}
		],
		"relations": [
			{"type": "PARENT_CHILD", "sourceWorkName": "story-a", "targetWorkName": "story-set"}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "BATCH", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), []string{"request", "story"}, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	requests := mf.getWorkRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if requests[0].Works[0].State != "waiting" {
		t.Fatalf("parent state = %q, want waiting", requests[0].Works[0].State)
	}
	if len(requests[0].Relations) != 1 {
		t.Fatalf("expected 1 request relation, got %d", len(requests[0].Relations))
	}
	if requests[0].Relations[0].Type != work.WorkRelationParentChild {
		t.Fatalf("request relation type = %q, want %q", requests[0].Relations[0].Type, work.WorkRelationParentChild)
	}
	if requests[0].Works[1].WorkTypeID != "story" {
		t.Fatalf("child work type = %q, want story", requests[0].Works[1].WorkTypeID)
	}
}

func TestFileWatcher_JSONFactoryRequestBatchRejectsWorkTypeIDAlias(t *testing.T) {
	dir := setupWatchDir(t)
	data := []byte(`{
		"requestId": "request-batch-work-type-id",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "first", "work_type_id": "request", "payload": {"step": "first"}}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)
	err := fw.PreseedInputs(context.Background())
	if err == nil {
		t.Fatal("expected retired work_type_id alias to fail")
	}
	want := "works[0].work_type_id is not supported; use workTypeName"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if submitted := mf.getWorkRequests(); len(submitted) != 0 {
		t.Fatalf("expected no partial submissions, got %d", len(submitted))
	}
	if requests := mf.getWorkRequests(); len(requests) != 0 {
		t.Fatalf("expected no submitted work requests, got %d", len(requests))
	}
}

func TestFileWatcher_JSONFactoryRequestBatchRejectsTargetStateAlias(t *testing.T) {
	dir := setupWatchDir(t)
	data := []byte(`{
		"requestId": "request-batch-target-state",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "story-set", "workTypeName": "request", "target_state": "waiting"}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "BATCH", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), []string{"request"}, map[string]map[string]bool{"request": {"waiting": true, "complete": true}}, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)

	err := fw.PreseedInputs(context.Background())
	if err == nil {
		t.Fatal("expected retired target_state alias to fail")
	}
	want := "works[0].target_state is not supported; use state"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if submitted := mf.getWorkRequests(); len(submitted) != 0 {
		t.Fatalf("expected no partial submissions, got %d", len(submitted))
	}
	if requests := mf.getWorkRequests(); len(requests) != 0 {
		t.Fatalf("expected no submitted work requests, got %d", len(requests))
	}
}

func TestFileWatcher_JSONFactoryRequestBatchRejectsConflictingTraceAliases(t *testing.T) {
	dir := setupWatchDir(t)
	data := []byte(`{
		"requestId": "request-batch-trace-conflict",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "draft", "workTypeName": "request", "currentChainingTraceId": "chain-a", "traceId": "trace-b", "payload": {"step": "draft"}}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "BATCH", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)
	err := fw.PreseedInputs(context.Background())
	if err == nil {
		t.Fatal("expected conflicting trace aliases to fail")
	}
	if !strings.Contains(err.Error(), "currentChainingTraceId and traceId must match") {
		t.Fatalf("error = %q, want conflicting trace alias rejection", err.Error())
	}
	if submitted := mf.getWorkRequests(); len(submitted) != 0 {
		t.Fatalf("expected no partial submissions, got %d", len(submitted))
	}
	if requests := mf.getWorkRequests(); len(requests) != 0 {
		t.Fatalf("expected no submitted work requests, got %d", len(requests))
	}
}

func TestFileWatcher_JSONFactoryRequestBatchRejectsConflictingWorkType(t *testing.T) {
	dir := setupWatchDir(t)
	batch := work.WorkRequest{
		RequestID: "request-batch-conflict",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{
				Name:       "wrong-folder",
				WorkTypeID: "chapter",
				Payload:    "do not submit",
			},
		},
	}
	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)
	if err := fw.PreseedInputs(context.Background()); err == nil {
		t.Fatal("expected conflicting batch work type to fail")
	}
	if submitted := mf.getWorkRequests(); len(submitted) != 0 {
		t.Fatalf("expected no partial submissions, got %d", len(submitted))
	}
}

func TestFileWatcher_PreseedValidatesAllFilesBeforeSubmitting(t *testing.T) {
	dir := setupWatchDir(t)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "a-valid.md"), []byte("valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	batch := work.WorkRequest{
		RequestID: "request-empty-batch",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works:     []work.Work{},
	}
	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "z-invalid.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)
	if err := fw.PreseedInputs(context.Background()); err == nil {
		t.Fatal("expected invalid preseed batch to fail")
	}
	if submitted := mf.getWorkRequests(); len(submitted) != 0 {
		t.Fatalf("expected no partial submissions, got %d", len(submitted))
	}
	if requests := mf.getWorkRequests(); len(requests) != 0 {
		t.Fatalf("expected no submitted work requests, got %d", len(requests))
	}
}

func TestFileWatcher_IgnoresTempFiles(t *testing.T) {
	requireFileWatcherIntegration(t)
	dir := setupWatchDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger, nil, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Create temp files that should be ignored.
	for _, name := range []string{"file.tmp", "file.swp", "file~"} {
		if err := os.WriteFile(filepath.Join(dir, "request", "default", name), []byte("temp"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Also create a valid file to prove the watcher is running.
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "real.md"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	if len(submitted) != 1 {
		t.Errorf("expected exactly 1 submission (temp files ignored), got %d", len(submitted))
	}
	if string(submitted[0].Works[0].Payload.([]byte)) != "content" {
		t.Errorf("expected real.md payload, got %q", string(submitted[0].Works[0].Payload.([]byte)))
	}
}

func TestFileWatcher_KnownWorkTypes(t *testing.T) {
	requireFileWatcherIntegration(t)
	dir := setupWatchDir(t)
	// Also create an unknown subdirectory with channel layout.
	if err := os.MkdirAll(filepath.Join(dir, "unknown", "default"), 0o755); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger, []string{"request"}, nil, localInputFiles{}, filepath.WalkDir, testWorkRequestIDGenerator)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// File in unknown subdirectory should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "unknown", "default", "task.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// File in known subdirectory should be accepted.
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "task.md"), []byte("accepted"), 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	if len(submitted) != 1 {
		t.Errorf("expected exactly 1 submission, got %d", len(submitted))
	}
	if string(submitted[0].Works[0].Payload.([]byte)) != "accepted" {
		t.Errorf("expected 'accepted' payload, got %q", string(submitted[0].Works[0].Payload.([]byte)))
	}
}

func TestIsTempFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"file.tmp", true},
		{"file.swp", true},
		{"file~", true},
		{".file.swp", true},
		{"file.md", false},
		{"file.json", false},
		{"readme.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTempFile(tt.name); got != tt.want {
				t.Errorf("isTempFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestPreseedInputs_UsesInjectedDirectoryWalker(t *testing.T) {
	walkErr := errors.New("injected walk failure")
	fw := NewFileWatcher(
		t.TempDir(),
		&mockFactory{},
		zap.NewNop(),
		nil,
		nil,
		localInputFiles{},
		func(string, fs.WalkDirFunc) error { return walkErr },
		testWorkRequestIDGenerator,
	)

	err := fw.PreseedInputs(context.Background())
	if !errors.Is(err, walkErr) {
		t.Fatalf("PreseedInputs error = %v, want injected walk error", err)
	}
}

func TestNewFileWatcher_RequiresDirectoryWalker(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("NewFileWatcher panic = nil, want missing directory walker failure")
		}
	}()
	_ = NewFileWatcher(t.TempDir(), &mockFactory{}, zap.NewNop(), nil, nil, localInputFiles{}, nil, testWorkRequestIDGenerator)
}
