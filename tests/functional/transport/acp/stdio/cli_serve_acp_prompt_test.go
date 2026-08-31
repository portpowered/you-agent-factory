package stdio_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const (
	fixtureFactoryScope     = "acp-serve-fixture"
	fixtureFactoryName      = "single-worker"
	fixtureFactoryTargetID  = operatorsettings.ACPFactoryTargetNamespace + "@" + fixtureFactoryScope + "/" + fixtureFactoryName
	fixtureFinalAnswerText  = "acknowledged fixture prompt via you server acp. COMPLETE"
	fixturePromptText       = "please answer this fixture prompt"
	fixtureInitializeParams = `{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true},"terminal":true}}`
)

// rpcFrame is the minimal JSON-RPC 2.0 line shape this test reads off the
// real ACP stdio Server's "you server acp" stdout: a response carries no
// "method", a notification (only "session/update" in this V1 slice) carries
// no "id".
type rpcFrame struct {
	ID     json.RawMessage      `json:"id,omitempty"`
	Method string               `json:"method,omitempty"`
	Params json.RawMessage      `json:"params,omitempty"`
	Result json.RawMessage      `json:"result,omitempty"`
	Error  *acpsdk.RequestError `json:"error,omitempty"`
}

// TestServeACP_RootBuildProcessCompletesOneFactoryPrompt proves the customer
// command end to end: it seeds a real installed Factory target and a real
// persisted ACP Agent profile, builds the reusable application through
// root.BuildProcess (the exact public entrypoint the you binary uses),
// executes "you server acp" itself (not Process.ACPServer() directly), and
// drives one real "initialize" -> "session/new" -> "session/prompt" exchange
// over the command's own caller-owned stdin/stdout. Provider effects are
// replaced only through edges.Edges, via a deterministic ProviderCommandRunner
// fixture shaped like a real provider (Codex-shaped stdout), so the terminal
// prompt response's mapped text is exactly the fixture's own literal answer
// rather than a fabricated or coincidental value.
//
// This is the "you server acp" transport-mechanics sibling of
// tests/functional/sessions/chat_sessions/root_composition/acp_server_composition_test.go,
// which drives Process.ACPServer() directly rather than the CLI command
// tree; both share support.SeedACPAgentProfile instead of each owning a
// private copy of that fixture-seeding helper.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestServeACP_RootBuildProcessCompletesOneFactoryPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving you server acp through root.BuildProcess")
	}

	// cwd is installed as the fixture Factory's own project-local root (see
	// seedFixtureFactory), so the one downstream provider dispatch's WorkDir
	// can be asserted against it below -- proving the client-supplied
	// session/new working root itself reaches exactly one downstream
	// execution, not merely a resolved Factory identifier.
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(fixtureFinalAnswerText),
	})
	scenario := newACPControlCase(t, "ACP-05", runner)
	home := scenario.home
	cwd := scenario.cwd
	factoryDir := scenario.factoryDir
	process := scenario.process

	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

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

	var stderr bytes.Buffer
	command := support.StartProcessCommand(t, process, root.Input{
		Args:             []string{"you", "server", "acp"},
		Env:              environment,
		Stdin:            stdinRead,
		Stdout:           stdoutWrite,
		Stderr:           &stderr,
		WorkingDirectory: cwd,
	})

	stdout := bufio.NewReader(stdoutRead)

	writeRPCLine(t, stdinWrite, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":%s}`,
		fixtureInitializeParams,
	))
	initResp := readRPCResponse(t, stdout)
	if initResp.Error != nil {
		t.Fatalf("initialize response error = %+v, want a successful result", initResp.Error)
	}

	writeRPCLine(t, stdinWrite, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":%q,"mcpServers":[]}}`,
		cwd,
	))
	newSessionResp := readRPCResponse(t, stdout)
	if newSessionResp.Error != nil {
		t.Fatalf("session/new response error = %+v, want a successful result", newSessionResp.Error)
	}
	var created acpsdk.NewSessionResponse
	if err := json.Unmarshal(newSessionResp.Result, &created); err != nil {
		t.Fatalf("unmarshal session/new result: %v", err)
	}
	if created.SessionId == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	if len(created.ConfigOptions) != 1 || created.ConfigOptions[0].Select == nil ||
		string(created.ConfigOptions[0].Select.CurrentValue) != fixtureFactoryTargetID {
		t.Fatalf("session/new configOptions = %+v, want current target %s", created.ConfigOptions, fixtureFactoryTargetID)
	}

	promptParams, err := json.Marshal(map[string]any{
		"sessionId": created.SessionId,
		"prompt":    []map[string]any{{"type": "text", "text": fixturePromptText}},
	})
	if err != nil {
		t.Fatalf("marshal session/prompt params: %v", err)
	}
	writeRPCLine(t, stdinWrite, fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":%s}`, promptParams))

	promptResp, notifications := readNotificationsUntilResponse(t, stdout)
	if got := agentMessageChunkTextFrom(t, notifications); got != fixtureFinalAnswerText {
		t.Fatalf("agent_message_chunk text = %q, want %q", got, fixtureFinalAnswerText)
	}

	if promptResp.Error != nil {
		t.Fatalf("session/prompt response error = %+v, want a successful final result", promptResp.Error)
	}
	var promptResult acpsdk.PromptResponse
	if err := json.Unmarshal(promptResp.Result, &promptResult); err != nil {
		t.Fatalf("unmarshal session/prompt result: %v", err)
	}
	if promptResult.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want %q", promptResult.StopReason, acpsdk.StopReasonEndTurn)
	}

	if err := stdinWrite.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	// command.Done() is the deterministic completion signal (closed exactly
	// when Process.Execute returns); the surrounding time.After is only a
	// hang guard against a genuine regression (Execute never returning after
	// clean stdin EOF), not a substitute for it -- 5s is generous versus this
	// in-process fixture's actual completion time (well under 1s in
	// practice), matching the same bounded-wait-as-hang-guard shape already
	// used for the real production stdio server's own cancellation test.
	select {
	case <-command.Done():
		if err := command.Err(); err != nil {
			t.Fatalf("Process.Execute(you server acp) error = %v after clean stdin EOF; stderr=%s", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Process.Execute(you server acp) did not return after stdin EOF")
	}
	command.AcceptError()

	if got := runner.CallCount(); got != 1 {
		t.Fatalf("provider command call count = %d, want exactly 1", got)
	}
	// The fixture Factory is installed project-locally under cwd itself
	// (factorydefinitions.ProjectFactoriesRoot(cwd), see seedFixtureFactory),
	// the same cwd supplied as this session's session/new working root. The
	// one downstream provider execution's WorkDir equals that installed
	// directory, proving the client-supplied working root itself -- not
	// merely a resolved Factory identifier -- reaches exactly one downstream
	// execution at the expected root.
	if got := runner.LastRequest().WorkDir; got != factoryDir {
		t.Fatalf("provider command WorkDir = %q, want the selected Factory's own install directory %q", got, factoryDir)
	}
	if wantProjectRoot := factorydefinitions.ProjectFactoriesRoot(cwd); !strings.HasPrefix(factoryDir, wantProjectRoot) {
		t.Fatalf("fixture factoryDir = %q, want it installed under the supplied working root's project catalog %q", factoryDir, wantProjectRoot)
	}

	// Process.Execute has already returned, so no further writer can append to
	// stdout; close this end explicitly so the trailing drain below observes
	// EOF instead of blocking on a pipe whose write end t.Cleanup would only
	// close after the test function returns.
	if err := stdoutWrite.Close(); err != nil {
		t.Fatalf("close stdout: %v", err)
	}
	remaining, readErr := io.ReadAll(stdout)
	if readErr != nil && readErr != io.EOF {
		t.Fatalf("read remaining stdout: %v", readErr)
	}
	if trimmed := strings.TrimSpace(string(remaining)); trimmed != "" {
		assertLineIsProtocolFrame(t, trimmed)
	}
	if strings.Contains(stderr.String(), fixtureFinalAnswerText) {
		t.Fatalf("stderr leaked the fixture final answer text: %s", stderr.String())
	}

	functionalevidence.Covers(t, "cli/you.server.acp")
}

func writeRPCLine(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := w.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write RPC line %q: %v", line, err)
	}
}

func readRPCFrame(t *testing.T, r *bufio.Reader) rpcFrame {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read RPC line: %v", err)
	}
	assertLineIsProtocolFrame(t, line)
	var frame rpcFrame
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		t.Fatalf("unmarshal RPC line %q: %v", line, err)
	}
	return frame
}

// assertLineIsProtocolFrame proves a captured stdout line parses as a
// complete JSON-RPC 2.0 object, so no CLI/log/banner text ever reaches
// stdout alongside real ACP protocol traffic.
func assertLineIsProtocolFrame(t *testing.T, line string) {
	t.Helper()
	var generic map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &generic); err != nil {
		t.Fatalf("stdout line is not a valid JSON-RPC frame: %q: %v", line, err)
	}
	if _, ok := generic["jsonrpc"]; !ok {
		t.Fatalf("stdout line missing jsonrpc member: %q", line)
	}
}

// readRPCResponse returns the next correlated response frame, skipping any
// session/update notifications the connection emits alongside it. session/new
// advertises its available commands, so a notification can legitimately
// precede the response it belongs to.
func readRPCResponse(t *testing.T, r *bufio.Reader) rpcFrame {
	t.Helper()
	for {
		frame := readRPCFrame(t, r)
		if frame.Method == "" {
			return frame
		}
		if frame.Method != string(acpsdk.ClientMethodSessionUpdate) {
			t.Fatalf("expected a response frame, got notification method %q", frame.Method)
		}
	}
}

// seedFixtureFactory installs a minimal single-worker Factory project-locally
// under the given working root (the same cwd this test later supplies as
// session/new's cwd param), at the exact
// <ProjectFactoriesRoot(cwd)>/@scope/name/factory.json layout the production
// named-Factory catalog and effective-catalog discovery both read -- the
// same layout seedInstalledPackagedFactory
// (tests/functional/sessions/chat_sessions/root_composition) writes for a real
// packaged Factory, but authored inline here as a single MODEL_WORKER
// pipeline instead of a real packaged Factory's own business workflow, so
// its one dispatch round is fully deterministic through a
// ProviderCommandRunner fixture. Installing project-locally (rather than
// under the global named-Factory root) is what lets the caller prove the
// client-supplied working root itself -- not just the selected Factory's
// identity -- reaches the one downstream execution: production resolves a
// project-local target's dispatch root from the same cwd-derived project
// catalog directory (see
// pkg/services/chat_sessions/internal/service/factory_target_catalog.go's
// validateWorkingRootCompatibility). It returns the installed Factory's own
// directory, which the caller asserts against the one recorded provider
// command's WorkDir.
func seedFixtureFactory(t *testing.T, cwd string) string {
	t.Helper()
	return seedFixtureFactoryForTarget(t, cwd, fixtureFactoryTargetID)
}

func seedFixtureFactoryForTarget(t *testing.T, cwd, targetID string) string {
	t.Helper()

	projectRoot := factorydefinitions.ProjectFactoriesRoot(cwd)
	factoryName := strings.TrimPrefix(targetID, operatorsettings.ACPFactoryTargetNamespace)
	if factoryName == targetID || !strings.HasPrefix(factoryName, "@") {
		t.Fatalf("fixture Factory target %q does not use the expected ACP namespace", targetID)
	}
	factoryDir := filepath.Join(projectRoot, filepath.FromSlash(factoryName))
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", factoryDir, err)
	}

	cfg := map[string]any{
		"name": factoryName,
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name":     "input",
				"required": true,
				"bindings": []any{
					map[string]any{"kind": "POSITIONAL", "position": 1},
					map[string]any{"kind": "STDIN"},
				},
			}},
		},
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal fixture Factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile), raw, 0o644); err != nil {
		t.Fatalf("write fixture factory.json: %v", err)
	}

	agentConfigPath := filepath.Join(factoryDir, "workers", "worker-a", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(agentConfigPath), 0o755); err != nil {
		t.Fatalf("create worker config dir %s: %v", filepath.Dir(agentConfigPath), err)
	}
	if err := os.WriteFile(
		agentConfigPath,
		[]byte(support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex")),
		0o644,
	); err != nil {
		t.Fatalf("write %s: %v", agentConfigPath, err)
	}

	workstationConfigPath := filepath.Join(factoryDir, "workstations", "process", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workstationConfigPath), 0o755); err != nil {
		t.Fatalf("create workstation config dir %s: %v", filepath.Dir(workstationConfigPath), err)
	}
	if err := os.WriteFile(
		workstationConfigPath,
		[]byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"),
		0o644,
	); err != nil {
		t.Fatalf("write %s: %v", workstationConfigPath, err)
	}

	return factoryDir
}

// readNotificationsUntilResponse drains every session/update notification the
// turn emits and returns them alongside the terminal response frame.
//
// A turn does not promise that its first notification is the assistant
// message. Worker Session tool calls are projected as soon as a Worker opens,
// so a tool_call notification legitimately precedes the message. Reading a
// single frame and asserting its shape encodes an ordering the transport never
// guaranteed.
func readNotificationsUntilResponse(t *testing.T, r *bufio.Reader) (rpcFrame, []acpsdk.SessionNotification) {
	t.Helper()
	var notifications []acpsdk.SessionNotification
	for {
		frame := readRPCFrame(t, r)
		if frame.Method == "" {
			return frame, notifications
		}
		if frame.Method != string(acpsdk.ClientMethodSessionUpdate) {
			t.Fatalf("notification method = %q, want %q", frame.Method, acpsdk.ClientMethodSessionUpdate)
		}
		var notification acpsdk.SessionNotification
		if err := json.Unmarshal(frame.Params, &notification); err != nil {
			t.Fatalf("unmarshal session/update params: %v", err)
		}
		notifications = append(notifications, notification)
	}
}

// agentMessageChunkTextFrom concatenates the assistant text delivered across a
// turn's notifications.
func agentMessageChunkTextFrom(t *testing.T, notifications []acpsdk.SessionNotification) string {
	t.Helper()
	text := ""
	found := false
	for _, notification := range notifications {
		chunk := notification.Update.AgentMessageChunk
		if chunk == nil || chunk.Content.Text == nil {
			continue
		}
		found = true
		text += chunk.Content.Text.Text
	}
	if !found {
		t.Fatalf("turn delivered no agent_message_chunk update; got %d notifications", len(notifications))
	}
	return text
}
