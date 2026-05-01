package providers

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

// TestWorktreePassthrough verifies the full worktree template pipeline:
// factory.json declares a canonical name-based worktree template on a workstation →
// the template is resolved from the token's Name → the resolved value
// arrives as InferenceRequest.Worktree on the mock provider call.
//
// The factory does NOT create the worktree or chdir — it only resolves the
// template and passes it as --worktree to CLI dispatchers.
func TestWorktreePassthrough(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "my-feature-branch",
		WorkID:     "work-wt-001",
		WorkTypeID: "task",
		TraceID:    "trace-wt-test",
		Payload:    []byte("worktree test payload"),
	})

	// Provider-focused functional tests run through the exec seam so the real
	// ScriptWrapProvider command construction stays under test. Broader workflow
	// tests still use MockProvider until their value comes from CLI behavior.
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("Done. COMPLETE")},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed").
		TokenCount(1)

	// Verify the subprocess runner was called exactly once and the real
	// ScriptWrapProvider built the expected Claude CLI command.
	if runner.CallCount() != 1 {
		t.Fatalf("expected provider runner called 1 time, got %d", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(workers.ModelProviderClaude) {
		t.Fatalf("expected command %q, got %q", workers.ModelProviderClaude, call.Command)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--worktree", "my-feature-branch"})
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", "test-model"})
	if len(call.Stdin) != 0 {
		t.Fatalf("expected Claude prompt to stay in args, got stdin %q", string(call.Stdin))
	}
}
