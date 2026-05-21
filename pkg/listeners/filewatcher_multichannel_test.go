package listeners

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestFileWatcher_MultiChannel_DefaultDir(t *testing.T) {
	dir := setupMultiChannelDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger,
		WithKnownWorkTypes([]string{"task"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	content := []byte("# Default task")
	if err := os.WriteFile(filepath.Join(dir, "task", "default", "work.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	if submitted[0].WorkTypeID != "task" {
		t.Errorf("expected WorkTypeID 'task', got %q", submitted[0].WorkTypeID)
	}
	if submitted[0].ExecutionID != "" {
		t.Errorf("expected empty ExecutionID for default channel, got %q", submitted[0].ExecutionID)
	}
	if string(submitted[0].Payload) != string(content) {
		t.Errorf("payload mismatch: got %q", string(submitted[0].Payload))
	}
}

func TestFileWatcher_MultiChannel_ExecutionIDDir(t *testing.T) {
	dir := setupMultiChannelDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	execDir := filepath.Join(dir, "task", "exec-123")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fw := NewFileWatcher(dir, mf, logger,
		WithKnownWorkTypes([]string{"task"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	content := []byte(`{"title": "executor-generated work"}`)
	if err := os.WriteFile(filepath.Join(execDir, "work.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	if submitted[0].WorkTypeID != "task" {
		t.Errorf("expected WorkTypeID 'task', got %q", submitted[0].WorkTypeID)
	}
	if submitted[0].ExecutionID != "exec-123" {
		t.Errorf("expected ExecutionID 'exec-123', got %q", submitted[0].ExecutionID)
	}
}

func TestFileWatcher_MultiChannel_BatchDefaultDir(t *testing.T) {
	dir := setupWatchDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger,
		WithKnownWorkTypes([]string{"request", "story"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	content := []byte(`{
		"requestId": "request-batch-default-live",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "story-set", "workTypeName": "request", "state": "waiting"},
			{"name": "story-a", "workTypeName": "story", "payload": {"step": "child"}}
		],
		"relations": [
			{"type": "PARENT_CHILD", "sourceWorkName": "story-a", "targetWorkName": "story-set"}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "BATCH", "default", "batch.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 2)
	requests := mf.getWorkRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(requests))
	}
	if requests[0].Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want FACTORY_REQUEST_BATCH", requests[0].Type)
	}
	if len(requests[0].Works) != 2 {
		t.Fatalf("expected 2 works in batch request, got %d", len(requests[0].Works))
	}
	if requests[0].Works[0].State != "waiting" {
		t.Fatalf("parent state = %q, want waiting", requests[0].Works[0].State)
	}
	if len(requests[0].Relations) != 1 {
		t.Fatalf("expected 1 batch relation, got %d", len(requests[0].Relations))
	}
	if requests[0].Relations[0].Type != interfaces.WorkRelationParentChild {
		t.Fatalf("request relation type = %q, want %q", requests[0].Relations[0].Type, interfaces.WorkRelationParentChild)
	}
	if len(submitted) != 2 {
		t.Fatalf("expected 2 submitted works, got %d", len(submitted))
	}
	if submitted[0].TargetState != "waiting" {
		t.Fatalf("normalized parent target state = %q, want waiting", submitted[0].TargetState)
	}
	if submitted[1].WorkTypeID != "story" {
		t.Fatalf("child work type = %q, want story", submitted[1].WorkTypeID)
	}
	if len(submitted[1].Relations) != 1 {
		t.Fatalf("expected child relation on second work, got %d", len(submitted[1].Relations))
	}
	if submitted[1].Relations[0].Type != interfaces.RelationParentChild {
		t.Fatalf("normalized relation type = %q, want %q", submitted[1].Relations[0].Type, interfaces.RelationParentChild)
	}
}

func TestFileWatcher_MultiChannel_DynamicSubdir(t *testing.T) {
	dir := setupMultiChannelDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger,
		WithKnownWorkTypes([]string{"task"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	execDir := filepath.Join(dir, "task", "exec-456")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(execDir, "work.md"), []byte("dynamic"), 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	if submitted[0].WorkTypeID != "task" {
		t.Errorf("expected WorkTypeID 'task', got %q", submitted[0].WorkTypeID)
	}
	if submitted[0].ExecutionID != "exec-456" {
		t.Errorf("expected ExecutionID 'exec-456', got %q", submitted[0].ExecutionID)
	}
}

func TestFileWatcher_MultiChannel_JSONNonBatchUsesExecutionIDDir(t *testing.T) {
	dir := setupMultiChannelDir(t)
	mf := &mockFactory{}
	logger := zap.NewNop()

	execDir := filepath.Join(dir, "task", "exec-789")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fw := NewFileWatcher(dir, mf, logger,
		WithKnownWorkTypes([]string{"task"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	content := []byte(`{"execution_id":"original-exec-id","payload":"raw json"}`)
	if err := os.WriteFile(filepath.Join(execDir, "batch.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	if submitted[0].ExecutionID != "exec-789" {
		t.Errorf("expected ExecutionID 'exec-789', got %q", submitted[0].ExecutionID)
	}
}

func TestFileWatcher_MultiChannel_MultipleWorkTypes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "chapter", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "page", "default"), 0o755); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger,
		WithKnownWorkTypes([]string{"chapter", "page"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, "chapter", "default", "ch1.md"), []byte("chapter 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "page", "default", "p1.md"), []byte("page 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 2)

	workTypes := map[string]bool{}
	for _, s := range submitted {
		workTypes[s.WorkTypeID] = true
	}
	if !workTypes["chapter"] {
		t.Error("expected submission for work type 'chapter'")
	}
	if !workTypes["page"] {
		t.Error("expected submission for work type 'page'")
	}
}

func TestFileWatcher_RejectsDirectWorkTypeDrop(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "task", "default"), 0o755); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger,
		WithKnownWorkTypes([]string{"task"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fw.Watch(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, "task", "work.md"), []byte("# Direct task"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertNoSubmissionWithin(t, mf, 300*time.Millisecond)

	content := []byte("# Canonical task")
	if err := os.WriteFile(filepath.Join(dir, "task", "default", "work.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	submitted := waitForSubmission(t, mf, 1)
	if submitted[0].WorkTypeID != "task" {
		t.Errorf("expected WorkTypeID 'task', got %q", submitted[0].WorkTypeID)
	}
	if submitted[0].ExecutionID != "" {
		t.Errorf("expected empty ExecutionID for default channel, got %q", submitted[0].ExecutionID)
	}
	if string(submitted[0].Payload) != string(content) {
		t.Errorf("payload mismatch: got %q", string(submitted[0].Payload))
	}
}

func TestFileWatcher_MDFile_NameFromFilename(t *testing.T) {
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

	content := []byte("# Factory Bug Init\nDetails here.")
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "factory-bug-init.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	if submitted[0].Name != "factory-bug-init" {
		t.Errorf("expected Name 'factory-bug-init', got %q", submitted[0].Name)
	}
}

func TestFileWatcher_JSONNonBatch_NameFromFilename(t *testing.T) {
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

	content := []byte(`{"name":"custom-name","payload":"some payload"}`)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "different-filename.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	submitted := waitForSubmission(t, mf, 1)
	if submitted[0].Name != "different-filename" {
		t.Errorf("expected Name 'different-filename', got %q", submitted[0].Name)
	}
}

func TestFileWatcher_PreseedSkipsDirectWorkTypeDrop(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "task", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task", "work.md"), []byte("# Invalid preseeded task"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Canonical preseeded task")
	if err := os.WriteFile(filepath.Join(dir, "task", "default", "work.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &mockFactory{}
	logger := zap.NewNop()

	fw := NewFileWatcher(dir, mf, logger,
		WithKnownWorkTypes([]string{"task"}))

	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	submitted := mf.getSubmitted()
	if len(submitted) != 1 {
		t.Fatalf("expected 1 submission, got %d", len(submitted))
	}
	if submitted[0].WorkTypeID != "task" {
		t.Errorf("expected WorkTypeID 'task', got %q", submitted[0].WorkTypeID)
	}
	if submitted[0].ExecutionID != "" {
		t.Errorf("expected empty ExecutionID for default channel, got %q", submitted[0].ExecutionID)
	}
	if string(submitted[0].Payload) != string(content) {
		t.Errorf("payload mismatch: got %q", string(submitted[0].Payload))
	}
}
