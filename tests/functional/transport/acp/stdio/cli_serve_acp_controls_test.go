package stdio_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const controlHarnessProviderResult = "control fixture result COMPLETE"

// TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt proves the
// public CLI and ACP path cancels an in-flight prompt, leaves the notification
// silent, and preserves a later prompt after both cancellation and ordinary
// completion. The controllable command runner is the sole external-effect
// replacement and provides channel-based synchronization with real Factory
// dispatch, rather than waiting on timing.
func TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving you serve acp cancellation through root.BuildProcess")
	}

	harness := newServeACPControlHarness(t, newControlProviderCommandRunner(1))
	sessionID := harness.openSession(t)

	harness.sendPrompt(t, 3, sessionID, "cancel this in-flight prompt")
	harness.runner.waitForStart(t, 1)
	harness.sendCancel(t, sessionID)
	assertPromptStopReason(t, harness.response(t, 3), acpsdk.StopReasonCancelled)
	harness.runner.waitForCancellation(t, 1)

	harness.sendPrompt(t, 4, sessionID, "complete after cancellation")
	assertPromptStopReason(t, harness.response(t, 4), acpsdk.StopReasonEndTurn)

	// Completion has already terminalized prompt 4 when this notification is
	// accepted. It must not reach a later prompt that happens to reuse the
	// same Factory Session.
	harness.sendCancel(t, sessionID)
	harness.sendPrompt(t, 5, sessionID, "complete after normal completion won")
	assertPromptStopReason(t, harness.response(t, 5), acpsdk.StopReasonEndTurn)

	if got := harness.runner.CallCount(); got != 3 {
		t.Fatalf("provider command calls = %d, want 3 for the captured and two later prompts", got)
	}
	harness.finish(t)

}

// TestServeACP_RootBuildProcessCloseStopsCapturedFactorySession proves a
// session/close request reaches the real Factory Sessions owner through
// canonical wiring, stops the active captured dispatch, terminalizes the
// Chat Session, and rejects later prompts without another provider command.
func TestServeACP_RootBuildProcessCloseStopsCapturedFactorySession(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving you serve acp close through root.BuildProcess")
	}

	harness := newServeACPControlHarness(t, newControlProviderCommandRunner(1))
	sessionID := harness.openSession(t)

	harness.sendPrompt(t, 3, sessionID, "close this in-flight prompt")
	harness.runner.waitForStart(t, 1)
	harness.sendClose(t, 4, sessionID)

	responses := harness.responses(t, "3", "4")
	assertPromptStopReason(t, responses["3"], acpsdk.StopReasonCancelled)
	assertCloseResponse(t, responses["4"])
	harness.runner.waitForCancellation(t, 1)

	callsBeforeRejectedPrompt := harness.runner.CallCount()
	harness.sendPrompt(t, 5, sessionID, "this must not restart a closed session")
	if response := harness.response(t, 5); response.Error == nil {
		t.Fatal("post-close session/prompt error = nil, want a closed-session rejection")
	}
	if got := harness.runner.CallCount(); got != callsBeforeRejectedPrompt {
		t.Fatalf("provider command calls after post-close prompt = %d, want unchanged from %d", got, callsBeforeRejectedPrompt)
	}
	harness.finish(t)

}

// TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities
// proves the composed close-and-reload journey. A completed first prompt
// contributes its sequenced output, an active second prompt is closed through
// the real Factory Sessions owner, and session/load then replays the retained
// first output without provider re-execution or a replacement item identity.
func TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving ACP close then load through root.BuildProcess")
	}

	harness := newServeACPControlHarness(t, newControlProviderCommandRunner(2))
	sessionID := harness.openSession(t)

	harness.sendPrompt(t, 3, sessionID, "complete before the active close")
	firstPrompt, originalUpdates := harness.responseWithUpdates(t, 3)
	assertPromptStopReason(t, firstPrompt, acpsdk.StopReasonEndTurn)
	originalItemIDs := agentMessageItemIDs(t, originalUpdates)
	if len(originalItemIDs) == 0 {
		t.Fatal("completed prompt produced no identified agent message update to retain")
	}
	if userIDs := userMessageItemIDs(t, originalUpdates); len(userIDs) != 0 {
		t.Fatalf("live prompt replayed user message IDs %v, want no echo of client-supplied prompt", userIDs)
	}

	harness.sendPrompt(t, 4, sessionID, "close this later active prompt")
	harness.runner.waitForStart(t, 2)
	harness.sendClose(t, 5, sessionID)
	responses := harness.responses(t, "4", "5")
	assertPromptStopReason(t, responses["4"], acpsdk.StopReasonCancelled)
	assertCloseResponse(t, responses["5"])
	harness.runner.waitForCancellation(t, 2)

	harness.sendLoad(t, 6, sessionID)
	loadResponse, loadedUpdates := harness.responseWithUpdates(t, 6)
	if loadResponse.Error != nil {
		t.Fatalf("session/load response error = %+v, want retained-history success", loadResponse.Error)
	}
	if got := agentMessageItemIDs(t, loadedUpdates); !slices.Equal(got, originalItemIDs) {
		t.Fatalf("loaded agent message IDs = %v, want original sequencer identities %v", got, originalItemIDs)
	}
	if userIDs := userMessageItemIDs(t, loadedUpdates); len(userIDs) != 2 || userIDs[0] == "" || userIDs[1] == "" {
		t.Fatalf("loaded user message IDs = %v, want both retained prompt identities", userIDs)
	}
	if got := harness.runner.CallCount(); got != 2 {
		t.Fatalf("provider command calls = %d, want exactly the completed and closed prompts", got)
	}
	harness.finish(t)
}

type serveACPControlHarness struct {
	runner      *controlProviderCommandRunner
	cwd         string
	stdinWrite  *os.File
	stdout      *bufio.Reader
	stdoutWrite *os.File
	command     *support.ProcessCommand
	stderr      *bytes.Buffer
}

func newServeACPControlHarness(t *testing.T, runner *controlProviderCommandRunner) *serveACPControlHarness {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cwd := t.TempDir()
	seedFixtureFactory(t, cwd)
	support.SeedACPAgentProfile(t, home, fixtureFactoryTargetID, []string{fixtureFactoryTargetID})

	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	var stderr bytes.Buffer
	command := support.StartProcessCommand(t, process, root.Input{
		Args:             []string{"you", "serve", "acp"},
		Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Stdin:            stdinRead,
		Stdout:           stdoutWrite,
		Stderr:           &stderr,
		WorkingDirectory: cwd,
	})

	return &serveACPControlHarness{
		runner:      runner,
		cwd:         cwd,
		stdinWrite:  stdinWrite,
		stdout:      bufio.NewReader(stdoutRead),
		stdoutWrite: stdoutWrite,
		command:     command,
		stderr:      &stderr,
	}
}

func (h *serveACPControlHarness) openSession(t *testing.T) string {
	t.Helper()
	writeRPCLine(t, h.stdinWrite, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":%s}`,
		fixtureInitializeParams,
	))
	if response := h.response(t, 1); response.Error != nil {
		t.Fatalf("initialize response error = %+v, want successful result", response.Error)
	}

	writeRPCLine(t, h.stdinWrite, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":%q,"mcpServers":[]}}`,
		h.cwd,
	))
	response := h.response(t, 2)
	if response.Error != nil {
		t.Fatalf("session/new response error = %+v, want successful result", response.Error)
	}
	var created acpsdk.NewSessionResponse
	if err := json.Unmarshal(response.Result, &created); err != nil {
		t.Fatalf("unmarshal session/new response: %v", err)
	}
	if created.SessionId == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	return string(created.SessionId)
}

func (h *serveACPControlHarness) sendPrompt(t *testing.T, id int, sessionID, text string) {
	t.Helper()
	params, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	})
	if err != nil {
		t.Fatalf("marshal session/prompt params: %v", err)
	}
	writeRPCLine(t, h.stdinWrite, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"session/prompt","params":%s}`, id, params))
}

func (h *serveACPControlHarness) sendCancel(t *testing.T, sessionID string) {
	t.Helper()
	writeRPCLine(t, h.stdinWrite, fmt.Sprintf(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":%q}}`, sessionID))
}

func (h *serveACPControlHarness) sendClose(t *testing.T, id int, sessionID string) {
	t.Helper()
	writeRPCLine(t, h.stdinWrite, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"session/close","params":{"sessionId":%q}}`, id, sessionID))
}

func (h *serveACPControlHarness) sendLoad(t *testing.T, id int, sessionID string) {
	t.Helper()
	writeRPCLine(t, h.stdinWrite, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"method":"session/load","params":{"sessionId":%q,"cwd":%q,"mcpServers":[]}}`,
		id, sessionID, h.cwd,
	))
}

func (h *serveACPControlHarness) response(t *testing.T, id int) rpcFrame {
	t.Helper()
	return h.responses(t, fmt.Sprintf("%d", id))[fmt.Sprintf("%d", id)]
}

// responseWithUpdates returns one response and every preceding session/update
// notification for that request. ACP load intentionally emits retained
// history before its response, so this keeps the functional test on the
// actual protocol ordering instead of dropping the evidence in responses.
func (h *serveACPControlHarness) responseWithUpdates(t *testing.T, id int) (rpcFrame, []rpcFrame) {
	t.Helper()
	wantID := fmt.Sprintf("%d", id)
	var updates []rpcFrame
	for {
		frame := readRPCFrame(t, h.stdout)
		if frame.Method != "" {
			if frame.Method != string(acpsdk.ClientMethodSessionUpdate) {
				t.Fatalf("unexpected ACP notification method %q", frame.Method)
			}
			updates = append(updates, frame)
			continue
		}
		if got := string(bytes.TrimSpace(frame.ID)); got != wantID {
			t.Fatalf("unexpected ACP response id %s while waiting for %s", got, wantID)
		}
		return frame, updates
	}
}

func (h *serveACPControlHarness) responses(t *testing.T, ids ...string) map[string]rpcFrame {
	t.Helper()
	pending := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		pending[id] = struct{}{}
	}
	responses := make(map[string]rpcFrame, len(ids))
	for len(pending) > 0 {
		frame := readRPCFrame(t, h.stdout)
		if frame.Method != "" {
			continue
		}
		id := string(bytes.TrimSpace(frame.ID))
		if _, ok := pending[id]; !ok {
			t.Fatalf("unexpected ACP response id %s while waiting for %v", id, ids)
		}
		responses[id] = frame
		delete(pending, id)
	}
	return responses
}

func (h *serveACPControlHarness) finish(t *testing.T) {
	t.Helper()
	if err := h.stdinWrite.Close(); err != nil {
		t.Fatalf("close ACP stdin: %v", err)
	}
	select {
	case <-h.command.Done():
		if err := h.command.Err(); err != nil {
			t.Fatalf("Process.Execute(serve acp) error = %v; stderr=%s", err, h.stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Process.Execute(serve acp) did not return after stdin EOF")
	}
	h.command.AcceptError()
	if err := h.stdoutWrite.Close(); err != nil {
		t.Fatalf("close ACP stdout: %v", err)
	}
}

func assertPromptStopReason(t *testing.T, response rpcFrame, want acpsdk.StopReason) {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("session/prompt response error = %+v, want stop reason %q", response.Error, want)
	}
	var prompt acpsdk.PromptResponse
	if err := json.Unmarshal(response.Result, &prompt); err != nil {
		t.Fatalf("unmarshal session/prompt result: %v", err)
	}
	if prompt.StopReason != want {
		t.Fatalf("session/prompt stopReason = %q, want %q", prompt.StopReason, want)
	}
}

func assertCloseResponse(t *testing.T, response rpcFrame) {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("session/close response error = %+v, want successful result", response.Error)
	}
	var closeResponse acpsdk.CloseSessionResponse
	if err := json.Unmarshal(response.Result, &closeResponse); err != nil {
		t.Fatalf("unmarshal session/close result: %v", err)
	}
}

func agentMessageItemIDs(t *testing.T, updates []rpcFrame) []string {
	t.Helper()
	ids := make([]string, 0, len(updates))
	for _, frame := range updates {
		var notification acpsdk.SessionNotification
		if err := json.Unmarshal(frame.Params, &notification); err != nil {
			t.Fatalf("unmarshal session/update notification: %v", err)
		}
		chunk := notification.Update.AgentMessageChunk
		if chunk == nil || chunk.MessageId == nil {
			continue
		}
		ids = append(ids, *chunk.MessageId)
	}
	return ids
}

func userMessageItemIDs(t *testing.T, updates []rpcFrame) []string {
	t.Helper()
	ids := make([]string, 0, len(updates))
	for _, frame := range updates {
		var notification acpsdk.SessionNotification
		if err := json.Unmarshal(frame.Params, &notification); err != nil {
			t.Fatalf("unmarshal session/update notification: %v", err)
		}
		chunk := notification.Update.UserMessageChunk
		if chunk == nil || chunk.MessageId == nil {
			continue
		}
		ids = append(ids, *chunk.MessageId)
	}
	return ids
}

type controlProviderCommandRunner struct {
	blocks    map[int]struct{}
	calls     atomic.Int32
	started   chan int
	cancelled chan int
}

func newControlProviderCommandRunner(blockCalls ...int) *controlProviderCommandRunner {
	blocks := make(map[int]struct{}, len(blockCalls))
	for _, call := range blockCalls {
		blocks[call] = struct{}{}
	}
	return &controlProviderCommandRunner{
		blocks:    blocks,
		started:   make(chan int, len(blockCalls)),
		cancelled: make(chan int, len(blockCalls)),
	}
}

func (r *controlProviderCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	call := int(r.calls.Add(1))
	if _, blocked := r.blocks[call]; blocked {
		r.started <- call
		<-ctx.Done()
		r.cancelled <- call
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(controlHarnessProviderResult)}, nil
}

func (r *controlProviderCommandRunner) CallCount() int {
	return int(r.calls.Load())
}

func (r *controlProviderCommandRunner) waitForStart(t *testing.T, want int) {
	t.Helper()
	select {
	case got := <-r.started:
		if got != want {
			t.Fatalf("blocked provider command call = %d, want %d", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("provider command call %d did not start", want)
	}
}

func (r *controlProviderCommandRunner) waitForCancellation(t *testing.T, want int) {
	t.Helper()
	select {
	case got := <-r.cancelled:
		if got != want {
			t.Fatalf("cancelled provider command call = %d, want %d", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("provider command call %d did not observe cancellation", want)
	}
}

var _ platformprocess.CommandRunner = (*controlProviderCommandRunner)(nil)
