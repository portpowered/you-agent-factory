package listeners

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
)

// mockFactory records SubmitWorkRequest calls for test assertions.
type mockFactory struct {
	mu           sync.Mutex
	submitted    []interfaces.SubmitRequest
	workRequests []interfaces.WorkRequest
}

func (m *mockFactory) SubmitWorkRequest(_ context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	normalized, err := factory.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{})
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workRequests = append(m.workRequests, cloneWorkRequest(request))
	m.submitted = append(m.submitted, normalized...)
	result := interfaces.WorkRequestSubmitResult{RequestID: request.RequestID, Accepted: true}
	if len(normalized) > 0 {
		result.TraceID = normalized[0].TraceID
	}
	return result, nil
}

func (m *mockFactory) Run(_ context.Context) error { return nil }
func (m *mockFactory) SubscribeFactoryEvents(_ context.Context) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{Events: make(chan factoryapi.FactoryEvent)}, nil
}
func (m *mockFactory) Pause(_ context.Context) error { return nil }

func (m *mockFactory) GetEngineStateSnapshot(_ context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{}, nil
}
func (m *mockFactory) GetFactoryEvents(_ context.Context) ([]factoryapi.FactoryEvent, error) {
	return nil, nil
}
func (m *mockFactory) WaitToComplete() <-chan struct{} {
	return make(chan struct{})
}

func (m *mockFactory) getSubmitted() []interfaces.SubmitRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]interfaces.SubmitRequest, len(m.submitted))
	copy(out, m.submitted)
	return out
}

func (m *mockFactory) getWorkRequests() []interfaces.WorkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]interfaces.WorkRequest, len(m.workRequests))
	for i := range m.workRequests {
		out[i] = cloneWorkRequest(m.workRequests[i])
	}
	return out
}

func cloneWorkRequest(request interfaces.WorkRequest) interfaces.WorkRequest {
	out := request
	out.Works = make([]interfaces.Work, len(request.Works))
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
	out.Relations = append([]interfaces.WorkRelation(nil), request.Relations...)
	return out
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

func waitForSubmission(t *testing.T, mf *mockFactory, count int) []interfaces.SubmitRequest {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		submitted := mf.getSubmitted()
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
	if got := len(mf.getSubmitted()); got != 0 {
		t.Fatalf("expected no submissions, got %d", got)
	}
}

func TestFileWatcher_MDFile(t *testing.T) {
	dir := setupWatchDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger)

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
	requests := mf.getWorkRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if requests[0].Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want FACTORY_REQUEST_BATCH", requests[0].Type)
	}
	if len(requests[0].Works) != 1 {
		t.Fatalf("expected 1 work in request, got %d", len(requests[0].Works))
	}
	if requests[0].Works[0].WorkTypeID != "request" {
		t.Errorf("expected wrapped work_type_name 'request', got %q", requests[0].Works[0].WorkTypeID)
	}
	if submitted[0].WorkTypeID != "request" {
		t.Errorf("expected WorkTypeID 'request', got %q", submitted[0].WorkTypeID)
	}
	if string(submitted[0].Payload) != string(content) {
		t.Errorf("payload mismatch: got %q", string(submitted[0].Payload))
	}
}

func TestFileWatcher_JSONNonBatchWrapsRawPayload(t *testing.T) {
	dir := setupWatchDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger)

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
	requests := mf.getWorkRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if requests[0].Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want FACTORY_REQUEST_BATCH", requests[0].Type)
	}
	if got := requests[0].Works[0].WorkTypeID; got != "request" {
		t.Errorf("wrapped work_type_name = %q, want request", got)
	}
	if submitted[0].WorkTypeID != "request" {
		t.Errorf("expected WorkTypeID 'request', got %q", submitted[0].WorkTypeID)
	}
	if string(submitted[0].Payload) != string(content) {
		t.Errorf("payload mismatch: got %q", string(submitted[0].Payload))
	}
	if submitted[0].Name != "batch" {
		t.Errorf("expected Name 'batch', got %q", submitted[0].Name)
	}
}

func TestFileWatcher_JSONFallbackPayload(t *testing.T) {
	dir := setupWatchDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger)

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
	requests := mf.getWorkRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if got := requests[0].Works[0].WorkTypeID; got != "request" {
		t.Errorf("wrapped work_type_name = %q, want request", got)
	}
	if submitted[0].WorkTypeID != "request" {
		t.Errorf("expected WorkTypeID 'request', got %q", submitted[0].WorkTypeID)
	}
	if string(submitted[0].Payload) != string(content) {
		t.Errorf("payload mismatch: got %q", string(submitted[0].Payload))
	}
}

func TestFileWatcher_JSONFactoryRequestBatch(t *testing.T) {
	dir := setupWatchDir(t)
	batch := interfaces.WorkRequest{
		RequestID: "request-batch-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
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
		Relations: []interfaces.WorkRelation{
			{
				Type:           interfaces.WorkRelationDependsOn,
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
	fw := NewFileWatcher(dir, mf, zap.NewNop(), WithKnownWorkTypes([]string{"request"}))
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	submitted := mf.getSubmitted()
	requests := mf.getWorkRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if requests[0].Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want FACTORY_REQUEST_BATCH", requests[0].Type)
	}
	if len(requests[0].Works) != 2 {
		t.Fatalf("expected 2 works in submitted batch, got %d", len(requests[0].Works))
	}
	if requests[0].Works[0].WorkTypeID != "request" || requests[0].Works[1].WorkTypeID != "request" {
		t.Fatalf("expected batch work_type_name fields filled from watched folder, got %#v", requests[0].Works)
	}
	if len(submitted) != 2 {
		t.Fatalf("expected 2 submissions, got %d", len(submitted))
	}
	if submitted[0].WorkTypeID != "request" {
		t.Errorf("expected inferred WorkTypeID 'request', got %q", submitted[0].WorkTypeID)
	}
	if submitted[1].TraceID != "trace-batch" {
		t.Errorf("expected shared trace ID from first item, got %q", submitted[1].TraceID)
	}
	if len(submitted[1].Relations) != 1 {
		t.Fatalf("expected dependency relation on second work, got %d", len(submitted[1].Relations))
	}
	if submitted[1].Relations[0].TargetWorkID != "batch-request-batch-1-first" {
		t.Errorf("expected relation target work ID for first item, got %q", submitted[1].Relations[0].TargetWorkID)
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
	fw := NewFileWatcher(dir, mf, zap.NewNop(),
		WithKnownWorkTypes([]string{"request"}),
		WithKnownWorkStates(map[string]map[string]bool{"request": {"queued": true, "complete": true}}),
	)
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	submitted := mf.getSubmitted()
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
	if len(submitted) != 2 {
		t.Fatalf("expected 2 submissions, got %d", len(submitted))
	}
	if submitted[0].WorkTypeID != "request" {
		t.Errorf("expected WorkTypeID 'request', got %q", submitted[0].WorkTypeID)
	}
	if submitted[0].TargetState != "queued" {
		t.Fatalf("expected normalized target state queued, got %q", submitted[0].TargetState)
	}
	if len(submitted[1].Relations) != 1 {
		t.Fatalf("expected dependency relation on second work, got %d", len(submitted[1].Relations))
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this watcher batch-contract test keeps parent-child mapping and canonical submit assertions in one scenario.
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
	fw := NewFileWatcher(dir, mf, zap.NewNop(), WithKnownWorkTypes([]string{"request"}))
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	submitted := mf.getSubmitted()
	if len(submitted) != 3 {
		t.Fatalf("expected 3 submissions, got %d", len(submitted))
	}

	var child interfaces.SubmitRequest
	for _, request := range submitted {
		if request.Name == "child" {
			child = request
			break
		}
	}
	if child.Name == "" {
		t.Fatal("expected child submission")
	}
	if child.TraceID != "trace-parent-child" {
		t.Fatalf("child trace ID = %q, want trace-parent-child", child.TraceID)
	}
	if len(child.Relations) != 2 {
		t.Fatalf("child relations = %d, want 2", len(child.Relations))
	}

	var foundParentChild bool
	var foundDependsOn bool
	for _, relation := range child.Relations {
		switch relation.Type {
		case interfaces.RelationParentChild:
			foundParentChild = true
			if relation.TargetWorkID != "batch-request-batch-parent-child-parent" {
				t.Fatalf("parent-child target = %q, want batch-request-batch-parent-child-parent", relation.TargetWorkID)
			}
		case interfaces.RelationDependsOn:
			foundDependsOn = true
			if relation.TargetWorkID != "batch-request-batch-parent-child-prerequisite" {
				t.Fatalf("depends_on target = %q, want batch-request-batch-parent-child-prerequisite", relation.TargetWorkID)
			}
		default:
			t.Fatalf("unexpected relation = %#v", relation)
		}
	}
	if !foundParentChild {
		t.Fatal("missing parent-child relation")
	}
	if !foundDependsOn {
		t.Fatal("missing depends_on relation")
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
	fw := NewFileWatcher(dir, mf, zap.NewNop(), WithKnownWorkTypes([]string{"request", "story"}))
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	submitted := mf.getSubmitted()
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
	if requests[0].Relations[0].Type != interfaces.WorkRelationParentChild {
		t.Fatalf("request relation type = %q, want %q", requests[0].Relations[0].Type, interfaces.WorkRelationParentChild)
	}
	if len(submitted) != 2 {
		t.Fatalf("expected 2 submissions, got %d", len(submitted))
	}
	if submitted[1].WorkTypeID != "story" {
		t.Fatalf("child work type = %q, want story", submitted[1].WorkTypeID)
	}
	if submitted[0].TargetState != "waiting" {
		t.Fatalf("normalized parent target state = %q, want waiting", submitted[0].TargetState)
	}
	if len(submitted[1].Relations) != 1 {
		t.Fatalf("expected child relation on second work, got %d", len(submitted[1].Relations))
	}
	if submitted[1].Relations[0].Type != interfaces.RelationParentChild {
		t.Fatalf("normalized relation type = %q, want %q", submitted[1].Relations[0].Type, interfaces.RelationParentChild)
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
	fw := NewFileWatcher(dir, mf, zap.NewNop(), WithKnownWorkTypes([]string{"request"}))
	err := fw.PreseedInputs(context.Background())
	if err == nil {
		t.Fatal("expected retired work_type_id alias to fail")
	}
	if !strings.Contains(err.Error(), "work_type_id") || !strings.Contains(err.Error(), "workTypeName") {
		t.Fatalf("error = %q, want work_type_id rejection with workTypeName guidance", err.Error())
	}
	if submitted := mf.getSubmitted(); len(submitted) != 0 {
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
	fw := NewFileWatcher(dir, mf, zap.NewNop(),
		WithKnownWorkTypes([]string{"request"}),
		WithKnownWorkStates(map[string]map[string]bool{"request": {"waiting": true, "complete": true}}),
	)
	err := fw.PreseedInputs(context.Background())
	if err == nil {
		t.Fatal("expected retired target_state alias to fail")
	}
	if !strings.Contains(err.Error(), "target_state") || !strings.Contains(err.Error(), "state") {
		t.Fatalf("error = %q, want target_state rejection with state guidance", err.Error())
	}
	if submitted := mf.getSubmitted(); len(submitted) != 0 {
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
	fw := NewFileWatcher(dir, mf, zap.NewNop(), WithKnownWorkTypes([]string{"request"}))
	err := fw.PreseedInputs(context.Background())
	if err == nil {
		t.Fatal("expected conflicting trace aliases to fail")
	}
	if !strings.Contains(err.Error(), "currentChainingTraceId and traceId must match") {
		t.Fatalf("error = %q, want conflicting trace alias rejection", err.Error())
	}
	if submitted := mf.getSubmitted(); len(submitted) != 0 {
		t.Fatalf("expected no partial submissions, got %d", len(submitted))
	}
	if requests := mf.getWorkRequests(); len(requests) != 0 {
		t.Fatalf("expected no submitted work requests, got %d", len(requests))
	}
}

func TestFileWatcher_JSONFactoryRequestBatchRejectsConflictingWorkType(t *testing.T) {
	dir := setupWatchDir(t)
	batch := interfaces.WorkRequest{
		RequestID: "request-batch-conflict",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
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
	fw := NewFileWatcher(dir, mf, zap.NewNop(), WithKnownWorkTypes([]string{"request"}))
	if err := fw.PreseedInputs(context.Background()); err == nil {
		t.Fatal("expected conflicting batch work type to fail")
	}
	if submitted := mf.getSubmitted(); len(submitted) != 0 {
		t.Fatalf("expected no partial submissions, got %d", len(submitted))
	}
}

func TestFileWatcher_PreseedValidatesAllFilesBeforeSubmitting(t *testing.T) {
	dir := setupWatchDir(t)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "a-valid.md"), []byte("valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	batch := interfaces.WorkRequest{
		RequestID: "request-empty-batch",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works:     []interfaces.Work{},
	}
	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "z-invalid.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	fw := NewFileWatcher(dir, mf, zap.NewNop(), WithKnownWorkTypes([]string{"request"}))
	if err := fw.PreseedInputs(context.Background()); err == nil {
		t.Fatal("expected invalid preseed batch to fail")
	}
	if submitted := mf.getSubmitted(); len(submitted) != 0 {
		t.Fatalf("expected no partial submissions, got %d", len(submitted))
	}
	if requests := mf.getWorkRequests(); len(requests) != 0 {
		t.Fatalf("expected no submitted work requests, got %d", len(requests))
	}
}

func TestFileWatcher_IgnoresTempFiles(t *testing.T) {
	dir := setupWatchDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger)

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
	if string(submitted[0].Payload) != "content" {
		t.Errorf("expected real.md payload, got %q", string(submitted[0].Payload))
	}
}

func TestFileWatcher_KnownWorkTypes(t *testing.T) {
	dir := setupWatchDir(t)
	// Also create an unknown subdirectory with channel layout.
	if err := os.MkdirAll(filepath.Join(dir, "unknown", "default"), 0o755); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger,
		WithKnownWorkTypes([]string{"request"}))

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
	if string(submitted[0].Payload) != "accepted" {
		t.Errorf("expected 'accepted' payload, got %q", string(submitted[0].Payload))
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
