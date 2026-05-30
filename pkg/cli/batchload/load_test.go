package batchload

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.json")

	req := interfaces.WorkRequest{
		RequestID: "request-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "source-file",
			WorkTypeID: "task",
			TraceID:    "trace-1",
			Payload:    map[string]any{"file": "test.go"},
			Tags:       map[string]string{"priority": "high"},
		}},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if got.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Errorf("Type = %q, want %q", got.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
	}
	if len(got.Works) != 1 || got.Works[0].WorkTypeID != "task" {
		t.Fatalf("Works = %#v, want one task work item", got.Works)
	}
	if got.Works[0].TraceID != "trace-1" {
		t.Errorf("TraceID = %q, want trace-1", got.Works[0].TraceID)
	}
}

func TestLoadFromFile_DocsExampleStartupWorkFile(t *testing.T) {
	path := testutil.MustRepoPath(t, "docs/examples/startup-work.json")

	got, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile(%q): %v", path, err)
	}
	if got.RequestID != "docs-example-story-001" {
		t.Fatalf("request ID = %q, want docs-example-story-001", got.RequestID)
	}
	if got.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("type = %q, want %q", got.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
	}
	if len(got.Works) != 1 {
		t.Fatalf("work count = %d, want 1", len(got.Works))
	}

	work := got.Works[0]
	if work.WorkTypeID != "story" {
		t.Fatalf("work type = %q, want story", work.WorkTypeID)
	}
	if work.State != "init" {
		t.Fatalf("state = %q, want init", work.State)
	}
	if work.Payload == nil {
		t.Fatal("payload is empty")
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadFromFile_RejectsRetiredTargetStateAlias(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.json")
	writeFile(t, path, `{
  "request_id": "request-cli-target-state",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "draft", "work_type_name": "task", "target_state": "waiting"}
  ]
}`)

	_, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected retired target_state alias to fail")
	}
	if !strings.Contains(err.Error(), "target_state") || !strings.Contains(err.Error(), "state") {
		t.Fatalf("error = %q, want target_state rejection with state guidance", err.Error())
	}
}

func TestLoadFromFile_RejectsConflictingTraceAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.json")
	writeFile(t, path, `{
  "requestId": "request-cli-trace-conflict",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "draft",
      "workTypeName": "task",
      "currentChainingTraceId": "chain-a",
      "traceId": "trace-b"
    }
  ]
}`)

	_, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected conflicting trace aliases to fail")
	}
	if !strings.Contains(err.Error(), "currentChainingTraceId and traceId must match") {
		t.Fatalf("error = %q, want conflicting trace alias rejection", err.Error())
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
