package realclient_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const reusableACPInitializeParams = `{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true},"terminal":true}}`

// TestReusableACPServerTurnsThroughOneProcess keeps the prompt assertions on
// one production root.BuildProcess and one Process.Execute ACP server. The
// provider is replaced only at edges.Edges; the protocol, session, Factory
// target, and stream remain production behavior. Each turn owns its prompt,
// output marker, and provider-call accounting while the application process
// and ACP connection are shared serially. The runtime currently cannot host
// two independent first-turn Factory activations in one process; that
// distinct-session boundary is recorded in the C15 ledger as unresolved.
func TestReusableACPServerTurnsThroughOneProcess(t *testing.T) {
	fixture := newReusableACPFixture(t)
	connection := fixture.startServer(t)
	connection.initialize(t)
	workspace := filepath.Join(t.TempDir(), "reusable-acp-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create reusable ACP workspace: %v", err)
	}
	sessionID := fixture.newSession(t, connection, workspace, defaultFactoryBuilderTarget)

	for _, testCase := range []reusableACPCase{
		{name: "first isolated turn", marker: "alpha1", prompt: "xxxxxxxxxxxxxxa1", output: "alpha1 reusable ACP result"},
		{name: "second isolated turn", marker: "bravo2", prompt: "xxxxxxxxxxxxxxb2", output: "bravo2 reusable ACP result"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			observation := fixture.runTurn(t, connection, sessionID, testCase)
			if strings.Contains(observation.assistantText, "alpha1") && testCase.marker == "bravo2" {
				t.Fatalf("%s assistant result crossed the prior turn marker: %q", testCase.name, observation.assistantText)
			}
		})
	}
	secondWorkspace := filepath.Join(t.TempDir(), "second-reusable-acp-workspace")
	if err := os.MkdirAll(secondWorkspace, 0o755); err != nil {
		t.Fatalf("create second reusable ACP workspace: %v", err)
	}
	secondSessionID := fixture.newSession(t, connection, secondWorkspace, defaultFactoryBuilderTarget)
	if secondSessionID == sessionID {
		t.Fatalf("reusable ACP session identities reused %q across distinct session/new workspaces", sessionID)
	}
	fixture.closeActiveSession(t, connection, sessionID)
}

type reusableACPCase struct {
	name          string
	marker        string
	prompt        string
	output        string
	blockProvider bool
}

type reusableACPFixture struct {
	home     string
	process  support.ApplicationProcess
	provider *reusableACPProviderRunner
}

func newReusableACPFixture(t *testing.T) *reusableACPFixture {
	t.Helper()
	rootDir := t.TempDir()
	baseHome := filepath.Join(rootDir, "build-home")
	if err := os.MkdirAll(baseHome, 0o755); err != nil {
		t.Fatalf("create reusable ACP build home: %v", err)
	}
	// ACP target resolution retains the production resolver's process-global
	// home lookup while a server invocation is active. The process is built once
	// here, while each serialized case gets its own home and resolver state.
	t.Setenv("HOME", baseHome)
	t.Setenv("USERPROFILE", baseHome)
	t.Setenv("YOU_DEFAULT_WORKER_MODEL_PROVIDER", deterministicProviderName)
	provider := &reusableACPProviderRunner{}
	fixture := &reusableACPFixture{home: baseHome, provider: provider}
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: provider,
	})
	support.CleanupProcess(t, process)
	bootstrapDir := filepath.Join(rootDir, "bootstrap")
	if err := os.MkdirAll(bootstrapDir, 0o755); err != nil {
		t.Fatalf("create reusable ACP bootstrap directory: %v", err)
	}
	env := reusableACPEnvironment(baseHome)
	support.InstallPackagedFactoryWithProcess(
		t,
		process,
		env,
		bootstrapDir,
		"@you/factory-builder",
	)
	support.SeedACPAgentProfile(t, baseHome, defaultFactoryBuilderTarget, []string{defaultFactoryBuilderTarget})
	fixture.process = process
	return fixture
}

func reusableACPEnvironment(home string) []string {
	return append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER="+deterministicProviderName,
	)
}

func (fixture *reusableACPFixture) startServer(t *testing.T) *reusableACPConnection {
	t.Helper()
	// The production ACP resolver reads the process-global home while the
	// command is serving. The one server owns the one fixture home for the
	// complete serialized turn sequence.
	t.Setenv("HOME", fixture.home)
	t.Setenv("USERPROFILE", fixture.home)
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("create reusable ACP stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		t.Fatalf("create reusable ACP stdout pipe: %v", err)
	}
	var stderr bytes.Buffer
	stdinTTY, stdoutTTY := false, false
	command := support.StartProcessCommand(t, fixture.process, root.Input{
		Args:             []string{"you", "server", "acp"},
		Env:              reusableACPEnvironment(fixture.home),
		Stdin:            stdinRead,
		Stdout:           stdoutWrite,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: fixture.home,
		StdinIsTTY:       &stdinTTY,
		StdoutIsTTY:      &stdoutTTY,
	})
	connection := &reusableACPConnection{
		stdin:   stdinWrite,
		stdout:  bufio.NewReader(stdoutRead),
		command: command,
	}
	t.Cleanup(func() {
		if err := stdinWrite.Close(); err != nil {
			t.Errorf("close reusable ACP stdin: %v", err)
		}
		command.Stop(t)
		for _, file := range []*os.File{stdinRead, stdoutRead, stdoutWrite} {
			_ = file.Close()
		}
	})
	return connection
}

type reusableACPConnection struct {
	stdin   *os.File
	stdout  *bufio.Reader
	command *support.ProcessCommand
	nextID  uint64
}

type reusableACPFrame struct {
	JSONRPC string               `json:"jsonrpc"`
	ID      json.RawMessage      `json:"id"`
	Method  string               `json:"method"`
	Params  json.RawMessage      `json:"params"`
	Result  json.RawMessage      `json:"result"`
	Error   *acpsdk.RequestError `json:"error"`
}

func (connection *reusableACPConnection) initialize(t *testing.T) {
	t.Helper()
	frame, _ := connection.request(t, "initialize", json.RawMessage(reusableACPInitializeParams))
	if frame.Error != nil {
		t.Fatalf("initialize response error = %+v, want success", frame.Error)
	}
	if frame.JSONRPC != "2.0" {
		t.Fatalf("initialize response jsonrpc = %q, want 2.0", frame.JSONRPC)
	}
}

func (connection *reusableACPConnection) request(
	t *testing.T,
	method string,
	params any,
) (reusableACPFrame, []acpsdk.SessionNotification) {
	t.Helper()
	wantID := connection.writeRequest(t, method, params)
	responses, notifications := connection.readResponses(t, wantID)
	return responses[wantID], notifications
}

func (connection *reusableACPConnection) writeRequest(t *testing.T, method string, params any) uint64 {
	t.Helper()
	connection.nextID++
	wantID := connection.nextID
	encodedParams, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal ACP %s params: %v", method, err)
	}
	if _, err := fmt.Fprintf(connection.stdin, `{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`+"\n", wantID, method, encodedParams); err != nil {
		t.Fatalf("write ACP %s request: %v", method, err)
	}
	return wantID
}

func (connection *reusableACPConnection) readResponses(
	t *testing.T,
	wantIDs ...uint64,
) (map[uint64]reusableACPFrame, []acpsdk.SessionNotification) {
	t.Helper()
	pending := make(map[uint64]struct{}, len(wantIDs))
	for _, wantID := range wantIDs {
		pending[wantID] = struct{}{}
	}
	responses := make(map[uint64]reusableACPFrame, len(wantIDs))
	var notifications []acpsdk.SessionNotification
	for len(pending) > 0 {
		line, err := connection.stdout.ReadBytes('\n')
		if err != nil {
			t.Fatalf("read ACP response: %v", err)
		}
		var frame reusableACPFrame
		if err := json.Unmarshal(bytes.TrimSpace(line), &frame); err != nil {
			t.Fatalf("decode ACP frame %q: %v", line, err)
		}
		if frame.Method == "session/update" {
			var notification acpsdk.SessionNotification
			if err := json.Unmarshal(frame.Params, &notification); err != nil {
				t.Fatalf("decode ACP session/update: %v", err)
			}
			notifications = append(notifications, notification)
			continue
		}
		responseID, err := strconv.ParseUint(string(frame.ID), 10, 64)
		if err != nil {
			t.Fatalf("ACP response id = %s: %v", frame.ID, err)
		}
		if _, ok := pending[responseID]; !ok {
			t.Fatalf("unexpected ACP response id = %d, want one of %v", responseID, wantIDs)
		}
		responses[responseID] = frame
		delete(pending, responseID)
	}
	return responses, notifications
}

func (fixture *reusableACPFixture) runTurn(
	t *testing.T,
	connection *reusableACPConnection,
	sessionID string,
	testCase reusableACPCase,
) reusableACPObservation {
	t.Helper()
	fixture.provider.begin(testCase.marker, testCase.output, testCase.blockProvider)
	defer fixture.provider.end(testCase.marker)
	frame, notifications := connection.request(t, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": testCase.prompt}},
	})
	if frame.Error != nil {
		t.Fatalf("%s session/prompt error = %+v, want success", testCase.name, frame.Error)
	}
	var result acpsdk.PromptResponse
	if err := json.Unmarshal(frame.Result, &result); err != nil {
		t.Fatalf("%s decode session/prompt result: %v", testCase.name, err)
	}
	if result.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("%s stopReason = %q, want %q", testCase.name, result.StopReason, acpsdk.StopReasonEndTurn)
	}
	assistantText := assertReusableACPNotifications(t, testCase, notifications)
	if got := fixture.provider.count(testCase.marker); got != 2 {
		t.Fatalf("%s provider invocations = %d, want exactly 2", testCase.name, got)
	}
	for index, marker := range fixture.provider.markers(testCase.marker) {
		if marker != deterministicProviderName {
			t.Fatalf("%s provider marker %d = %q, want %q", testCase.name, index, marker, deterministicProviderName)
		}
	}
	return reusableACPObservation{assistantText: assistantText}
}

func (fixture *reusableACPFixture) closeActiveSession(
	t *testing.T,
	connection *reusableACPConnection,
	sessionID string,
) {
	t.Helper()
	testCase := reusableACPCase{
		name:          "close active session",
		marker:        "charlie3",
		prompt:        "xxxxxxxxxxxxxxc3",
		output:        "charlie3 reusable ACP result",
		blockProvider: true,
	}
	fixture.provider.begin(testCase.marker, testCase.output, testCase.blockProvider)
	defer fixture.provider.end(testCase.marker)
	promptID := connection.writeRequest(t, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": testCase.prompt}},
	})
	fixture.provider.waitForStart(t, testCase.marker)
	closeID := connection.writeRequest(t, "session/close", map[string]string{"sessionId": sessionID})
	responses, _ := connection.readResponses(t, promptID, closeID)
	assertReusableACPStopReason(t, responses[promptID], acpsdk.StopReasonCancelled)
	assertReusableACPCloseResponse(t, responses[closeID])
}

func assertReusableACPStopReason(t *testing.T, frame reusableACPFrame, want acpsdk.StopReason) {
	t.Helper()
	if frame.Error != nil {
		t.Fatalf("session/prompt response error = %+v, want stop reason %q", frame.Error, want)
	}
	var result acpsdk.PromptResponse
	if err := json.Unmarshal(frame.Result, &result); err != nil {
		t.Fatalf("decode session/prompt result: %v", err)
	}
	if result.StopReason != want {
		t.Fatalf("session/prompt stopReason = %q, want %q", result.StopReason, want)
	}
}

func assertReusableACPCloseResponse(t *testing.T, frame reusableACPFrame) {
	t.Helper()
	if frame.Error != nil {
		t.Fatalf("session/close response error = %+v, want success", frame.Error)
	}
	var result acpsdk.CloseSessionResponse
	if err := json.Unmarshal(frame.Result, &result); err != nil {
		t.Fatalf("decode session/close result: %v", err)
	}
}

type reusableACPObservation struct {
	assistantText string
}

func (fixture *reusableACPFixture) newSession(
	t *testing.T,
	connection *reusableACPConnection,
	cwd string,
	wantTarget string,
) string {
	t.Helper()
	frame, _ := connection.request(t, "session/new", map[string]any{
		"cwd":        cwd,
		"mcpServers": []any{},
	})
	if frame.Error != nil {
		t.Fatalf("session/new response error = %+v, want success", frame.Error)
	}
	var created acpsdk.NewSessionResponse
	if err := json.Unmarshal(frame.Result, &created); err != nil {
		t.Fatalf("decode session/new result: %v", err)
	}
	if created.SessionId == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	if len(created.ConfigOptions) != 1 || created.ConfigOptions[0].Select == nil {
		t.Fatalf("session/new configOptions = %+v, want one target option", created.ConfigOptions)
	}
	if got := string(created.ConfigOptions[0].Select.CurrentValue); got != wantTarget {
		t.Fatalf("session/new target = %q, want %q", got, wantTarget)
	}
	return string(created.SessionId)
}

func assertReusableACPNotifications(
	t *testing.T,
	testCase reusableACPCase,
	notifications []acpsdk.SessionNotification,
) string {
	t.Helper()
	assistantText := ""
	toolCallIndex, toolUpdateIndex, assistantIndex := -1, -1, -1
	for index, notification := range notifications {
		switch {
		case notification.Update.ToolCall != nil && toolCallIndex == -1:
			toolCallIndex = index
		case notification.Update.ToolCallUpdate != nil && toolUpdateIndex == -1:
			toolUpdateIndex = index
		case notification.Update.AgentMessageChunk != nil:
			assistantIndex = index
			if notification.Update.AgentMessageChunk.Content.Text != nil {
				assistantText += notification.Update.AgentMessageChunk.Content.Text.Text
			}
		}
	}
	if strings.TrimSpace(assistantText) == "" || !strings.Contains(assistantText, testCase.marker) {
		t.Fatalf("%s assistant result = %q, want non-empty isolated output containing %q", testCase.name, assistantText, testCase.marker)
	}
	if toolCallIndex < 0 || toolUpdateIndex < 0 {
		t.Fatalf("%s notifications lack Worker tool_call/tool_call_update: %s", testCase.name, summarizeReusableACPNotifications(notifications))
	}
	if assistantIndex < 0 || toolCallIndex > toolUpdateIndex || toolUpdateIndex > assistantIndex {
		t.Fatalf("%s notification order = %s, want tool_call, tool_call_update, assistant result", testCase.name, summarizeReusableACPNotifications(notifications))
	}
	return assistantText
}

func summarizeReusableACPNotifications(notifications []acpsdk.SessionNotification) string {
	values := make([]string, 0, len(notifications))
	for _, notification := range notifications {
		switch {
		case notification.Update.ToolCall != nil:
			values = append(values, "tool_call")
		case notification.Update.ToolCallUpdate != nil:
			values = append(values, "tool_call_update")
		case notification.Update.AgentMessageChunk != nil:
			values = append(values, "agent_message_chunk")
		default:
			values = append(values, "other")
		}
	}
	return strings.Join(values, ",")
}

type reusableACPProviderRunner struct {
	mu              sync.Mutex
	active          string
	outputs         map[string]string
	calls           map[string]int
	providerMarkers map[string][]string
	block           map[string]bool
	started         map[string]chan struct{}
	startOnce       map[string]*sync.Once
}

func (runner *reusableACPProviderRunner) begin(marker, output string, block bool) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.active != "" {
		panic("reusable ACP provider runner: overlapping cases")
	}
	if runner.outputs == nil {
		runner.outputs = make(map[string]string)
		runner.calls = make(map[string]int)
		runner.providerMarkers = make(map[string][]string)
		runner.block = make(map[string]bool)
		runner.started = make(map[string]chan struct{})
		runner.startOnce = make(map[string]*sync.Once)
	}
	runner.active = marker
	runner.outputs[marker] = output
	runner.calls[marker] = 0
	runner.providerMarkers[marker] = nil
	runner.block[marker] = block
	if runner.block[marker] {
		runner.started[marker] = make(chan struct{})
		runner.startOnce[marker] = &sync.Once{}
	}
}

func (runner *reusableACPProviderRunner) end(marker string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.active != marker {
		panic("reusable ACP provider runner: case ended out of order")
	}
	runner.active = ""
}

func (runner *reusableACPProviderRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	marker := runner.active
	output := runner.outputs[marker]
	block := runner.block[marker]
	started := runner.started[marker]
	startOnce := runner.startOnce[marker]
	if marker != "" {
		runner.calls[marker]++
		runner.providerMarkers[marker] = append(runner.providerMarkers[marker], deterministicProviderName)
	}
	runner.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	if marker == "" {
		return platformprocess.CommandResult{}, fmt.Errorf("reusable ACP provider request has no active case")
	}
	if block {
		startOnce.Do(func() { close(started) })
		select {
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	if strings.Contains(strings.ToLower(string(request.Stdin)), "return exactly one lowercase label") {
		output = "help"
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(output)}, nil
}

func (runner *reusableACPProviderRunner) waitForStart(t *testing.T, marker string) {
	t.Helper()
	runner.mu.Lock()
	started := runner.started[marker]
	runner.mu.Unlock()
	if started == nil {
		t.Fatalf("provider marker %q was not configured as a blocking case", marker)
	}
	<-started
}

func (runner *reusableACPProviderRunner) count(marker string) int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls[marker]
}

func (runner *reusableACPProviderRunner) markers(marker string) []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.providerMarkers[marker]...)
}

var _ platformprocess.CommandRunner = (*reusableACPProviderRunner)(nil)
