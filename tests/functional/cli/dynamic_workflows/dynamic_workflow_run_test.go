package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// THIS IS THE CANONICAL ROUGH SHAPE OF HOW INJECTION AND SERVICE RUNNING IS INTENDED TO BE SHAPED.
func TestRunJavaScriptFactoryWithMockWorkersUsesFakeChildExecutor(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic"))
	support.SetWorkingDirectory(t, dir)

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	fakeEnv := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", "./basic.js", "--with-mock-workers",
	})

	// Act
	process, err := root.BuildProcess(t.Context(), edges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	err = process.Execute(fakeEnv.Input)

	// Assert
	if err != nil {
		t.Fatalf("Process.Execute() error = %v; stdout=%q stderr=%q", err, fakeEnv.Stdout(), fakeEnv.Stderr())
	}
	if !strings.Contains(fakeEnv.Stdout(), " completed (SUCCEEDED).") {
		t.Fatalf("stdout = %q, want successful Factory Session", fakeEnv.Stdout())
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}
}

func TestRunJavaScriptFactoryResponseStreamPublishesCanonicalLifecycle(t *testing.T) {
	// Arrange
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-response-stream",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "workflow.js",
				"argsSchema": map[string]any{
					"type": "object", "required": []any{"prompt"},
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	workflow := `phase("plan");
workflow.checkpoint({ label: "plan-ready", state: { ready: true } });
phase("execute");
return args.prompt;`
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(workflow), 0o600); err != nil {
		t.Fatalf("write JavaScript workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mock-workers.json"), []byte(`{"mockWorkers":[]}`), 0o600); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	support.SetWorkingDirectory(t, dir)

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	fakeEnv := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--factory", "./factory.json", "--output", "response-stream", "--no-record", "--with-mock-workers", "./mock-workers.json", "hello",
	})

	// Act
	process, err := root.BuildProcess(t.Context(), edges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	err = process.Execute(fakeEnv.Input)

	// Assert
	if err != nil {
		t.Fatalf("Process.Execute() error = %v; stdout=%q stderr=%q", err, fakeEnv.Stdout(), fakeEnv.Stderr())
	}
	stdout := fakeEnv.Stdout()
	cursor := 0
	for _, fragment := range []string{
		`/plan/active/0"`,
		`"type":"ORCHESTRATOR_CHECKPOINT_WRITTEN"`,
		`/plan/completed/2"`,
		`/execute/active/2"`,
		`/execute/completed/3"`,
		`"type":"SESSION_COMPLETED"`,
	} {
		offset := strings.Index(stdout[cursor:], fragment)
		if offset < 0 {
			t.Fatalf("stdout = %q, want ordered canonical lifecycle fragment %q", stdout, fragment)
		}
		cursor += offset + len(fragment)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}
}
