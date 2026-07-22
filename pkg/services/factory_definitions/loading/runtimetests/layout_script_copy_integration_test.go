package runtimetests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type portableCopyRoundTrip struct {
	targetDir string
	loaded    *LoadedFactoryConfig
}

func TestFlattenExpandInlineScriptFactory_RoundTripsCopyFlagThroughLoad(t *testing.T) {
	tests := []struct {
		name       string
		copyScript bool
		wantCopied bool
	}{
		{
			name:       "copy enabled",
			copyScript: true,
			wantCopied: true,
		},
		{
			name:       "copy disabled",
			copyScript: false,
			wantCopied: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roundTrip := buildPortableCopyRoundTrip(t, tt.copyScript)
			assertPortableExpandedScriptCopy(t, roundTrip.targetDir, tt.wantCopied)
			assertPortableExpandedRuntimeConfig(t, roundTrip.loaded, tt.copyScript)
		})
	}
}

func buildPortableCopyRoundTrip(t *testing.T, copyScript bool) portableCopyRoundTrip {
	t.Helper()

	sourceDir := t.TempDir()
	writePortableSourceFactory(t, sourceDir, copyScript)

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	if err := os.WriteFile(filepath.Join(sourceDir, interfaces.FactoryConfigFile), flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(flattened factory.json): %v", err)
	}
	expandedDir, err := ExpandFactoryConfigLayout(sourceDir)
	if err != nil {
		t.Fatalf("ExpandFactoryConfigLayout: %v", err)
	}

	loaded, err := LoadRuntimeConfig(expandedDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded layout): %v", err)
	}

	return portableCopyRoundTrip{targetDir: expandedDir, loaded: loaded}
}

func writePortableSourceFactory(t *testing.T, sourceDir string, copyScript bool) {
	t.Helper()

	writePortableFactoryJSON(t, sourceDir, map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{"name": "executor"},
		},
		"workstations": []map[string]any{
			{
				"name":                  "execute-story",
				"worker":                "executor",
				"copyReferencedScripts": copyScript,
				"inputs":                []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":               []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure":             []map[string]string{{"workType": "task", "state": "failed"}},
				"type":                  "MODEL_WORKSTATION",
				"body":                  "Execute {{ (index .Inputs 0).Payload }}.",
				"workingDirectory":      "repo/{{ (index .Inputs 0).WorkID }}",
				"env":                   map[string]string{"SCRIPT_MODE": "portable"},
			},
		},
	})
	writePortableFile(t, filepath.Join(sourceDir, "workers", "executor", "AGENTS.md"), `---
type: SCRIPT_WORKER
command: powershell
args: ["-File", "scripts/execute-story.ps1"]
timeout: 45m
---
Execute the story script.
`)
	writePortableFile(t, filepath.Join(sourceDir, "scripts", "execute-story.ps1"), "Write-Output 'portable'\n")
}

func assertPortableExpandedScriptCopy(t *testing.T, targetDir string, wantCopied bool) {
	t.Helper()

	copiedPath := filepath.Join(targetDir, "scripts", "execute-story.ps1")
	_, statErr := os.Stat(copiedPath)
	if wantCopied {
		if statErr != nil {
			t.Fatalf("expected copied script at %s: %v", copiedPath, statErr)
		}
		return
	}
}

func assertPortableExpandedRuntimeConfig(t *testing.T, loaded *LoadedFactoryConfig, copyScript bool) {
	t.Helper()

	worker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected expanded script worker definition to load")
	}
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "powershell" || worker.Timeout != "45m" {
		t.Fatalf("loaded worker = %#v", worker)
	}
	if len(worker.Args) != 2 || worker.Args[1] != "scripts/execute-story.ps1" {
		t.Fatalf("loaded worker args = %#v", worker.Args)
	}

	workstation, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected expanded workstation definition to load")
	}
	if workstation.Type != interfaces.WorkstationTypeModel || workstation.CopyReferencedScripts != copyScript {
		t.Fatalf("loaded workstation = %#v", workstation)
	}
}

func writePortableFactoryJSON(t *testing.T, factoryDir string, cfg map[string]any) {
	t.Helper()
	if _, ok := cfg["name"]; !ok {
		cfg["name"] = filepath.Base(factoryDir)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	writePortableFile(t, filepath.Join(factoryDir, interfaces.FactoryConfigFile), string(data)+"\n")
}

func writePortableFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
