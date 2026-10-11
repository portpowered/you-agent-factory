package acceptance

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The two direct executions share one production host with immutable command
// routes. Progress is released only after all HTTP observers receive headers;
// observer cancellation must leave both commands and the peer observer alive.
func TestWorkerEventsDirectIdleFollowAndObserverDetach(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	root, dir, peerDir := t.TempDir(), t.TempDir(), t.TempDir()
	hostDir, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	runner, peer := newWorkerEventsRunner("target"), newWorkerEventsRunner("peer")
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{
		{workingDirectory: dir, runner: runner}, {workingDirectory: peerDir, runner: peer},
	}}
	host := startContinuationRestartHost(t, root, hostDir, home, route)
	// Cleanup releases only these two effects before the shared host is joined.
	defer runner.releaseProgress()
	defer peer.releaseProgress()
	invokeWorkerEventsDirect(t, host, home, dir, "events-target")
	invokeWorkerEventsDirect(t, host, home, peerDir, "events-peer")
	t19AwaitSignal(t, ctx, runner.ready, "target provider idle")
	t19AwaitSignal(t, ctx, peer.ready, "peer provider idle")
	endpoint := host.baseURL + "/worker-sessions/events-target/events"
	first := openWorkerEventsDirect(t, ctx, endpoint)
	defer first.Body.Close()
	second := openWorkerEventsDirect(t, ctx, endpoint)
	defer second.Body.Close()
	observerCtx, detach := context.WithCancel(ctx)
	detached := openWorkerEventsDirect(t, observerCtx, endpoint)
	defer detached.Body.Close()
	detach()
	if _, err := drainWorkerEventsDirect(detached); !errors.Is(err, context.Canceled) {
		t.Fatalf("detached observer: %v, want context cancellation", err)
	}
	assertWorkerEventsCommandIdle(t, runner)
	assertWorkerEventsCommandIdle(t, peer)
	assertWorkerEventsDirectDenial(t, ctx, host.baseURL+"/worker-sessions/unknown-worker/events", http.StatusNotFound, api.ErrorResponseCodeNOTFOUND)
	assertWorkerEventsDirectDenial(t, ctx, host.baseURL+"/factory-sessions/foreign/worker-sessions/events-target/events", http.StatusNotFound, api.ErrorResponseCodeNOTFOUND)
	assertWorkerEventsDirectDenial(t, ctx, endpoint+"?after_position=9223372036854775807", http.StatusBadRequest, api.ErrorResponseCodeWORKERSESSIONEVENTCURSORFUTURE)
	cliFollow, cliWriter := startWorkerEventsDirectCLI(t, ctx, host, home, dir)
	logsCtx, detachLogs := context.WithCancel(ctx)
	detachedLogs, _ := startWorkerEventsDirectCLI(t, logsCtx, host, home, dir, "read", "--worker-session-id", "events-target", "--view", "logs", "--follow")
	detachLogs()
	select {
	case <-detachedLogs.Done():
	case <-ctx.Done():
		t.Fatal("CLI logs observer did not detach")
	}
	assertWorkerEventsCommandIdle(t, runner)
	assertWorkerEventsCommandIdle(t, peer)
	logsFollow, logsWriter := startWorkerEventsDirectCLI(t, ctx, host, home, dir, "read", "--worker-session-id", "events-target", "--view", "logs", "--follow")
	runner.releaseProgress()
	frames, err := drainWorkerEventsDirect(first)
	if err != nil {
		t.Fatal(err)
	}
	other, err := drainWorkerEventsDirect(second)
	if err != nil {
		t.Fatal(err)
	}
	records := assertWorkerEventsDirectRecords(t, frames, "events-target")
	otherRecords := assertWorkerEventsDirectRecords(t, other, "events-target")
	if len(records) != len(otherRecords) {
		t.Fatalf("peer observer lost records: %d / %d", len(records), len(otherRecords))
	}
	for i := range records {
		if !reflect.DeepEqual(records[i].Event, otherRecords[i].Event) {
			t.Fatal("observers disagree on captured records")
		}
	}
	assertWorkerEventsCommandIdle(t, peer)
	assertWorkerEventsDirectReplay(t, ctx, endpoint, records)
	assertWorkerEventsCLIComplete(t, ctx, cliFollow, cliWriter)
	assertWorkerEventsCLIComplete(t, ctx, logsFollow, logsWriter)
	// The existing finite CLI consumer must terminate without waiting for peer.
	output := summaryRestartCLI(t, host, home, dir, "stream", "--worker-session-id", "events-target", "--replay-only")
	if !strings.Contains(output, "events-later-target") {
		t.Fatal("finite CLI stream omitted captured provider progress")
	}
	assertWorkerEventsFiniteMCP(t, ctx, host, home, dir, endpoint)
	assertWorkerEventsCommandIdle(t, peer)
	peer.releaseProgress()
	t19AwaitSignal(t, ctx, peer.joined, "peer provider joined")
}

// The existing finite MCP view must match public SSE frames and its bounded
// truncation meaning while another provider execution remains idle/alive.
func assertWorkerEventsFiniteMCP(t *testing.T, ctx context.Context, host invokeContinueStartedProcess, home, dir, endpoint string) {
	t.Helper()
	frames := readWorkerEventsFinite(t, ctx, endpoint+"?replayOnly=true")
	mcpCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	input := support.FakeInputs(mcpCtx, []string{"you", "--server", host.baseURL, "server", "mcp", "--project-root", dir})
	input.Input.Env, input.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()
	input.Input.Stdin, input.Input.Stdout = stdinRead, stdoutWrite
	done := make(chan error, 1)
	go func() {
		err := host.process.Execute(input.Input)
		_ = stdinRead.CloseWithError(err)
		_ = stdoutWrite.CloseWithError(err)
		done <- err
	}()
	defer func() {
		cancel()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("finite MCP process: %v", err)
			}
		case <-ctx.Done():
			t.Error("finite MCP process did not join")
		}
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "worker-events-finite", Version: "test"}, nil)
	session, err := client.Connect(mcpCtx, &mcp.IOTransport{Reader: stdoutRead, Writer: stdinWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, limit := range []int{1, 1000} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{
			"action": "READ", "workerSessionId": "events-target", "view": "events", "limit": limit,
		}})
		if err != nil || result.IsError {
			t.Fatalf("finite MCP events: %v %+v", err, result)
		}
		var envelope struct {
			Result struct {
				Events struct {
					Events    []api.WorkerSessionEvent `json:"events"`
					Truncated bool                     `json:"truncated"`
				} `json:"events"`
			} `json:"result"`
		}
		if len(result.Content) != 1 {
			t.Fatalf("finite MCP content count: %d", len(result.Content))
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok || json.Unmarshal([]byte(content.Text), &envelope) != nil {
			t.Fatalf("finite MCP envelope: %+v", result.Content)
		}
		want := frames
		if limit == 1 {
			want = frames[:1]
		}
		if !reflect.DeepEqual(envelope.Result.Events.Events, want) || envelope.Result.Events.Truncated != (limit == 1) {
			t.Fatalf("finite MCP changed frames/truncation: %s", content.Text)
		}
	}
}

func assertWorkerEventsCLIComplete(t *testing.T, ctx context.Context, command *support.ProcessCommand, writer *s8SignalWriter) {
	t.Helper()
	select {
	case <-command.Done():
	case <-ctx.Done():
		t.Fatal("CLI follow did not finish at terminal EOF")
	}
	if command.Err() != nil || !strings.Contains(string(writer.bytes()), "events-later-target") {
		t.Fatalf("CLI follow lost later progress: %v %s", command.Err(), writer.bytes())
	}
}

// A one-record page forces retained replay to cross a page boundary. The
// provider advances only after both the finite snapshot and cursor follower
// have established, so later activity cannot change the finite replay head.
func TestWorkerEventsPagedFixedHeadAndReconnect(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	root, dir := t.TempDir(), t.TempDir()
	hostDir, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := newWorkerEventsRunner("target")
	runner.initial = directCodexSessionOutput("events-thread-target", "events-prefix-target")
	defer runner.releaseProgress()
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
	host := startContinuationRestartHost(t, root, hostDir, home, route)
	invokeWorkerEventsDirect(t, host, home, dir, "events-target")
	t19AwaitSignal(t, ctx, runner.ready, "retained provider prefix")
	endpoint := host.baseURL + "/worker-sessions/events-target/events"
	baseline := readWorkerEventsFinite(t, ctx, endpoint+"?replayOnly=true&limit=1")
	prefix := assertWorkerEventsActivePrefix(t, baseline)
	head := prefix[len(prefix)-1].Event.Position
	fixed := openWorkerEventsDirect(t, ctx, endpoint+"?replayOnly=true&limit=1")
	defer fixed.Body.Close()
	follow := openWorkerEventsDirect(t, ctx, endpoint+"?after_position="+strconv.FormatInt(head, 10))
	defer follow.Body.Close()
	runner.releaseProgress()
	frames, err := drainWorkerEventsDirect(fixed)
	if err != nil {
		t.Fatalf("fixed replay EOF: %v", err)
	}
	got := assertWorkerEventsActivePrefix(t, frames)
	if !reflect.DeepEqual(got, prefix) {
		t.Fatal("head advancement changed the pinned prefix, cursors or stored times")
	}
	suffix, err := drainWorkerEventsDirect(follow)
	if err != nil {
		t.Fatalf("reconnected follower EOF: %v", err)
	}
	var combined []api.WorkerSessionEvent
	combined = append(combined, prefix...)
	for _, frame := range suffix {
		if frame.Event.Position > 0 && frame.Event.Position <= head {
			t.Fatal("reconnect duplicated an acknowledged prefix record")
		}
		combined = append(combined, frame)
	}
	records := assertWorkerEventsDirectRecords(t, combined, "events-target")
	ended := readWorkerEventsFinite(t, ctx, endpoint+"?replayOnly=true&limit=1")
	endedRecords := assertWorkerEventsDirectRecords(t, ended, "events-target")
	if len(endedRecords) != len(records) {
		t.Fatal("reconnect missed records at the retained/live boundary")
	}
	for i := range records {
		if !reflect.DeepEqual(records[i].Event.Payload, endedRecords[i].Event.Payload) || !reflect.DeepEqual(records[i].Event.Cursor, endedRecords[i].Event.Cursor) {
			t.Fatal("reconnect changed captured content or cursors")
		}
	}
}

func readWorkerEventsFinite(t *testing.T, ctx context.Context, endpoint string) []api.WorkerSessionEvent {
	t.Helper()
	response := openWorkerEventsDirect(t, ctx, endpoint)
	defer response.Body.Close()
	frames, err := drainWorkerEventsDirect(response)
	if err != nil {
		t.Fatalf("finite replay EOF: %v", err)
	}
	return frames
}

func assertWorkerEventsActivePrefix(t *testing.T, frames []api.WorkerSessionEvent) []api.WorkerSessionEvent {
	t.Helper()
	var records []api.WorkerSessionEvent
	var summary *api.WorkerSessionReplaySummary
	for _, frame := range frames {
		if frame.WorkerSessionId != "events-target" || frame.FactorySessionId != nil || len(frame.WorkIds) != 0 || frame.Delivery == api.WorkerSessionEventDeliverySourceFailure {
			t.Fatalf("active prefix identity/failure: %+v", frame)
		}
		if frame.ReplaySummary != nil {
			summary = frame.ReplaySummary
		}
		if frame.Event.Position > 0 {
			if frame.Event.Position != int64(len(records)+1) {
				t.Fatal("paged prefix omitted or duplicated a position")
			}
			records = append(records, frame)
		}
	}
	if len(records) < 2 || summary == nil || summary.Complete || summary.EventsEmitted != int64(len(records)) {
		t.Fatalf("active paged prefix/completion: records=%d summary=%+v", len(records), summary)
	}
	return records
}

// Canonical IDs select only the host's private profile. A foreign ID never
// establishes SSE, and each host's retained replay contains only its output.
func TestWorkerEventsPrivateProfileDenial(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	var profiles []recordingPrivateProfile
	for _, id := range []string{"events-private-alpha", "events-private-beta"} {
		root, dir := t.TempDir(), t.TempDir()
		hostDir, home, err := prepareInvokeContinuePackageRoot(t, root)
		if err != nil {
			t.Fatal(err)
		}
		marker := id + "-content"
		runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: directCodexSessionOutput(id+"-thread", marker)})
		route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
		host := startContinuationRestartHost(t, root, hostDir, home, route)
		path := filepath.Join(dir, "execution.json")
		writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt", workingDirectory: dir, userMessage: id})
		summaryRestartCLI(t, host, home, dir, "invoke", "--execution", path)
		profiles = append(profiles, recordingPrivateProfile{host: host, home: home, dir: dir, id: id, marker: marker, runner: runner})
	}
	for i, profile := range profiles {
		foreign := profiles[1-i]
		assertWorkerEventsDirectDenial(t, ctx, profile.host.baseURL+"/worker-sessions/"+foreign.id+"/events", http.StatusNotFound, api.ErrorResponseCodeNOTFOUND)
		response := openWorkerEventsDirect(t, ctx, profile.host.baseURL+"/worker-sessions/"+profile.id+"/events?replayOnly=true")
		frames, err := drainWorkerEventsDirect(response)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(frames)
		if err != nil || !strings.Contains(string(body), profile.marker) || strings.Contains(string(body), foreign.marker) || strings.Contains(string(body), foreign.id) {
			t.Fatalf("private profile replay changed or leaked: %s (%v)", body, err)
		}
		if profile.runner.CallCount() != 1 {
			t.Fatal("event reads admitted another provider execution")
		}
	}
}

func startWorkerEventsDirectCLI(t *testing.T, ctx context.Context, host invokeContinueStartedProcess, home, dir string, args ...string) (*support.ProcessCommand, *s8SignalWriter) {
	t.Helper()
	if len(args) == 0 {
		args = []string{"stream", "--worker-session-id", "events-target", "--follow"}
	}
	input := support.FakeInputs(ctx, append([]string{"you", "--remote", "--server", host.baseURL, "--json", "worker-sessions"}, args...))
	input.Input.Env, input.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	writer := newS8SignalWriter("events-target", input.Stderr)
	input.Input.Stdout = writer
	command := support.StartProcessCommand(t, host.process, input.Input)
	t.Cleanup(func() { command.Stop(t) })
	writer.waitWorkerSessionFrame(t, "events-target")
	return command, writer
}

func invokeWorkerEventsDirect(t *testing.T, host invokeContinueStartedProcess, home, dir, id string) {
	t.Helper()
	path := filepath.Join(dir, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt", workingDirectory: dir, userMessage: id})
	summaryRestartCLI(t, host, home, dir, "invoke", "--execution", path, "--async")
}

func openWorkerEventsDirect(t *testing.T, ctx context.Context, endpoint string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("Cache-Control") != "no-cache" || response.Header.Get("X-Accel-Buffering") != "no" {
		response.Body.Close()
		t.Fatalf("unsupported SSE response: %d %v", response.StatusCode, response.Header)
	}
	return response
}

func drainWorkerEventsDirect(response *http.Response) ([]api.WorkerSessionEvent, error) {
	var frames []api.WorkerSessionEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if !strings.HasPrefix(scanner.Text(), "data:") {
			continue
		}
		var frame api.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data:")), &frame); err != nil {
			return nil, err
		}
		frames = append(frames, frame)
	}
	return frames, scanner.Err()
}

func assertWorkerEventsDirectRecords(t *testing.T, frames []api.WorkerSessionEvent, id string) []api.WorkerSessionEvent {
	t.Helper()
	var records []api.WorkerSessionEvent
	terminal, progress := false, false
	for _, frame := range frames {
		if frame.WorkerSessionId != id || frame.FactorySessionId != nil || len(frame.WorkIds) != 0 || frame.Delivery == api.WorkerSessionEventDeliverySourceFailure {
			t.Fatalf("direct stream identity/failure: %+v", frame)
		}
		terminal = terminal || frame.Delivery == api.WorkerSessionEventDeliveryTerminal || frame.Delivery == api.WorkerSessionEventDeliveryTerminalReplay
		if frame.Event.Position <= 0 {
			continue
		}
		if frame.Event.Position != int64(len(records)+1) || frame.Event.Cursor.Position != frame.Event.Position || frame.Event.Cursor.WorkerSessionId == nil || *frame.Event.Cursor.WorkerSessionId != id {
			t.Fatalf("ordered captured cursor/time: %+v", frame.Event)
		}
		payload, _ := json.Marshal(frame.Event.Payload)
		progress = progress || strings.Contains(string(payload), "events-later-target")
		if strings.Contains(string(payload), "events-later-peer") {
			t.Fatal("peer content leaked into target stream")
		}
		records = append(records, frame)
	}
	if !terminal || !progress {
		t.Fatal("stream reached EOF without later progress and terminal")
	}
	return records
}

func assertWorkerEventsDirectReplay(t *testing.T, ctx context.Context, endpoint string, records []api.WorkerSessionEvent) {
	t.Helper()
	var retained []api.WorkerSessionEvent
	for _, query := range []string{"", "?replayOnly=true", "?replayOnly=true", "?replayOnly=true&after_position=" + strconv.FormatInt(records[len(records)-1].Event.Position, 10)} {
		response := openWorkerEventsDirect(t, ctx, endpoint+query)
		frames, err := drainWorkerEventsDirect(response)
		response.Body.Close()
		if err != nil {
			t.Fatalf("finite replay did not reach EOF: %v", err)
		}
		if strings.Contains(query, "after_position") {
			for _, frame := range frames {
				if frame.Event.Position > 0 {
					t.Fatal("acknowledged-head replay duplicated a record")
				}
			}
		} else {
			got := assertWorkerEventsDirectRecords(t, frames, "events-target")
			if len(got) != len(records) {
				t.Fatal("finite replay changed retained prefix")
			}
			for i := range got {
				if !reflect.DeepEqual(got[i].Event.Payload, records[i].Event.Payload) || !reflect.DeepEqual(got[i].Event.Cursor, records[i].Event.Cursor) {
					t.Fatal("finite replay changed provider content or cursor")
				}
				if retained != nil && !reflect.DeepEqual(got[i].Event, retained[i].Event) {
					t.Fatal("repeated replay changed stored records or timestamps")
				}
			}
			// Default retained delivery can omit commit metadata. Compare stored
			// timestamp fidelity between the two explicit finite replay reads.
			if query == "?replayOnly=true" {
				retained = got
			}
		}
	}
}

func assertWorkerEventsDirectDenial(t *testing.T, ctx context.Context, endpoint string, status int, code api.ErrorResponseCode) {
	t.Helper()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var failure api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil || response.StatusCode != status || failure.Code != code || response.Header.Get("Content-Type") == "text/event-stream" {
		t.Fatalf("pre-stream typed denial: %d %+v %v", response.StatusCode, failure, err)
	}
}

type workerEventsRunner struct {
	id                     string
	initial                []byte
	ready, release, joined chan struct{}
}

func newWorkerEventsRunner(id string) *workerEventsRunner {
	return &workerEventsRunner{id: id, ready: make(chan struct{}), release: make(chan struct{}), joined: make(chan struct{})}
}

func (runner *workerEventsRunner) releaseProgress() {
	select {
	case <-runner.release:
	default:
		close(runner.release)
	}
}

func (runner *workerEventsRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner *workerEventsRunner) RunStreaming(ctx context.Context, _ platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	defer close(runner.joined)
	emitWSRFT015CodexOutput(observe, []byte(fmt.Sprintf("{\"type\":\"thread.started\",\"thread_id\":%q}\n", "events-thread-"+runner.id)))
	emitWSRFT015CodexOutput(observe, runner.initial)
	close(runner.ready)
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-runner.release:
		emitWSRFT015CodexOutput(observe, directCodexSessionOutput("events-thread-"+runner.id, "events-later-"+runner.id))
		return platformprocess.CommandResult{}, nil
	}
}

func assertWorkerEventsCommandIdle(t *testing.T, runner *workerEventsRunner) {
	t.Helper()
	select {
	case <-runner.joined:
		t.Fatal("observer request ended an unreleased provider command")
	default:
	}
}
