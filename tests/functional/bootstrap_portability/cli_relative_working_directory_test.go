package bootstrap_portability

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=branch-carried functional fixture review=2026-07-22 removal=split setup and command assertions on next relative-working-directory test change
func TestRelativeWorkingDirectory_UsesFactoryRuntimeRoot(t *testing.T) {
	projectRoot := t.TempDir()
	factoryDir := filepath.Join(projectRoot, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create factory dir: %v", err)
	}

	writeRelativeWorkingDirectoryFactoryConfig(t, factoryDir)
	support.WriteAgentConfig(t, factoryDir, "worker-a", `---
type: MODEL_WORKER
modelProvider: codex
executorProvider: script_wrap
skipPermissions: true
stopToken: COMPLETE
---
	Process the input task.
`)
	writeRelativeWorkingDirectoryWorkstationConfig(t, factoryDir, "process", `---
type: MODEL_WORKSTATION
---

Process {{ (index .Inputs 0).Name }} from the current working directory.
`)

	workName := "relative-working-directory-branch"
	expectedWorkDir := filepath.Join(factoryDir, ".claude", "worktrees", workName)
	if err := os.MkdirAll(expectedWorkDir, 0o755); err != nil {
		t.Fatalf("create expected work dir: %v", err)
	}

	testutil.WriteSeedRequest(t, factoryDir, interfaces.SubmitRequest{
		Name:       workName,
		WorkID:     "work-relative-working-directory",
		WorkTypeID: "task",
		TraceID:    "trace-relative-working-directory",
		Payload:    []byte("relative working directory payload"),
	})

	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("Done. COMPLETE")},
	)

	h := testutil.NewServiceTestHarness(t, factoryDir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed").
		TokenCount(1)

	if runner.CallCount() != 1 {
		t.Fatalf("expected provider runner called 1 time, got %d", runner.CallCount())
	}

	req := runner.LastRequest()
	if req.Command != string(workers.ModelProviderCodex) {
		t.Fatalf("command = %q, want %q", req.Command, workers.ModelProviderCodex)
	}
	support.AssertArgsContainSequence(t, req.Args, []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "-"})
	if req.WorkDir != expectedWorkDir {
		t.Fatalf("work dir = %q, want %q", req.WorkDir, expectedWorkDir)
	}
	if string(req.Stdin) == "" {
		t.Fatal("expected Codex request prompt to be sent over stdin")
	}
}

func writeRelativeWorkingDirectoryFactoryConfig(t *testing.T, factoryDir string) {
	t.Helper()

	config := `{
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "worker-a" }
  ],
  "workstations": [
    {
      "name": "process",
      "worker": "worker-a",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" },
      "workingDirectory": ".claude/worktrees/{{ (index .Inputs 0).Name }}",
      "worktree": "{{ (index .Inputs 0).Name }}"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(config), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeRelativeWorkingDirectoryWorkstationConfig(t *testing.T, dir, workstationName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
