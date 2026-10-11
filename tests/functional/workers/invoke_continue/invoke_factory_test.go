package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// The same immutable command edge serves source/resume and the occupied peer.
// Only the peer waits; source revival must finish before the peer is released.
type durableRevivalCommandRunner struct {
	source *testutil.ProviderCommandRunner
	peer   *t7GatedProviderRunner
}

func (runner *durableRevivalCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner *durableRevivalCommandRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	if strings.Contains(string(request.Stdin), "durable occupied peer input") {
		return runner.peer.RunStreaming(ctx, request, observe)
	}
	result, err := runner.source.Run(ctx, request)
	if observe != nil && len(result.Stdout) != 0 {
		observe(platformprocess.OutputStreamStdout, result.Stdout)
	}
	return result, err
}

func writeDurableRevivalCapacity(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "factory.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var definition map[string]any
	if err := json.Unmarshal(content, &definition); err != nil {
		t.Fatal(err)
	}
	resource := []map[string]any{{"name": "executor-slot", "capacity": 1}}
	definition["resources"] = resource
	definition["workstations"].([]any)[0].(map[string]any)["resources"] = resource
	writeInvokeContinueJSON(t, path, definition)
}

func assertDurableRevivalPeerCompletion(t *testing.T, host invokeContinueStartedProcess, session, id string, runner *t7GatedProviderRunner) {
	t.Helper()
	t7AssertFactorySibling(t, t.Context(), host.baseURL, id, session, "RUNNING")
	if runner.CallCount() != 1 {
		t.Fatalf("revival changed peer admission: calls=%d", runner.CallCount())
	}
	close(runner.release)
	t19AwaitSignal(t, t.Context(), runner.stopped, "Factory peer completed independently")
	support.WaitForSessionTerminalStatus(t, host.baseURL, session, 30*time.Second)
	t7AssertFactorySibling(t, t.Context(), host.baseURL, id, session, "COMPLETED")
}

// F-10 retains real healthy capture history while the external store returns
// missing or stale execution authority. Restart, observation and refusal cross
// production wiring; no Worker Sessions or Provider Sessions peer is replaced.
func assertDurableRevivalRecipeRefusal(t *testing.T, host invokeContinueStartedProcess, home, dir, sourceID string, runner *testutil.ProviderCommandRunner, expectedHealth ...string) {
	t.Helper()
	health := "COMPLETE"
	if len(expectedHealth) != 0 {
		health = expectedHealth[0]
	} else {
		awaitContinuationRestartLogs(t, host, home, dir, sourceID)
	}
	status, logs := t7HTTP(t, t.Context(), http.MethodGet, host.baseURL+"/worker-sessions/"+sourceID+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(logs, `"health":"`+health+`"`) {
		t.Fatalf("recipe refusal lost healthy history: %s", logs)
	}
	observation := support.GetJSON[api.WorkerSessionObservation](t, host.baseURL+"/worker-sessions/"+sourceID)
	if observation.State != "COMPLETED" || observation.Revivable == nil || *observation.Revivable || observation.SuccessorWorkerSessionId != nil {
		t.Fatalf("unavailable recipe granted revival authority: %+v", observation)
	}
	rows := support.GetJSON[api.ListWorkerSessionsResponse](t, host.baseURL+"/worker-sessions?history=archived")
	if len(rows.Sessions) != 1 || !reflect.DeepEqual(observation, rows.Sessions[0]) {
		t.Fatalf("archived recipe capability disagrees with show: %+v", rows)
	}
	assertDurableRevivalCLIRefusal(t, host, home, dir, sourceID)
	status, body := t7HTTP(t, t.Context(), http.MethodPost, host.baseURL+"/worker-sessions/"+sourceID+"/continue",
		map[string]any{"resolveHead": true, "requestId": "recipe-http-request", "successorWorkerSessionId": "recipe-http-successor", "followUpInput": "follow-up"})
	if status != http.StatusServiceUnavailable || !strings.Contains(body, "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED") || runner.CallCount() != 1 {
		t.Fatalf("recipe refusal: status=%d body=%s calls=%d", status, body, runner.CallCount())
	}
	after := support.GetJSON[api.WorkerSessionObservation](t, host.baseURL+"/worker-sessions/"+sourceID)
	if !reflect.DeepEqual(observation, after) {
		t.Fatalf("recipe refusal mutated source: before=%+v after=%+v", observation, after)
	}
}

func assertDurableRevivalCLIRefusal(t *testing.T, host invokeContinueStartedProcess, home, dir, sourceID string) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		flags := []string{"you", "--json"}
		if remote {
			flags = append(flags, "--remote", "--server", host.baseURL)
		}
		request := support.FakeInputs(t.Context(), append(flags, "worker-sessions", "continue", sourceID,
			"--head", "--request-id", "recipe-refusal-request", "--successor-worker-session-id", "recipe-refusal-successor", "--user-message", "follow-up"))
		request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		if err := host.process.Execute(request.Input); err == nil {
			t.Fatal("unavailable recipe admitted a successor")
		}
		assertDirectWorkerSessionCLIError(t, request, "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED")
		if strings.Contains(request.Stderr()+request.Stdout(), "private-revival") {
			t.Fatal("recipe diagnostics exposed private persistence detail")
		}
	}
}

// F6-04 admits actual Factory Work beside a direct invocation through one
// assembled host. Each attempt has its own command gate and captured identity.
func TestT7DirectStopLeavesFactorySiblingRunning(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.cancel", "cli/you.worker-sessions.show", "cli/you.worker-sessions.list", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	target := fixture.scenario(t, "t7-factory-target")
	defer target.close(t)
	peer := fixture.scenario(t, "t7-factory")
	defer peer.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	targetRunner := target.providerRunner.(*t7GatedProviderRunner)
	peerRunner := peer.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, targetRunner)()
	defer t7ReleaseAndJoin(t, ctx, peerRunner)()
	t7WriteFactorySibling(t, peer.workingDirectory)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, peer.workingDirectory)
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	feedback := strings.Repeat("ordinary feedback ", 3000)
	work := support.SubmitSessionWorkAt(t, fixture.baseURL, opened.Session.Id, api.SubmitWorkRequest{
		WorkTypeName: "task", Payload: "T7 Factory sibling input",
		Tags: &api.StringMap{"project": "unattributed", "role": "root", "_last_output": feedback},
	})
	if work.WorkId == nil {
		t.Fatal("Factory Work has no ID")
	}
	t19AwaitSignal(t, ctx, peerRunner.started, "Factory sibling started")
	canonical := support.GetJSON[api.Work](t, support.SessionWorkURL(fixture.baseURL, opened.Session.Id, "/work/"+*work.WorkId))
	if canonical.Tags == nil || (*canonical.Tags)["_last_output"] != feedback {
		t.Fatal("Worker metadata projection changed canonical feedback")
	}
	endpoint := fixture.baseURL + "/factory-sessions/" + opened.Session.Id + "/worker-sessions?workId=" + *work.WorkId
	rows := support.GetJSON[api.ListWorkerSessionsResponse](t, endpoint)
	if len(rows.Sessions) != 1 {
		t.Fatalf("Factory Worker Sessions = %#v", rows)
	}
	peerID := rows.Sessions[0].WorkerSessionId
	assertRequesterUnattributedFactory(t, fixture, peer, ctx, peerID, *work.WorkId, opened.Session.Id, rows.Sessions[0])
	id := startRequesterFactoryChild(t, fixture, target, ctx, peerRunner, peerID)
	t19AwaitSignal(t, ctx, targetRunner.started, "direct invocation started")
	assertRequesterFactoryChild(t, fixture, target, ctx, id, peerID, requesterSourceToken(t, peerRunner, peerID), *work.WorkId, opened.Session.Id)
	t7AssertFactorySibling(t, ctx, fixture.baseURL, peerID, opened.Session.Id, "RUNNING")
	stop := t7RemoteCLIInputs(target, ctx, fixture.baseURL, "cancel", id)
	if err := fixture.process.Execute(stop.Input); err != nil {
		t.Fatalf("direct cancel: %v: %s", err, stop.Stderr())
	}
	t7AssertStopResult(t, stop.Stdout(), id, "cancel")
	t19AwaitSignal(t, ctx, targetRunner.stopped, "direct joined")
	t7AssertSingleTerminal(t, ctx, fixture.baseURL, id, "CANCELED")
	t7AssertFactorySibling(t, ctx, fixture.baseURL, peerID, opened.Session.Id, "RUNNING")
	close(peerRunner.release)
	t19AwaitSignal(t, ctx, peerRunner.stopped, "Factory command joined")
	body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+peerID, nil)
		return body, nil
	}, func(body string) bool { return strings.Contains(body, `"state":"COMPLETED"`) })
	if err != nil {
		t.Fatalf("Factory sibling completion: %v: %s", err, body)
	}
	t7AssertFactorySibling(t, ctx, fixture.baseURL, peerID, opened.Session.Id, "COMPLETED")
	status, logs := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+peerID+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(logs, "T7 Factory sibling COMPLETE") || strings.Contains(logs, id) {
		t.Fatalf("Factory sibling captured logs = %d %s", status, logs)
	}
	command := peerRunner.Requests()[0]
	if command.Command != "codex" || !strings.Contains(strings.Join(command.Args, " "), "--model opaque-factory-model") ||
		!strings.Contains(strings.Join(command.Args, " "), `model_reasoning_effort="high"`) ||
		!strings.Contains(string(command.Stdin), "T7 Factory sibling input") || targetRunner.CallCount() != 1 || peerRunner.CallCount() != 1 {
		t.Fatalf("Factory command settings or isolated attempt counts: command=%s args=%q prompt=%q target=%d peer=%d", command.Command, command.Args, command.Stdin, targetRunner.CallCount(), peerRunner.CallCount())
	}
}

// M4: submitted Work tags are descriptive. Runner identity and public reads
// agree on known dispatch context without inferring a requester from tags.
func assertRequesterUnattributedFactory(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, id, workID, sessionID string, listed api.WorkerSessionObservation) {
	t.Helper()
	observation := requesterObservation(t, fixture, scenario, ctx, id)
	wantLabels := []string{"factory-session:" + sessionID, "tag:_work_name=work-1", "tag:_work_type=task", "tag:project=unattributed", "tag:role=root", "work:" + workID, "workstation:process"}
	if observation.Requester != nil || !requesterCorrelationAgrees(observation, workID, sessionID) ||
		observation.Labels == nil || !reflect.DeepEqual(*observation.Labels, wantLabels) {
		metadata, _ := json.Marshal(map[string]any{"correlation": observation.Correlation, "labels": observation.Labels, "requester": observation.Requester})
		t.Fatalf("unattributed Factory metadata = %s, want Work %s, Factory Session %s and labels %v", metadata, workID, sessionID, wantLabels)
	}
	if listed.Requester != nil || !reflect.DeepEqual(listed.Correlation, observation.Correlation) || !reflect.DeepEqual(listed.Labels, observation.Labels) {
		t.Fatal("Factory list and CLI show disagree on admitted metadata")
	}
	assertRequesterFactoryListed(t, fixture, scenario, ctx, workID, sessionID, observation)
	environment := requesterEnvironment(scenario.providerRunner.Requests()[0].Env)
	requesterSourceToken(t, scenario.providerRunner.(*t7GatedProviderRunner), id)
	assertRequesterEndpoint(t, environment, fixture.baseURL)
	if environment["YOU_WORK_ID"] != workID || environment["YOU_FACTORY_SESSION_ID"] != sessionID {
		t.Fatal("runner environment disagrees with admitted Factory correlation")
	}
	for _, key := range []string{"YOU_MESSAGE_TARGET", "YOU_MESSAGE_TARGET_WORK_ID"} {
		if environment[key] != "" {
			t.Fatal("unattributed Factory execution invented a message target")
		}
	}
}

func assertRequesterFactoryListed(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, workID, sessionID string, observation api.WorkerSessionObservation) {
	t.Helper()
	selected := requesterFactoryListSelection(t, fixture, scenario, ctx, workID, sessionID, observation.WorkerSessionId)
	if len(selected) != 1 || !reflect.DeepEqual(selected[0].Requester, observation.Requester) || !reflect.DeepEqual(selected[0].Correlation, observation.Correlation) || !reflect.DeepEqual(selected[0].Labels, observation.Labels) {
		got, _ := json.Marshal(selected)
		want, _ := json.Marshal(observation)
		t.Fatalf("CLI list lost exact Factory dispatch metadata: selected=%s want=%s", got, want)
	}

}

// Workflow provider attempts can have no Work. That input selects the fleet
// collection, whose bounded pages must be traversed despite concurrent peers.
func requesterFactoryListSelection(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, workID, sessionID, workerID string) []api.WorkerSessionObservation {
	t.Helper()
	args := []string{"list", "--session", sessionID, "--work-id", workID}
	if workID == "" {
		args = []string{"list", "--scope", "factory", "--limit", "2"}
	}
	seen := make(map[string]bool)
	for {
		list := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, args...)
		if err := fixture.process.Execute(list.Input); err != nil {
			t.Fatal(err)
		}
		var page api.ListWorkerSessionsResponse
		decodeDirectWorkerSessionResult(t, list.Stdout(), &page)
		selected := requesterListedWorker(page.Sessions, workerID)
		next := requesterFactoryListCursor(page, workID)
		if len(selected) > 0 || next == "" {
			return selected
		}
		if seen[next] {
			t.Fatal("Factory fleet list repeated a pagination cursor")
		}
		seen[next] = true
		args = []string{"list", "--scope", "factory", "--limit", "2", "--next-token", next}
	}
}

func requesterListedWorker(rows []api.WorkerSessionObservation, workerID string) []api.WorkerSessionObservation {
	var selected []api.WorkerSessionObservation
	for _, row := range rows {
		if row.WorkerSessionId == workerID {
			selected = append(selected, row)
		}
	}
	return selected
}

func requesterFactoryListCursor(page api.ListWorkerSessionsResponse, workID string) string {
	if workID != "" || page.PaginationContext == nil || page.PaginationContext.NextToken == nil {
		return ""
	}
	return *page.PaginationContext.NextToken
}

func requesterCorrelationAgrees(observation api.WorkerSessionObservation, workID, sessionID string) bool {
	return observation.Correlation != nil && observation.Correlation.WorkId != nil &&
		*observation.Correlation.WorkId == workID && observation.Correlation.FactorySessionId != nil && *observation.Correlation.FactorySessionId == sessionID
}

func startRequesterFactoryChild(t *testing.T, fixture *invokeContinuePackageFixture, target *invokeContinueScenario, ctx context.Context, source *t7GatedProviderRunner, sourceID string) string {
	t.Helper()
	id := scenarioScopedID(target, "factory-requester-child")
	invoke := t7RemoteCLIInputs(target, ctx, fixture.baseURL, "invoke", "--execution", requesterExecutionPath(t, target, id), "--async")
	invoke.Input.Env = append(invoke.Input.Env, "YOU_WORKER_SESSION_ID="+sourceID, "YOU_WORKER_SESSION_TOKEN="+requesterSourceToken(t, source, sourceID))
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("Factory caller's direct invocation failed: %v", err)
	}
	return id
}

func assertRequesterFactoryChild(t *testing.T, fixture *invokeContinuePackageFixture, target *invokeContinueScenario, ctx context.Context, id, sourceID, sourceToken, workID, sessionID string) {
	t.Helper()
	observation := requesterObservation(t, fixture, target, ctx, id)
	if !requesterCorrelationAgrees(observation, workID, sessionID) || observation.Requester == nil || observation.Requester.WorkerSessionId != sourceID ||
		observation.Requester.WorkId == nil || *observation.Requester.WorkId != workID || observation.Labels == nil || !reflect.DeepEqual(*observation.Labels, []string{"parent:" + sourceID}) {
		t.Fatal("Factory caller's child lost verified requester/correlation or inherited source tags")
	}
	environment := requesterEnvironment(target.providerRunner.Requests()[0].Env)
	assertRequesterSuccessorEnvironment(t, environment, id, sourceID, sourceToken)
	if environment["YOU_MESSAGE_TARGET"] != sourceID || environment["YOU_MESSAGE_TARGET_WORK_ID"] != workID ||
		environment["YOU_WORK_ID"] != workID || environment["YOU_FACTORY_SESSION_ID"] != sessionID {
		t.Fatal("Factory caller's child environment disagrees with admitted metadata")
	}
}

func t7AssertFactorySibling(t *testing.T, ctx context.Context, baseURL, id, session, state string) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id, nil)
	var observation api.WorkerSessionObservation
	if err := json.Unmarshal([]byte(body), &observation); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || observation.Direct || observation.WorkerSessionId != id || observation.FactorySessionId == nil ||
		*observation.FactorySessionId != session || string(observation.State) != state || observation.Model == nil || *observation.Model != "opaque-factory-model" {
		t.Fatalf("Factory sibling observation = %d %s", status, body)
	}
}

func t7WriteFactorySibling(t *testing.T, directoryPath string) {
	t.Helper()
	// Copy authored inputs only: the running host contains recordings that
	// parallel attempts can atomically rename while a directory walk is active.
	if err := copyInvokeContinueDirectory(support.LegacyFixtureDir(t, "executor_success"), directoryPath); err != nil {
		t.Fatal(err)
	}
	// Only the explicitly submitted sibling Work should start this Factory.
	support.ClearSeedInputs(t, directoryPath)
	directory, _ := json.Marshal(directoryPath)
	worker := "---\ntype: MODEL_WORKER\nexecutorProvider: CODEX\nmodel: opaque-factory-model\nmodelProvider: codex\nreasoningEffort: high\nstopToken: COMPLETE\n---\nFactory sibling instruction.\n"
	if err := os.WriteFile(filepath.Join(directoryPath, "workers", "worker", "AGENTS.md"), []byte(worker), 0o600); err != nil {
		t.Fatal(err)
	}
	station := fmt.Sprintf("---\ntype: MODEL_WORKSTATION\nrunner: codex\nworkingDirectory: %s\n---\nFactory sibling input: {{ (index .Inputs 0).Payload }}\n", directory)
	if err := os.WriteFile(filepath.Join(directoryPath, "workstations", "process", "AGENTS.md"), []byte(station), 0o600); err != nil {
		t.Fatal(err)
	}
}

// F-9 changes only the selected reference's storage membership at the existing
// writer boundary. Inspection and admission still use their production owners.
type revivalAvailabilityStore struct {
	interruptPhaseAckStore
	unavailable       atomic.Bool
	gateInspection    atomic.Bool
	inspectionEntered chan chan struct{}
}

func (store *revivalAvailabilityStore) ListPreparedWorkerSessionCaptures(ctx context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	page, err := store.WorkerRecordingStore.ListPreparedWorkerSessionCaptures(ctx, request)
	if store.gateInspection.Load() {
		release := make(chan struct{})
		store.inspectionEntered <- release
		select {
		case <-release:
		case <-ctx.Done():
			return recordings.WorkerCapturedCatalogPage{}, ctx.Err()
		}
	}
	if store.unavailable.Load() {
		items := make([]recordings.WorkerCapturedCatalogItem, 0, len(page.Items))
		for _, item := range page.Items {
			if item.Catalog.WorkerSessionID != "revival-unavailable-provider" {
				items = append(items, item)
			}
		}
		page.Items = items
	}
	return page, err
}

func TestDurableRevivalProviderUnavailablePreservesHistory(t *testing.T) {
	t.Parallel()
	root, dir := t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("opaque-restart-thread", "initial COMPLETE")},
		continuationRestartCommandResult(false), continuationRestartCommandResult(false),
	)
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
	store := &revivalAvailabilityStore{}
	first := startRevivalAvailabilityHost(t, host, home, route, store)
	const id = "revival-unavailable-provider"
	path := filepath.Join(dir, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt", workingDirectory: dir, userMessage: "initial input"})
	executeHeadRestartCLI(t, first, home, dir, "worker-sessions", "invoke", "--execution", path)
	awaitContinuationRestartLogs(t, first, home, dir, id)
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	first = startRevivalAvailabilityHost(t, host, home, route, store)
	before := support.GetJSON[api.WorkerSessionObservation](t, first.baseURL+"/worker-sessions/"+id)
	if before.Revivable == nil || !*before.Revivable {
		t.Fatalf("initial exact reference unavailable: %+v", before)
	}
	store.unavailable.Store(true)
	assertDurableRevivalRecipeRefusal(t, first, home, dir, id, runner)
	// Restoring current reference availability restores capability, without
	// changing the source identity or treating the earlier refusal as admission.
	store.unavailable.Store(false)
	restored := support.GetJSON[api.WorkerSessionObservation](t, first.baseURL+"/worker-sessions/"+id)
	if !reflect.DeepEqual(before, restored) || runner.CallCount() != 1 {
		t.Fatalf("availability recovery mutated source: %+v calls=%d", restored, runner.CallCount())
	}
	executeHeadRestartCLI(t, first, home, dir, "worker-sessions", "continue", id, "--head", "--request-id", "recipe-refusal-request", "--successor-worker-session-id", "recipe-refusal-successor", "--user-message", "follow-up")
	if runner.CallCount() != 2 {
		t.Fatalf("availability recovery calls=%d", runner.CallCount())
	}
	assertDurableRevivalFastAdmissionFence(t, first, store, runner, home, dir)
}

// Both callers freeze the same terminal head at the public inspection storage
// boundary. Finish the winner before releasing the second inspection; the
// second request must refuse its old head rather than chase the new successor.
func assertDurableRevivalFastAdmissionFence(t *testing.T, host invokeContinueStartedProcess, store *revivalAvailabilityStore, runner *testutil.ProviderCommandRunner, home, dir string) {
	t.Helper()
	store.inspectionEntered = make(chan chan struct{}, 2)
	store.gateInspection.Store(true)
	type completion struct {
		request *support.CapturedInputs
		err     error
	}
	done := make(chan completion, 2)
	for index := range 2 {
		name := []string{"one", "two"}[index]
		flags := []string{"you", "--json"}
		if index == 1 {
			flags = append(flags, "--remote", "--server", host.baseURL)
		}
		request := support.FakeInputs(t.Context(), append(flags, "worker-sessions", "continue", "revival-unavailable-provider", "--head", "--request-id", "fast-"+name, "--successor-worker-session-id", "fast-successor-"+name, "--user-message", "follow-up", "--async"))
		request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		go func() { done <- completion{request: request, err: host.process.Execute(request.Input)} }()
	}
	first, second := awaitRevivalInspections(t, store)
	releaseSecond := sync.OnceFunc(func() { close(second) })
	defer releaseSecond()
	store.gateInspection.Store(false)
	close(first)
	result := <-done
	if result.err != nil {
		t.Fatalf("first inspected head failed admission: %v %s", result.err, result.request.Stderr())
	}
	// The async response proves admission; joining public logs proves the
	// successor and its durable terminal are complete before the loser resumes.
	var admitted directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, result.request.Stdout(), &admitted)
	if !admitted.Accepted || !strings.HasPrefix(admitted.SuccessorWorkerSessionID, "fast-successor-") {
		t.Fatalf("winning admission has no observable successor: %+v", admitted)
	}
	awaitContinuationRestartLogs(t, host, home, dir, admitted.SuccessorWorkerSessionID)
	releaseSecond()
	if result := <-done; result.err == nil {
		t.Fatal("completed winner allowed competing head admission")
	} else {
		assertDirectWorkerSessionCLIError(t, result.request, "WORKER_SESSION_CONTINUATION_CONFLICT")
	}
	if runner.CallCount() != 3 {
		t.Fatalf("fast winning head branched: calls=%d", runner.CallCount())
	}
}

func awaitRevivalInspections(t *testing.T, store *revivalAvailabilityStore) (chan struct{}, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	releases := make([]chan struct{}, 0, 2)
	for range 2 {
		select {
		case release := <-store.inspectionEntered:
			releases = append(releases, release)
		case <-ctx.Done():
			for _, release := range releases {
				close(release)
			}
			t.Fatal("competing head inspections did not overlap")
		}
	}
	return releases[0], releases[1]
}

func startRevivalAvailabilityHost(t *testing.T, host, home string, route *invokeContinueStaticCommandRoute, store *revivalAvailabilityStore) invokeContinueStartedProcess {
	t.Helper()
	started, err := startInvokeContinuePackageProcessWithEdges(t, host, home, route, serviceedges.Edges{
		WorkerRecordingWriter:        store,
		WorkerRecordingStoreObserver: func(real recordings.WorkerRecordingStore) { store.WorkerRecordingStore = real },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := started.command.stop(); err != nil {
			t.Error(err)
		}
		if err := started.process.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	return started
}

// F-5/F-8 race distinct public callers against one ended source. The winning
// successor stays active while both placements prove refusal without canceling
// it; the same immutable request still rejoins its accepted result.
func TestContinuationHeadCompetingAdmissionPreservesActiveHead(t *testing.T) {
	t.Parallel()
	root, dir := t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	source := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: directCodexSessionOutput("opaque-restart-thread", "initial COMPLETE")})
	active := &t7GatedProviderRunner{}
	active.reset()
	runner := &durableRevivalCommandRunner{source: source, peer: active}
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
	started := startContinuationRestartHost(t, root, host, home, route)
	defer t7ReleaseAndJoin(t, t.Context(), active)()
	path := filepath.Join(dir, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: "active-source-request", workerSessionID: "active-source", dispatchID: "active-source-attempt", workingDirectory: dir, userMessage: "initial input"})
	executeHeadRestartCLI(t, started, home, dir, "worker-sessions", "invoke", "--execution", path)
	winner, accepted := raceCompetingHeadRequests(t, started, home, dir)
	t19AwaitSignal(t, t.Context(), active.started, "winning continuation active")
	before := support.GetJSON[api.WorkerSessionObservation](t, started.baseURL+"/worker-sessions/active-source")
	if before.Revivable == nil || *before.Revivable || before.ContinuationHeadWorkerSessionId == nil || *before.ContinuationHeadWorkerSessionId != accepted.SuccessorWorkerSessionID {
		t.Fatalf("active head capability: %+v", before)
	}
	assertActiveHeadRefusal(t, started, home, dir)
	after := support.GetJSON[api.WorkerSessionObservation](t, started.baseURL+"/worker-sessions/active-source")
	head := support.GetJSON[api.WorkerSessionObservation](t, started.baseURL+"/worker-sessions/"+accepted.SuccessorWorkerSessionID)
	if !reflect.DeepEqual(before, after) || head.State != "RUNNING" || active.CallCount() != 1 || source.CallCount() != 1 {
		t.Fatalf("refusal mutated/canceled active head: %+v calls=%d/%d", head, source.CallCount(), active.CallCount())
	}
	retry := support.FakeInputs(t.Context(), winner.Input.Args)
	retry.Input.Env, retry.Input.WorkingDirectory = winner.Input.Env, winner.Input.WorkingDirectory
	if err := started.process.Execute(retry.Input); err != nil {
		t.Fatalf("accepted active retry: %v", err)
	}
	if active.CallCount() != 1 {
		t.Fatal("accepted request launched a duplicate")
	}
	close(active.release)
	t19AwaitSignal(t, t.Context(), active.stopped, "active head joined")
}

// M8: distinct local/HTTP head requests compete over an attributed terminal
// source. The loser cannot change the admitted metadata or the running caller.
func TestRequesterCompetingHead(t *testing.T) {
	t.Parallel()
	root, dir, parentDir := t.TempDir(), t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	source := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: directCodexSessionOutput("opaque-restart-thread", "initial COMPLETE")})
	active, parentRunner := &t7GatedProviderRunner{}, &t7GatedProviderRunner{}
	active.reset()
	parentRunner.reset()
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{
		{workingDirectory: dir, runner: &durableRevivalCommandRunner{source: source, peer: active}},
		{workingDirectory: parentDir, runner: parentRunner},
	}}
	started := startContinuationRestartHost(t, root, host, home, route)
	defer t7ReleaseAndJoin(t, t.Context(), active)()
	defer t7ReleaseAndJoin(t, t.Context(), parentRunner)()
	fixture := &invokeContinuePackageFixture{process: started.process, baseURL: started.baseURL, hostDir: host}
	parent := &invokeContinueScenario{fixture: fixture, name: "requester-race-parent", runNumber: 1, workingDirectory: parentDir, homeDirectory: home, providerRunner: parentRunner, session: fixture.openSession(t)}
	child := &invokeContinueScenario{fixture: fixture, name: "requester-race-child", runNumber: 1, workingDirectory: dir, homeDirectory: home, providerRunner: source, session: fixture.openSession(t)}
	defer parent.close(t)
	defer child.close(t)
	parentID := scenarioScopedID(parent, "requester")
	start := t7RemoteCLIInputs(parent, t.Context(), started.baseURL, "invoke", "--execution", requesterExecutionPath(t, parent, parentID), "--async")
	if err := started.process.Execute(start.Input); err != nil {
		t.Fatal(err)
	}
	t19AwaitSignal(t, t.Context(), parentRunner.started, "competing-head requester running")
	parentToken := requesterSourceToken(t, parentRunner, parentID)
	invoke := t7RemoteCLIInputs(child, t.Context(), started.baseURL, "invoke", "--execution", requesterExecutionPath(t, child, "active-source"))
	invoke.Input.Env = append(invoke.Input.Env, "YOU_WORKER_SESSION_ID="+parentID, "YOU_WORKER_SESSION_TOKEN="+parentToken)
	if err := started.process.Execute(invoke.Input); err != nil {
		t.Fatal(err)
	}
	sourceBefore := requesterObservation(t, fixture, child, t.Context(), "active-source")
	parentBefore := requesterObservation(t, fixture, parent, t.Context(), parentID)
	awaitContinuationRestartLogs(t, started, home, dir, "active-source", "opaque-restart-thread")
	winner, accepted := raceCompetingHeadRequests(t, started, home, dir)
	t19AwaitSignal(t, t.Context(), active.started, "attributed winning head active")
	assertActiveHeadRefusal(t, started, home, dir)
	retry := support.FakeInputs(t.Context(), winner.Input.Args)
	retry.Input.Env, retry.Input.WorkingDirectory = winner.Input.Env, winner.Input.WorkingDirectory
	if err := started.process.Execute(retry.Input); err != nil {
		t.Fatal(err)
	}
	assertRequesterHeadRace(t, fixture, parent, child, active, accepted.SuccessorWorkerSessionID, sourceBefore, parentBefore)
	if source.CallCount() != 1 || active.CallCount() != 1 || parentRunner.CallCount() != 1 {
		t.Fatal("competing admission or replay launched an extra provider")
	}
	functionalevidence.Covers(t, "cli/you.worker-sessions.continue", "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.show", "rest/continueWorkerSession")
}

func assertRequesterHeadRace(t *testing.T, fixture *invokeContinuePackageFixture, parent, child *invokeContinueScenario, active *t7GatedProviderRunner, successorID string, source, parentBefore api.WorkerSessionObservation) {
	t.Helper()
	head := requesterObservation(t, fixture, child, t.Context(), successorID)
	if head.State != "RUNNING" || source.Requester == nil || source.Requester.WorkerSessionId == "" ||
		!reflect.DeepEqual(source.Requester, head.Requester) || !reflect.DeepEqual(source.Correlation, head.Correlation) || !reflect.DeepEqual(source.Labels, head.Labels) {
		t.Fatal("competing head lost the exact attributed metadata")
	}
	environment := requesterEnvironment(active.Requests()[0].Env)
	sourceToken := requesterEnvironment(child.providerRunner.Requests()[0].Env)["YOU_WORKER_SESSION_TOKEN"]
	assertRequesterSuccessorEnvironment(t, environment, successorID, source.Requester.WorkerSessionId, sourceToken)
	sourceAfter := requesterObservation(t, fixture, child, t.Context(), "active-source")
	if sourceAfter.SuccessorWorkerSessionId == nil || *sourceAfter.SuccessorWorkerSessionId != successorID ||
		!reflect.DeepEqual(source.Requester, sourceAfter.Requester) || !reflect.DeepEqual(source.Correlation, sourceAfter.Correlation) || !reflect.DeepEqual(source.Labels, sourceAfter.Labels) || source.State != sourceAfter.State || source.AttemptId != sourceAfter.AttemptId {
		t.Fatal("competing admission mutated the source or branched its chain")
	}
	assertRequesterHeadRacePeer(t, fixture, parent, parentBefore)
}

func assertRequesterHeadRacePeer(t *testing.T, fixture *invokeContinuePackageFixture, parent *invokeContinueScenario, parentBefore api.WorkerSessionObservation) {
	t.Helper()
	parentAfter := requesterObservation(t, fixture, parent, t.Context(), parentBefore.WorkerSessionId)
	// The caller's elapsed active time advances while head admission runs.
	if parentBefore.DurationMillis == nil || parentAfter.DurationMillis == nil || *parentAfter.DurationMillis < *parentBefore.DurationMillis {
		t.Fatal("independent requester lost its active clock")
	}
	parentBefore.DurationMillis, parentAfter.DurationMillis = nil, nil
	if !reflect.DeepEqual(parentBefore, parentAfter) {
		t.Fatal("competing continuation mutated the independent requester")
	}
}

func raceCompetingHeadRequests(t *testing.T, started invokeContinueStartedProcess, home, dir string) (*support.CapturedInputs, directWorkerSessionCLIResult) {
	t.Helper()
	requests := make([]*support.CapturedInputs, 2)
	barrier, done := make(chan struct{}), make(chan error, 2)
	for index := range requests {
		name := []string{"one", "two"}[index]
		flags := []string{"you", "--json"}
		if index == 1 {
			flags = append(flags, "--remote", "--server", started.baseURL)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel() // Accepted successors remain active after callers leave.
		request := support.FakeInputs(ctx, append(flags, "worker-sessions", "continue", "active-source", "--head", "--request-id", "competing-"+name, "--successor-worker-session-id", "active-"+name, "--user-message", "durable occupied peer input", "--async"))
		request.Input.Env, request.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		requests[index] = request
		go func() { <-barrier; done <- started.process.Execute(request.Input) }()
	}
	close(barrier)
	for range requests {
		<-done
	}
	var winner *support.CapturedInputs
	var accepted directWorkerSessionCLIResult
	for _, request := range requests {
		if strings.Contains(request.Stderr(), "WORKER_SESSION_CONTINUATION_CONFLICT") {
			assertDirectWorkerSessionCLIError(t, request, "WORKER_SESSION_CONTINUATION_CONFLICT")
			continue
		}
		if winner != nil {
			t.Fatal("competing callers both admitted")
		}
		winner = request
		decodeDirectWorkerSessionResult(t, request.Stdout(), &accepted)
		if !accepted.Accepted {
			t.Fatalf("winning admission: %s %s", request.Stdout(), request.Stderr())
		}
	}
	if winner == nil {
		t.Fatal("neither competing caller admitted")
	}
	return winner, accepted
}

func assertActiveHeadRefusal(t *testing.T, started invokeContinueStartedProcess, home, dir string) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		flags := []string{"you", "--json"}
		if remote {
			flags = append(flags, "--remote", "--server", started.baseURL)
		}
		denied := support.FakeInputs(t.Context(), append(flags, "worker-sessions", "continue", "active-source", "--head", "--request-id", "active-denied", "--successor-worker-session-id", "active-denied-successor", "--user-message", "refused", "--async"))
		denied.Input.Env, denied.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
		if err := started.process.Execute(denied.Input); err == nil {
			t.Fatal("active head admitted another successor")
		}
		assertDirectWorkerSessionCLIError(t, denied, "WORKER_SESSION_CONTINUATION_CONFLICT")
	}
	status, body := t7HTTP(t, t.Context(), http.MethodPost, started.baseURL+"/worker-sessions/active-source/continue", map[string]any{"resolveHead": true, "requestId": "active-http-denied", "successorWorkerSessionId": "active-http-successor", "followUpInput": "refused"})
	if status != http.StatusConflict || !strings.Contains(body, "WORKER_SESSION_CONTINUATION_CONFLICT") {
		t.Fatalf("active HTTP refusal: %d %s", status, body)
	}
}

// M1: a real Factory output has an exact producing dispatch. The child
// observes the producer identity and Project Work via public show/list and the
// controlled command edge, independently of the producer's terminal lifetime.
func TestRequesterFactoryProducingDispatch(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.continue", "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.list", "cli/you.worker-sessions.read", "cli/you.worker-sessions.show", "rest/moveWorkBySessionId")
	runRequesterFactoryProducingDispatch(t, false)
}

func TestRequesterFactoryGeneratedBatch(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.continue", "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.list", "cli/you.worker-sessions.read", "cli/you.worker-sessions.show", "rest/moveWorkBySessionId")
	runRequesterFactoryProducingDispatch(t, true)
}

func runRequesterFactoryProducingDispatch(t *testing.T, batch bool) {
	fixture := ensureInvokeContinuePackageFixture(t)
	leadName, laneName := "requester-factory-lead", "requester-factory-lane"
	if batch {
		leadName, laneName = "requester-batch-lead", "requester-batch-lane"
	}
	lead, lane := fixture.scenario(t, leadName), fixture.scenario(t, laneName)
	defer lead.close(t)
	defer lane.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := lane.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	writeRequesterFactoryLineage(t, lead.workingDirectory, lane.workingDirectory)
	if batch {
		writeRequesterBatchCompletion(t, lead.workingDirectory)
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, lead.workingDirectory)
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	projectName := "requester-project"
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, opened.Session.Id, api.SubmitWorkRequest{Name: &projectName, WorkTypeName: "project", Payload: "project input", Tags: &api.StringMap{"project": projectName}})
	if submitted.WorkId == nil {
		t.Fatal("Project Work has no ID")
	}
	support.SubmitSessionWorkAt(t, fixture.baseURL, opened.Session.Id, api.SubmitWorkRequest{WorkTypeName: "seed", Payload: "lead input", Tags: &api.StringMap{"project": projectName}})
	t19AwaitSignal(t, ctx, runner.started, "produced lane started")

	assertRequesterProducedLineage(t, fixture, lead, lane, ctx, opened.Session.Id, *submitted.WorkId)
	environment := requesterEnvironment(runner.Requests()[0].Env)
	source := requesterObservation(t, fixture, lane, ctx, environment["YOU_WORKER_SESSION_ID"])
	assertRequesterProducedContinuation(t, fixture, lead, lane, ctx, opened.Session.Id)
	assertRequesterProducedRedispatch(t, fixture, lead, lane, ctx, source, environment)
}

// The batch producer completes its seed without also propagating a task.
func writeRequesterBatchCompletion(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	seed := document["workTypes"].([]any)[0].(map[string]any)
	seed["states"] = append(seed["states"].([]any), map[string]any{"name": "done", "type": "TERMINAL"})
	station := document["workstations"].([]any)[0].(map[string]any)
	station["outputs"] = []any{map[string]any{"workType": "seed", "state": "done"}}
	writeInvokeContinueJSON(t, path, document)
}

// Factory-origin continuation is a direct execution. It keeps the original
// producing requester and Factory correlation without returning a Work result.
func assertRequesterProducedContinuation(t *testing.T, fixture *invokeContinuePackageFixture, lead, lane *invokeContinueScenario, ctx context.Context, sessionID string) {
	t.Helper()
	runner := lane.providerRunner.(*t7GatedProviderRunner)
	environment := requesterEnvironment(runner.Requests()[0].Env)
	sourceID := environment["YOU_WORKER_SESSION_ID"]
	t7ReleaseAndJoin(t, ctx, runner)()
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, lane.homeDirectory, lane.workingDirectory, sourceID, "requester-lineage-thread")
	source := requesterObservation(t, fixture, lane, ctx, sourceID)
	producer := requesterObservation(t, fixture, lead, ctx, environment["YOU_MESSAGE_TARGET"])
	if source.State != "COMPLETED" || source.Revivable == nil || !*source.Revivable {
		t.Fatal("produced Factory source did not retain continuation authority")
	}
	workURL := support.SessionWorkURL(fixture.baseURL, sessionID, "/work/"+*source.Correlation.WorkId)
	workBefore, err := support.WaitForObservation(30*time.Second, func() (api.Work, error) {
		return support.GetJSON[api.Work](t, workURL), nil
	}, func(item api.Work) bool { return support.WorkItemCustomerLocation(item) == "task:done" })
	if err != nil {
		t.Fatalf("produced Work did not complete: %v", err)
	}
	eventsBefore := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	assertRequesterRefusal(t, fixture, lane, ctx, "produced-ended", sourceID, environment["YOU_WORKER_SESSION_TOKEN"])
	runner.reset() // The joined source no longer uses the scenario's command gates.
	successorID := scenarioScopedID(lane, "produced-successor")
	request := t7RemoteCLIInputs(lane, ctx, fixture.baseURL, "continue", sourceID, "--head", "--request-id", successorID+"-request", "--successor-worker-session-id", successorID, "--user-message", "produced lane follow-up", "--async")
	if err := fixture.process.Execute(request.Input); err != nil {
		t.Fatalf("produced continuation: %v: %s", err, request.Stderr())
	}
	t19AwaitSignal(t, ctx, runner.started, "produced direct successor running")
	successor := requesterObservation(t, fixture, lane, ctx, successorID)
	assertRequesterCopiedMetadata(t, source, successor)
	assertRequesterFactoryListed(t, fixture, lane, ctx, *source.Correlation.WorkId, sessionID, successor)
	successorEnv := requesterEnvironment(runner.Requests()[0].Env)
	assertRequesterSuccessorEnvironment(t, successorEnv, successorID, producer.WorkerSessionId, environment["YOU_WORKER_SESSION_TOKEN"])
	assertRequesterEndpoint(t, successorEnv, fixture.baseURL)
	for _, key := range []string{"YOU_MESSAGE_TARGET_WORK_ID", "YOU_WORK_ID", "YOU_FACTORY_SESSION_ID"} {
		if successorEnv[key] != environment[key] {
			t.Fatalf("produced continuation changed %s", key)
		}
	}
	t7ReleaseAndJoin(t, ctx, runner)()
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, lane.homeDirectory, lane.workingDirectory, successorID, "requester-lineage-thread")
	after := requesterObservation(t, fixture, lane, ctx, sourceID)
	assertRequesterCopiedMetadata(t, source, after)
	assertRequesterProducedIndependence(t, source, after, successorID, producer, requesterObservation(t, fixture, lead, ctx, producer.WorkerSessionId), lead.providerRunner.CallCount(), runner.CallCount())
	if !reflect.DeepEqual(workBefore, support.GetJSON[api.Work](t, workURL)) || !reflect.DeepEqual(eventsBefore, support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)) {
		t.Fatal("direct continuation changed canonical Work or ordered Factory Events")
	}
}

func assertRequesterProducedIndependence(t *testing.T, source, after api.WorkerSessionObservation, successorID string, producer, producerAfter api.WorkerSessionObservation, producerCalls, successorCalls int) {
	t.Helper()
	if after.State != source.State || after.ContinuationHeadWorkerSessionId == nil || *after.ContinuationHeadWorkerSessionId != successorID {
		t.Fatal("direct continuation changed its source state or lost the exact head")
	}
	if !reflect.DeepEqual(producer, producerAfter) || producerCalls != 1 || successorCalls != 1 {
		t.Fatal("direct continuation changed its producer or admitted duplicate execution")
	}
}

func assertRequesterCopiedMetadata(t *testing.T, source, successor api.WorkerSessionObservation) {
	t.Helper()
	if !reflect.DeepEqual(source.Requester, successor.Requester) || !reflect.DeepEqual(source.Correlation, successor.Correlation) || !reflect.DeepEqual(source.Labels, successor.Labels) {
		t.Fatal("produced continuation lost retained requester/correlation/labels")
	}
}

func assertRequesterProducedLineage(t *testing.T, fixture *invokeContinuePackageFixture, lead, lane *invokeContinueScenario, ctx context.Context, sessionID, projectWorkID string) {
	t.Helper()
	runner := lane.providerRunner.(*t7GatedProviderRunner)

	leadEnvironment := requesterEnvironment(lead.providerRunner.Requests()[0].Env)
	producer := support.GetJSON[api.WorkerSessionObservation](t, fixture.baseURL+"/worker-sessions/"+leadEnvironment["YOU_WORKER_SESSION_ID"])

	environment := requesterEnvironment(runner.Requests()[0].Env)
	childID := environment["YOU_WORKER_SESSION_ID"]
	child := support.GetJSON[api.WorkerSessionObservation](t, fixture.baseURL+"/worker-sessions/"+childID)
	if child.Correlation == nil || child.Correlation.WorkId == nil {
		t.Fatal("produced Work correlation absent")
	}

	observation := requesterObservation(t, fixture, lane, ctx, child.WorkerSessionId)
	if observation.Requester == nil || observation.Requester.WorkerSessionId != producer.WorkerSessionId || observation.Requester.WorkId == nil || *observation.Requester.WorkId != projectWorkID || !requesterCorrelationAgrees(observation, *child.Correlation.WorkId, sessionID) {
		metadata, _ := json.Marshal(map[string]any{"childCorrelation": observation.Correlation, "childRequester": observation.Requester, "producerCorrelation": producer.Correlation, "producerID": producer.WorkerSessionId, "expectedProject": projectWorkID, "expectedFactory": sessionID})
		t.Fatalf("produced requester mismatch: %s", metadata)
	}
	if !reflect.DeepEqual(child.Requester, observation.Requester) || !reflect.DeepEqual(child.Labels, observation.Labels) {
		t.Fatal("show/list disagree on producing requester")
	}
	assertRequesterFactoryListed(t, fixture, lane, ctx, *child.Correlation.WorkId, sessionID, observation)
	assertRequesterProducedEnvironment(t, fixture, lead, runner, producer, child, environment, sessionID, projectWorkID)
}

func assertRequesterProducedEnvironment(t *testing.T, fixture *invokeContinuePackageFixture, lead *invokeContinueScenario, runner *t7GatedProviderRunner, producer, child api.WorkerSessionObservation, environment map[string]string, sessionID, projectWorkID string) {
	t.Helper()
	assertRequesterSuccessorEnvironment(t, environment, child.WorkerSessionId, producer.WorkerSessionId, requesterEnvironment(lead.providerRunner.Requests()[0].Env)["YOU_WORKER_SESSION_TOKEN"])
	assertRequesterEndpoint(t, environment, fixture.baseURL)
	if environment["YOU_MESSAGE_TARGET"] != producer.WorkerSessionId || environment["YOU_MESSAGE_TARGET_WORK_ID"] != projectWorkID || environment["YOU_WORK_ID"] != *child.Correlation.WorkId || environment["YOU_FACTORY_SESSION_ID"] != sessionID {
		t.Fatal("controlled runner disagrees with verified producing lineage")
	}
	if producer.Requester != nil || lead.providerRunner.CallCount() != 1 || runner.CallCount() != 1 {
		t.Fatal("root attribution or canonical attempt counts changed")
	}
}

func writeRequesterFactoryLineage(t *testing.T, root, lane string) {
	t.Helper()
	t7WriteFactorySibling(t, root)
	document := map[string]any{
		"name": "requester-lineage", "workTypes": []any{map[string]any{"name": "seed", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "failed", "type": "FAILED"}}},
			map[string]any{"name": "project", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "done", "type": "TERMINAL"}, map[string]any{"name": "failed", "type": "FAILED"}}},
			map[string]any{"name": "task", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "done", "type": "TERMINAL"}, map[string]any{"name": "failed", "type": "FAILED"}}},
		}, "workers": []any{map[string]any{"name": "worker"}},
		"workstations": []any{
			map[string]any{"name": "lead", "worker": "worker", "inputs": []any{map[string]any{"workType": "seed", "state": "init"}}, "workPropagation": map[string]any{"mode": "PRESERVE_INPUT"}, "outputs": []any{map[string]any{"workType": "task", "state": "init"}}, "onFailure": []any{map[string]any{"workType": "seed", "state": "failed"}}},
			map[string]any{"name": "process", "worker": "worker", "inputs": []any{map[string]any{"workType": "task", "state": "init"}}, "outputs": []any{map[string]any{"workType": "task", "state": "done"}}, "onFailure": []any{map[string]any{"workType": "task", "state": "failed"}}},
		},
	}
	writeInvokeContinueJSON(t, filepath.Join(root, "factory.json"), document)
	for name, dir := range map[string]string{"lead": root, "process": lane} {
		path := filepath.Join(root, "workstations", name)
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(dir)
		station := fmt.Sprintf("---\ntype: MODEL_WORKSTATION\nrunner: codex\nworkingDirectory: %s\n---\nRequester lineage {{ (index .Inputs 0).Payload }}\n", encoded)
		if err := os.WriteFile(filepath.Join(path, "AGENTS.md"), []byte(station), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
