package root_composition_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText
// is story 005's functional streaming cell ("Prove streaming through the
// canonical application graph"). Unlike acp_prompt_delegation_test.go and
// acp_server_composition_test.go -- which both call process.ACPServer().Serve
// directly, bypassing the CLI command tree -- this cell drives the real
// customer-facing "you serve acp" Cobra command (pkg/transports/cli/root_serve.go)
// through Process.Execute, exactly the entrypoint the published you binary
// runs, exercising strictly more of the production graph: Cobra command
// construction -> the manifest-bound "you.serve.acp" handler ->
// Process.ACPServer()'s same singular acp.Server -> the real Chat Sessions,
// Events, and Factory Sessions authorities root.BuildProcess composes. It
// supplies the one external provider effect through
// serviceedges.Edges.ProviderCommandRunner with a real-provider-shaped raw
// stdout fixture (support.NewShapedProviderCommandRunner), not
// ProviderOverride or a MockWorkersConfig shortcut.
//
// The seeded @you/goal packaged Factory's own goal loop (packages/
// packaged-factories/factories/goal/factory.yaml) reaches its one TERMINAL
// route ("accepted" -> goal:complete, the declared invocationReturn.terminalState)
// from exactly one queued provider call whose stdout is the decision-envelope
// JSON its own executor.md prompt instructs the model to emit -- so this is a
// genuine InvocationTerminalStatusCompleted outcome, not the safe FAILED
// fallback acp_prompt_delegation_test.go's bare mock reaches (both map to the
// same acpsdk.StopReasonEndTurn, so this test additionally asserts on the
// streamed primary-result text, the one signal that distinguishes them --
// see protocol.MapFactoryInvocationOutcome's own doc comment).
//
// What this cell can and cannot observe, and why: prompt_stream.go's own
// package doc (unchanged since story 003) documents that nothing in this
// repository yet calls chatsessions.Service.Sequence to place a Factory
// response workers.Draft onto chat-session/<id>/events -- confirmed still
// true at this story's HEAD by a repo-wide search for non-test Sequence
// call sites -- so for any turn driven purely through production wiring
// (this cell's only legal construction, per this PRD's top-level acceptance
// criteria barring "hidden lookup, secondary injection, ... or Factory
// Sessions event logic") streamTurnUpdates always observes an empty
// aggregate topic and every real turn falls back to the unchanged V1
// synchronous final-text notifier. This cell therefore proves exactly the
// behavior that is true of the shipped product today: one truthful
// terminal prompt result, at most one agent_message_chunk (the final-only
// fallback, never duplicated once canonical streaming exists), and zero
// agent_thought_chunk/usage_update/session_info_update notifications --
// which is the same scope cut story 002 already applied to
// session_info_update, for the identical reason (no production producer
// exists, and building one is explicitly out of this transport-owned PRD's
// scope). Proving the ordered message/thought/usage/session-info sequence
// AC3 also describes requires that future producer bridge; it cannot be
// fabricated here without a hidden test-only Sequence call the top-level
// acceptance criteria already rule out for this package.
func TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you serve acp CLI command")
	}

	home := t.TempDir()
	// Process.ACPServer()'s factory target catalog resolves the home
	// directory from the real process environment at request time (the same
	// way acp_server_composition_test.go/acp_prompt_delegation_test.go set
	// it up), independently of the per-invocation Input.Env this test also
	// supplies to Process.Execute for the CLI command layer itself.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedInstalledPackagedFactory(t, home, "@you/goal")
	support.SeedACPAgentProfile(t, home, "factory:@you/goal", []string{"factory:@you/goal"})

	const wantPrimaryResultText = "goal genuinely completed through you serve acp"
	runner := support.NewShapedProviderCommandRunner(process.CommandResult{
		Stdout: fmt.Appendf(nil, `{"decision":"accepted","feedback":"","output":%q}`, wantPrimaryResultText),
	})

	cwd := t.TempDir()
	stdin, stdout := startServeACPHarness(t, home, cwd, runner)

	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	promptResp, notifications := driveServeACPSessionPrompt(t, stdin, stdout, sessionID, "please pursue this goal")
	if promptResp.Error != nil {
		t.Fatalf("session/prompt response error = %+v, want a successful final result", promptResp.Error)
	}
	var decodedResult acpsdk.PromptResponse
	if err := json.Unmarshal(promptResp.Result, &decodedResult); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if decodedResult.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want %q", decodedResult.StopReason, acpsdk.StopReasonEndTurn)
	}

	assertServeACPStreamingNotifications(t, notifications, wantPrimaryResultText)
}

// startServeACPHarness builds the real *application.Process against home and
// runner, then drives the real "you serve acp" Cobra command through
// Process.Execute over a pair of OS pipes, in a background goroutine, exactly
// the entrypoint the published you binary runs. It registers cleanup that
// cancels the invocation's context and closes stdin, then waits (bounded by a
// fixed terminal deadline, not a retry loop) for Execute to actually return.
// It returns the pipe ends the caller writes requests to and reads responses/
// notifications from.
func startServeACPHarness(t *testing.T, home, cwd string, runner *support.ShapedProviderCommandRunner) (*os.File, *bufio.Reader) {
	t.Helper()

	buildProcess, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	closeProcessCleanly(t, buildProcess)

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

	serveErr := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		serveErr <- buildProcess.Execute(root.Input{
			Args:             []string{"you", "serve", "acp"},
			Env:              env,
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Stderr:           &stderr,
			Context:          ctx,
			WorkingDirectory: cwd,
		})
	}()
	var shutdownOnce sync.Once
	t.Cleanup(func() {
		shutdownOnce.Do(func() {
			cancel()
			_ = stdinWrite.Close()
			select {
			case err := <-serveErr:
				if err != nil && err != context.Canceled {
					t.Errorf("serve acp Execute() error = %v, want context.Canceled or nil on shutdown", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("serve acp did not shut down after stdin closed")
			}
		})
	})

	return stdinWrite, bufio.NewReader(stdoutRead)
}

// assertServeACPStreamingNotifications asserts story 005's observable
// contract over notifications: exactly one final-only agent_message_chunk
// (the V1 fallback, since no production caller sequences Factory response
// Drafts onto the chat-session events topic yet -- see this file's own
// TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText
// doc comment) carrying the genuine completed outcome's primary-result text,
// never duplicated, and zero thought/usage/session-info notifications.
func assertServeACPStreamingNotifications(t *testing.T, notifications []acpsdk.SessionNotification, wantPrimaryResultText string) {
	t.Helper()

	var messageChunks, thoughtChunks, usageUpdates, sessionInfoUpdates int
	var lastMessageText string
	for _, n := range notifications {
		switch {
		case n.Update.AgentMessageChunk != nil:
			messageChunks++
			if n.Update.AgentMessageChunk.Content.Text != nil {
				lastMessageText = n.Update.AgentMessageChunk.Content.Text.Text
			}
		case n.Update.AgentThoughtChunk != nil:
			thoughtChunks++
		case n.Update.UsageUpdate != nil:
			usageUpdates++
		case n.Update.SessionInfoUpdate != nil:
			sessionInfoUpdates++
		}
	}

	if messageChunks != 1 {
		t.Fatalf("agent_message_chunk notifications = %d, want exactly 1 (no duplicate final-only notification)", messageChunks)
	}
	if !strings.Contains(lastMessageText, wantPrimaryResultText) {
		t.Fatalf("final agent_message_chunk text = %q, want it to contain the genuine completed outcome's primary result %q", lastMessageText, wantPrimaryResultText)
	}
	if thoughtChunks != 0 || usageUpdates != 0 || sessionInfoUpdates != 0 {
		t.Fatalf("thought/usage/session-info notifications = %d/%d/%d, want 0/0/0: no production caller sequences Factory response Drafts onto the chat-session events topic yet",
			thoughtChunks, usageUpdates, sessionInfoUpdates)
	}
}

// serveACPLine is the minimal shape shared by every JSON-RPC line "you serve
// acp" writes to stdout: a response carries "result"/"error" and echoes the
// request "id"; a notification carries "method" and no "id".
type serveACPLine struct {
	ID     json.RawMessage            `json:"id"`
	Method string                     `json:"method"`
	Result json.RawMessage            `json:"result"`
	Error  *acpsdk.RequestError       `json:"error"`
	Params acpsdk.SessionNotification `json:"params"`
}

// driveServeACPSessionNew writes one "session/new" request to stdin and reads
// stdout lines until it observes that request's own response (a real "you
// serve acp" connection can interleave request/response and asynchronous
// "session/update" notifications on the same stdout stream, though none are
// possible before any session is admitted).
func driveServeACPSessionNew(t *testing.T, stdin *os.File, stdout *bufio.Reader, cwd string) string {
	t.Helper()

	params, err := json.Marshal(map[string]any{"cwd": cwd, "mcpServers": []any{}})
	if err != nil {
		t.Fatalf("marshal session/new params: %v", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":%s}`, params) + "\n"
	if _, err := stdin.Write([]byte(line)); err != nil {
		t.Fatalf("write session/new request: %v", err)
	}

	resp := readServeACPResponse(t, stdout, "1")
	if resp.Error != nil {
		t.Fatalf("session/new response error = %+v, want a successful result", resp.Error)
	}
	var created acpsdk.NewSessionResponse
	if err := json.Unmarshal(resp.Result, &created); err != nil {
		t.Fatalf("unmarshal session/new result: %v", err)
	}
	return string(created.SessionId)
}

// driveServeACPSessionPrompt writes one "session/prompt" request to stdin and
// reads stdout lines until the matching response, returning that response
// alongside every "session/update" notification observed strictly before it
// -- the aggregate order deliverPromptUpdates itself guarantees (streamed
// updates, then the terminal prompt result).
func driveServeACPSessionPrompt(t *testing.T, stdin *os.File, stdout *bufio.Reader, sessionID, text string) (serveACPLine, []acpsdk.SessionNotification) {
	t.Helper()

	params, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	})
	if err != nil {
		t.Fatalf("marshal session/prompt params: %v", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":%s}`, params) + "\n"
	if _, err := stdin.Write([]byte(line)); err != nil {
		t.Fatalf("write session/prompt request: %v", err)
	}

	var notifications []acpsdk.SessionNotification
	for {
		raw := readServeACPLine(t, stdout)
		var decoded serveACPLine
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("unmarshal serve acp line %q: %v", raw, err)
		}
		if decoded.Method == "session/update" {
			notifications = append(notifications, decoded.Params)
			continue
		}
		if string(decoded.ID) == `2` {
			return decoded, notifications
		}
		t.Fatalf("unexpected serve acp line before the session/prompt response: %s", raw)
	}
}

// readServeACPResponse reads stdout lines until it finds the response whose
// "id" matches wantID, failing the test on any interleaved line that is
// neither a "session/update" notification nor that response (this repo's ACP
// stdio server never sends unsolicited responses with a foreign id).
func readServeACPResponse(t *testing.T, stdout *bufio.Reader, wantID string) serveACPLine {
	t.Helper()
	for {
		raw := readServeACPLine(t, stdout)
		var decoded serveACPLine
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("unmarshal serve acp line %q: %v", raw, err)
		}
		if decoded.Method == "session/update" {
			continue
		}
		if string(decoded.ID) == wantID {
			return decoded
		}
		t.Fatalf("unexpected serve acp line while waiting for response id %s: %s", wantID, raw)
	}
}

func readServeACPLine(t *testing.T, stdout *bufio.Reader) []byte {
	t.Helper()
	line, err := stdout.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read serve acp stdout line: %v", err)
	}
	return bytes.TrimSpace(line)
}
