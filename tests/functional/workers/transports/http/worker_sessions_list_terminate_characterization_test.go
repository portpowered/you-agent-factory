package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workercli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// The ordered journey owns one isolated host because /worker-sessions is a
// process-wide fleet, even with a CLI --session flag. List-before-control and
// terminal-after-control share identities; independent package peers run in parallel.
func TestWorkerSessionHTTPListTerminateCharacterization(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldSingleStepFactory(t, "wsv-rest-list-terminate")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	runner := newFleetCharacterizationRunner()
	sessionID := uuid.NewString()
	home := t.TempDir()
	server := &fleetCharacterizationServer{env: []string{"HOME=" + home, "USERPROFILE=" + home, "HOMEDRIVE=" + filepath.VolumeName(home), "HOMEPATH=" + home[len(filepath.VolumeName(home)):]}, dir: dir}
	server.FunctionalAPIServer = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Args: []string{"--session", sessionID},
		Env:   server.env,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
		BeforeStart: func(tb testing.TB, process support.Process, inputs root.Input) {
			// Fleet listing and termination do not cover first-run installation;
			// complete profile bootstrap before the hosted readiness clock starts.
			support.InitializeCustomerHomeWithProcess(tb, process, inputs.Env, inputs.WorkingDirectory)
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(runner.releaseAll)
	assertFleetCharacterizationList(t, server, sessionID, url.Values{}, nil)
	t.Log("REST-01: idle HTTP/remote CLI fleet is empty")

	for index, id := range []string{"fleet-a", "fleet-b"} {
		response := postDirectWorkerSession(t, t.Context(), server.URL(), id+"-request", id, id+"-dispatch")
		response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("admit %s: status %d", id, response.StatusCode)
		}
		waitFleetCharacterizationSignal(t, runner.slots[index].started, "direct admission")
	}
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), sessionID))
	name := "fleet-sibling"
	submitted := support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": name},
	})
	waitFleetCharacterizationSignal(t, runner.slots[2].started, "Factory admission")
	sibling := waitForRouteCharacterizationAssociation(t, stream, support.StringPointerValue(submitted.WorkId))
	assertFleetCharacterizationCohort(t, server, sessionID, sibling)

	joinedFleetCharacterizationTerminate(t, server, "fleet-a", runner.slots[0], false)
	assertFleetCharacterizationTerminal(t, server, sessionID, "fleet-a", "fleet-a-dispatch", "TERMINATED")
	assertCapturedStopCause(t, server.URL(), "fleet-a", "OPERATOR_TERMINATE")
	assertFleetCharacterizationControl(t, fleetCharacterizationControl(t, server, "fleet-a", false), "fleet-a", "fleet-a-dispatch", "NOOP", "TERMINATED")
	t.Log("REST-05: HTTP terminate joins exact no-reference A; repeated control is NOOP")
	joinedFleetCharacterizationTerminate(t, server, "fleet-b", runner.slots[1], true)
	assertFleetCharacterizationTerminal(t, server, sessionID, "fleet-b", "fleet-b-dispatch", "TERMINATED")
	assertCapturedStopCause(t, server.URL(), "fleet-b", "OPERATOR_TERMINATE")
	assertFleetCharacterizationControl(t, fleetCharacterizationControl(t, server, "fleet-a", true), "fleet-a", "fleet-a-dispatch", "NOOP", "TERMINATED")
	t.Log("REST-06: remote CLI terminate joins B and matches A's terminal NOOP")
	assertFleetCharacterizationMissing(t, server)
	assertFleetCharacterizationList(t, server, sessionID, url.Values{"scope": {"factory"}, "state": {"RUNNING"}}, []string{sibling.workerSessionID})
	select {
	case <-runner.slots[2].canceled:
		t.Fatal("unrelated Factory Worker received cancellation")
	default:
	}
	completeFleetCharacterizationSibling(t, server, runner.slots[2], stream, sibling)
	assertCapturedStopCause(t, server.URL(), sibling.workerSessionID, "COMPLETED")
	for _, cli := range []bool{false, true} {
		assertFleetCharacterizationControl(t, fleetCharacterizationControl(t, server, sibling.workerSessionID, cli), sibling.workerSessionID, sibling.dispatchID, "NOOP", "COMPLETED")
	}
	if runner.callCount() != 3 {
		t.Fatalf("provider calls = %d, want exactly three admitted Workers", runner.callCount())
	}
	t.Log("REST-08: sibling survives all controls, completes naturally, HTTP/CLI preserve COMPLETED")
	functionalevidence.Covers(t, "rest/listWorkerSessions", "rest/terminateWorkerSession")
}

// Every invocation uses the same test-owned profile after the host's public
// bootstrap has completed; remote CLI calls cannot initialize the user's home.
type fleetCharacterizationServer struct {
	*support.FunctionalAPIServer
	env []string
	dir string
}

func (server *fleetCharacterizationServer) Execute(t testing.TB, input root.Input) error {
	input.Env = append([]string(nil), server.env...)
	input.WorkingDirectory = server.dir
	return server.FunctionalAPIServer.Execute(t, input)
}

type fleetCharacterizationSlot struct {
	started, canceled, returned, gate chan struct{}
	once                              sync.Once
}

func (slot *fleetCharacterizationSlot) release() { slot.once.Do(func() { close(slot.gate) }) }

type fleetCharacterizationRunner struct {
	mu    sync.Mutex
	calls int
	slots []*fleetCharacterizationSlot
}

func newFleetCharacterizationRunner() *fleetCharacterizationRunner {
	runner := &fleetCharacterizationRunner{}
	for range 3 {
		runner.slots = append(runner.slots, &fleetCharacterizationSlot{
			started: make(chan struct{}), canceled: make(chan struct{}), returned: make(chan struct{}), gate: make(chan struct{}),
		})
	}
	return runner
}

func (runner *fleetCharacterizationRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	index := runner.calls
	runner.calls++
	runner.mu.Unlock()
	if index >= len(runner.slots) {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider call %d", index)
	}
	// Each admission is observed before the next starts, so immutable slots
	// belong to A, B, S without a shared mutable routing selector.
	slot := runner.slots[index]
	close(slot.started)
	defer close(slot.returned)
	select {
	case <-ctx.Done():
		close(slot.canceled)
		<-slot.gate
		return platformprocess.CommandResult{}, ctx.Err()
	case <-slot.gate:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("sibling completed. COMPLETE")}, nil
	}
}

func (runner *fleetCharacterizationRunner) releaseAll() {
	for _, slot := range runner.slots {
		slot.release()
	}
}

func (runner *fleetCharacterizationRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func waitFleetCharacterizationSignal(t *testing.T, signal <-chan struct{}, purpose string) {
	t.Helper()
	// The existing ceiling bounds failures; channels, not elapsed time, prove readiness.
	select {
	case <-signal:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatalf("missing controlled signal: %s", purpose)
	}
}

func fleetCharacterizationGET(t *testing.T, server *fleetCharacterizationServer, query url.Values) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+"/worker-sessions?"+query.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("fleet GET %s: %v", query.Encode(), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("fleet GET status = %d", response.StatusCode)
	}
	var listed factoryapi.ListWorkerSessionsResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	return listed
}

func assertFleetCharacterizationList(t *testing.T, server *fleetCharacterizationServer, sessionID string, query url.Values, want []string) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	listed := fleetCharacterizationGET(t, server, query)
	args := []string{"you", "--remote", "--server", server.URL(), "worker-sessions", "list", "--session", sessionID, "--output", "json"}
	for _, key := range []string{"scope", "state", "limit", "nextToken"} {
		flag := key
		if key == "nextToken" {
			flag = "next-token"
		}
		for _, value := range query[key] {
			args = append(args, "--"+flag, value)
		}
	}
	inputs := support.FakeInputs(t.Context(), args)
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("remote list: %v; %s", err, inputs.Stderr())
	}
	var cli factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for index := range listed.Sessions {
		ids = append(ids, listed.Sessions[index].WorkerSessionId)
		// Active duration changes between sequential public reads; all stable facts match.
		if listed.Sessions[index].DurationBasis == "ACTIVE_CLOCK" {
			listed.Sessions[index].DurationMillis = nil
		}
	}
	for index := range cli.Sessions {
		if cli.Sessions[index].DurationBasis == "ACTIVE_CLOCK" {
			cli.Sessions[index].DurationMillis = nil
		}
	}
	if !reflect.DeepEqual(ids, want) || !reflect.DeepEqual(listed, cli) {
		t.Fatalf("fleet %s: IDs %v want %v; REST %#v CLI %#v", query.Encode(), ids, want, listed, cli)
	}
	return listed
}

func assertFleetCharacterizationCohort(t *testing.T, server *fleetCharacterizationServer, sessionID string, sibling routeCharacterizationDispatch) {
	t.Helper()
	all := []string{"fleet-a", "fleet-b", sibling.workerSessionID}
	sort.Strings(all)
	for _, query := range []url.Values{{}, {"scope": {"all"}}} {
		listed := assertFleetCharacterizationList(t, server, sessionID, query, all)
		for _, observation := range listed.Sessions {
			if observation.ProviderSessionAvailable || observation.State != "RUNNING" {
				t.Fatalf("held no-reference observation = %#v", observation)
			}
			if observation.WorkerSessionId == sibling.workerSessionID {
				if observation.Direct || observation.AttemptId != sibling.dispatchID || support.StringPointerValue(observation.FactorySessionId) != sessionID || support.StringPointerValue(observation.WorkId) != sibling.workID || !reflect.DeepEqual(observation.WorkIds, []string{sibling.workID}) {
					t.Fatalf("Factory correlation = %#v", observation)
				}
			} else if !observation.Direct || observation.AttemptId != observation.WorkerSessionId+"-dispatch" || len(observation.WorkIds) != 0 || observation.FactorySessionId != nil || observation.WorkId != nil {
				t.Fatalf("direct correlation = %#v", observation)
			}
		}
	}
	assertFleetCharacterizationList(t, server, sessionID, url.Values{"scope": {"direct"}, "state": {"RUNNING"}}, []string{"fleet-a", "fleet-b"})
	assertFleetCharacterizationList(t, server, sessionID, url.Values{"scope": {"factory"}, "state": {"RUNNING"}}, []string{sibling.workerSessionID})
	assertFleetCharacterizationList(t, server, sessionID, url.Values{"state": {"PAUSED"}}, nil)
	for _, scope := range []string{"all", "direct", "factory"} {
		want := all
		if scope == "direct" {
			want = []string{"fleet-a", "fleet-b"}
		}
		if scope == "factory" {
			want = []string{sibling.workerSessionID}
		}
		query := url.Values{"scope": {scope}, "state": {"RUNNING"}, "limit": {"1"}}
		for index, id := range want {
			page := assertFleetCharacterizationList(t, server, sessionID, query, []string{id})
			next := ""
			if page.PaginationContext != nil {
				next = support.StringPointerValue(page.PaginationContext.NextToken)
			}
			if (next == "") != (index == len(want)-1) {
				t.Fatalf("page %d cursor = %q", index, next)
			}
			query.Set("nextToken", next)
		}
	}
	t.Logf("REST-02..04: exact mixed cohort %v, canonical correlations, origin/state filters and ordered HTTP/CLI pages", all)
}

func fleetCharacterizationControl(t *testing.T, server *fleetCharacterizationServer, id string, cli bool) factoryapi.WorkerSessionControlResponse {
	t.Helper()
	result, err := executeFleetCharacterizationControl(t, server, id, cli)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func executeFleetCharacterizationControl(t *testing.T, server *fleetCharacterizationServer, id string, cli bool) (factoryapi.WorkerSessionControlResponse, error) {
	var result factoryapi.WorkerSessionControlResponse
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	defer cancel()
	if cli {
		inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", server.URL(), "worker-sessions", "terminate", id, "--output", "json"})
		if err := server.Execute(t, inputs.Input); err != nil {
			return result, fmt.Errorf("CLI terminate: %w; %s", err, inputs.Stderr())
		}
		err := json.Unmarshal([]byte(inputs.Stdout()), &result)
		return result, err
	} else {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL()+"/worker-sessions/"+url.PathEscape(id)+"/terminate", nil)
		if err != nil {
			return result, err
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return result, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return result, fmt.Errorf("HTTP terminate status = %d", response.StatusCode)
		}
		err = json.NewDecoder(response.Body).Decode(&result)
		return result, err
	}
}

func joinedFleetCharacterizationTerminate(t *testing.T, server *fleetCharacterizationServer, id string, slot *fleetCharacterizationSlot, cli bool) {
	t.Helper()
	type outcome struct {
		response factoryapi.WorkerSessionControlResponse
		err      error
		returned bool
	}
	results := make(chan outcome, 1)
	done := make(chan struct{})
	t.Cleanup(func() {
		slot.release()
		select {
		case <-done:
		case <-time.After(functionalWorkerSignalTimeout):
			t.Error("owned terminate request did not join cleanup")
		}
	})
	go func() {
		defer close(done)
		response, err := executeFleetCharacterizationControl(t, server, id, cli)
		// Record ordering at response receipt, before the test goroutine can release the runner.
		returned := false
		select {
		case <-slot.returned:
			returned = true
		default:
		}
		results <- outcome{response, err, returned}
	}()
	waitFleetCharacterizationSignal(t, slot.canceled, "terminate cancellation acknowledgement")
	select {
	case result := <-results:
		t.Fatalf("terminate returned before runner release: %#v", result)
	default:
	}
	slot.release()
	select {
	case result := <-results:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if !result.returned {
			t.Fatal("successful terminate preceded controlled runner return")
		}
		assertFleetCharacterizationControl(t, result.response, id, id+"-dispatch", "APPLIED", "TERMINATED")
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("terminate did not join")
	}
}

func assertFleetCharacterizationControl(t *testing.T, result factoryapi.WorkerSessionControlResponse, id, dispatch, outcome, state string) {
	t.Helper()
	if result.WorkerSessionId != id || result.DispatchId != dispatch || string(result.Action) != "TERMINATE" || string(result.Outcome) != outcome || string(result.State) != state {
		t.Fatalf("control = %#v, want %s/%s TERMINATE/%s/%s", result, id, dispatch, outcome, state)
	}
}

func openFleetCharacterizationEvents(t *testing.T, server *fleetCharacterizationServer, sessionID, id string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+"/factory-sessions/"+sessionID+"/worker-sessions/"+id+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	if response.StatusCode != http.StatusOK {
		t.Fatalf("terminal events status = %d", response.StatusCode)
	}
	return response
}

func assertFleetCharacterizationTerminal(t *testing.T, server *fleetCharacterizationServer, sessionID, id, dispatch, state string) {
	t.Helper()
	// Public terminal events join publication before the terminal fleet snapshot.
	response := openFleetCharacterizationEvents(t, server, sessionID, id)
	defer response.Body.Close()
	frames, phase, err := readWorkerSessionEventStreamUntilTerminal(response)
	// Current lifecycle drafts share CANCELED phase for cancel and terminate;
	// their nested status preserves the distinct canonical control state.
	wantPhase := state
	if state == "TERMINATED" {
		wantPhase = "CANCELED"
	}
	if err != nil || phase != wantPhase {
		t.Fatalf("terminal events phase %q error %v, want %s; frames=%#v", phase, err, wantPhase, frames)
	}
	assertSingleTerminalWorkerSessionEvent(t, frames, id, wantPhase)
	terminal := frames[len(frames)-1]
	if payload := workerSessionControlPayload(terminal); payload["status"] != state {
		t.Fatalf("terminal lifecycle canonical status = %#v, want %s", payload, state)
	}
	assertFleetCharacterizationSnapshot(t, server, id, dispatch, state)
}

func assertFleetCharacterizationSnapshot(t *testing.T, server *fleetCharacterizationServer, id, dispatch, state string) {
	t.Helper()
	listed := fleetCharacterizationGET(t, server, url.Values{"state": {state}})
	for _, observation := range listed.Sessions {
		if observation.WorkerSessionId == id && observation.AttemptId == dispatch && string(observation.State) == state {
			return
		}
	}
	t.Fatalf("terminal fleet lacks exact %s/%s/%s: %#v", id, dispatch, state, listed)
}

func completeFleetCharacterizationSibling(t *testing.T, server *fleetCharacterizationServer, slot *fleetCharacterizationSlot, stream *support.FactoryEventStream, sibling routeCharacterizationDispatch) {
	t.Helper()
	slot.release()
	waitFleetCharacterizationSignal(t, slot.returned, "sibling natural return")
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	defer cancel()
	// Factory Workers expose Factory dispatch events, rather than the direct
	// Worker lifecycle envelope. The already-open session stream owns completion.
	for {
		event := stream.NextEventContext(ctx)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || support.StringPointerValue(event.Context.DispatchId) != sibling.dispatchID {
			continue
		}
		result, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil || result.Outcome != "ACCEPTED" || result.Cancellation != nil || result.OutputWork == nil {
			t.Fatalf("sibling terminal dispatch = %#v, error %v", result, err)
		}
		found := false
		for _, work := range *result.OutputWork {
			if support.StringPointerValue(work.WorkId) == sibling.workID && work.State.Name == "complete" && work.State.Type == "TERMINAL" {
				found = true
			}
		}
		if !found {
			t.Fatalf("sibling terminal Work missing from %#v", result)
		}
		break
	}
	assertFleetCharacterizationSnapshot(t, server, sibling.workerSessionID, sibling.dispatchID, "COMPLETED")
}

func assertFleetCharacterizationMissing(t *testing.T, server *fleetCharacterizationServer) {
	t.Helper()
	id := uuid.NewString()
	response := postWorkerSessionControl(t, server.URL(), id, "terminate")
	defer response.Body.Close()
	var diagnostic struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&diagnostic); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound || diagnostic.Code != "NOT_FOUND" {
		t.Fatalf("missing HTTP: status %d diagnostic %#v", response.StatusCode, diagnostic)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "--remote", "--server", server.URL(), "worker-sessions", "terminate", id, "--output", "json"})
	err := server.Execute(t, inputs.Input)
	var typed *workercli.CLIError
	if !errors.As(err, &typed) || typed.Code != "NOT_FOUND" {
		t.Fatalf("missing CLI typed error = %v", err)
	}
	if err := json.Unmarshal([]byte(inputs.Stderr()), &diagnostic); err != nil {
		t.Fatalf("missing CLI diagnostic: %v; stdout %s stderr %s", err, inputs.Stdout(), inputs.Stderr())
	}
	if diagnostic.Code != "NOT_FOUND" {
		t.Fatalf("missing CLI diagnostic = %#v", diagnostic)
	}
	t.Log("REST-07: never-admitted identity returns HTTP 404/NOT_FOUND and structured CLI NOT_FOUND")
}
