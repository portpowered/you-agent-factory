package customer_journeys_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
type legacyOpeningNameStore struct {
	*terminalCommitGate
	namelessScope string
	readFault     atomic.Pointer[archivedSummaryFault]
}

// Faults affect only the selected materialized summary at the authorized
// recording edge. Other captures and the stored records remain real.
type archivedSummaryFault struct {
	id                string
	err               error
	entered, detached chan struct{}
}

func (store *legacyOpeningNameStore) ListWorkerSessionCaptures(ctx context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	page, err := store.WorkerRecordingStore.ListWorkerSessionCaptures(ctx, request)
	return store.applyCatalogReadFault(ctx, page, err)
}

func (store *legacyOpeningNameStore) ListPreparedWorkerSessionCaptures(ctx context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	page, err := store.WorkerRecordingStore.ListPreparedWorkerSessionCaptures(ctx, request)
	return store.applyCatalogReadFault(ctx, page, err)
}

func (store *legacyOpeningNameStore) applyCatalogReadFault(ctx context.Context, page recordings.WorkerCapturedCatalogPage, err error) (recordings.WorkerCapturedCatalogPage, error) {
	if err != nil {
		return page, err
	}
	for _, item := range page.Items {
		if fault := store.readFault.Load(); fault != nil && fault.id == item.Catalog.WorkerSessionID {
			return recordings.WorkerCapturedCatalogPage{}, fault.read(ctx)
		}
	}
	return page, nil
}

func (store *legacyOpeningNameStore) LookupWorkerSessionCapture(ctx context.Context, id string) (recordings.WorkerSessionCatalogEntry, error) {
	if fault := store.readFault.Load(); fault != nil && fault.id == id {
		return recordings.WorkerSessionCatalogEntry{}, fault.read(ctx)
	}
	return store.WorkerRecordingStore.LookupWorkerSessionCapture(ctx, id)
}

func (fault *archivedSummaryFault) read(ctx context.Context) error {
	if fault.err != nil {
		return fault.err
	}
	close(fault.entered)
	<-ctx.Done()
	close(fault.detached)
	return ctx.Err()
}

func (store *legacyOpeningNameStore) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	if fault := store.readFault.Load(); fault != nil && fault.id == id {
		return recordings.WorkerCapturedSummary{}, fault.read(ctx)
	}
	return store.terminalCommitGate.LookupWorkerSessionSummary(ctx, id)
}

func (store *legacyOpeningNameStore) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	// Compatible legacy captures can retain exact membership without a name or
	// originating board artifact. Supply that absence at the authorized storage
	// edge; the current board still knows the public Work's name.
	if record.FactorySessionID == store.namelessScope {
		record.WorkName, record.OriginatingArtifact = "", ""
		var err error
		record.Record.Payload, err = namelessArchivedPayload(record.Record.Payload)
		if err != nil {
			return err
		}
	}
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

func namelessArchivedPayload(payload json.RawMessage) (json.RawMessage, error) {
	var draft workers.Draft
	if err := json.Unmarshal(payload, &draft); err != nil {
		return nil, err
	}
	// Explicit zero counters are compatible committed input. Normal native
	// command normalization can omit zero-only usage; actual configured mock
	// emission is a separate prebuilt-artifact witness, not claimed here.
	if draft.Kind == workers.KindUsage {
		draft.Payload = json.RawMessage(`{"origin":"SYNTHETIC","inputTokens":0,"outputTokens":0,"totalTokens":0}`)
		return json.Marshal(draft)
	}
	if draft.Kind != workers.KindSession {
		return payload, nil
	}
	var session workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &session); err != nil {
		return nil, err
	}
	var metadata map[string]json.RawMessage
	if len(session.SessionMetadata) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(session.SessionMetadata, &metadata); err != nil {
		return nil, err
	}
	delete(metadata, "labels")
	var err error
	session.SessionMetadata, err = json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	draft.Payload, err = json.Marshal(session)
	if err != nil {
		return nil, err
	}
	return json.Marshal(draft)
}
