package replay_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
)

func writeEmbeddedFactoryFixture(t *testing.T, factoryDir string) {
	t.Helper()

	factoryfixtures.WriteFactoryJSON(t, factoryDir, map[string]any{
		"name": "customer-facing-name",
		"id":   "internal-id",
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
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper keeps the embedded generated-factory artifact contract together across all authored sections.
func assertEmbeddedGeneratedFactory(t *testing.T, generated factoryapi.Factory, factoryDir string) {
	t.Helper()

	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one", generated.Workstations)
	}
	if generated.Name != "customer-facing-name" {
		t.Fatalf("generated factory name = %q, want customer-facing-name", generated.Name)
	}
	if generated.Id == nil || *generated.Id != "internal-id" {
		t.Fatalf("generated factory id = %#v, want internal-id", generated.Id)
	}
	workstation := (*generated.Workstations)[0]
	if workstation.Name != "execute-story" {
		t.Fatalf("generated workstation name = %q, want execute-story", workstation.Name)
	}
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("generated workers = %#v, want one", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.Command == nil || *worker.Command != "go" {
		t.Fatalf("generated worker command = %#v, want go", worker.Command)
	}
	if workstation.Type == nil || *workstation.Type != "MODEL_WORKSTATION" {
		t.Fatalf("generated workstation runtime type = %#v, want MODEL_WORKSTATION", workstation.Type)
	}
	if workstation.Body == nil || *workstation.Body != "Implement {{ .WorkID }}." {
		t.Fatalf("generated workstation body = %#v, want prompt file content", workstation.Body)
	}
	if generated.SourceDirectory == nil || *generated.SourceDirectory != factoryDir {
		t.Fatalf("source directory = %#v, want %q", generated.SourceDirectory, factoryDir)
	}
	if generated.Metadata == nil {
		t.Fatal("expected generated metadata")
	}
	if !strings.HasPrefix((*generated.Metadata)["factory_hash"], "sha256:") {
		t.Fatalf("factory_hash metadata = %q, want sha256 prefix", (*generated.Metadata)["factory_hash"])
	}
	if !strings.HasPrefix((*generated.Metadata)["runtime_config_hash"], "sha256:") {
		t.Fatalf("runtime_config_hash metadata = %q, want sha256 prefix", (*generated.Metadata)["runtime_config_hash"])
	}
	if (*generated.Metadata)["code_version"] != "test-sha" {
		t.Fatalf("code_version metadata = %q, want test-sha", (*generated.Metadata)["code_version"])
	}
}

func assertEmbeddedRuntimeConfig(t *testing.T, runtimeCfg interfaces.RuntimeConfigLookup, factoryDir string) {
	t.Helper()

	assertCanonicalReplayFactoryDir(t, runtimeCfg, factoryDir)
	assertCanonicalReplayRuntimeBaseDir(t, runtimeCfg, factoryDir)
	workerDef, workstationDef := assertCanonicalRuntimeDefinitionLookupByName(t, runtimeCfg, "executor", "execute-story")
	if workerDef.Command != "go" {
		t.Fatalf("runtime worker = %#v, want command go", workerDef)
	}
	if workstationDef.PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("runtime workstation = %#v, want prompt template", workstationDef)
	}
	if workstationDef.Timeout != "" {
		t.Fatalf("runtime workstation timeout alias = %q, want cleared", workstationDef.Timeout)
	}
	if workstationDef.Limits.MaxExecutionTime != "20m" {
		t.Fatalf("runtime workstation max execution time = %q, want 20m", workstationDef.Limits.MaxExecutionTime)
	}
	if workstationDef.ID != "execute-story-id" {
		t.Fatalf("runtime workstation ID = %q, want execute-story-id", workstationDef.ID)
	}
}

func writeRebuildGeneratedFactoryFixture(t *testing.T, factoryDir string) {
	t.Helper()

	factoryfixtures.WriteFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"resources": []map[string]any{},
		"workers":   []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"id":      "execute-story-id",
			"name":    "execute-story",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
			"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
		}},
	})
	writeAgentsMD(t, filepath.Join(factoryDir, "workers", "executor"), `---
type: SCRIPT_WORKER
command: echo
args: ["ok"]
---
Script worker body.
`)
	writeAgentsMD(t, filepath.Join(factoryDir, "workstations", "execute-story"), `---
type: LOGICAL_MOVE
worker: executor
---
Move the token.
`)
}

func assertRebuiltRuntimeConfig(t *testing.T, runtimeCfg interfaces.RuntimeConfigLookup, factoryDir string) {
	t.Helper()

	assertCanonicalReplayFactoryDir(t, runtimeCfg, factoryDir)
	assertCanonicalReplayRuntimeBaseDir(t, runtimeCfg, factoryDir)
	embedded, ok := runtimeCfg.(*replay.EmbeddedRuntimeConfig)
	if !ok {
		t.Fatalf("runtime config = %#v, want embedded replay runtime config", runtimeCfg)
	}
	factoryCfg := embedded.FactoryConfig()
	if factoryCfg.WorkTypes[0].Name != "story" {
		t.Fatalf("runtime factory work type = %q, want story", factoryCfg.WorkTypes[0].Name)
	}
	workerDef, workstationDef := assertCanonicalRuntimeDefinitionLookupByName(t, runtimeCfg, "executor", "execute-story")
	if workerDef.Command != "echo" || len(workerDef.Args) != 1 || workerDef.Args[0] != "ok" {
		t.Fatalf("embedded worker command = %q args=%v, want echo [ok]", workerDef.Command, workerDef.Args)
	}
	if workstationDef.Type != "LOGICAL_MOVE" {
		t.Fatalf("embedded workstation runtime type = %q, want LOGICAL_MOVE", workstationDef.Type)
	}
	if workstationDef.ID != "execute-story-id" {
		t.Fatalf("embedded workstation ID = %q, want execute-story-id", workstationDef.ID)
	}
}
