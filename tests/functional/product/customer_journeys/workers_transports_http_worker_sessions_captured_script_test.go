package customer_journeys_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workercli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type completedInspectionScript struct{}

func (completedInspectionScript) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("inspection script output\n")}, nil
}

func TestWorkerSessionUnavailableTerminalReads(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldSingleStepFactory(t, "unavailable-terminal-inspection")
	if err := os.MkdirAll(filepath.Join(dir, "workers", "processor"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workers", "processor", "AGENTS.md"), []byte("---\ntype: SCRIPT_WORKER\ncommand: inspection-script\n---\nExecute the script.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	config := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Env: []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{ScriptCommandRunner: completedInspectionScript{}, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	}
	server := support.StartFunctionalAPIServer(t, config)
	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	submitted := support.SubmitSessionWorkAt(t, server.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: map[string]string{"title": "inspect ended script"}})
	support.WaitForSessionTerminalStatus(t, server.URL(), opened.Session.Id, functionalWorkerSignalTimeout)
	listed := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, workerSessionsListURL(server.URL(), opened.Session.Id, support.StringPointerValue(submitted.WorkId)))
	if len(listed.Sessions) != 1 {
		t.Fatalf("Worker count = %d", len(listed.Sessions))
	}
	worker := listed.Sessions[0]
	if worker.State != factoryapi.WorkerSessionObservationStateCompleted || worker.ProviderSession != nil || string(worker.Transcript) != "UNAVAILABLE" {
		t.Fatalf("script identity = %+v", worker)
	}
	assertUnavailableInspection(t, server, opened.Session.Id, worker.WorkerSessionId)
	before := assertCapturedLogsCLIHTTPParity(t, server, worker.WorkerSessionId)
	assertUnavailableFiniteStreams(t, server, opened.Session.Id, before, false)
	server.Close(t)
	restarted := support.StartFunctionalAPIServer(t, config)
	assertUnavailableInspection(t, restarted, opened.Session.Id, worker.WorkerSessionId)
	after := assertCapturedLogsCLIHTTPParity(t, restarted, worker.WorkerSessionId)
	if !reflect.DeepEqual(before.Events, after.Events) || before.Health != after.Health {
		t.Fatal("owned reopen changed script history or health")
	}
	assertUnavailableFiniteStreams(t, restarted, opened.Session.Id, after, true)
}

func assertUnavailableInspection(t *testing.T, server *support.FunctionalAPIServer, scope, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second) //nolint:testsleep // Operator 2026-10-11T20:11Z: separate harness ceiling from the one-second read contract.
	defer cancel()
	for _, base := range []string{server.URL() + "/worker-sessions/" + id, server.URL() + "/factory-sessions/" + scope + "/worker-sessions/" + id} {
		assertUnavailableHTTP(t, ctx, base+"/transcript", http.StatusInternalServerError, "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE")
	}
	for _, selector := range [][]string{nil, {"--session", scope}} {
		args := []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--server", server.URL(), "--output", "json"}
		args = append(args, selector...)
		inputs := support.FakeInputs(ctx, args)
		started := time.Now()
		err := server.Execute(t, inputs.Input)
		var diagnostic *workercli.CLIError
		if !errors.As(err, &diagnostic) || diagnostic.Code != "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE" || time.Since(started) > time.Second { //nolint:testsleep // Operator 2026-10-11T20:11Z requires this one-second read/cleanup contract.
			t.Fatalf("CLI unavailable: %v elapsed=%s", err, time.Since(started))
		}
	}
	assertUnavailableHTTP(t, ctx, server.URL()+"/worker-sessions/missing-worker/transcript", http.StatusNotFound, "NOT_FOUND")
}

func assertUnavailableHTTP(t *testing.T, ctx context.Context, endpoint string, status int, code string) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	var diagnostic factoryapi.ErrorResponse
	if json.Unmarshal(body, &diagnostic) != nil || response.StatusCode != status || string(diagnostic.Code) != code || elapsed > time.Second { //nolint:testsleep // Operator 2026-10-11T20:11Z requires this one-second read/cleanup contract.
		t.Fatalf("unavailable transcript: status=%d elapsed=%s body=%s", response.StatusCode, elapsed, body)
	}
	t.Logf("typed %s elapsed=%s", code, elapsed)
}

func assertUnavailableFiniteStreams(t *testing.T, server *support.FunctionalAPIServer, scope string, page factoryapi.WorkerSessionLogPage, archived bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second) //nolint:testsleep // Operator 2026-10-11T20:11Z requires one-second headers/EOF with separate harness ceiling.
	defer cancel()
	for _, route := range []string{"/worker-sessions/" + page.WorkerSessionId, "/factory-sessions/" + scope + "/worker-sessions/" + page.WorkerSessionId} {
		queries := []string{"", "?replayOnly=true"}
		for queryIndex := 0; queryIndex < len(queries); queryIndex++ {
			query := queries[queryIndex]
			absent := archived && strings.HasPrefix(route, "/factory-sessions/")
			body := readUnavailableFiniteStream(t, ctx, server.URL()+route+"/events"+query, absent)
			if absent {
				continue
			}
			lastPosition := assertUnavailableFiniteFrames(t, body, page, strings.Contains(query, "after_position="))
			if queryIndex == 1 {
				queries = append(queries, fmt.Sprintf("?after_position=%d", lastPosition), fmt.Sprintf("?replayOnly=true&after_position=%d", lastPosition))
			}

		}
	}
}

func readUnavailableFiniteStream(t *testing.T, ctx context.Context, endpoint string, absent bool) []byte {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	headers := time.Since(started)
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	elapsed := time.Since(started)
	// A stopped Factory runtime no longer owns a scoped event route.
	// Captured canonical reads remain available; scope absence stays typed.
	if absent {
		var diagnostic factoryapi.ErrorResponse
		if readErr != nil || json.Unmarshal(body, &diagnostic) != nil || response.StatusCode != http.StatusNotFound || diagnostic.Code != "NOT_FOUND" || elapsed > time.Second { //nolint:testsleep // Operator 2026-10-11T20:11Z requires this one-second read/cleanup contract.
			t.Fatalf("archived scoped absence: %d %s %v", response.StatusCode, body, readErr)
		}
		return body
	}
	if readErr != nil || response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" || headers > time.Second || elapsed > time.Second { //nolint:testsleep // Operator 2026-10-11T20:11Z requires this one-second read/cleanup contract.
		t.Fatalf("finite stream headers=%s EOF=%s status=%d err=%v", headers, elapsed, response.StatusCode, readErr)
	}
	t.Logf("finite %s headers=%s EOF=%s", endpoint, headers, elapsed)
	return body
}

func assertUnavailableFiniteFrames(t *testing.T, body []byte, page factoryapi.WorkerSessionLogPage, atHead bool) int64 {
	t.Helper()
	var records int
	var lastPosition int64
	factoryRecords := false
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil || frame.WorkerSessionId != page.WorkerSessionId {
			t.Fatalf("finite frame identity: %+v err=%v", frame, err)
		}
		if frame.Event.Position == 0 {
			continue
		}
		factoryRecords = factoryRecords || frame.Event.SourceType == "factory_event"
		if frame.Event.Position <= lastPosition || frame.Event.Cursor.Position != frame.Event.Position {
			t.Fatal("finite stream lost ordered exclusive cursor")
		}
		lastPosition = frame.Event.Position
		if !factoryRecords {
			assertUnavailableCapturedRecord(t, frame.Event, page, records)
		}
		records++
	}
	if atHead && records != 0 {
		t.Fatal("at-head cursor replay duplicated retained records")
	}
	if !factoryRecords && !atHead && records != len(page.Events) {
		t.Fatal("finite stream lost captured prefix")
	}
	return lastPosition
}

func assertUnavailableCapturedRecord(t *testing.T, event factoryapi.WorkerSessionEventRecord, page factoryapi.WorkerSessionLogPage, index int) {
	t.Helper()
	if index >= len(page.Events) {
		t.Fatal("finite stream gained a captured record")
	}
	expected := page.Events[index].Event
	if event.Position != expected.Position || event.SourceEventId != expected.SourceEventId || !reflect.DeepEqual(event.Payload, expected.Payload) {
		t.Fatalf("finite stream changed record: got=%+v expected=%+v", event, expected)
	}
}

type capturedScriptGate struct {
	started, failNext, canceled chan struct{}
}

func (gate *capturedScriptGate) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return gate.RunStreaming(ctx, request, nil)
}

func (gate *capturedScriptGate) RunStreaming(ctx context.Context, _ platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	observe(platformprocess.OutputStreamStdout, []byte("committed script output\n"))
	close(gate.started)
	select {
	case <-gate.failNext:
		observe(platformprocess.OutputStreamStderr, []byte("uncaptured script output\n"))
	case <-ctx.Done():
		close(gate.canceled)
		return platformprocess.CommandResult{}, ctx.Err()
	}
	<-ctx.Done()
	close(gate.canceled)
	return platformprocess.CommandResult{}, ctx.Err()
}

// A real Script runner crosses the controlled command edge while a real
// journal accepts the opening and one chunk, then loses its write effect.
// Public CLI/HTTP reads and cancellation observe that same active attempt.
// This fault shape needs its own process; all mutable gates/store/home/session
// identities are local to this parallel scenario, with no executable build.
func TestWorkerSessionCapturedLogsActiveScriptWriteFailure(t *testing.T) {
	t.Parallel()
	fault := &capturedFaultStore{acceptedThrough: 2, failed: make(chan struct{})}
	gate := &capturedScriptGate{started: make(chan struct{}), failNext: make(chan struct{}), canceled: make(chan struct{})}
	dir := support.ScaffoldSingleStepFactory(t, "captured-script-failure")
	if err := os.MkdirAll(filepath.Join(dir, "workers", "processor"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workers", "processor", "AGENTS.md"), []byte("---\ntype: SCRIPT_WORKER\ncommand: captured-script\n---\nExecute the script.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Env: []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{ScriptCommandRunner: gate, WorkerRecordingWriter: fault, WorkerRecordingStoreObserver: func(store recordings.WorkerRecordingStore) { fault.WorkerRecordingStore = store }, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	})
	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	if opened.Session == nil {
		t.Fatal("missing Factory Session")
	}
	name := "captured-script-work"
	submitted := support.SubmitSessionWorkAt(t, server.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{
		Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": "capture script output"},
	})
	select {
	case <-gate.started:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("script did not publish its first output")
	}
	workID := support.StringPointerValue(submitted.WorkId)
	waitForScopedProcessingWork(t, server.URL(), opened.Session.Id, workID)
	list := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, workerSessionsListURL(server.URL(), opened.Session.Id, workID))
	if len(list.Sessions) != 1 {
		t.Fatalf("active Worker Sessions = %d", len(list.Sessions))
	}
	id := list.Sessions[0].WorkerSessionId
	// The output observer queues the chunk before its durable commit finishes.
	// Observe the public committed prefix before comparing CLI and HTTP reads.
	_, err := support.WaitForObservation(functionalWorkerSignalTimeout, func() (factoryapi.WorkerSessionLogPage, error) {
		return support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/"+id+"/logs"), nil
	}, func(page factoryapi.WorkerSessionLogPage) bool {
		return page.CommittedPosition == 2 && len(page.Events) == 2
	})
	if err != nil {
		t.Fatalf("observe committed script output: %v", err)
	}
	prefix := assertCapturedLogsCLIHTTPParity(t, server, id)
	if prefix.CommittedPosition != 2 || len(prefix.Events) != 2 {
		t.Fatalf("script chunk was not committed: %+v", prefix)
	}
	assertCapturedScriptChunk(t, prefix.Events[1])
	close(gate.failNext)
	select {
	case <-fault.failed:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("active output did not reach write fault")
	}
	assertCapturedActiveFailureAndCancel(t, server, id, prefix, gate)
}

func assertCapturedActiveFailureAndCancel(t *testing.T, server *support.FunctionalAPIServer, id string, prefix factoryapi.WorkerSessionLogPage, gate *capturedScriptGate) {
	t.Helper()
	incomplete := assertCapturedLogsCLIHTTPParity(t, server, id)
	if incomplete.Health != factoryapi.INCOMPLETE || incomplete.CommittedPosition != prefix.CommittedPosition || !reflect.DeepEqual(incomplete.Events, prefix.Events) {
		t.Fatal("failed active write changed the committed prefix")
	}
	active := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+id)
	if active.State != factoryapi.WorkerSessionObservationStateRunning {
		t.Fatalf("capture loss changed execution state: %s", active.State)
	}
	response := postWorkerSessionControl(t, server.URL(), id, "cancel")
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live cancel after capture loss = %d", response.StatusCode)
	}
	select {
	case <-gate.canceled:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("live cancellation did not reach the running command")
	}
	// The command's cancellation callback precedes authoritative terminal
	// publication, so observe the resulting public capture health explicitly.
	degraded, err := support.WaitForObservation(functionalWorkerSignalTimeout, func() (factoryapi.WorkerSessionLogPage, error) {
		return support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/"+id+"/logs"), nil
	}, func(page factoryapi.WorkerSessionLogPage) bool { return page.Health == factoryapi.DEGRADED })
	if err != nil {
		t.Fatal(err)
	}
	if degraded.CommittedPosition != prefix.CommittedPosition || !reflect.DeepEqual(degraded.Events, prefix.Events) || !reflect.DeepEqual(assertCapturedLogsCLIHTTPParity(t, server, id), degraded) {
		t.Fatal("terminal capture loss changed committed activity or CLI/HTTP parity")
	}
}

func assertCapturedScriptChunk(t *testing.T, frame factoryapi.WorkerSessionEvent) {
	t.Helper()
	data, err := json.Marshal(frame.Event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var draft workers.Draft
	if err := json.Unmarshal(data, &draft); err != nil {
		t.Fatal(err)
	}
	var progress workers.ProgressPayload
	if err := json.Unmarshal(draft.Payload, &progress); err != nil {
		t.Fatal(err)
	}
	if draft.Kind != workers.KindProgress || progress.Label != "stdout" || progress.Message != "committed script output\n" || frame.Event.CapturedAt == nil {
		t.Fatalf("captured script chunk lost content/time: %+v %+v", draft, progress)
	}
}
