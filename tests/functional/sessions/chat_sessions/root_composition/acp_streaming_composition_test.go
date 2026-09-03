// Functional owner: sessions/chat_sessions/root_composition.
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

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText
// is story 005's functional streaming cell ("Prove streaming through the
// canonical application graph"). Unlike acp_prompt_delegation_test.go and
// acp_server_composition_test.go -- which both call process.ACPServer().Serve
// directly, bypassing the CLI command tree -- this cell drives the real
// customer-facing "you server acp" Cobra command (pkg/transports/cli/root_serve.go)
// through Process.Execute, exactly the entrypoint the published you binary
// runs, exercising strictly more of the production graph: Cobra command
// construction -> the manifest-bound "you.server.acp" handler ->
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
// What this cell now proves, and what changed since the prior iteration: a
// real production producer now exists (factorysessionsshim.RunWithResponseBridge,
// wired through acp.ResponseBridge/pkg/wire) that sequences a Factory
// Session's response events onto chat-session/<id>/events concurrently with
// the synchronous Factory invocation dispatchFactoryInvocation wraps -- so
// this cell's turn no longer falls back to the V1 synchronous final-text
// notifier; it observes genuine canonical MESSAGE records delivered through
// prompt_stream.go's streamTurnUpdates before the terminal prompt response,
// and the V1 fallback is correctly suppressed once a canonical message is
// delivered (deliverPromptUpdates' own duplicate-suppression contract, this
// PRD's central "no duplicate final text" requirement).
//
// This turn observes exactly two agent_message_chunk notifications, not one,
// and that is a real, already-tracked, pre-existing defect in a different
// service's own event production -- not a bug in this PRD's projection or
// delivery code, which is what this cell actually exercises. Both chunks are
// genuine, independently-committed Factory response MESSAGE/COMPLETED
// records: (1) the generic provider adapter's own always-published
// final-only message (pkg/services/providers/.../agy/progress.go's
// finalOnlyMessageEvent, which publishes the provider process' raw stdout --
// here the undecoded decision-envelope JSON -- as role "assistant" whenever
// no native streaming format applies), followed by (2) the agent-run path's
// own decision-envelope-aware final message
// (pkg/services/workers/.../agentrun/final_message.go's publishAgentFinalMessage, the
// genuinely customer-facing extracted "output" field). Both existed and
// published independently before this PRD ever consumed the response-event
// stream; nothing about this PRD's own scope (message/reasoning/usage/gap
// projection, and now the producer bridge that mechanically forwards
// whatever Factory Sessions already publishes) can suppress one of two
// upstream producers without adding "Workers/Providers event logic" this
// PRD's own top-level acceptance criteria bar it from adding. This is
// recorded here, not silently asserted around, so a reader does not mistake
// it for correct behavior: Workers/Providers owns deciding which one
// producer should be authoritative for a decision-envelope workstation's
// final customer-facing text.
//
// Zero agent_thought_chunk/usage_update/session_info_update notifications are
// accurate for this fixture: @you/goal's provider call emits none. The ACP
// mapping package proves those individual representation rules in isolation;
// this functional cell stays focused on the customer message journey.
func TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}

	cohort := newControlledACPCohort(t, "streaming-composition")
	t.Parallel()

	const wantPrimaryResultText = "goal genuinely completed through you server acp"
	cwd := controlledACPWorkingDirectoryForCohort(t, cohort, "streaming-composition")
	stdin, stdout := startControlledServeACPHarness(t, cohort, cwd)

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
// edges (the one external provider effect this cell's caller supplies --
// either a decision-envelope ProviderCommandRunner fixture or an ACP-execution
// PlatformProcessCommandFactory/ProvidersExecutableLocator pair, see
// acp_streaming_usage_composition_test.go's usage-update sibling cell), then
// drives the real "you server acp" Cobra command through Process.Execute over a
// pair of OS pipes, in a background goroutine, exactly the entrypoint the
// published you binary runs. It registers cleanup that cancels the
// invocation's context and closes stdin, then waits (bounded by a fixed
// terminal deadline, not a retry loop) for Execute to actually return. It
// returns the pipe ends the caller writes requests to and reads responses/
// notifications from.
func startServeACPHarness(t *testing.T, home, cwd string, edges serviceedges.Edges) (*os.File, *bufio.Reader) {
	t.Helper()

	buildProcess, err := buildChatProcess(t, "standalone ACP harness", edges)
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	closeProcessCleanly(t, buildProcess)
	return startServeACPProcess(t, buildProcess, home, cwd)
}

// startControlledServeACPHarness runs a fresh customer-facing ACP command
// invocation on a scenario-scoped process and initialization home. The
// invocation's pipes and context are owned by the calling test; the process is
// closed by that scenario's cleanup.
func startControlledServeACPHarness(t *testing.T, cohort *controlledACPCohort, cwd string) (*os.File, *bufio.Reader) {
	t.Helper()
	return startServeACPProcess(t, cohort.process, cohort.home, cwd)
}

func startServeACPProcess(
	t *testing.T,
	buildProcess support.ApplicationProcess,
	home, cwd string,
) (*os.File, *bufio.Reader) {
	t.Helper()
	stdinReadFile, stdinWriteFile, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdinRead := newChatPipeEndpoint(stdinReadFile, "ACP stdin reader")
	stdinWrite := newChatPipeEndpoint(stdinWriteFile, "ACP stdin writer")
	stdoutReadFile, stdoutWriteFile, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		t.Fatalf("stdout pipe: %v", err)
	}
	stdoutRead := newChatPipeEndpoint(stdoutReadFile, "ACP stdout reader")
	stdoutWrite := newChatPipeEndpoint(stdoutWriteFile, "ACP stdout writer")
	t.Cleanup(func() {
		for name, pipe := range map[string]*chatPipeEndpoint{
			"stdin reader":  stdinRead,
			"stdin writer":  stdinWrite,
			"stdout reader": stdoutRead,
			"stdout writer": stdoutWrite,
		} {
			if err := pipe.Close(); err != nil {
				t.Errorf("close %s: %v", name, err)
			}
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	env := append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER=codex",
		"YOU_DEFAULT_WORKER_MODEL=gpt-5",
	)

	serveErr := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		serveErr <- buildProcess.Execute(root.Input{
			Args:             []string{"you", "server", "acp"},
			Env:              env,
			Stdin:            stdinRead.file,
			Stdout:           stdoutWrite.file,
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
					t.Errorf("you server acp Execute() error = %v, want context.Canceled or nil on shutdown", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("you server acp did not shut down after stdin closed")
			}
		})
	})

	return stdinWrite.file, bufio.NewReader(stdoutRead.file)
}

// assertServeACPStreamingNotifications asserts story 005's now-achievable
// observable contract: this turn's canonical MESSAGE records were delivered
// through the real producer bridge and prompt_stream.go's streamTurnUpdates
// -- not the V1 synchronous final-text fallback -- strictly in commit order,
// ending on the genuine completed outcome's primary-result text. It observes
// exactly two agent_message_chunk notifications, not one: see this file's
// own TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText
// doc comment for why that is a real, already-tracked, pre-existing defect
// in a different service's own event production (two independent upstream
// producers each publishing their own "final message" for a decision-envelope
// workstation), not a duplicate this PRD's own delivery/suppression logic
// failed to prevent -- the V1 fallback itself is still never also emitted
// once a canonical message is delivered, which is what that logic actually
// guarantees. Zero thought/usage/session-info notifications remain accurate:
// this fixture's single provider call produces no REASONING/USAGE/SESSION
// response events for the bridge to forward.
func assertServeACPStreamingNotifications(t *testing.T, notifications []acpsdk.SessionNotification, wantPrimaryResultText string) {
	t.Helper()

	var messageTexts []string
	var thoughtChunks, usageUpdates, sessionInfoUpdates int
	for _, n := range notifications {
		switch {
		case n.Update.AgentMessageChunk != nil:
			var text string
			if n.Update.AgentMessageChunk.Content.Text != nil {
				text = n.Update.AgentMessageChunk.Content.Text.Text
			}
			messageTexts = append(messageTexts, text)
		case n.Update.AgentThoughtChunk != nil:
			thoughtChunks++
		case n.Update.UsageUpdate != nil:
			usageUpdates++
		case n.Update.SessionInfoUpdate != nil:
			sessionInfoUpdates++
		}
	}

	// Exactly one, not two. The second chunk this cell used to observe was the
	// Worker's own raw decision-envelope stdout, published as a separate
	// assistant message by a second upstream producer -- the duplicate this
	// test's name is about. Worker output is now content inside that Worker's
	// tool call rather than assistant output, so only the Factory's own
	// extracted primary result reaches the customer as a message.
	if len(messageTexts) != 1 {
		t.Fatalf("agent_message_chunk notifications = %d (%q), want exactly 1 -- the Factory's own result; a Worker's raw output belongs inside its tool call", len(messageTexts), messageTexts)
	}
	if !strings.Contains(messageTexts[0], wantPrimaryResultText) {
		t.Fatalf("agent_message_chunk text = %q, want it to contain the genuine completed outcome's extracted primary result %q", messageTexts[0], wantPrimaryResultText)
	}
	if thoughtChunks != 0 || usageUpdates != 0 || sessionInfoUpdates != 0 {
		t.Fatalf("thought/usage/session-info notifications = %d/%d/%d, want 0/0/0: this fixture's provider call publishes no REASONING/USAGE/SESSION response events",
			thoughtChunks, usageUpdates, sessionInfoUpdates)
	}
}

// serveACPLine is the minimal shape shared by every JSON-RPC line `you server
// acp` writes to stdout: a response carries "result"/"error" and echoes the
// request "id"; a notification carries "method" and no "id".
type serveACPLine struct {
	ID     json.RawMessage            `json:"id"`
	Method string                     `json:"method"`
	Result json.RawMessage            `json:"result"`
	Error  *acpsdk.RequestError       `json:"error"`
	Params acpsdk.SessionNotification `json:"params"`
}

// driveServeACPSessionNew writes one "session/new" request to stdin and reads
// stdout lines until it observes that request's own response (a real `you server
// acp` connection can interleave request/response and asynchronous
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
	trackChatSessionOnConnection(t, stdin, stdout, string(created.SessionId))
	return string(created.SessionId)
}

// driveServeACPFactoryTarget selects a target from the shared immutable
// profile's catalog before the session's first prompt. The target change is
// Chat Session state, not home/profile mutation, so each activation-owning
// process can reuse the same catalog while retaining its own runtime.
func driveServeACPFactoryTarget(
	t *testing.T,
	stdin *os.File,
	stdout *bufio.Reader,
	sessionID, target string,
) {
	t.Helper()

	params, err := json.Marshal(map[string]string{
		"sessionId": sessionID,
		"configId":  "target",
		"value":     target,
	})
	if err != nil {
		t.Fatalf("marshal session/set_config_option params: %v", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"session/set_config_option","params":%s}`, params) + "\n"
	if _, err := stdin.Write([]byte(line)); err != nil {
		t.Fatalf("write session/set_config_option request: %v", err)
	}
	response := readServeACPResponse(t, stdout, "3")
	if response.Error != nil {
		t.Fatalf("session/set_config_option response error = %+v, want a successful target selection", response.Error)
	}
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
			t.Fatalf("unmarshal you server acp line %q: %v", raw, err)
		}
		if decoded.Method == "session/update" {
			notifications = append(notifications, decoded.Params)
			continue
		}
		if string(decoded.ID) == `2` {
			return decoded, notifications
		}
		t.Fatalf("unexpected you server acp line before the session/prompt response: %s", raw)
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
			t.Fatalf("unmarshal you server acp line %q: %v", raw, err)
		}
		if decoded.Method == "session/update" {
			continue
		}
		if string(decoded.ID) == wantID {
			return decoded
		}
		t.Fatalf("unexpected you server acp line while waiting for response id %s: %s", wantID, raw)
	}
}

func readServeACPLine(t *testing.T, stdout *bufio.Reader) []byte {
	t.Helper()
	line, err := stdout.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read you server acp stdout line: %v", err)
	}
	return bytes.TrimSpace(line)
}
