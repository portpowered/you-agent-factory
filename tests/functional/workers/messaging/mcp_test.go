package messaging

import (
	"context"
	"encoding/json"
	"errors"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/root"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Each connection owns streams and execution credentials, while sharing the
// already constructed process and selected real HTTP host with its scenario.
func (f *fixture) mcpCaller(t *testing.T, caller map[string]string) func(map[string]any) map[string]json.RawMessage {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()
	done := make(chan error, 1)
	env := append([]string{}, f.env...)
	if caller != nil {
		env = append(env, "YOU_WORKER_SESSION_ID="+caller["YOU_WORKER_SESSION_ID"], "YOU_WORKER_SESSION_TOKEN="+caller["YOU_WORKER_SESSION_TOKEN"])
	}
	go func() {
		err := f.server.Execute(t, root.Input{Args: []string{"you", "--server", f.baseURL(), "server", "mcp"}, Context: ctx, Env: env, WorkingDirectory: f.dir, Stdin: stdinRead, Stdout: stdoutWrite, Stderr: io.Discard})
		_ = stdinRead.CloseWithError(err)
		_ = stdoutWrite.CloseWithError(err)
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		_ = stdinWrite.Close()
		_ = stdinRead.Close()
		_ = stdoutWrite.Close()
		_ = stdoutRead.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("MCP process: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("MCP process did not join")
		}
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "message-parity", Version: "test"}, nil)
	connectCtx, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	session, err := client.Connect(connectCtx, &mcp.IOTransport{Reader: stdoutRead, Writer: stdinWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return func(args map[string]any) map[string]json.RawMessage {
		t.Helper()
		callCtx, stop := context.WithTimeout(ctx, 30*time.Second)
		defer stop()
		result, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: "you.subagent", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		var envelope map[string]json.RawMessage
		if result.IsError {
			raw, _ := json.Marshal(result.StructuredContent)
			if json.Unmarshal(raw, &envelope) != nil {
				t.Fatal("missing structured MCP error")
			}
			return envelope
		}
		if len(result.Content) != 1 {
			t.Fatal("missing MCP envelope")
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok || json.Unmarshal([]byte(content.Text), &envelope) != nil {
			t.Fatal("malformed MCP envelope")
		}
		return envelope
	}
}

func mcpMessage(t *testing.T, call func(map[string]any) map[string]json.RawMessage, input map[string]any) api.AgentMessage {
	t.Helper()
	result := call(input)
	var response api.AgentMessageSendResponse
	if json.Unmarshal(result["result"], &response) != nil || response.Message.MessageId == "" {
		t.Fatalf("MESSAGE failed: %s", result["error"])
	}
	return response.Message
}

func mcpPage(t *testing.T, call func(map[string]any) map[string]json.RawMessage, input map[string]any) api.AgentMessageListResponse {
	t.Helper()
	result := call(input)
	var response api.AgentMessageListResponse
	if json.Unmarshal(result["result"], &response) != nil || response.Messages == nil {
		t.Fatalf("message inspection failed: %s", result["error"])
	}
	return response
}

func TestT8MCPMessageReplyPagingAndDenial(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), "", "lead", "lane", "sibling")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	sibling := f.invoke(t, "sibling", lead)
	leadCall, laneCall, siblingCall := f.mcpCaller(t, lead), f.mcpCaller(t, lane), f.mcpCaller(t, sibling)
	before := messageFactoryStreams(t, f, "lead", "lane", "sibling")
	request := map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "private message", "requestId": "mcp-ask", "bodySecret": true, "ifEnded": "HOLD", "replyIfEnded": "HOLD"}
	message := mcpMessage(t, laneCall, request)
	replay := f.send(t, map[string]any{"to": map[string]string{"workerSessionId": "lead"}, "body": "private message", "requestId": "mcp-ask", "bodySecret": true, "ifEnded": "HOLD", "replyIfEnded": "HOLD"}, lane)
	cli := f.cliSend(t, lane, "lead", "--request-id", "mcp-ask", "--body", "private message", "--body-secret", "--if-ended", "hold", "--reply-hold")
	if !reflect.DeepEqual(message, replay) || !reflect.DeepEqual(message, cli) || message.Body != "<redacted>" {
		t.Fatalf("MCP/HTTP/CLI durable privacy identity differs: MCP=%+v HTTP=%+v CLI=%+v", message, replay, cli)
	}
	read := assertMCPReplyAndRead(t, f, leadCall, laneCall, message, lane)
	denied := siblingCall(map[string]any{"action": "MESSAGE", "workerSessionId": "lane", "message": "denied", "requestId": "denied"})
	if !strings.Contains(string(denied["error"]), "MESSAGE_NOT_PERMITTED") {
		t.Fatal("sibling send not denied")
	}
	private := mcpPage(t, siblingCall, map[string]any{"action": "READ", "messages": true, "workerSessionId": "lane", "threadId": message.ThreadId})
	if len(private.Messages) != 0 {
		t.Fatal("sibling acquired inbox authority")
	}
	operator := f.mcpCaller(t, nil)
	observed := mcpPage(t, operator, map[string]any{"action": "READ", "messages": true, "workerSessionId": "lane", "threadId": message.ThreadId})
	if !reflect.DeepEqual(observed, read) {
		t.Fatal("operator observation changed facts")
	}
	denied = operator(map[string]any{"action": "MESSAGE", "workerSessionId": "lane", "message": "denied", "requestId": "operator"})
	if !strings.Contains(string(denied["error"]), "WORKER_SESSION_CALLER_INVALID") {
		t.Fatal("operator send admitted")
	}
	if !reflect.DeepEqual(before, messageFactoryStreams(t, f, "lead", "lane", "sibling")) {
		t.Fatal("messaging changed Factory Events")
	}
}

func TestT8MCPExplicitReviveHasStableContinuationAndNoMessage(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), "", "source", "successor")
	f.invoke(t, "source", nil)
	f.endWorker(t, "source")
	call := f.mcpCaller(t, nil)
	request := map[string]any{"action": "REVIVE", "workerSessionId": "source", "message": "continue exact provider", "requestId": "mcp-revive", "successorWorkerSessionId": "successor"}
	first := call(request)
	var admission api.WorkerSessionContinueResponse
	if json.Unmarshal(first["result"], &admission) != nil || !admission.Accepted || admission.SuccessorWorkerSessionId != "successor" || admission.SourceWorkerSessionId != "source" {
		t.Fatalf("revive admission: %s", first["error"])
	}
	second := call(request)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("exact tuple retry changed admission")
	}
	status, raw := f.request(t, http.MethodPost, "/worker-sessions/source/continue", map[string]any{"followUpInput": "continue exact provider", "requestId": "mcp-revive", "successorWorkerSessionId": "successor", "resolveHead": true}, nil)
	if status != 202 {
		t.Fatalf("HTTP replay failed: %d", status)
	}
	var httpAdmission api.WorkerSessionContinueResponse
	if json.Unmarshal(raw, &httpAdmission) != nil || !reflect.DeepEqual(admission, httpAdmission) {
		t.Fatal("revive HTTP lineage differs")
	}
	request["message"] = "changed"
	if failure := call(request); !strings.Contains(string(failure["error"]), "WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT") {
		t.Fatalf("changed tuple not conflict: %s", failure["error"])
	}
	page := readPage(t, f, "/messages", nil)
	if len(page.Messages) != 0 {
		t.Fatal("explicit REVIVE created Agent Message")
	}
	missing := call(map[string]any{"action": "REVIVE", "workerSessionId": "missing", "message": "follow up", "requestId": "missing", "successorWorkerSessionId": "missing-next"})
	if !strings.Contains(string(missing["error"]), "WORKER_SESSION_NOT_FOUND") && !strings.Contains(string(missing["error"]), "NOT_FOUND") {
		t.Fatalf("missing source lost host code: %s", missing["error"])
	}
}

func assertMCPReplyAndRead(t *testing.T, f *fixture, leadCall, laneCall func(map[string]any) map[string]json.RawMessage, message api.AgentMessage, lane map[string]string) api.AgentMessageListResponse {
	t.Helper()
	page := mcpPage(t, leadCall, map[string]any{"action": "LIST", "messages": true, "maxResults": 1, "threadId": message.ThreadId})
	if len(page.Messages) != 1 || page.Messages[0].MessageId != message.MessageId {
		t.Fatal("own inbox lost request")
	}
	reply := mcpMessage(t, leadCall, map[string]any{"action": "MESSAGE", "inReplyTo": message.MessageId, "message": "lead answer", "requestId": "mcp-reply"})
	if reply.ThreadId != message.ThreadId || reply.InReplyTo == nil || *reply.InReplyTo != message.MessageId || reply.IfEnded != api.AgentMessageIfEndedHOLD || reply.To.WorkerSessionId != "lane" {
		t.Fatal("reply lineage or inherited HOLD changed")
	}
	read := mcpPage(t, laneCall, map[string]any{"action": "READ", "messages": true, "workerSessionId": "lane", "threadId": message.ThreadId, "markRead": true})
	if len(read.Messages) != 1 || read.Messages[0].Status != api.AgentMessageDeliveryStatusREAD {
		t.Fatal("recipient mark-read lost")
	}
	httpRead := readPage(t, f, "/messages?toWorkerSessionId=lane&threadId="+message.ThreadId, lane)
	if !reflect.DeepEqual(read, httpRead) {
		t.Fatal("thread read differs from HTTP")
	}
	return read
}

func TestT8MCPDefaultRevivalRequiresProviderProgress(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name, reason string
		result       *platformprocess.CommandResult
	}{
		{"progress", "", nil},
		{"rejected", "REVIVE_UNSUPPORTED", &platformprocess.CommandResult{ExitCode: 1, Stderr: []byte("No rollout found for thread id controlled-thread\n")}},
		{"failed", "REVIVE_FAILED", &platformprocess.CommandResult{ExitCode: 1, Stderr: []byte("controlled failure\n")}},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			progress := make(chan struct{})
			revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{}), progress: progress, result: cell.result}
			f := startHostRunner(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{}, nil, revival, "lead", "lane")
			lead := f.invoke(t, "lead", nil)
			lane := f.invoke(t, "lane", lead)
			f.endWorker(t, "lead")
			call := f.mcpCaller(t, lane)
			request := map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "normalized continuation input", "requestId": "provider-confirmation"}
			pending := mcpMessage(t, call, request)
			assertPendingProviderResponse(t, pending)
			var successor map[string]string
			select {
			case successor = <-revival.started:
			case <-time.After(15 * time.Second):
				t.Fatal("successor did not start")
			}
			command := <-revival.requests
			if string(command.Stdin) != pending.Body || !strings.Contains(strings.Join(command.Args, " "), "resume controlled-thread") {
				t.Fatal("provider continuation changed")
			}
			assertPendingCLIParity(t, f, lane, pending)
			if cell.reason == "" {
				close(progress)
			} else {
				close(revival.release)
			}
			settled := f.awaitDeliveryOutcome(t, lane, pending)
			page := mcpPage(t, call, map[string]any{"action": "READ", "messages": true, "workerSessionId": "lead", "threadId": pending.ThreadId})
			if len(page.Messages) != 1 || !reflect.DeepEqual(page.Messages[0], settled) {
				t.Fatal("MCP delivery facts differ from HTTP")
			}
			assertMCPProviderOutcome(t, settled, cell.reason, successor, revival)
			if replay := mcpMessage(t, call, request); !reflect.DeepEqual(replay, settled) {
				t.Fatal("delivery retry changed identity")
			}
			f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
		})
	}
}

func TestT8MCPConcurrentSendFinitePagingAndRestart(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), t.TempDir()
	f := startHost(t, home, dir, "", "lead", "lane", "lane-next")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	call, leadCall := f.mcpCaller(t, lane), f.mcpCaller(t, lead)
	request := map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "durable ask", "requestId": "concurrent", "ifEnded": "HOLD", "replyIfEnded": "HOLD"}
	result := make(chan api.AgentMessage, 2)
	var joined sync.WaitGroup
	for range 2 {
		joined.Add(1)
		go func() { defer joined.Done(); result <- mcpMessage(t, call, request) }()
	}
	joined.Wait()
	first, second := <-result, <-result
	if !reflect.DeepEqual(first, second) {
		t.Fatal("concurrent same-key send differed")
	}
	mcpMessage(t, call, map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "next", "requestId": "next", "ifEnded": "HOLD"})
	filter := map[string]any{"action": "LIST", "messages": true, "maxResults": 1, "fromWorkerSessionId": "lane", "status": []string{"QUEUED"}}
	page := mcpPage(t, leadCall, filter)
	if len(page.Messages) != 1 || page.NextToken == nil || page.Messages[0].MessageId != first.MessageId {
		t.Fatal("snapshot page missing")
	}
	mcpMessage(t, call, map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "post snapshot", "requestId": "post", "ifEnded": "HOLD"})
	filter["nextToken"] = *page.NextToken
	next := mcpPage(t, leadCall, filter)
	retry := mcpPage(t, leadCall, filter)
	if len(next.Messages) != 1 || next.NextToken != nil || next.Messages[0].MessageId == first.MessageId || !reflect.DeepEqual(next, retry) {
		t.Fatal("snapshot cursor skipped, duplicated, or included post-snapshot admission")
	}
	filter["threadId"] = first.ThreadId
	if failure := leadCall(filter); !strings.Contains(string(failure["error"]), "MESSAGE_CURSOR_INVALID") {
		t.Fatal("changed cursor filter accepted")
	}
	f.endWorker(t, "lane")
	f.endWorker(t, "lead")
	f.server.Close(t)
	restored := startHost(t, home, dir, "", "lane-next")
	observed := mcpPage(t, restored.mcpCaller(t, nil), map[string]any{"action": "READ", "messages": true, "workerSessionId": "lead", "threadId": first.ThreadId})
	if len(observed.Messages) != 1 || !reflect.DeepEqual(observed.Messages[0], first) {
		t.Fatal("restart lost durable message")
	}
	stale := restored.mcpCaller(t, lane)(map[string]any{"action": "READ", "messages": true, "workerSessionId": "lead", "threadId": first.ThreadId})
	if !strings.Contains(string(stale["error"]), "MESSAGE_NOT_PERMITTED") {
		t.Fatal("old execution token authorized after restart")
	}
	fresh := restored.continueCaller(t, "lane", "lane-next")
	replay := mcpMessage(t, restored.mcpCaller(t, fresh), request)
	if !reflect.DeepEqual(replay, first) {
		t.Fatal("fresh exact-chain retry admitted twice")
	}
}

func TestT8MCPDisabledAndInterruptHaveNoMessageEffect(t *testing.T) {
	t.Parallel()
	for _, disabled := range []bool{false, true} {
		name, settings, code := "interrupt", "", "MESSAGE_INTERRUPT_UNSUPPORTED"
		if disabled {
			name, settings, code = "disabled", `{"messaging":{"enabled":false}}`, "MESSAGING_DISABLED"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startHost(t, t.TempDir(), t.TempDir(), settings, "lead", "lane")
			lead := f.invoke(t, "lead", nil)
			lane := f.invoke(t, "lane", lead)
			before := messageFactoryStreams(t, f, "lead", "lane")
			failure := f.mcpCaller(t, lane)(map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "unsupported delivery", "requestId": "unsupported", "delivery": "INTERRUPT"})
			if !strings.Contains(string(failure["error"]), code) {
				t.Fatalf("lost host typed refusal: %s", failure["error"])
			}
			if !reflect.DeepEqual(before, messageFactoryStreams(t, f, "lead", "lane")) {
				t.Fatal("refused send changed Factory history")
			}
			if !disabled && len(readPage(t, f, "/messages", nil).Messages) != 0 {
				t.Fatal("unsupported interrupt persisted a message")
			}
		})
	}
}

func assertMCPProviderOutcome(t *testing.T, settled api.AgentMessage, reason string, successor map[string]string, revival *workerGate) {
	t.Helper()
	if reason == "" {
		if settled.Status != api.AgentMessageDeliveryStatusDELIVEREDREVIVE || settled.RevivedWorkerSessionId == nil || *settled.RevivedWorkerSessionId != successor["YOU_WORKER_SESSION_ID"] {
			t.Fatal("confirmed progress lost delivery successor")
		}
		close(revival.release)
	} else if settled.Status != api.AgentMessageDeliveryStatusQUEUED || settled.Reason == nil || *settled.Reason != reason || settled.RevivedWorkerSessionId != nil {
		t.Fatal("non-progress guessed delivery")
	}
}

func TestT8MCPEndedReplyRetainsHoldForFreshVisit(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{}, nil, revival, "lead", "lane", "lane-next")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	leadCall := f.mcpCaller(t, lead)
	ask := mcpMessage(t, f.mcpCaller(t, lane), map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "answer on a later visit", "requestId": "ended-ask", "ifEnded": "HOLD", "replyIfEnded": "HOLD"})
	f.endWorker(t, "lane")
	reply := mcpMessage(t, leadCall, map[string]any{"action": "MESSAGE", "inReplyTo": ask.MessageId, "message": "retained answer", "requestId": "ended-reply"})
	if reply.Status != "QUEUED" || reply.IfEnded != api.AgentMessageIfEndedHOLD || reply.To.WorkerSessionId != "lane" || reply.ThreadId != ask.ThreadId || reply.InReplyTo == nil || *reply.InReplyTo != ask.MessageId || reply.RevivedWorkerSessionId != nil {
		t.Fatal("ended reply changed recipient, thread, or inherited HOLD")
	}
	select {
	case <-revival.requests:
		t.Fatal("HOLD reply started a provider")
	default:
	}
	parent := mcpPage(t, leadCall, map[string]any{"action": "READ", "messages": true, "workerSessionId": "lead", "threadId": ask.ThreadId, "markRead": true})
	if len(parent.Messages) != 1 || parent.Messages[0].Status != "REPLIED" {
		t.Fatal("mark-read regressed the replied parent")
	}
	fresh := f.continueCaller(t, "lane", "lane-next")
	page := mcpPage(t, f.mcpCaller(t, fresh), map[string]any{"action": "LIST", "messages": true, "threadId": ask.ThreadId})
	if len(page.Messages) != 1 || !reflect.DeepEqual(page.Messages[0], reply) {
		t.Fatal("fresh same-Work visit lost the held reply")
	}
	httpPage := readPage(t, f, "/messages?toMe=true&threadId="+ask.ThreadId, fresh)
	if !reflect.DeepEqual(page, httpPage) {
		t.Fatal("later-visit MCP inbox differs from HTTP")
	}
}

func TestT8MCPExpiryDoesNotRegressOnMarkRead(t *testing.T) {
	t.Parallel()
	clock := platformclock.NewDeterministic(time.Now().UTC(), time.Second)
	// Control message timestamps only; lifecycle readiness uses real timers.
	f := startHostEffects(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{Clock: struct{ platformclock.Source }{clock}}, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	call := f.mcpCaller(t, lead)
	ask := mcpMessage(t, f.mcpCaller(t, lane), map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "expires exactly", "requestId": "expiry", "expiresInSeconds": 60, "ifEnded": "HOLD"})
	clock.SetTick(59)
	page := mcpPage(t, call, map[string]any{"action": "LIST", "messages": true, "threadId": ask.ThreadId})
	if len(page.Messages) != 1 || page.Messages[0].Status != "QUEUED" {
		t.Fatal("MCP message expired before its deadline")
	}
	clock.SetTick(60)
	page = mcpPage(t, call, map[string]any{"action": "READ", "messages": true, "workerSessionId": "lead", "threadId": ask.ThreadId, "markRead": true})
	if len(page.Messages) != 1 || page.Messages[0].Status != "EXPIRED" || !reflect.DeepEqual(page, readPage(t, f, "/messages?toMe=true&threadId="+ask.ThreadId, lead)) {
		t.Fatal("MCP expiry or mark-read differs from HTTP")
	}
	failure := call(map[string]any{"action": "MESSAGE", "inReplyTo": ask.MessageId, "message": "too late", "requestId": "expired-reply"})
	if !strings.Contains(string(failure["error"]), "MESSAGE_INVALID_REQUEST") {
		t.Fatal("MCP admitted a reply to an expired message")
	}
}

func TestT8MCPMissingReferencePreservesQueuedAsk(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{}, nil, revival, "lead", "lane")
	f.runner.workers["lead"].result = &platformprocess.CommandResult{Stdout: []byte("{\"type\":\"turn.completed\"}\n")}
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	call := f.mcpCaller(t, lane)
	input := map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "retain unsupported ask", "requestId": "missing-reference"}
	ask := mcpMessage(t, call, input)
	if ask.Status != "QUEUED" || ask.Reason == nil || *ask.Reason != "REVIVE_UNSUPPORTED" || ask.RevivedWorkerSessionId != nil {
		t.Fatal("missing reference claimed delivery")
	}
	if replay := mcpMessage(t, call, input); !reflect.DeepEqual(ask, replay) {
		t.Fatal("unsupported retry changed identity")
	}
	cli := f.cliSend(t, lane, "lead", "--request-id", "missing-reference", "--body", "retain unsupported ask")
	page := mcpPage(t, call, map[string]any{"action": "READ", "messages": true, "workerSessionId": "lead", "threadId": ask.ThreadId})
	if !reflect.DeepEqual(ask, cli) || len(page.Messages) != 1 || !reflect.DeepEqual(page.Messages[0], ask) {
		t.Fatal("MCP unsupported outcome differs from CLI or durable inspection")
	}
	select {
	case <-revival.requests:
		t.Fatal("missing reference started a provider")
	default:
	}
	if !reflect.DeepEqual(before, messageFactoryStreams(t, f, "lead", "lane")) {
		t.Fatal("unsupported revival changed Factory Events")
	}
}

func TestT8MCPQuarantinedStorePreservesTypedRefusal(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct{ name, settings, journal, code string }{
		{"invalid-settings", `{"messaging":{"limits":{"maxBodyBytes":0}}}`, "", "MESSAGE_STORE_UNAVAILABLE"},
		{"corrupt-store", "", "controlled invalid journal\n", "MESSAGE_STORE_CORRUPT"},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			if cell.journal != "" {
				writeJournal(t, filepath.Join(home, ".you-agent-factory", "messages", "messages.jsonl"), cell.journal)
			}
			f := startHost(t, home, t.TempDir(), cell.settings, "lead", "lane")
			lead := f.invoke(t, "lead", nil)
			lane := f.invoke(t, "lane", lead)
			call := f.mcpCaller(t, lane)
			before := messageFactoryStreams(t, f, "lead", "lane")
			for _, input := range []map[string]any{
				{"action": "MESSAGE", "workerSessionId": "lead", "message": "private refused body", "requestId": "quarantine"},
				{"action": "LIST", "messages": true},
				{"action": "READ", "messages": true, "workerSessionId": "lead"},
			} {
				failure := call(input)
				diagnostic := string(failure["error"])
				if !strings.Contains(diagnostic, cell.code) || strings.Contains(diagnostic, "private refused body") || strings.Contains(diagnostic, lane["YOU_WORKER_SESSION_TOKEN"]) {
					t.Fatal("MCP quarantine lost safe typed refusal")
				}
			}
			assertError(t, f, http.MethodGet, "/messages", nil, lane, http.StatusServiceUnavailable, cell.code)
			if !reflect.DeepEqual(before, messageFactoryStreams(t, f, "lead", "lane")) {
				t.Fatal("quarantined messaging changed Factory Events")
			}
		})
	}
}

func TestT8MCPReviveLimitRetainsReadableQueuedAsk(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	close(revival.release)
	f := startHostRunner(t, t.TempDir(), t.TempDir(), `{"messaging":{"limits":{"revivesPerChainPerHour":1}}}`, serviceedges.Edges{}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	call := f.mcpCaller(t, lane)
	first := mcpMessage(t, call, map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "first allowed resume", "requestId": "first-revive"})
	first = f.awaitDeliveryOutcome(t, lane, first)
	head := assertReviveAdmission(t, f, revival, first, lead)
	f.waitEndedWorker(t, head["YOU_WORKER_SESSION_ID"])
	input := map[string]any{"action": "MESSAGE", "workerSessionId": "lead", "message": "retain limited ask", "requestId": "limited"}
	limited := mcpMessage(t, call, input)
	if limited.Status != "QUEUED" || limited.Reason == nil || *limited.Reason != "REVIVE_LIMIT" || limited.RevivedWorkerSessionId != nil {
		t.Fatal("MCP revive limit guessed delivery")
	}
	page := mcpPage(t, call, map[string]any{"action": "READ", "messages": true, "workerSessionId": "lead", "threadId": limited.ThreadId})
	cli := f.cliSend(t, lane, "lead", "--request-id", "limited", "--body", "retain limited ask")
	if len(page.Messages) != 1 || !reflect.DeepEqual(page.Messages[0], limited) || !reflect.DeepEqual(cli, limited) || !reflect.DeepEqual(mcpMessage(t, call, input), limited) {
		t.Fatal("limited MCP read/retry differs from durable CLI outcome")
	}
	select {
	case <-revival.requests:
		t.Fatal("limited request started another provider")
	default:
	}
	if !reflect.DeepEqual(before, messageFactoryStreams(t, f, "lead", "lane")) {
		t.Fatal("revive quota changed Factory Events")
	}
}
