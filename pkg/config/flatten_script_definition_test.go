package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

// portos:func-length-exception owner=agent-factory reason=script-backed-flatten-portability-fixture review=2026-07-21 removal=extract-shared-inline-script-fixture-before-next-flatten-expand-portability-change
func TestFlattenFactoryConfig_FlattensInlineScriptBackedWorkstationWithoutSplitAgentsFile(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{"name": "executor"},
		},
		"workstations": []map[string]any{
			{
				"name":             "execute-story",
				"worker":           "executor",
				"inputs":           []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":          []map[string]string{{"workType": "story", "state": "complete"}},
				"workingDirectory": "/repo/{{ .WorkID }}",
				"worktree":         "worktrees/{{ .WorkID }}",
				"env":              map[string]string{"SCRIPT_MODE": "portable"},
			},
		},
	})
	writeRuntimeWorkerAgentsMD(t, factoryDir, "executor", `---
type: SCRIPT_WORKER
command: powershell
args: ["-File", "scripts/execute-story.ps1"]
timeout: 45m
---
Execute the story script.
`)

	flattened, err := FlattenFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	cfg, err := FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if len(cfg.Workers) != 1 || len(cfg.Workstations) != 1 {
		t.Fatalf("expected flattened config to preserve one worker and workstation, got %d/%d", len(cfg.Workers), len(cfg.Workstations))
	}
	worker := cfg.Workers[0]
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "powershell" {
		t.Fatalf("flattened worker definition = %#v", worker)
	}
	if len(worker.Args) != 2 || worker.Args[0] != "-File" || worker.Args[1] != "scripts/execute-story.ps1" {
		t.Fatalf("flattened worker args = %#v", worker.Args)
	}
	workstation := cfg.Workstations[0]
	if workstation.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("flattened workstation type = %q, want %q", workstation.Type, interfaces.WorkstationTypeModel)
	}
	if workstation.WorkingDirectory != "/repo/{{ .WorkID }}" || workstation.Worktree != "worktrees/{{ .WorkID }}" {
		t.Fatalf("flattened workstation execution context = %#v", workstation)
	}
	if workstation.Env["SCRIPT_MODE"] != "portable" {
		t.Fatalf("flattened workstation env = %#v", workstation.Env)
	}

	standaloneDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(standaloneDir, interfaces.FactoryConfigFile), flattened, 0o644); err != nil {
		t.Fatalf("write standalone factory.json: %v", err)
	}

	loaded, err := LoadRuntimeConfig(standaloneDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(standalone flattened config): %v", err)
	}
	loadedWorker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected flattened script worker definition to load")
	}
	if loadedWorker.Type != interfaces.WorkerTypeScript || loadedWorker.Command != "powershell" || loadedWorker.Timeout != "45m" {
		t.Fatalf("loaded script worker definition = %#v", loadedWorker)
	}
	loadedWorkstation, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected flattened inline workstation definition to load")
	}
	if loadedWorkstation.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("loaded workstation type = %q, want %q", loadedWorkstation.Type, interfaces.WorkstationTypeModel)
	}
	if loadedWorkstation.WorkingDirectory != "/repo/{{ .WorkID }}" || loadedWorkstation.Worktree != "worktrees/{{ .WorkID }}" {
		t.Fatalf("loaded workstation execution context = %#v", loadedWorkstation)
	}
	if loadedWorkstation.Env["SCRIPT_MODE"] != "portable" {
		t.Fatalf("loaded workstation env = %#v", loadedWorkstation.Env)
	}
}

func TestLoadRuntimeConfig_RejectsMissingSplitWorkstationWhenScriptExecutionContextIsInline(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name":    "executor",
				"type":    "SCRIPT_WORKER",
				"command": "powershell",
				"args":    []string{"-File", "scripts/execute-story.ps1"},
			},
		},
		"workstations": []map[string]any{
			{
				"name":             "execute-story",
				"worker":           "executor",
				"inputs":           []map[string]string{{"workType": "story", "state": "init"}},
				"outputs":          []map[string]string{{"workType": "story", "state": "complete"}},
				"workingDirectory": "/repo/{{ .WorkID }}",
				"worktree":         "worktrees/{{ .WorkID }}",
				"env":              map[string]string{"SCRIPT_MODE": "portable"},
			},
			{
				"name":    "review-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "complete"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
			},
		},
	})

	_, err := LoadRuntimeConfig(factoryDir, nil)
	if err == nil {
		t.Fatal("expected missing split workstation definition to be rejected")
	}
	if !strings.Contains(err.Error(), `load workstation "review-story" config`) {
		t.Fatalf("expected targeted workstation path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `workstation "review-story" is missing definition and no AGENTS.md was found`) {
		t.Fatalf("expected missing workstation definition error, got %v", err)
	}
	if strings.Contains(err.Error(), `workstation "execute-story"`) {
		t.Fatalf("expected error to point at the missing workstation only, got %v", err)
	}
}
