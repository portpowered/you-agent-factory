// Functional owner: sessions/chat_sessions/root_composition.
package root_composition_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// A Worker is a tool call, and everything a Worker produces is content inside
// that tool call.
//
// These cells assert that contract end to end: an ACP Worker emitting the full
// variety of session updates must produce, on the Chat Session, one child
// record per element parented to that Worker's opening tool call -- projected
// to the ACP client as tool_call_update addressed to that Worker's ToolCallId
// -- and must never surface as top-level assistant output. The Factory, not
// the Worker, is what speaks to the customer.
//
// The child records are asserted through the production Worker Session and
// Chat Session response-event path. The tests keep the local-real ACP peer
// edge intact so a passing result proves both attribution and the subprocess
// protocol, rather than only an in-process projection.

const acpWorkerChildPeerFlag = "you-test-acp-worker-child-peer"

var runACPWorkerChildPeer = flag.Bool(acpWorkerChildPeerFlag, false, "run the scripted ACP worker-child peer")

// acpWorkerChildCompletionText ends in the fixture worker's own stopToken, so
// the Work genuinely reaches its declared terminal state. Without a literal
// stopToken match this MODEL_WORKER path never advances past PROCESSING and
// every downstream assertion would be vacuous.
const acpWorkerChildCompletionText = "the child-events fixture genuinely finished COMPLETE"

// TestOneACPWorkerDeliversEveryUpdateAsChildContent drives one Factory
// dispatch backed by a scripted ACP agent that emits every update variant the
// production client models, and asserts each one arrives as content inside
// that Worker's tool call.
func TestOneACPWorkerDeliversEveryUpdateAsChildContent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}
	t.Parallel()

	notifications := runACPWorkerChildFixture(t, "one-worker", 1)
	children, topLevel := splitACPWorkerChildNotifications(notifications)

	if len(children.openings) != 1 {
		t.Fatalf("worker tool_call openings = %d, want exactly 1 (one dispatch, one Worker Session)",
			len(children.openings))
	}
	toolCallID := children.openings[0]

	// Every update the Worker produced must be addressed to that Worker's own
	// tool call. A zero count here means the Worker topic carried no content
	// records at all, which is the producer gap these cells exist to close.
	if len(children.updatesByToolCall[toolCallID]) == 0 {
		t.Fatalf("tool_call_update notifications for %q = 0, want one per emitted Worker update; "+
			"all notifications = %s", toolCallID, describeACPWorkerNotifications(notifications))
	}

	// A Worker does not speak to the customer, so none of its reasoning may
	// surface as top-level assistant output.
	if len(topLevel.thoughtTexts) > 0 {
		t.Fatalf("top-level agent_thought_chunk texts = %q, want none -- a Worker's reasoning "+
			"belongs inside its tool call", topLevel.thoughtTexts)
	}

	// Exactly one top-level message is correct and expected: the Factory's own
	// extracted result, which for this fixture is derived from the Worker's
	// output. What must not happen is the Worker's individual chunks arriving
	// as separate top-level assistant messages -- that is the streamed-Worker
	// -output shape this routing removes, and it is distinguishable by count.
	if len(topLevel.messageTexts) > 1 {
		t.Fatalf("top-level agent_message_chunk notifications = %d (%q), want at most 1 (the "+
			"Factory's own result) -- a Worker's chunks must be tool-call content, not assistant output",
			len(topLevel.messageTexts), topLevel.messageTexts)
	}

	// The scripted peer emits thought, message, tool_call, plan, usage, a diff
	// -bearing tool_call_update, and a completion message. Each must be
	// represented in the child stream.
	body := strings.Join(children.updatesByToolCall[toolCallID], "\n")
	for _, want := range []string{
		"considering ",          // agent_thought_chunk -> REASONING
		"working on ",           // agent_message_chunk -> MESSAGE
		"Inspect Factory",       // tool_call / tool_call_update -> TOOL
		"Complete the ACP turn", // plan -> PLAN
		"17",                    // usage_update -> USAGE
		"factory/result.txt",    // diff -> FILE_CHANGE
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("child content for %q is missing %q; got:\n%s", toolCallID, want, body)
		}
	}
}

// TestTwoACPWorkersKeepChildStreamsAttributed proves two Workers produce two
// independent child streams. It is the cell that catches a shared sequence
// counter or a single global projection budget, which is the mistake a first
// implementation makes.
func TestTwoACPWorkersKeepChildStreamsAttributed(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}
	t.Parallel()

	notifications := runACPWorkerChildFixture(t, "two-workers", 2)
	children, _ := splitACPWorkerChildNotifications(notifications)

	if len(children.openings) != 2 {
		t.Fatalf("worker tool_call openings = %d, want exactly 2 (one per dispatch); all = %s",
			len(children.openings), describeACPWorkerNotifications(notifications))
	}
	first, second := children.openings[0], children.openings[1]
	if first == second {
		t.Fatalf("both Workers opened the same tool call %q, want distinct sequencer-assigned identities", first)
	}

	// Every Worker's opening and terminal lifecycle records already exist today,
	// so a bare "has some updates" check would pass without any content ever
	// being routed. Each stream must carry its own Worker's scripted text.
	markers := map[string]string{}
	for _, toolCallID := range children.openings {
		body := strings.Join(children.updatesByToolCall[toolCallID], "\n")
		marker := acpWorkerChildPeerMarker(body)
		if marker == "" {
			t.Fatalf("child content for %q carries no Worker-authored text -- only lifecycle status. "+
				"got:\n%s\nall notifications = %s",
				toolCallID, body, describeACPWorkerNotifications(notifications))
		}
		markers[toolCallID] = marker
	}

	// Cross-attribution is the failure this cell exists to catch: each dispatch
	// gets its own provider session, so two Workers carrying the same marker
	// means their streams merged into one.
	if markers[first] == markers[second] {
		t.Fatalf("both Workers' child streams carry provider session %q, want one each -- "+
			"a shared sequence counter or a merged stream is the likely cause", markers[first])
	}
}

// acpWorkerChildPeerMarker extracts the scripted peer's own provider session
// id from a Worker's child content. The peer embeds it in every update it
// emits, so its presence proves Worker-authored content reached the stream and
// its value proves which Worker authored it.
func acpWorkerChildPeerMarker(body string) string {
	const prefix = "tool-acp-child-"
	index := strings.Index(body, prefix)
	if index < 0 {
		return ""
	}
	rest := body[index+len(prefix):]
	end := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
	if end <= 0 {
		return ""
	}
	return prefix + rest[:end]
}

// TestACPWorkerChildStreamSurvivesRetainedReplay proves the retained replay
// that runs in every turn is safe.
//
// responsebridge's workerChildren.finish re-reads each Worker topic from a
// zero cursor after the live drain, so every child record this Worker
// produced is processed a second time. That sweep is where the ordering
// hazards live: a child record observed before its opening trips
// ErrWorkerChildOpeningRequired, one observed after the terminal record trips
// ErrWorkerChildAfterTerminal, and a re-sequenced record that is not resolved
// to its original identity is delivered to the client twice.
//
// Cross-process replay is deliberately not the vehicle here: Chat Sessions
// are process-local, so a second `you server acp` process cannot resolve the
// session at all. Same-connection session/load is also not it -- that path
// correctly reuses its acknowledged cursor and redelivers nothing by design
// (see loadSession's own doc comment). The in-run retained sweep is the
// replay this system actually performs, and duplication or loss in it is
// observable right here.
func TestACPWorkerChildStreamSurvivesRetainedReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}
	t.Parallel()

	notifications := runACPWorkerChildFixture(t, "replay", 1)
	children, _ := splitACPWorkerChildNotifications(notifications)

	if len(children.openings) != 1 {
		t.Fatalf("worker tool_call openings = %d, want exactly 1; all = %s",
			len(children.openings), describeACPWorkerNotifications(notifications))
	}
	toolCallID := children.openings[0]
	updates := children.updatesByToolCall[toolCallID]
	if len(updates) == 0 {
		t.Fatalf("child updates for %q = 0, want the Worker's own content", toolCallID)
	}

	// The retained sweep re-reads every record. A record it re-sequences
	// without resolving to its original identity arrives at the client a
	// second time, which is what this counts.
	seen := map[string]int{}
	for _, update := range updates {
		seen[update]++
	}
	for update, count := range seen {
		if count > 1 {
			t.Fatalf("child update delivered %d times after the retained replay, want exactly once:\n%s",
				count, update)
		}
	}

	// Ordering: the opening must precede every update addressed to it, or the
	// retained sweep would have rejected the child as parentless.
	openingIndex := -1
	for index, n := range notifications {
		if n.Update.ToolCall != nil && string(n.Update.ToolCall.ToolCallId) == toolCallID {
			openingIndex = index
			break
		}
	}
	if openingIndex < 0 {
		t.Fatalf("no opening tool_call for %q in the delivered stream", toolCallID)
	}
	for index, n := range notifications {
		if n.Update.ToolCallUpdate == nil || string(n.Update.ToolCallUpdate.ToolCallId) != toolCallID {
			continue
		}
		if index < openingIndex {
			t.Fatalf("child update at position %d precedes its opening at %d -- a parentless child "+
				"would be dropped by the retained sweep", index, openingIndex)
		}
	}
}

// acpWorkerChildNotifications is the split view these cells assert on: what
// arrived as content inside a Worker's tool call, keyed by that tool call.
type acpWorkerChildNotifications struct {
	openings          []string
	updatesByToolCall map[string][]string
}

type acpWorkerTopLevelNotifications struct {
	messageTexts []string
	thoughtTexts []string
}

func splitACPWorkerChildNotifications(
	notifications []acpsdk.SessionNotification,
) (acpWorkerChildNotifications, acpWorkerTopLevelNotifications) {
	children := acpWorkerChildNotifications{updatesByToolCall: map[string][]string{}}
	var topLevel acpWorkerTopLevelNotifications
	for _, n := range notifications {
		switch {
		case n.Update.ToolCall != nil:
			children.openings = append(children.openings, string(n.Update.ToolCall.ToolCallId))
		case n.Update.ToolCallUpdate != nil:
			id := string(n.Update.ToolCallUpdate.ToolCallId)
			encoded, err := json.Marshal(n.Update.ToolCallUpdate)
			if err != nil {
				encoded = []byte(fmt.Sprintf("<unencodable: %v>", err))
			}
			children.updatesByToolCall[id] = append(children.updatesByToolCall[id], string(encoded))
		case n.Update.AgentMessageChunk != nil:
			if n.Update.AgentMessageChunk.Content.Text != nil {
				topLevel.messageTexts = append(topLevel.messageTexts, n.Update.AgentMessageChunk.Content.Text.Text)
			}
		case n.Update.AgentThoughtChunk != nil:
			if n.Update.AgentThoughtChunk.Content.Text != nil {
				topLevel.thoughtTexts = append(topLevel.thoughtTexts, n.Update.AgentThoughtChunk.Content.Text.Text)
			}
		}
	}
	return children, topLevel
}

// describeACPWorkerNotifications renders the observed stream so a failure says
// what actually arrived instead of only what did not.
func describeACPWorkerNotifications(notifications []acpsdk.SessionNotification) string {
	var parts []string
	for _, n := range notifications {
		encoded, err := json.Marshal(n.Update)
		if err != nil {
			parts = append(parts, fmt.Sprintf("<unencodable: %v>", err))
			continue
		}
		parts = append(parts, string(encoded))
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return "\n  " + strings.Join(parts, "\n  ")
}

func runACPWorkerChildFixture(t *testing.T, name string, stages int) []acpsdk.SessionNotification {
	t.Helper()

	// The ACP-execution provider keeps one self-reexec'd peer process for all
	// stages in this Factory and opens distinct protocol sessions for distinct
	// Workers. This fixture deliberately keeps its root scenario-local: the
	// seeded stage topology and the completed on-demand Factory activation are
	// immutable process-scoped inputs that cannot be safely combined with
	// another cell until Factory release/reopen is publicly supported.
	home, cwd := seedACPWorkerChildHome(t, name, stages)
	var starts atomic.Int32
	peerOwner := "ACP worker child peer " + t.Name()
	trackChatPeerOwner(t, peerOwner)
	stdin, stdout := startServeACPHarness(t, home, cwd, serviceedges.Edges{
		PlatformProcessCommandFactory: acpWorkerChildCommandFactory(&starts, peerOwner),
		ProvidersExecutableLocator:    acpWorkerChildExecutableLocator{},
	})

	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	driveServeACPFactoryTarget(t, stdin, stdout, sessionID, "factory:@acp-child-test/"+name)
	promptResp, notifications := driveServeACPSessionPrompt(t, stdin, stdout, sessionID, "please run this work")
	if promptResp.Error != nil {
		t.Fatalf("session/prompt response error = %+v, want a successful final result", promptResp.Error)
	}
	var decoded acpsdk.PromptResponse
	if err := json.Unmarshal(promptResp.Result, &decoded); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if decoded.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want %q; notifications = %s",
			decoded.StopReason, acpsdk.StopReasonEndTurn, describeACPWorkerNotifications(notifications))
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("ACP peer subprocess starts = %d, want exactly 1 (one local-real peer shared by this Factory's Worker sessions)", got)
	}
	return notifications
}

func seedACPWorkerChildHome(t *testing.T, name string, stages int) (home, cwd string) {
	t.Helper()

	home = sharedACPActivationHomeForTest(t)

	return home, chatTempDir(t, "ACP worker child "+name+" working directory", "worker-child-cwd-")
}

func seedSharedACPWorkerChildFactories(t testing.TB, home string) {
	t.Helper()
	seedACPWorkerChildFactory(t, home, "one-worker", 1)
	seedACPWorkerChildFactory(t, home, "two-workers", 2)
	seedACPWorkerChildFactory(t, home, "replay", 1)
}

func seedACPWorkerChildFactory(t testing.TB, home, name string, stages int) {
	t.Helper()
	root, err := factorydefinitions.NamedFactoriesRootForHome(home)
	if err != nil {
		t.Fatalf("NamedFactoriesRootForHome() error = %v", err)
	}
	dir := filepath.Join(root, "@acp-child-test", name)
	registerChatFactoryPath(t, dir)
	writeACPWorkerChildFile(t, dir, "factory.json", acpWorkerChildFactoryJSON(name, stages))
	for stage := 1; stage <= stages; stage++ {
		worker := fmt.Sprintf("worker%d", stage)
		writeACPWorkerChildFile(t, filepath.Join(dir, "workers", worker), "AGENTS.md", acpWorkerChildWorkerAgents)
		writeACPWorkerChildFile(t, filepath.Join(dir, "workstations", fmt.Sprintf("stage%d", stage)),
			"AGENTS.md", acpWorkerChildWorkstationAgents)
	}
}

func writeACPWorkerChildFile(t testing.TB, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", filepath.Join(dir, name), err)
	}
}

// acpWorkerChildFactoryJSON builds a linear pipeline of `stages`
// ACP-execution workstations. Each stage is one dispatch and therefore one
// Worker Session, so a two-stage factory yields the two independently
// attributed Workers the two-Worker cell asserts on.
func acpWorkerChildFactoryJSON(name string, stages int) string {
	states := []string{`{"name": "s0", "type": "INITIAL"}`}
	for stage := 1; stage < stages; stage++ {
		states = append(states, fmt.Sprintf(`{"name": "s%d", "type": "PROCESSING"}`, stage))
	}
	states = append(states,
		fmt.Sprintf(`{"name": "s%d", "type": "TERMINAL"}`, stages),
		`{"name": "failed", "type": "FAILED"}`)

	var workers, workstations []string
	for stage := 1; stage <= stages; stage++ {
		workers = append(workers, fmt.Sprintf(`{"name": "worker%d"}`, stage))
		workstations = append(workstations, fmt.Sprintf(`{
      "name": "stage%d",
      "worker": "worker%d",
      "inputs": [{"workType": "task", "state": "s%d"}],
      "outputs": [{"workType": "task", "state": "s%d"}],
      "onFailure": [{"workType": "task", "state": "failed"}]
    }`, stage, stage, stage-1, stage))
	}

	return fmt.Sprintf(`{
  "name": %q,
  "invocationSignature": {
    "parameters": [
      {
        "name": "input",
        "description": "Message to process.",
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
    "terminalState": "s%d"
  },
  "workTypes": [
    {
      "name": "task",
      "handlingBehavior": ["DEFAULT"],
      "states": [%s]
    }
  ],
  "workers": [%s],
  "workstations": [%s]
}
`, name, stages, strings.Join(states, ", "), strings.Join(workers, ", "), strings.Join(workstations, ", "))
}

const acpWorkerChildWorkerAgents = "---\n" +
	"executorProvider: ACP\n" +
	"modelProvider: cursor-acp\n" +
	"model: test-model\n" +
	"stopToken: COMPLETE\n" +
	"type: MODEL_WORKER\n" +
	"---\n\nTest ACP child-events worker.\n"

const acpWorkerChildWorkstationAgents = "---\n" +
	"type: MODEL_WORKSTATION\n" +
	"---\n\nTest ACP child-events workstation.\n"

// acpWorkerChildCommandFactory intercepts the built-in cursor-acp integration's
// own command and re-execs this test binary as the scripted peer, matching the
// pattern in acp_streaming_usage_composition_test.go.
func acpWorkerChildCommandFactory(starts *atomic.Int32, peerOwner string) platformprocess.CommandFactory {
	return func(name string, args ...string) *exec.Cmd {
		if name == "cursor-agent" && len(args) == 1 && args[0] == "acp" {
			starts.Add(1)
			beginChatPeer(peerOwner)
			return exec.Command(os.Args[0], "-test.run=^TestACPWorkerChildPeerProcess$", "-"+acpWorkerChildPeerFlag)
		}
		return exec.Command(name, args...)
	}
}

type acpWorkerChildExecutableLocator struct{}

func (acpWorkerChildExecutableLocator) LookPath(file string) (string, error) { return file, nil }

// TestACPWorkerChildPeerProcess is the self-re-exec'd scripted ACP subprocess
// entrypoint. Under a normal `go test` run of this package the gate variable is
// unset and this returns immediately.
func TestACPWorkerChildPeerProcess(t *testing.T) {
	if !*runACPWorkerChildPeer {
		return
	}
	err := support.RunACPWorkerPeer(support.ACPWorkerPeerConfig{
		SessionIDPrefix: "acp-child",
		CompletionText:  acpWorkerChildCompletionText,
	}, os.Stdin, os.Stdout)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

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
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}

	// This cell intentionally owns its root process: the @you/spawn activation
	// depends on the process environment's default Worker provider and its
	// completed activation remains retained under the process-scoped ~default
	// binding. Its immutable home/catalog/profile is shared with the other
	// activation cells; sharing the process would change that production
	// lifecycle edge.
	home := sharedACPActivationHomeForTest(t)

	runner := &spawnACPRunner{}
	cwd := chatTempDir(t, "JavaScript worker child working directory", "javascript-child-cwd-")
	t.Parallel()
	stdin, stdout := startServeACPHarness(t, home, cwd, serviceedges.Edges{ProviderCommandRunner: runner})

	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	driveServeACPFactoryTarget(t, stdin, stdout, sessionID, "factory:@you/spawn")

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
