package customer_journeys_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Reuse the scenario's production root for the selected-host MCP adapter.
// Protocol pipes are controlled edges; no executable or provider is launched.
func archivedReplayMCP(t *testing.T, server *support.FunctionalAPIServer) (*mcp.ClientSession, context.Context, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()
	inputs := support.FakeInputs(ctx, []string{"you", "--server", server.URL(), "server", "mcp"})
	inputs.Input.Stdin, inputs.Input.Stdout = stdinRead, stdoutWrite
	done := make(chan error, 1)
	go func() {
		err := server.Execute(t, inputs.Input)
		_ = stdinRead.CloseWithError(err)
		_ = stdoutWrite.CloseWithError(err)
		done <- err
	}()
	var connection *mcp.ClientSession
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			defer cancel()
			if connection != nil {
				_ = connection.Close()
			}
			_ = stdinWrite.Close()
			_ = stdinRead.Close()
			_ = stdoutWrite.Close()
			_ = stdoutRead.Close()
			select {
			case err := <-done:
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, io.EOF) {
					t.Errorf("MCP shutdown: %v", err)
				}
			case <-time.After(30 * time.Second):
				t.Error("MCP shutdown did not join")
			}
		})
	}
	t.Cleanup(cleanup)
	client := mcp.NewClient(&mcp.Implementation{Name: "archive-replay", Version: "test"}, nil)
	var err error
	connection, err = client.Connect(ctx, &mcp.IOTransport{Reader: stdoutRead, Writer: stdinWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return connection, ctx, cleanup
}

func assertArchivedCapturedMCPReplay(t *testing.T, server *support.FunctionalAPIServer, logs factoryapi.WorkerSessionLogPage, complete bool) {
	t.Helper()
	connection, ctx, cleanup := archivedReplayMCP(t, server)
	defer cleanup()
	result, err := connection.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "READ", "view": "events", "workerSessionId": logs.WorkerSessionId, "limit": 1000}})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("MCP replay: %+v %v", result, err)
	}
	var envelope struct {
		Result struct {
			Events struct {
				Events    []factoryapi.WorkerSessionEvent `json:"events"`
				Truncated bool                            `json:"truncated"`
			} `json:"events"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
		t.Fatal(err)
	}
	frames := envelope.Result.Events.Events
	var summary *factoryapi.WorkerSessionReplaySummary
	if len(frames) > 0 && frames[len(frames)-1].Delivery == "REPLAY_SUMMARY" {
		summary = frames[len(frames)-1].ReplaySummary
		frames = frames[:len(frames)-1]
	}
	assertArchivedMCPReplayRecords(t, logs, frames, summary, envelope.Result.Events.Truncated, complete)
	assertArchivedUnknownCLIHTTP(t, server, logs.WorkerSessionId)
	assertArchivedUnknownMCPReads(t, ctx, connection)
}

func assertArchivedMCPReplayRecords(t *testing.T, logs factoryapi.WorkerSessionLogPage, frames []factoryapi.WorkerSessionEvent, summary *factoryapi.WorkerSessionReplaySummary, truncated, complete bool) {
	t.Helper()
	if truncated || len(frames) != len(logs.Events) {
		t.Fatalf("MCP replay records=%d logs=%d", len(frames), len(logs.Events))
	}
	for i, frame := range frames {
		if frame.WorkerSessionId != logs.WorkerSessionId || !reflect.DeepEqual(frame.Event, logs.Events[i].Event) {
			t.Fatalf("MCP replay record %d differs from logs", i)
		}
	}
	if summary == nil {
		summary = frames[len(frames)-1].ReplaySummary
	}
	if summary == nil || summary.Complete != complete || summary.EventsEmitted != int64(len(frames)) {
		t.Fatalf("MCP completeness: %+v", summary)
	}
}

func assertArchivedUnknownMCPReads(t *testing.T, ctx context.Context, connection *mcp.ClientSession) {
	t.Helper()
	assertArchivedMissingMCPReads(t, ctx, connection, "unknown-archive-id")
}

func assertArchivedMissingMCPReads(t *testing.T, ctx context.Context, connection *mcp.ClientSession, id string) {
	t.Helper()
	for _, view := range []string{"summary", "logs", "events"} {
		result, err := connection.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "READ", "view": view, "workerSessionId": id}})
		if err != nil || !result.IsError || len(result.Content) != 1 {
			t.Fatalf("unknown MCP %s: %+v %v", view, result, err)
		}
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		encoded, marshalErr := json.Marshal(result.StructuredContent)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := json.Unmarshal(encoded, &envelope); err != nil || envelope.Error.Code != "worker_session.not_found" {
			t.Fatalf("MCP unknown classification: %+v %v", envelope, err)
		}
	}
}

func assertIncompleteCapturedReplay(t *testing.T, server *support.FunctionalAPIServer, logs factoryapi.WorkerSessionLogPage) {
	t.Helper()
	response, err := http.Get(server.URL() + "/worker-sessions/" + logs.WorkerSessionId + "/events?replayOnly=true")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("incomplete replay status=%d", response.StatusCode)
	}
	// An incomplete replay has no committed terminal; consume to finite EOF.
	frames := decodeIncompleteReplayFrames(t, response.Body)
	if len(frames) != len(logs.Events)+1 {
		t.Fatalf("prefix replay=%+v", frames)
	}
	for i, want := range logs.Events {
		if !reflect.DeepEqual(frames[i].Event, want.Event) {
			t.Fatalf("prefix changed at %d", i)
		}
	}
	last := frames[len(frames)-1]
	if last.Delivery != "REPLAY_SUMMARY" || last.ReplaySummary == nil || last.ReplaySummary.Complete || last.RecordingHealth == nil || *last.RecordingHealth != "INCOMPLETE" {
		t.Fatalf("incomplete replay fabricated terminal: %+v", last)
	}
	active := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, server.URL()+"/worker-sessions?history=active")
	if len(active.Sessions) != 0 {
		t.Fatalf("archive restored live authority: %+v", active)
	}
}

func decodeIncompleteReplayFrames(t *testing.T, body io.Reader) []factoryapi.WorkerSessionEvent {
	t.Helper()
	decoder := bufio.NewScanner(body)
	var frames []factoryapi.WorkerSessionEvent
	for decoder.Scan() {
		line := decoder.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if err := decoder.Err(); err != nil {
		t.Fatal(err)
	}
	return frames
}

// The public resolved script contract has no command field. Its failed
// admission still has a captured opening/terminal, independently of providers.
func TestArchivedOrdinaryReplayFailedAdmission(t *testing.T) {
	t.Parallel()
	dir, home := support.ScaffoldSingleStepFactory(t, "archive-failed-admission"), t.TempDir()
	runner := &archiveDeniedRunner{}
	config := support.FunctionalAPIServerConfig{FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{ScriptCommandRunner: runner, ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)}}
	server := support.StartFunctionalAPIServer(t, config)
	payload := directWorkerSessionPayload("failed-request", "failed-worker", "failed-dispatch")
	payload.Execution.RunnerId = functionalStringPtr("script")
	payload.Execution.ExecutorProvider = functionalStringPtr("SCRIPT_WRAP")
	payload.Execution.ModelProvider, payload.Execution.Model = nil, nil
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL()+"/worker-sessions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusAccepted {
		t.Fatalf("failed admission status=%d body=%s error=%v", response.StatusCode, data, err)
	}
	// The host accepts the Worker identity; the missing command then fails
	// before any physical script admission. Wait for its actual terminal record.
	waitCapturedTerminal(t, server.URL(), "failed-worker")
	original := assertCapturedLogsCLIHTTPParity(t, server, "failed-worker")
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/failed-worker")
	if shown.State != "FAILED" || shown.TerminalCause == nil {
		t.Fatalf("failed admission facts: %+v", shown)
	}
	server.Close(t)
	restarted := support.StartFunctionalAPIServer(t, config)
	recovered := assertRestoredLogsCLIHTTPParity(t, restarted, "failed-worker")
	assertRestoredCapturedEvents(t, recovered, original)
	archived := support.GetJSON[factoryapi.WorkerSessionObservation](t, restarted.URL()+"/worker-sessions/failed-worker")
	if archived.State != "FAILED" || archived.TerminalCause == nil || *archived.TerminalCause != *shown.TerminalCause {
		t.Fatalf("recovery rewrote failed admission: %+v", archived)
	}
	assertArchivedCapturedCLIReplay(t, restarted, recovered, true)
	assertArchivedCapturedMCPReplay(t, restarted, recovered, true)
	if runner.calls.Load() != 0 {
		t.Fatal("failed script admission launched an external command")
	}
}

type archiveDeniedRunner struct{ calls atomic.Int32 }

func (r *archiveDeniedRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	return platformprocess.CommandResult{}, errors.New("unexpected command after rejected script admission")
}

func assertArchivedUnknownCLIHTTP(t *testing.T, server *support.FunctionalAPIServer, ownedID string) {
	t.Helper()
	for _, suffix := range []string{"", "/logs", "/events?replayOnly=true"} {
		response, err := http.Get(server.URL() + "/worker-sessions/unknown-archive-id" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		var failure factoryapi.ErrorResponse
		err = json.NewDecoder(response.Body).Decode(&failure)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusNotFound || (failure.Code != "WORKER_SESSION_NOT_FOUND" && failure.Code != "NOT_FOUND") {
			t.Fatalf("unknown HTTP read=%+v status=%d error=%v", failure, response.StatusCode, err)
		}
	}
	for _, args := range [][]string{{"show"}, {"read", "--view", "logs"}, {"stream", "--replay-only"}} {
		command := append([]string{"you", "--server", server.URL(), "--json", "worker-sessions"}, args...)
		command = append(command, "--worker-session-id", "unknown-archive-id")
		inputs := support.FakeInputs(t.Context(), command)
		err := server.Execute(t, inputs.Input)
		var failure interface{ CLIErrorCode() string }
		if !errors.As(err, &failure) || failure.CLIErrorCode() != "WORKER_SESSION_NOT_FOUND" {
			t.Fatalf("unknown CLI %v error=%v", args, err)
		}
	}
	// A known ordinary ID must remain hidden under a foreign Factory scope.
	response, err := http.Get(server.URL() + "/factory-sessions/foreign-archive-scope/worker-sessions/" + ownedID + "/events?replayOnly=true")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign scope replay status=%d", response.StatusCode)
	}
}

// The legacy cell supplies compatible committed input at the authorized store
// edge. It proves public reading of a pre-catalog name, not current production
// emission of that format. All identities and terminal records remain real.
type legacyOpeningNameStore struct{ *terminalCommitGate }

func (store *legacyOpeningNameStore) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if record.WorkName == "legacy-name" {
		var draft workers.Draft
		var opening workers.SessionPayload
		if err := json.Unmarshal(record.Record.Payload, &draft); err != nil {
			return err
		}
		if draft.Kind == workers.KindSession && draft.Phase == workers.PhaseStarted {
			if err := json.Unmarshal(draft.Payload, &opening); err != nil {
				return err
			}
			metadata := make(map[string]json.RawMessage)
			if len(opening.SessionMetadata) > 0 {
				if err := json.Unmarshal(opening.SessionMetadata, &metadata); err != nil {
					return err
				}
			}
			metadata["labels"] = json.RawMessage(`["tag:_work_name=legacy-name"]`)
			var err error
			opening.SessionMetadata, err = json.Marshal(metadata)
			if err != nil {
				return err
			}
			draft.Payload, err = json.Marshal(opening)
			if err != nil {
				return err
			}
			record.Record.Payload, err = json.Marshal(draft)
			if err != nil {
				return err
			}
			record.WorkName = ""
		}
	}
	return store.terminalCommitGate.PersistWorkerRecord(ctx, record)
}

func TestWorkerSessionArchivedOwnScopeParity(t *testing.T) {
	t.Parallel()
	runner, gates, cases, config := committedTerminalFixture(t)
	// Two independent sessions share one host and edge shape. Joined reopen
	// must be sequential: the captured profile has exactly one writer.
	cases = []committedTerminalCase{cases[8], cases[12]}
	first := make(map[string]*functionalWorkerGate)
	for _, cell := range cases {
		first[cell.model] = newFunctionalWorkerGate(make(chan struct{}))
	}
	config.Edges.ProviderCommandRunner = &archivedAttemptRunner{first: first, final: runner, calls: make(map[string]int)}
	home := strings.TrimPrefix(config.Env[0], "HOME=")
	config.BeforeStart = func(tb testing.TB, process support.Process, _ root.Input) {
		startCommittedTerminalBoards(tb, process, cases, home, ".older.json")
	}
	backing := &terminalCommitGate{entered: make(chan struct{}), proceed: make(chan struct{})}
	backing.release()
	config.Edges.WorkerRecordingWriter = &legacyOpeningNameStore{terminalCommitGate: backing}
	config.Edges.WorkerRecordingStoreObserver = func(store recordings.WorkerRecordingStore) { backing.WorkerRecordingStore = store }
	// Close a real empty-board recording before admitting any Work. Later
	// restoration consumes this immutable public recording, never a rewritten
	// board or a fresh runtime disguised as recovery.
	earlier := support.StartFunctionalAPIServer(t, config)
	earlier.Close(t)
	config.BeforeStart = archivedResumeBoards(cases, home, ".older.json", ".latest.json")
	server := support.StartFunctionalAPIServer(t, config)
	names := []string{"catalog-name", "catalog-name", "legacy-name", "legacy-name"}
	before := make([]factoryapi.WorkerSessionObservation, 2*len(cases))
	logs := make([]factoryapi.WorkerSessionLogPage, len(before))
	cursors := make([]string, len(before))
	transcripts := make([]*factoryapi.WorkerSessionTranscriptResponse, len(before))
	t.Run("committed named attempts", func(t *testing.T) {
		for index, cell := range cases {
			t.Run(string(cell.provider), func(t *testing.T) {
				t.Parallel()
				attempts := runArchivedAttempts(t, server, cell, names[2*index], first[cell.model], runner.slots[cell.model], gates[cell.model])
				copy(before[2*index:], attempts)
			})
		}
	})
	if t.Failed() {
		return
	}
	for index := range before {
		assertArchivedKnownName(t, assertTerminalReadParity(t, server, before[index].WorkerSessionId), names[index])
		assertArchivedOwnScopeReads(t, server, before[index])
		transcripts[index] = assertArchivedTranscriptParity(t, server, before[index])
		logs[index] = assertCommittedTerminalCapture(t, server, before[index].WorkerSessionId)
		page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/"+before[index].WorkerSessionId+"/logs?limit=1")
		if page.NextToken == nil {
			t.Fatal("bounded committed page omitted continuation")
		}
		cursors[index] = *page.NextToken
	}
	server.Close(t)
	config.BeforeStart = archivedResumeBoards(cases, home, ".latest.json", ".reopened.json")
	latest := support.StartFunctionalAPIServer(t, config)
	assertArchivedResumedBoard(t, latest, before, logs, names, cursors, transcripts, true)
	latest.Close(t)
	config.BeforeStart = archivedResumeBoards(cases, home, ".older.json", ".restored.json")
	reopened := support.StartFunctionalAPIServer(t, config)
	assertArchivedResumedBoard(t, reopened, before, logs, names, cursors, transcripts, false)
	reopened.Close(t)
	// The retained original Factory need not have a reopened runtime at all.
	// Its capture remains read-only, exact-scope authority on the same profile.
	config.BeforeStart = nil
	closedScopes := support.StartFunctionalAPIServer(t, config)
	assertArchivedClosedScopeParity(t, closedScopes, before, logs, names, transcripts)
	closedScopes.Close(t)
	assertArchivedPrivateProfileFence(t, config, before)
}

func assertArchivedClosedScopeParity(t *testing.T, server *support.FunctionalAPIServer, attempts []factoryapi.WorkerSessionObservation, logs []factoryapi.WorkerSessionLogPage, names []string, transcripts []*factoryapi.WorkerSessionTranscriptResponse) {
	t.Helper()
	for index, attempt := range attempts {
		got := assertTerminalReadParity(t, server, attempt.WorkerSessionId)
		assertArchivedKnownName(t, got, names[index])
		assertArchivedOwnScopeReads(t, server, got)
		if !reflect.DeepEqual(assertArchivedTranscriptParity(t, server, got), transcripts[index]) {
			t.Fatal("closed-scope transcript changed")
		}
		assertArchivedStopHasNoEffect(t, server, got, logs[index])
	}
}

// Cancellation requeues the same public Work. The next dispatch is held at its
// own command gate before pausing the Factory, so no timing race or board edit
// manufactures the second attempt. The two independent Factories overlap.
func runArchivedAttempts(t *testing.T, server *support.FunctionalAPIServer, cell committedTerminalCase, name string, first, final *functionalWorkerGate, finish chan struct{}) []factoryapi.WorkerSessionObservation {
	t.Helper()
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), cell.scope))
	work := support.SubmitSessionWorkAt(t, server.URL(), cell.scope, factoryapi.SubmitWorkRequest{Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": name}})
	workID := support.StringPointerValue(work.WorkId)
	initial := waitForRouteCharacterizationAssociation(t, stream, workID)
	first.waitStarted(t)
	response := postWorkerSessionControl(t, server.URL(), initial.workerSessionID, "cancel")
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("first attempt cancellation: %d", response.StatusCode)
	}
	first.waitCanceled(t)
	waitCapturedTerminal(t, server.URL(), initial.workerSessionID)
	replacement := waitForRouteCharacterizationAssociation(t, stream, workID)
	final.waitStarted(t)
	postLiveRuntimeLifecycleControl(t, server.URL(), cell.scope, factoryapi.FactorySessionLifecycleControlKindPause)
	close(finish)
	final.waitCompleted(t)
	waitCapturedTerminal(t, server.URL(), replacement.workerSessionID)
	attempts := []factoryapi.WorkerSessionObservation{
		support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+initial.workerSessionID),
		support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+replacement.workerSessionID),
	}
	if attempts[0].WorkerSessionId == attempts[1].WorkerSessionId || attempts[0].AttemptId == attempts[1].AttemptId || attempts[0].State != "CANCELED" || attempts[1].State != "COMPLETED" || !reflect.DeepEqual(attempts[0].WorkId, attempts[1].WorkId) {
		t.Fatalf("same Work lost distinct attempts: %+v", attempts)
	}
	return attempts
}

// Controlled provider command output supplies a continuation ID and normalized
// content. It never writes or reads native provider rollout files.
type archivedAttemptRunner struct {
	first map[string]*functionalWorkerGate
	final *committedTerminalRunner
	mu    sync.Mutex
	calls map[string]int
}

func (r *archivedAttemptRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	model := copiedLedgerModelArgument(request.Args)
	r.mu.Lock()
	r.calls[model]++
	count := r.calls[model]
	r.mu.Unlock()
	if count == 1 && r.first[model] != nil {
		return r.first[model].Run(ctx, request)
	}
	if count != 2 {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected archived attempt %q #%d", model, count)
	}
	result, err := r.final.Run(ctx, request)
	if err == nil {
		opening := fmt.Sprintf("{\"type\":\"thread.started\",\"thread_id\":%q}\n", "archive-provider-"+model)
		if request.Command == "claude" {
			opening = fmt.Sprintf("{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":%q}\n", "archive-provider-"+model)
		}
		result.Stdout = append([]byte(opening), result.Stdout...)
	}
	return result, err
}

func assertArchivedResumedBoard(t *testing.T, server *support.FunctionalAPIServer, before []factoryapi.WorkerSessionObservation, logs []factoryapi.WorkerSessionLogPage, names, cursors []string, transcripts []*factoryapi.WorkerSessionTranscriptResponse, retainsWork bool) {
	t.Helper()
	for index, original := range before {
		scope := support.StringPointerValue(original.FactorySessionId)
		board := support.GetJSON[factoryapi.ListWorkResponse](t, server.URL()+"/factory-sessions/"+scope+"/work")
		if retainsWork {
			if len(board.Results) != 1 || support.StringPointerValue(board.Results[0].WorkId) != support.StringPointerValue(original.WorkId) {
				t.Fatalf("latest recording lost original Work: %+v", board)
			}
		} else if len(board.Results) != 0 {
			t.Fatalf("older recording unexpectedly contains later Work: %+v", board)
		}
		got := assertTerminalReadParity(t, server, original.WorkerSessionId)
		assertArchivedKnownName(t, got, names[index])
		assertArchivedOwnScopeReads(t, server, got)
		if !reflect.DeepEqual(assertArchivedTranscriptParity(t, server, got), transcripts[index]) {
			t.Fatal("joined restart changed transcript identity or content")
		}
		if !committedTerminalFactsEqual(got, original) || !reflect.DeepEqual(got.FactorySessionId, original.FactorySessionId) || got.AttemptId != original.AttemptId || !reflect.DeepEqual(got.WorkId, original.WorkId) {
			t.Fatalf("restart changed committed identity/facts: before=%+v after=%+v", original, got)
		}
		page := assertCommittedTerminalCapture(t, server, got.WorkerSessionId)
		assertRestoredCapturedEvents(t, page, logs[index])
		assertArchivedStopHasNoEffect(t, server, got, page)
		// Resume a cursor acknowledged before shutdown, rather than merely
		// obtaining a new cursor after reopening the same journal.
		continued := support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/"+got.WorkerSessionId+"/logs?nextToken="+url.QueryEscape(cursors[index]))
		if !reflect.DeepEqual(continued.Events, logs[index].Events[1:]) || continued.CommittedPosition != page.CommittedPosition {
			t.Fatalf("acknowledged cursor replayed records or changed head: %+v", continued)
		}
	}
	assertArchivedPeerFences(t, server, before, cursors)
}

// Archived terminal identity supports idempotent stop acknowledgement only.
// Compare the complete capture and fleet after both public control boundaries;
// a successful response must not manufacture an execution or change facts.
func assertArchivedStopHasNoEffect(t *testing.T, server *support.FunctionalAPIServer, original factoryapi.WorkerSessionObservation, logs factoryapi.WorkerSessionLogPage) {
	t.Helper()
	before := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, server.URL()+"/worker-sessions?history=all")
	for _, action := range []string{"cancel", "terminate"} {
		response := postWorkerSessionControl(t, server.URL(), original.WorkerSessionId, action)
		var result factoryapi.WorkerSessionControlResponse
		err := json.NewDecoder(response.Body).Decode(&result)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || result.Outcome != "NOOP" || result.WorkerSessionId != original.WorkerSessionId || result.DispatchId != original.AttemptId || string(result.State) != string(original.State) {
			t.Fatalf("archived %s acquired control authority: status=%d result=%+v error=%v", action, response.StatusCode, result, err)
		}
		input := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "--json", "worker-sessions", action, original.WorkerSessionId})
		if err := server.Execute(t, input.Input); err != nil {
			t.Fatalf("archived CLI %s: %v %s", action, err, input.Stderr())
		}
		var cli factoryapi.WorkerSessionControlResponse
		if err := json.Unmarshal([]byte(input.Stdout()), &cli); err != nil || !reflect.DeepEqual(cli, result) {
			t.Fatalf("archived CLI/HTTP control differs: cli=%+v http=%+v error=%v", cli, result, err)
		}
	}
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+original.WorkerSessionId)
	after := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, server.URL()+"/worker-sessions?history=all")
	if !reflect.DeepEqual(shown, original) || !reflect.DeepEqual(after, before) {
		t.Fatalf("archived stop changed identity, facts or fleet: before=%+v after=%+v", before, after)
	}
	assertRestoredCapturedEvents(t, assertCommittedTerminalCapture(t, server, original.WorkerSessionId), logs)
}

// A second private profile has no permission to the first profile's captures,
// even when the caller supplies the exact original identities.
func assertArchivedPrivateProfileFence(t *testing.T, config support.FunctionalAPIServerConfig, attempts []factoryapi.WorkerSessionObservation) {
	t.Helper()
	home, captures := t.TempDir(), t.TempDir()
	config.BeforeStart = nil
	config.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + home, "LOCALAPPDATA=" + home, "XDG_CONFIG_HOME=" + home, "XDG_DATA_HOME=" + home, "TEMP=" + home, "TMP=" + home}
	config.Edges.WorkerRecordingWriter = nil
	config.Edges.WorkerRecordingStoreObserver = nil
	config.Edges.FactorySessionsWorkingDirectory = capturedRecordingDirectory(captures)
	server := support.StartFunctionalAPIServer(t, config)
	list := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, server.URL()+"/worker-sessions?history=all")
	if len(list.Sessions) != 0 {
		t.Fatalf("private profile exposed archived fleet: %+v", list)
	}
	connection, ctx, cleanup := archivedReplayMCP(t, server)
	defer cleanup()
	for _, attempt := range attempts {
		assertArchivedMissingMCPReads(t, ctx, connection, attempt.WorkerSessionId)
		input := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "--json", "worker-sessions", "show", "--worker-session-id", attempt.WorkerSessionId})
		var failure interface{ CLIErrorCode() string }
		if err := server.Execute(t, input.Input); !errors.As(err, &failure) || failure.CLIErrorCode() != "WORKER_SESSION_NOT_FOUND" {
			t.Fatalf("foreign profile CLI exposed original identity: %v", err)
		}
		for _, suffix := range []string{"", "/logs", "/transcript"} {
			response, err := http.Get(server.URL() + "/worker-sessions/" + attempt.WorkerSessionId + suffix)
			if err != nil {
				t.Fatal(err)
			}
			var failure factoryapi.ErrorResponse
			err = json.NewDecoder(response.Body).Decode(&failure)
			_ = response.Body.Close()
			if err != nil || response.StatusCode != http.StatusNotFound || (failure.Code != "WORKER_SESSION_NOT_FOUND" && failure.Code != "NOT_FOUND") {
				t.Fatalf("foreign profile read %s: status=%d failure=%+v error=%v", suffix, response.StatusCode, failure, err)
			}
		}
	}
}

func assertArchivedPeerFences(t *testing.T, server *support.FunctionalAPIServer, attempts []factoryapi.WorkerSessionObservation, cursors []string) {
	t.Helper()
	for index, attempt := range attempts {
		peer := attempts[(index+2)%len(attempts)]
		base := server.URL() + "/factory-sessions/" + support.StringPointerValue(peer.FactorySessionId) + "/worker-sessions"
		for _, suffix := range []string{"?workId=" + url.QueryEscape(support.StringPointerValue(attempt.WorkId)), "/" + attempt.WorkerSessionId, "/" + attempt.WorkerSessionId + "/transcript"} {
			response, err := http.Get(base + suffix)
			if err != nil {
				t.Fatal(err)
			}
			var failure factoryapi.ErrorResponse
			err = json.NewDecoder(response.Body).Decode(&failure)
			_ = response.Body.Close()
			if err != nil || response.StatusCode != http.StatusNotFound || failure.Code == "" {
				t.Fatalf("peer scope exposed archived identity: status=%d failure=%+v error=%v", response.StatusCode, failure, err)
			}
		}
		response, err := http.Get(server.URL() + "/worker-sessions/" + peer.WorkerSessionId + "/logs?nextToken=" + url.QueryEscape(cursors[index]))
		if err != nil {
			t.Fatal(err)
		}
		var failure factoryapi.ErrorResponse
		err = json.NewDecoder(response.Body).Decode(&failure)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusBadRequest || failure.Code != "BAD_REQUEST" {
			t.Fatalf("peer cursor error: status=%d failure=%+v error=%v", response.StatusCode, failure, err)
		}
	}
}

func archivedResumeBoards(cases []committedTerminalCase, home, sourceSuffix, recordSuffix string) func(testing.TB, support.Process, root.Input) {
	return func(tb testing.TB, process support.Process, _ root.Input) {
		tb.Helper()
		for _, cell := range cases {
			_, err := process.(support.ApplicationProcess).FactorySessions().Start(tb.Context(), factorysessions.SessionStartRequest{
				SessionID: cell.scope, Mode: factorysessions.SessionOperationModeLive, FolderPath: cell.dir, ActivationOnly: true,
				RuntimeSelection: &factorysessions.SessionRuntimeSelection{
					Mode: factorysessions.SessionRuntimeModeService, SystemConfigHome: home,
					DefinitionSourcePath: filepath.Join(cell.dir, "factory.json"), ExecutionBaseDir: cell.dir, RuntimeInstanceID: uuid.NewString(),
					Recording: factorysessions.SessionRecordingSelection{
						ResumePath: filepath.Join(cell.dir, cell.scope+sourceSuffix), RecordPath: filepath.Join(cell.dir, cell.scope+recordSuffix),
					},
				},
			})
			if err != nil {
				tb.Fatal(err)
			}
		}
	}
}

func assertArchivedOwnScopeReads(t *testing.T, server *support.FunctionalAPIServer, shown factoryapi.WorkerSessionObservation) {
	t.Helper()
	scope, workID := support.StringPointerValue(shown.FactorySessionId), support.StringPointerValue(shown.WorkId)
	base := server.URL() + "/factory-sessions/" + scope + "/worker-sessions"
	list := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, base+"?workId="+workID)
	if len(list.Sessions) != 2 || list.Sessions[0].WorkerSessionId == list.Sessions[1].WorkerSessionId || list.Sessions[0].AttemptId == list.Sessions[1].AttemptId || list.Sessions[0].State != "CANCELED" || list.Sessions[1].State != "COMPLETED" {
		t.Fatalf("scoped LIST lost distinct chronological attempts: %+v", list)
	}
	assertTerminalListed(t, list, shown)
	own := support.GetJSON[factoryapi.WorkerSessionObservation](t, base+"/"+shown.WorkerSessionId)
	if !reflect.DeepEqual(own, shown) {
		t.Fatalf("own-scope summary differs: own=%+v unscoped=%+v", own, shown)
	}
	assertArchivedTranscriptParity(t, server, shown)
	input := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "worker-sessions", "list", "--session", scope, "--work-id", workID, "--output", "json"})
	if err := server.Execute(t, input.Input); err != nil {
		t.Fatalf("own-scope CLI list: %v %s", err, input.Stderr())
	}
	var cli factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(input.Stdout()), &cli); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cli, list) {
		t.Fatalf("own-scope CLI/HTTP list differs: %+v / %+v", cli, list)
	}
	for _, query := range []string{"?workId=unknown-work", "/unknown-worker"} {
		response, err := http.Get(base + query)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("unknown identity %s: %d", query, response.StatusCode)
		}
	}
}

func assertArchivedTranscriptParity(t *testing.T, server *support.FunctionalAPIServer, shown factoryapi.WorkerSessionObservation) *factoryapi.WorkerSessionTranscriptResponse {
	t.Helper()
	scope := support.StringPointerValue(shown.FactorySessionId)
	base := server.URL() + "/factory-sessions/" + scope + "/worker-sessions"
	if shown.ProviderSession != nil {
		return assertArchivedAvailableTranscript(t, server, shown, base)
	}
	// The canceled attempt has no provider association. Transcript absence must
	// remain typed, even beside a second attempt with available captured content.
	for _, url := range []string{base + "/" + shown.WorkerSessionId + "/transcript", server.URL() + "/worker-sessions/" + shown.WorkerSessionId + "/transcript"} {
		response, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		var failure factoryapi.ErrorResponse
		err = json.NewDecoder(response.Body).Decode(&failure)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusInternalServerError || failure.Code != "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE" {
			t.Fatalf("transcript absence differs: status=%d error=%+v decode=%v", response.StatusCode, failure, err)
		}
	}
	return nil
}

func assertArchivedAvailableTranscript(t *testing.T, server *support.FunctionalAPIServer, shown factoryapi.WorkerSessionObservation, base string) *factoryapi.WorkerSessionTranscriptResponse {
	t.Helper()
	transcript := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t, base+"/"+shown.WorkerSessionId+"/transcript")
	unscoped := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t, server.URL()+"/worker-sessions/"+shown.WorkerSessionId+"/transcript")
	if !reflect.DeepEqual(transcript, unscoped) || transcript.WorkerSessionId != shown.WorkerSessionId || transcript.AttemptId != shown.AttemptId || !reflect.DeepEqual(transcript.FactorySessionId, shown.FactorySessionId) || !reflect.DeepEqual(transcript.WorkIds, shown.WorkIds) || transcript.ProviderSession.Id != shown.ProviderSession.Id || transcript.ProviderSession.Provider != shown.ProviderSession.Provider || string(transcript.ProviderSession.Kind) != string(shown.ProviderSession.Kind) || len(transcript.Entries) == 0 {
		t.Fatalf("transcript lost exact identity or captured content: scoped=%+v unscoped=%+v summary=%+v", transcript, unscoped, shown)
	}
	input := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "worker-sessions", "read", "--worker-session-id", shown.WorkerSessionId, "--view", "transcript", "--output", "json"})
	if err := server.Execute(t, input.Input); err != nil {
		t.Fatalf("CLI transcript: %v %s", err, input.Stderr())
	}
	var cli factoryapi.WorkerSessionTranscriptResponse
	if err := json.Unmarshal([]byte(input.Stdout()), &cli); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cli, transcript) {
		t.Fatalf("CLI/HTTP transcript differs: %+v / %+v", cli, transcript)
	}
	connection, ctx, cleanup := archivedReplayMCP(t, server)
	defer cleanup()
	result, err := connection.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "READ", "view": "transcript", "workerSessionId": shown.WorkerSessionId}})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("MCP transcript: %+v %v", result, err)
	}
	var envelope struct {
		Result struct {
			Transcript factoryapi.WorkerSessionTranscriptResponse `json:"transcript"`
		} `json:"result"`
	}
	encoded := []byte(result.Content[0].(*mcp.TextContent).Text)
	if err := json.Unmarshal(encoded, &envelope); err != nil || !reflect.DeepEqual(envelope.Result.Transcript, transcript) {
		t.Fatalf("MCP/HTTP transcript differs: %+v / %+v error=%v", envelope, transcript, err)
	}
	return &transcript
}

func assertArchivedKnownName(t *testing.T, row factoryapi.WorkerSessionObservation, name string) {
	t.Helper()
	if row.WorkName == nil || *row.WorkName != name {
		t.Fatalf("committed name lost: %+v; want %q", row, name)
	}
}
