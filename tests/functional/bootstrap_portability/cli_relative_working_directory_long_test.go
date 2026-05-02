//go:build functionallong

package bootstrap_portability

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=branch-carried functional fixture review=2026-07-22 removal=split setup and command assertions on next relative-working-directory test change
func TestRelativeWorkingDirectory_UsesFactoryRuntimeRoot(t *testing.T) {
	support.SkipLongFunctional(t, "slow relative-working-directory runtime-root sweep")

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
