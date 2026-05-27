package layouttests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
)

type portableCopyRoundTrip struct {
	targetDir string
	loaded    *factoryconfig.LoadedFactoryConfig
}

func TestFlattenExpandInlineScriptFactory_RoundTripsCopyFlagThroughLoadAndExecution(t *testing.T) {
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
			if tt.wantCopied {
				assertPortableExpandedExecution(t, roundTrip.targetDir, roundTrip.loaded)
			}
		})
	}
}

func buildPortableCopyRoundTrip(t *testing.T, copyScript bool) portableCopyRoundTrip {
	t.Helper()

	sourceDir := t.TempDir()
	writePortableSourceFactory(t, sourceDir, copyScript)

	flattened, err := factoryconfig.FlattenFactoryConfig(sourceDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}

	if err := os.WriteFile(filepath.Join(sourceDir, interfaces.FactoryConfigFile), flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(flattened factory.json): %v", err)
	}
	expandedDir, err := factoryconfig.ExpandFactoryConfigLayout(sourceDir)
	if err != nil {
		t.Fatalf("ExpandFactoryConfigLayout: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfig(expandedDir, nil)
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

func assertPortableExpandedRuntimeConfig(t *testing.T, loaded *factoryconfig.LoadedFactoryConfig, copyScript bool) {
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

func assertPortableExpandedExecution(t *testing.T, targetDir string, loaded *factoryconfig.LoadedFactoryConfig) {
	t.Helper()

	worker, _ := loaded.Worker("executor")
	runner := &recordingScriptRunner{stdout: "portable copied script accepted"}
	executor := &workerexecutor.WorkstationExecutor{
		RuntimeConfig: loaded,
		Executor:      workerexecutor.NewScriptExecutorWithRunner(worker, runner, nil),
		Renderer:      &workerprompting.DefaultPromptRenderer{},
	}

	result, err := executor.Execute(context.Background(), portableWorkDispatch())
	if err != nil {
		t.Fatalf("execute expanded workstation: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted || result.Output != "portable copied script accepted" {
		t.Fatalf("result = %#v", result)
	}

	req := runner.LastRequest()
	if req.Command != "powershell" {
		t.Fatalf("command = %q, want powershell", req.Command)
	}
	if len(req.Args) != 2 || req.Args[0] != "-File" || req.Args[1] != "scripts/execute-story.ps1" {
		t.Fatalf("args = %#v", req.Args)
	}
	wantWorkDir := filepath.Join(targetDir, "repo", "work-001")
	if req.WorkDir != wantWorkDir {
		t.Fatalf("work dir = %q, want %q", req.WorkDir, wantWorkDir)
	}
	if !containsScriptEnv(req.Env, "SCRIPT_MODE=portable") {
		t.Fatalf("expected SCRIPT_MODE env in %v", req.Env)
	}
}

func portableWorkDispatch() interfaces.WorkDispatch {
	now := time.Now()
	return interfaces.WorkDispatch{
		DispatchID:      "dispatch-1",
		TransitionID:    "transition-1",
		WorkerType:      "executor",
		WorkstationName: "execute-story",
		InputTokens: []any{
			interfaces.Token{
				ID:      "token-1",
				PlaceID: "task:init",
				Color: interfaces.TokenColor{
					WorkID:     "work-001",
					WorkTypeID: "task",
					DataType:   interfaces.DataTypeWork,
					TraceID:    "trace-001",
					Payload:    []byte("portable task"),
				},
				CreatedAt: now,
				EnteredAt: now,
			},
		},
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

type recordingScriptRunner struct {
	requests []workerexecutor.CommandRequest
	stdout   string
}

func (r *recordingScriptRunner) Run(_ context.Context, req workerexecutor.CommandRequest) (workerexecutor.CommandResult, error) {
	copied := req
	copied.Args = append([]string(nil), req.Args...)
	copied.Env = append([]string(nil), req.Env...)
	r.requests = append(r.requests, copied)
	return workerexecutor.CommandResult{Stdout: []byte(r.stdout)}, nil
}

func (r *recordingScriptRunner) LastRequest() workerexecutor.CommandRequest {
	if len(r.requests) == 0 {
		return workerexecutor.CommandRequest{}
	}
	return r.requests[len(r.requests)-1]
}

func containsScriptEnv(env []string, expected string) bool {
	for _, entry := range env {
		if entry == expected {
			return true
		}
	}
	return false
}
