package root_composition_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// acpStreamUsagePeerEnvironment gates usageStreamRPCPeer's serve loop so it is
// a no-op under a normal `go test` run and only behaves as the scripted ACP
// subprocess peer when TestACPServeCommandStreamsUsageUpdateThroughRootBuildProcess
// itself launches it (through acpStreamUsageCommandFactory's self-re-exec of
// os.Args[0]), matching golden_rpc_peer_test.go's YOU_TEST_ACP_GOLDEN_MODE
// pattern for the same reason: this file lives in package
// root_composition_test, a different package than
// tests/functional/providers/acp's own peer, so self-re-exec only works
// against a peer test function defined in this package.
const acpStreamUsagePeerEnvironment = "YOU_TEST_ACP_STREAM_USAGE_PEER"

// acpStreamUsageSessionID is the fixed sessionId this cell's scripted peer
// always returns from session/new and expects back on session/prompt -- there
// is only ever one session per test run, so a fixed constant (rather than a
// generated ID) keeps the peer and the test assertions trivially in sync.
const acpStreamUsageSessionID = "sess_stream_test_1"

// acpStreamUsageCompletionText is the distinctive agent_message_chunk text
// the scripted peer streams, ending in the fixture worker's own configured
// stopToken ("COMPLETE") so the Factory Runtime actually routes the admitted
// Work to its declared terminal state -- without a literal stopToken match,
// this decision-envelope-free MODEL_WORKER path never advances past PROCESSING.
const acpStreamUsageCompletionText = "the streaming fixture genuinely finished COMPLETE"

const acpStreamUsageReasoningText = "the fixture is considering a safe delivery"

const acpStreamUsageSessionTitle = "Delivering the ACP streaming fixture"

// TestACPServeCommandStreamsUsageUpdateThroughRootBuildProcess extends story
// 005's sibling cell (acp_streaming_composition_test.go's
// TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText,
// which proves only agent_message_chunk delivery, because its @you/goal
// AGENT_RUN/decision-envelope fixture never produces USAGE or REASONING
// response events at all) to also prove a real usage_update notification
// delivered through the full production graph: root.BuildProcess -> the real
// "you serve acp" Cobra command via Process.Execute -> the real ACP stdio
// transport -> the real Chat Sessions/Events authorities -> the real producer
// bridge -> this PRD's own outbound projector.
//
// This cell reaches USAGE by using a different kind of fixture than the
// sibling cell: an ACP-EXECUTION worker (executorProvider: ACP), which is
// itself an ACP *client* driving a scripted subprocess ACP *server* over
// stdio (platformprocess.CommandFactory/ExecutableLocator edges), not the
// decision-envelope AGENT_RUN shape @you/goal uses. The scripted subprocess
// peer (usageStreamRPCPeer, self-reexec'd via os.Args[0], see
// TestACPStreamUsagePeerProcess below) sends real reasoning, usage, session
// title, and message updates before the final stop reason. The message ends
// in the fixture worker's stopToken, so the underlying Factory Runtime
// genuinely reaches its declared TERMINAL work state -- this is a real,
// independently proven production path: see
// tests/functional/providers/acp/run_parameters_content_test.go's
// TestYouRunUsesPinnedACPWireGoldensAndProjectsTerminalOutput, whose pinned
// golden response_stream.ndjson shows a real USAGE response event produced
// from an identically-shaped usage_update notification.
//
// The peer also sends reasoning and title-bearing session metadata updates.
// The production path preserves REASONING's DELTA phase and SESSION's UPDATED
// phase, then projects their public ACP updates in the aggregate order below.
//
// This cell also exposed and closed a THIRD, previously undocumented defect,
// inside this PRD's own package
// (pkg/transports/acp/internal/stdio/prompt_stream.go), unlike the two
// out-of-scope Factory Sessions gaps above: a genuine concurrency race in the
// mid-generation live-streaming mechanism (liveDrainTurnUpdates /
// streamTurnUpdates sharing one attachment cursor). drainRecords (shared by
// both drains) calls notify(...) for a record BEFORE it calls
// s.chatSessions.AcknowledgeAttachment(...) to advance and persist that
// record's cursor position. liveDrainTurnUpdates runs against the
// bridge-derived ctx that factorysessionsshim.RunWithResponseBridge cancels
// the instant the wrapped Factory invocation returns (see
// response_bridge.go's dispatchFactoryInvocation doc comment). When that
// cancellation landed between drainRecords' notify(...) and its
// AcknowledgeAttachment call for the SAME record -- entirely possible
// whenever the wrapped invocation completes as fast as this cell's scripted
// peer does, unlike the sibling composition test's much slower
// decision-envelope round trip -- AcknowledgeAttachment observed ctx already
// canceled, returned an error, and drainRecords' cursor advance
// (`*attachment = ackResult.Attachment; cache.set(...)`) never ran; the
// record was already delivered over the wire, but the cached attachment
// cursor was not persisted past it, so the subsequent, guaranteed-correct
// streamTurnUpdates catch-up sweep reused that stale cached cursor and
// redelivered the identical record a second time -- directly contradicting
// liveDrainTurnUpdates' own doc comment ("No record is ever skipped or
// duplicated regardless of how far this live drain gets") and this PRD's own
// central "at most once per attachment" delivery guarantee. Fixed by giving
// the AcknowledgeAttachment call in both drainRecords and deliverReadTimeGap
// a context.WithoutCancel(ctx), the same idiom this file's detachAttachments
// already uses for the identical "must finish even though the surrounding
// ctx was just canceled for a reason unrelated to this specific record"
// reason: the client has already received the record by the time notify
// returns, so persisting its cursor position must not itself be abortable by
// the wrapped invocation's own unrelated completion. Confirmed fixed via
// -count=30 on this cell with zero redelivery observed (every prior run
// before the fix flaked within a handful of iterations).
func TestACPServeCommandStreamsUsageUpdateThroughRootBuildProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you serve acp CLI command")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(acpStreamUsagePeerEnvironment, "1")

	const factoryName = "@acp-stream-test/usage"
	seedACPStreamUsageFactory(t, home)
	support.SeedACPAgentProfile(t, home, "factory:"+factoryName, []string{"factory:" + factoryName})

	var processStarts atomic.Int32
	cwd := t.TempDir()
	stdin, stdout := startServeACPHarness(t, home, cwd, serviceedges.Edges{
		PlatformProcessCommandFactory: acpStreamUsageCommandFactory(&processStarts),
		ProvidersExecutableLocator:    acpStreamUsageExecutableLocator{},
	})

	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	promptResp, notifications := driveServeACPSessionPrompt(t, stdin, stdout, sessionID, "please stream this work")
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
	if got := processStarts.Load(); got != 1 {
		t.Fatalf("ACP subprocess starts = %d, want exactly 1", got)
	}

	assertACPStreamUsageNotifications(t, notifications)
}

// assertACPStreamUsageNotifications proves all four ACP update classes are
// observed before the terminal prompt response. The terminal message remains
// singular even though the provider also supplied live reasoning and metadata.
func assertACPStreamUsageNotifications(t *testing.T, notifications []acpsdk.SessionNotification) {
	t.Helper()

	var messageTexts []string
	var thoughtTexts, sessionTitles []string
	var usedValues []int
	positions := map[string]int{}
	for _, n := range notifications {
		switch {
		case n.Update.AgentMessageChunk != nil:
			positions["message"] = len(positions)
			var text string
			if n.Update.AgentMessageChunk.Content.Text != nil {
				text = n.Update.AgentMessageChunk.Content.Text.Text
			}
			messageTexts = append(messageTexts, text)
		case n.Update.AgentThoughtChunk != nil:
			if _, ok := positions["thought"]; !ok {
				positions["thought"] = len(positions)
			}
			if n.Update.AgentThoughtChunk.Content.Text != nil {
				thoughtTexts = append(thoughtTexts, n.Update.AgentThoughtChunk.Content.Text.Text)
			}
		case n.Update.UsageUpdate != nil:
			positions["usage"] = len(positions)
			usedValues = append(usedValues, n.Update.UsageUpdate.Used)
		case n.Update.SessionInfoUpdate != nil:
			positions["session"] = len(positions)
			if n.Update.SessionInfoUpdate.Title != nil {
				sessionTitles = append(sessionTitles, *n.Update.SessionInfoUpdate.Title)
			}
		}
	}

	if len(usedValues) != 1 {
		t.Fatalf("usage_update notifications = %d, want exactly 1", len(usedValues))
	}
	if usedValues[0] != 17 {
		t.Fatalf("usage_update Used = %d, want 17 (the scripted peer's own published value)", usedValues[0])
	}
	if len(thoughtTexts) == 0 || thoughtTexts[0] != acpStreamUsageReasoningText {
		t.Fatalf("agent_thought_chunk texts = %q, want first %q", thoughtTexts, acpStreamUsageReasoningText)
	}
	if len(sessionTitles) != 1 || sessionTitles[0] != acpStreamUsageSessionTitle {
		t.Fatalf("session_info_update titles = %q, want exactly %q", sessionTitles, acpStreamUsageSessionTitle)
	}
	if len(messageTexts) != 1 {
		t.Fatalf("agent_message_chunk notifications = %d (%q), want exactly 1", len(messageTexts), messageTexts)
	}
	if messageTexts[0] != acpStreamUsageCompletionText {
		t.Fatalf("agent_message_chunk text = %q, want %q", messageTexts[0], acpStreamUsageCompletionText)
	}
	if !(positions["thought"] < positions["usage"] && positions["usage"] < positions["session"] && positions["session"] < positions["message"]) {
		t.Fatalf("first update positions = %#v, want thought < usage < session-info < message", positions)
	}
}

// seedACPStreamUsageFactory writes a hand-authored, split-layout Factory
// fixture directly under home's named-Factory root, at
// @acp-stream-test/usage/ -- the exact layout and location the production
// named-Factory catalog and ACP factory:@scope/leaf target resolution both
// read, matching seedInstalledPackagedFactory's own approach for the
// packaged @you/goal fixture. Unlike tests/functional_test/testdata/
// executor_success/ (which this fixture's shape otherwise mirrors), this one
// also declares invocationSignature/invocationReturn -- required for a
// session/prompt turn's text to bind into the created Work's input and for
// the ACP transport to observe genuine termination, matching the shape
// packages/packaged-factories/factories/goal/factory.yaml uses -- and its
// one worker/workstation pair use executorProvider: ACP (an ACP-execution
// worker, not @you/goal's decision-envelope AGENT_RUN shape).
func seedACPStreamUsageFactory(t *testing.T, home string) {
	t.Helper()

	globalRoot, err := factorydefinitions.NamedFactoriesRootForHome(home)
	if err != nil {
		t.Fatalf("NamedFactoriesRootForHome() error = %v", err)
	}
	factoryDir := filepath.Join(globalRoot, "@acp-stream-test", "usage")
	writeACPStreamUsageFile(t, factoryDir, "factory.json", acpStreamUsageFactoryJSON)
	writeACPStreamUsageFile(t, filepath.Join(factoryDir, "workers", "worker"), "AGENTS.md", acpStreamUsageWorkerAgents)
	writeACPStreamUsageFile(t, filepath.Join(factoryDir, "workstations", "process"), "AGENTS.md", acpStreamUsageWorkstationAgents)
}

func writeACPStreamUsageFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

const acpStreamUsageFactoryJSON = `{
  "name": "usage",
  "invocationSignature": {
    "parameters": [
      {
        "name": "input",
        "description": "Message to stream.",
        "externalName": "message",
        "required": true,
        "bindings": [
          {"kind": "POSITIONAL", "position": 1},
          {"kind": "STDIN"},
          {"kind": "NAMED"}
        ]
      }
    ]
  },
  "invocationReturn": {
    "policy": "EXPLICIT",
    "workTypeName": "task",
    "terminalState": "done"
  },
  "workTypes": [
    {
      "name": "task",
      "handlingBehavior": ["DEFAULT"],
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "done", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "workers": [
    {"name": "worker"}
  ],
  "workstations": [
    {
      "name": "process",
      "worker": "worker",
      "inputs": [
        {"workType": "task", "state": "init"}
      ],
      "outputs": [
        {"workType": "task", "state": "done"}
      ],
      "onFailure": [
        {"workType": "task", "state": "failed"}
      ]
    }
  ]
}
`

const acpStreamUsageWorkerAgents = "---\n" +
	"executorProvider: ACP\n" +
	"modelProvider: cursor-acp\n" +
	"model: test-model\n" +
	"stopToken: COMPLETE\n" +
	"type: MODEL_WORKER\n" +
	"---\n\nTest ACP streaming worker.\n"

const acpStreamUsageWorkstationAgents = "---\n" +
	"type: MODEL_WORKSTATION\n" +
	"---\n\nTest ACP streaming workstation.\n"

// acpStreamUsageCommandFactory intercepts the built-in "cursor-acp" ACP
// integration's own command ("cursor-agent acp", see
// pkg/services/providers/internal/services/builtins/wire/catalog.json) and
// re-execs this same test binary at TestACPStreamUsagePeerProcess, matching
// tests/functional/providers/acp/basic_factory_run_test.go's
// acpHelperCommandFactory pattern.
func acpStreamUsageCommandFactory(starts *atomic.Int32) platformprocess.CommandFactory {
	return func(name string, args ...string) *exec.Cmd {
		if name == "cursor-agent" && len(args) == 1 && args[0] == "acp" {
			starts.Add(1)
			return exec.Command(os.Args[0], "-test.run=^TestACPStreamUsagePeerProcess$")
		}
		return exec.Command(name, args...)
	}
}

// acpStreamUsageExecutableLocator is a trivial platformprocess.ExecutableLocator
// that reports every executable name as available, matching
// tests/functional/providers/acp/basic_factory_run_test.go's
// availableExecutableLocator -- this fixture's ACP-execution worker never
// actually shells out to a real "cursor-agent" binary (acpStreamUsageCommandFactory
// substitutes the scripted peer above), so path resolution must not fail
// availability checks in its place.
type acpStreamUsageExecutableLocator struct{}

func (acpStreamUsageExecutableLocator) LookPath(file string) (string, error) { return file, nil }

// TestACPStreamUsagePeerProcess is the self-re-exec'd scripted ACP subprocess
// peer entrypoint. It is a no-op unless acpStreamUsageCommandFactory launched
// this same test binary with acpStreamUsagePeerEnvironment set (see
// golden_rpc_peer_test.go's identical TestACPGoldenRPCPeerProcess pattern in
// the sibling providers/acp package) -- under a normal `go test` run of this
// package, os.Getenv returns "" and this function returns immediately.
func TestACPStreamUsagePeerProcess(t *testing.T) {
	if os.Getenv(acpStreamUsagePeerEnvironment) == "" {
		return
	}
	peer := &usageStreamRPCPeer{scanner: bufio.NewScanner(os.Stdin), writer: bufio.NewWriter(os.Stdout)}
	if err := peer.serve(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

// usageStreamRPCPeer is a minimal, hand-rolled JSON-RPC 2.0 ACP *server* peer
// -- deliberately not an acp-go-sdk Agent, matching
// tests/functional/providers/acp/golden_rpc_peer_test.go's own goldenRPCPeer,
// so the real production ACP-execution client encodes, transports, and
// decodes every message over real OS pipes exactly like it would against a
// genuine subprocess agent.
type usageStreamRPCPeer struct {
	scanner *bufio.Scanner
	writer  *bufio.Writer
}

type usageStreamRPCEnvelope struct {
	JSONRPC string               `json:"jsonrpc"`
	ID      json.RawMessage      `json:"id,omitempty"`
	Method  string               `json:"method,omitempty"`
	Params  json.RawMessage      `json:"params,omitempty"`
	Result  json.RawMessage      `json:"result,omitempty"`
	Error   *acpsdk.RequestError `json:"error,omitempty"`
}

func (p *usageStreamRPCPeer) serve() error {
	for p.scanner.Scan() {
		var request usageStreamRPCEnvelope
		if err := json.Unmarshal(p.scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode client RPC: %w", err)
		}
		switch request.Method {
		case "initialize":
			if err := p.respondInitialize(request.ID); err != nil {
				return err
			}
		case "session/new":
			if err := p.respondNewSession(request.ID); err != nil {
				return err
			}
		case "session/prompt":
			if err := p.respondPrompt(request.ID); err != nil {
				return err
			}
			return nil
		case "$/cancel_request", "session/cancel":
			return nil
		default:
			return fmt.Errorf("unexpected client RPC method %q", request.Method)
		}
	}
	if err := p.scanner.Err(); err != nil {
		return fmt.Errorf("read client RPC: %w", err)
	}
	return nil
}

// respondInitialize answers with a zero-value AgentCapabilities -- the real
// production ACP-execution adapter only validates ProtocolVersion (see
// pkg/services/providers/internal/services/acp/internal/service/service.go's
// Initialize call site, which checks initialized.ProtocolVersion but never
// inspects AgentCapabilities/AuthMethods).
func (p *usageStreamRPCPeer) respondInitialize(id json.RawMessage) error {
	response := acpsdk.InitializeResponse{
		ProtocolVersion:   acpsdk.ProtocolVersionNumber,
		AgentCapabilities: acpsdk.AgentCapabilities{},
		AuthMethods:       []acpsdk.AuthMethod{},
	}
	result, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return p.respond(id, result)
}

func (p *usageStreamRPCPeer) respondNewSession(id json.RawMessage) error {
	result := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"configOptions":[]}`, acpStreamUsageSessionID))
	return p.respond(id, result)
}

// respondPrompt sends real reasoning, usage, session-title, and message
// updates in source order before its truthful terminal stop reason.
func (p *usageStreamRPCPeer) respondPrompt(id json.RawMessage) error {
	thoughtUpdate := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":%q}}}`, acpStreamUsageSessionID, acpStreamUsageReasoningText))
	if err := p.write(usageStreamRPCEnvelope{JSONRPC: "2.0", Method: "session/update", Params: thoughtUpdate}); err != nil {
		return err
	}
	usageUpdate := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"update":{"sessionUpdate":"usage_update","used":17,"size":4096}}`, acpStreamUsageSessionID))
	if err := p.write(usageStreamRPCEnvelope{JSONRPC: "2.0", Method: "session/update", Params: usageUpdate}); err != nil {
		return err
	}
	sessionUpdate := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"update":{"sessionUpdate":"session_info_update","title":%q}}`, acpStreamUsageSessionID, acpStreamUsageSessionTitle))
	if err := p.write(usageStreamRPCEnvelope{JSONRPC: "2.0", Method: "session/update", Params: sessionUpdate}); err != nil {
		return err
	}
	messageUpdate := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":%q}}}`, acpStreamUsageSessionID, acpStreamUsageCompletionText))
	if err := p.write(usageStreamRPCEnvelope{JSONRPC: "2.0", Method: "session/update", Params: messageUpdate}); err != nil {
		return err
	}
	return p.respond(id, json.RawMessage(`{"stopReason":"end_turn"}`))
}

func (p *usageStreamRPCPeer) respond(id, result json.RawMessage) error {
	return p.write(usageStreamRPCEnvelope{JSONRPC: "2.0", ID: id, Result: result})
}

func (p *usageStreamRPCPeer) write(message usageStreamRPCEnvelope) error {
	if err := json.NewEncoder(p.writer).Encode(message); err != nil {
		return err
	}
	return p.writer.Flush()
}
