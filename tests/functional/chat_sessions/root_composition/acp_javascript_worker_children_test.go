package root_composition_test

import (
	"encoding/json"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestJavaScriptFactoryChildrenAreVisibleAsWorkers proves that a JavaScript
// workflow's agent.run children are Workers on the wire, exactly as a Petri
// Factory's Workers are.
//
// A child agent.run IS a Worker: it is one provider execution, with a prompt,
// a model, and an output. The only thing that distinguishes it from a Petri
// Worker is what drives it -- a workflow stage rather than a work-state
// transition -- and that difference belongs in how progress is reported, not
// in whether the Worker exists.
//
// Before this was fixed, a JavaScript Factory ran its children through a
// provider executor that never reserved a Worker Session, so no
// dispatch/Worker-Session association was ever committed and the transport had
// nothing to open a tool call for. Every acpx sweep showed @you/spawn,
// @you/deep-research, and @you/tournament completing with zero tool calls: the
// Factory delivered a correct final answer, but everything it did to produce
// that answer was invisible while it ran. A Petri Factory's Worker opens a
// tool call and streams its content inside it; these showed nothing at all.
//
// @you/spawn is the load-bearing case because its children are not uniform: a
// planner runs first, three task agents then run concurrently through the
// workflow's own parallel() call, and a merger runs last. That shape is what
// catches an implementation that opens one tool call for the workflow rather
// than one per child, or that cannot keep concurrent children apart.
func TestJavaScriptFactoryChildrenAreVisibleAsWorkers(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you serve acp CLI command")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, "codex")
	t.Setenv(operatorsettings.EnvDefaultWorkerModel, "gpt-5")
	seedInstalledPackagedFactory(t, home, "@you/spawn")
	support.SeedACPAgentProfile(t, home, "factory:@you/spawn", []string{"factory:@you/spawn"})

	runner := &spawnACPRunner{}
	cwd := t.TempDir()
	stdin, stdout := startServeACPHarness(t, home, cwd, serviceedges.Edges{ProviderCommandRunner: runner})

	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	response, notifications := driveServeACPSessionPrompt(t, stdin, stdout, sessionID, "spawn parallel work")
	if response.Error != nil {
		t.Fatalf("session/prompt response error = %+v, want a successful result", response.Error)
	}
	var decoded acpsdk.PromptResponse
	if err := json.Unmarshal(response.Result, &decoded); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if decoded.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want %q", decoded.StopReason, acpsdk.StopReasonEndTurn)
	}

	// Five children run: one planner, three parallel tasks, one merger. The
	// provider call count is asserted first so a failure below cannot be
	// misread as the workflow never having run.
	if got := runner.callCount(); got != 5 {
		t.Fatalf("provider calls = %d, want 5 (planner + 3 tasks + merger)", got)
	}

	opened := workerToolCalls(notifications)
	if len(opened) != 5 {
		t.Fatalf("Worker tool calls = %d, want one per agent.run child (5); "+
			"a JavaScript Factory's children must be Workers on the wire like a Petri Factory's are",
			len(opened))
	}

	// Every opened tool call must carry a distinct identity, or concurrent
	// children are sharing one and their content will interleave.
	seen := make(map[string]bool, len(opened))
	for _, toolCallID := range opened {
		if toolCallID == "" {
			t.Fatal("a Worker tool call was opened with a blank toolCallId")
		}
		if seen[toolCallID] {
			t.Fatalf("toolCallId %q was opened twice; concurrent children must not share one Worker identity", toolCallID)
		}
		seen[toolCallID] = true
	}

	// Each child's own provider output must arrive inside its tool call, not
	// as top-level assistant text.
	byToolCall := workerToolCallContent(notifications)
	var withContent int
	for _, toolCallID := range opened {
		if strings.TrimSpace(byToolCall[toolCallID]) != "" {
			withContent++
		}
	}
	if withContent != len(opened) {
		t.Fatalf("Worker tool calls carrying content = %d of %d; a Worker that runs but streams nothing "+
			"leaves the client watching an empty box", withContent, len(opened))
	}

	// The workflow's own final answer still reaches the customer as assistant
	// text -- routing children into tool calls must not swallow the result.
	assistantText := agentMessageText(t, notifications)
	if !strings.Contains(assistantText, "merged spawn result over ACP") {
		t.Fatalf("assistant text = %q, want the workflow's merged result", assistantText)
	}
}

// workerToolCalls returns the toolCallId of every opened Worker tool call, in
// the order they were opened.
func workerToolCalls(notifications []acpsdk.SessionNotification) []string {
	var opened []string
	for _, notification := range notifications {
		if call := notification.Update.ToolCall; call != nil {
			opened = append(opened, string(call.ToolCallId))
		}
	}
	return opened
}

// workerToolCallContent concatenates the text delivered inside each Worker's
// own tool call, keyed by that tool call's identity.
func workerToolCallContent(notifications []acpsdk.SessionNotification) map[string]string {
	byToolCall := make(map[string]string)
	for _, notification := range notifications {
		update := notification.Update.ToolCallUpdate
		if update == nil {
			continue
		}
		for _, content := range update.Content {
			if content.Content == nil || content.Content.Content.Text == nil {
				continue
			}
			byToolCall[string(update.ToolCallId)] += content.Content.Content.Text.Text
		}
	}
	return byToolCall
}
