package replay_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
)

// portos:func-length-exception owner=agent-factory reason=generated-factory-serialization-fixture review=2026-07-18 removal=split-fixture-builder-before-next-factory-serialization-change
func TestGeneratedFactoryFromLoadedConfig_EmbedsSplitRuntimeDefinitionsInGeneratedFactory(t *testing.T) {
	factoryDir := t.TempDir()
	writeFactoryJSON(t, factoryDir, map[string]any{
		"name": "customer-project",
		"id":   "customer-project",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
		"workers":   []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"id":        "execute-story-id",
			"name":      "execute-story",
			"worker":    "executor",
			"inputs":    []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "story", "state": "complete"}},
			"resources": []map[string]any{{"name": "agent-slot", "capacity": 1}},
			"stopWords": []string{"BLOCKED"},
		}},
	})
	writeAgentsMD(t, filepath.Join(factoryDir, "workers", "executor"), `---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
timeout: 30s
---
Run the test suite.
`)
	writeAgentsMD(t, filepath.Join(factoryDir, "workstations", "execute-story"), `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompt.md
stopWords: ["DONE"]
limits:
  maxExecutionTime: 20m
  maxRetries: 2
---
Fallback body.
`)
	if err := os.WriteFile(filepath.Join(factoryDir, "workstations", "execute-story", "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := replay.GeneratedFactoryFromLoadedConfig(
		loaded,
		replay.WithGeneratedFactoryWorkflowID("workflow-123"),
		replay.WithGeneratedFactoryMetadata(map[string]string{"code_version": "test-sha"}),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}

	assertGeneratedFactoryMetadata(t, generated, factoryDir, "workflow-123")
	worker := onlyGeneratedWorker(t, generated)
	if worker.Command == nil || *worker.Command != "go" {
		t.Fatalf("generated worker command = %#v, want go", worker.Command)
	}
	if worker.Type == nil || *worker.Type != "SCRIPT_WORKER" {
		t.Fatalf("generated worker type = %#v, want SCRIPT_WORKER", worker.Type)
	}
	workstation := onlyGeneratedWorkstation(t, generated)
	if workstation.Body == nil || *workstation.Body != "Implement {{ .WorkID }}." {
		t.Fatalf("generated workstation body = %#v, want prompt file content", workstation.Body)
	}
	if workstation.Type == nil || *workstation.Type != "MODEL_WORKSTATION" {
		t.Fatalf("generated workstation runtime type = %#v, want MODEL_WORKSTATION", workstation.Type)
	}
	if workstation.StopWords == nil || len(*workstation.StopWords) != 2 || (*workstation.StopWords)[0] != "BLOCKED" || (*workstation.StopWords)[1] != "DONE" {
		t.Fatalf("generated canonical stop words = %#v, want [BLOCKED DONE]", workstation.StopWords)
	}
	if workstation.Resources == nil || len(*workstation.Resources) != 1 || (*workstation.Resources)[0].Capacity != 1 {
		t.Fatalf("generated resources = %#v, want capacity 1", workstation.Resources)
	}
	assertFactoryArtifactUsesGeneratedFactoryOnly(t, generated)
}

func TestGeneratedFactoryFromLoadedConfig_EmbedsInlineDefinitionsWithoutConfigOnlyMaps(t *testing.T) {
	factoryDir := t.TempDir()
	writeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"resources": []map[string]any{},
		"workers": []map[string]any{{
			"name":    "executor",
			"type":    "SCRIPT_WORKER",
			"command": "echo",
			"args":    []string{"ok"},
			"body":    "Run inline.",
		}},
		"workstations": []map[string]any{{
			"id":             "execute-story-id",
			"name":           "execute-story",
			"worker":         "executor",
			"inputs":         []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":        []map[string]string{{"workType": "story", "state": "complete"}},
			"type":           "MODEL_WORKSTATION",
			"body":           "Implement inline {{ .WorkID }}.",
			"stopWords":      []string{"DONE"},
			"limits":         map[string]any{"maxRetries": 2},
		}},
	})

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generated, err := replay.GeneratedFactoryFromLoadedConfig(loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}

	worker := onlyGeneratedWorker(t, generated)
	if worker.Command == nil || *worker.Command != "echo" {
		t.Fatalf("generated inline worker command = %#v, want echo", worker.Command)
	}
	workstation := onlyGeneratedWorkstation(t, generated)
	if workstation.Body == nil || *workstation.Body != "Implement inline {{ .WorkID }}." {
		t.Fatalf("generated inline workstation body = %#v, want inline prompt", workstation.Body)
	}
	if workstation.Limits == nil || workstation.Limits.MaxRetries == nil || *workstation.Limits.MaxRetries != 2 {
		t.Fatalf("generated inline limits = %#v, want max retries 2", workstation.Limits)
	}

	data, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated factory: %v", err)
	}
	for _, forbidden := range forbiddenConfigSerializationKeys() {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("generated factory JSON contains forbidden %q: %s", forbidden, data)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode generated factory JSON: %v", err)
	}
	assertJSONArrayField(t, decoded, "workers")
	assertJSONArrayField(t, decoded, "workstations")
}

func assertGeneratedFactoryMetadata(t *testing.T, generated factoryapi.Factory, factoryDir, workflowID string) {
	t.Helper()
	if generated.FactoryDirectory == nil || *generated.FactoryDirectory != factoryDir {
		t.Fatalf("generated factoryDirectory = %#v, want %q", generated.FactoryDirectory, factoryDir)
	}
	if generated.SourceDirectory == nil || *generated.SourceDirectory != factoryDir {
		t.Fatalf("generated sourceDirectory = %#v, want %q", generated.SourceDirectory, factoryDir)
	}
	if generated.Metadata == nil {
		t.Fatal("expected generated metadata")
	}
	for _, key := range []string{"factory_hash", "workers_hash", "workstations_hash", "runtime_config_hash"} {
		if !strings.HasPrefix((*generated.Metadata)[key], "sha256:") {
			t.Fatalf("metadata %s = %q, want sha256 prefix", key, (*generated.Metadata)[key])
		}
	}
	if (*generated.Metadata)["source_format"] != replay.CurrentSchemaVersion {
		t.Fatalf("source_format = %q, want %q", (*generated.Metadata)["source_format"], replay.CurrentSchemaVersion)
	}
	if (*generated.Metadata)["code_version"] != "test-sha" {
		t.Fatalf("code_version = %q, want test-sha", (*generated.Metadata)["code_version"])
	}
}

func assertFactoryArtifactUsesGeneratedFactoryOnly(t *testing.T, generated factoryapi.Factory) {
	t.Helper()
	artifact, err := replay.NewEventLogArtifactFromFactory(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC), generated, nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
	}
	data, err := replay.MarshalArtifact(artifact)
	if err != nil {
		t.Fatalf("MarshalArtifact: %v", err)
	}
	for _, forbidden := range forbiddenConfigSerializationKeys() {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("artifact JSON contains forbidden %q: %s", forbidden, data)
		}
	}
	if !strings.Contains(string(data), `"factory"`) {
		t.Fatalf("artifact JSON does not contain generated factory payload: %s", data)
	}
}

func onlyGeneratedWorker(t *testing.T, generated factoryapi.Factory) factoryapi.Worker {
	t.Helper()
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("generated workers = %#v, want one worker", generated.Workers)
	}
	return (*generated.Workers)[0]
}

func forbiddenConfigSerializationKeys() []string {
	return []string{
		strings.Join([]string{"effective", "Config"}, ""),
		strings.Join([]string{"__replay", "Effective", "Config"}, ""),
		strings.Join([]string{"runtime", "Worker", "Config"}, ""),
	}
}

func onlyGeneratedWorkstation(t *testing.T, generated factoryapi.Factory) factoryapi.Workstation {
	t.Helper()
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one workstation", generated.Workstations)
	}
	return (*generated.Workstations)[0]
}

func assertJSONArrayField(t *testing.T, decoded map[string]any, field string) {
	t.Helper()
	if _, ok := decoded[field].([]any); !ok {
		t.Fatalf("generated %s field = %#v, want JSON array", field, decoded[field])
	}
}
