package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWithExecutionBaseDir_SetsHarnessServiceConfig(t *testing.T) {
	cfg := &harnessConfig{}

	WithExecutionBaseDir("C:/factory-root")(cfg)

	if got := cfg.serviceConfig.ExecutionBaseDir; got != "C:/factory-root" {
		t.Fatalf("ExecutionBaseDir = %q, want %q", got, "C:/factory-root")
	}
}

func TestWriteSeedMarkdownFile_WritesCanonicalMarkdownSeed(t *testing.T) {
	dir := t.TempDir()

	WriteSeedMarkdownFile(t, dir, "idea", "architecture-review", []byte("draft"))

	path := filepath.Join(dir, interfaces.InputsDir, "idea", interfaces.DefaultChannelName, "architecture-review.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if string(data) != "draft" {
		t.Fatalf("markdown seed contents = %q, want %q", string(data), "draft")
	}
}

func TestWriteSeedBatchFile_WritesCanonicalBatchSeed(t *testing.T) {
	dir := t.TempDir()
	request := interfaces.WorkRequest{
		RequestID: "request-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "seed",
			WorkID:     "work-1",
			WorkTypeID: "task",
			Payload:    "payload",
		}},
	}

	WriteSeedBatchFile(t, dir, request)

	path := filepath.Join(dir, interfaces.InputsDir, "BATCH", interfaces.DefaultChannelName, "request-1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	var got interfaces.WorkRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", path, err)
	}
	if got.RequestID != request.RequestID {
		t.Fatalf("RequestID = %q, want %q", got.RequestID, request.RequestID)
	}
	if len(got.Works) != 1 || got.Works[0].WorkTypeID != "task" {
		t.Fatalf("batch seed works = %#v, want one task work item", got.Works)
	}
}
