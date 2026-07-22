package providers

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
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

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
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
		platformprocess.CommandResult{Stdout: []byte(
			`{"type":"stream_event","session_id":"session-worktree","event":{"type":"message_start","message":{"id":"msg-worktree","role":"assistant","content":[]}}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Done. COMPLETE"}}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_stop","index":0}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"message_stop"}}` + "\n" +
				`{"type":"assistant","session_id":"session-worktree","message":{"id":"msg-worktree","role":"assistant","content":[{"type":"text","text":"Done. COMPLETE"}]}}` + "\n" +
				`{"type":"result","subtype":"success","is_error":false,"result":"Done. COMPLETE","session_id":"session-worktree"}` + "\n",
		)},
	)

	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 15*time.Second)
	assertCursorProviderCompleted(t, session)

	// Verify the subprocess runner was called exactly once and the real
	// ScriptWrapProvider built the expected Claude CLI command.
	if runner.CallCount() != 1 {
		t.Fatalf("expected provider runner called 1 time, got %d", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("expected command %q, got %q", modelprovider.ProviderClaude, call.Command)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--worktree", "my-feature-branch"})
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", "test-model"})
	support.AssertArgsContainSequence(t, call.Args, []string{"--output-format", "stream-json", "--include-partial-messages"})
	if len(call.Stdin) != 0 {
		t.Fatalf("expected Claude prompt to stay in args, got stdin %q", string(call.Stdin))
	}
}
