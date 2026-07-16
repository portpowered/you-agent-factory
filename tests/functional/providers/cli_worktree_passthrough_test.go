package providers

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

// TestWorktreePassthrough verifies the full worktree template pipeline:
// factory.json declares a canonical name-based worktree template on a workstation →
// the template is resolved from the token's Name → the resolved value
// arrives as InferenceRequest.Worktree on the mock provider call.
//
// The factory materializes the resolved worktree before dispatch and passes
// its name as --worktree to CLI dispatchers that support that flag.
func TestWorktreePassthrough(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))
	initGitRepositoryForProviderWorktreeFunctionalTest(t, dir)

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
		workers.CommandResult{Stdout: []byte(
			`{"type":"stream_event","session_id":"session-worktree","event":{"type":"message_start","message":{"id":"msg-worktree","role":"assistant","content":[]}}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Done. COMPLETE"}}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_stop","index":0}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"message_stop"}}` + "\n" +
				`{"type":"assistant","session_id":"session-worktree","message":{"id":"msg-worktree","role":"assistant","content":[{"type":"text","text":"Done. COMPLETE"}]}}` + "\n" +
				`{"type":"result","subtype":"success","is_error":false,"result":"Done. COMPLETE","session_id":"session-worktree"}` + "\n",
		)},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

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
	wantCheckout := filepath.Join(dir, ".worktrees", "my-feature-branch")
	if _, err := os.Stat(wantCheckout); err != nil {
		t.Fatalf("expected factory-managed worktree at %q: %v", wantCheckout, err)
	}
	call := runner.LastRequest()
	if call.Command != string(interfaces.ModelProviderClaude) {
		t.Fatalf("expected command %q, got %q", interfaces.ModelProviderClaude, call.Command)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--worktree", "my-feature-branch"})
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", "test-model"})
	support.AssertArgsContainSequence(t, call.Args, []string{"--output-format", "stream-json", "--include-partial-messages"})
	if len(call.Stdin) != 0 {
		t.Fatalf("expected Claude prompt to stay in args, got stdin %q", string(call.Stdin))
	}
}

func TestWorktreePassthroughFailsBeforeProviderForNonGitFactory(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "my-feature-branch",
		WorkID:     "work-wt-non-git",
		WorkTypeID: "task",
		TraceID:    "trace-wt-non-git",
		Payload:    []byte("worktree test payload"),
	})
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	runner := testutil.NewProviderCommandRunner()
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete").
		TokenCount(1)
	if runner.CallCount() != 0 {
		t.Fatalf("expected worktree preparation to fail before provider dispatch, got %d calls", runner.CallCount())
	}
}
