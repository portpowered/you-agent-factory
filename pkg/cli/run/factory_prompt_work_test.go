package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareFactoryPromptWorkFile_WritesCanonicalBatchForDefaultWorkType(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(`{
  "name": "portable",
  "workTypes": [{
    "name": "story",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"}
    ]
  }],
  "workstations": [{
    "name": "ws",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}]
  }]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	workFile, err := PrepareFactoryPromptWorkFile(factoryPath, "Fix the lint issues")
	if err != nil {
		t.Fatalf("PrepareFactoryPromptWorkFile: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(workFile) })

	got, err := LoadWorkFile(workFile)
	if err != nil {
		t.Fatalf("LoadWorkFile: %v", err)
	}
	if len(got.Works) != 1 {
		t.Fatalf("works = %#v, want one item", got.Works)
	}
	work := got.Works[0]
	if work.WorkTypeID != "story" {
		t.Fatalf("work type = %q, want story", work.WorkTypeID)
	}
	if payload, ok := work.Payload.(string); !ok || payload != "Fix the lint issues" {
		t.Fatalf("payload = %#v, want raw prompt text", work.Payload)
	}
	if strings.TrimSpace(work.Name) == "" {
		t.Fatal("expected generated work name")
	}
}

func TestPrepareFactoryPromptWorkFile_RejectsEmptyPrompt(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(`{
  "name": "portable",
  "workTypes": [{
    "name": "story",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"}
    ]
  }]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := PrepareFactoryPromptWorkFile(factoryPath, "   ")
	if err == nil {
		t.Fatal("expected empty prompt rejection")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("error = %q, want prompt requirement", err.Error())
	}
}
