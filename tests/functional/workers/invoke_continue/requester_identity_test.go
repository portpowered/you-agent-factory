package acceptance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// M2: revisiting the same produced Work recomputes lineage from its original
// admission, rather than making its last physical attempt the requester.
func assertRequesterProducedRedispatch(t *testing.T, fixture *invokeContinuePackageFixture, lead, lane *invokeContinueScenario, ctx context.Context, source api.WorkerSessionObservation, originalEnv map[string]string) {
	t.Helper()
	runner := lane.providerRunner.(*t7GatedProviderRunner)
	previousEnv := requesterEnvironment(runner.Requests()[0].Env)
	previous := requesterObservation(t, fixture, lane, ctx, previousEnv["YOU_WORKER_SESSION_ID"])
	sourceBefore := requesterObservation(t, fixture, lane, ctx, source.WorkerSessionId)
	producer := requesterObservation(t, fixture, lead, ctx, source.Requester.WorkerSessionId)
	runner.reset() // The direct continuation has joined before the next visit.
	workURL := support.SessionWorkURL(fixture.baseURL, *source.Correlation.FactorySessionId, "/work/"+*source.Correlation.WorkId)
	status, body := t7HTTP(t, ctx, http.MethodPost, workURL+"/move", map[string]any{"stateName": "init", "requestId": source.WorkerSessionId + "-revisit"})
	if status != http.StatusOK {
		t.Fatalf("produced Work revisit: %d %s", status, body)
	}
	t19AwaitSignal(t, ctx, runner.started, "produced Work redispatched")
	environment := requesterEnvironment(runner.Requests()[0].Env)
	id := environment["YOU_WORKER_SESSION_ID"]
	if id == source.WorkerSessionId || id == previous.WorkerSessionId {
		t.Fatal("Work revisit reused a prior physical identity")
	}
	current := requesterObservation(t, fixture, lane, ctx, id)
	if current.State != "RUNNING" || current.AttemptId == source.AttemptId {
		t.Fatal("Work revisit did not admit a new running dispatch/attempt")
	}
	assertRequesterCopiedMetadata(t, source, current)
	assertRequesterFactoryListed(t, fixture, lane, ctx, *source.Correlation.WorkId, *source.Correlation.FactorySessionId, current)
	for _, prior := range []map[string]string{originalEnv, previousEnv} {
		assertRequesterSuccessorEnvironment(t, environment, id, producer.WorkerSessionId, prior["YOU_WORKER_SESSION_TOKEN"])
	}
	assertRequesterEndpoint(t, environment, fixture.baseURL)
	for _, key := range []string{"YOU_MESSAGE_TARGET_WORK_ID", "YOU_WORK_ID", "YOU_FACTORY_SESSION_ID"} {
		if environment[key] != originalEnv[key] {
			t.Fatalf("Work revisit changed %s", key)
		}
	}
	assertRequesterRefusal(t, fixture, lane, ctx, "redispatch-source", source.WorkerSessionId, originalEnv["YOU_WORKER_SESSION_TOKEN"])
	assertRequesterRefusal(t, fixture, lane, ctx, "redispatch-direct-head", previous.WorkerSessionId, previousEnv["YOU_WORKER_SESSION_TOKEN"])
	if !reflect.DeepEqual(sourceBefore, requesterObservation(t, fixture, lane, ctx, source.WorkerSessionId)) {
		t.Fatal("Work revisit mutated its original physical source or continuation head")
	}
	if !reflect.DeepEqual(previous, requesterObservation(t, fixture, lane, ctx, previous.WorkerSessionId)) ||
		!reflect.DeepEqual(producer, requesterObservation(t, fixture, lead, ctx, producer.WorkerSessionId)) || lead.providerRunner.CallCount() != 1 || runner.CallCount() != 1 {
		t.Fatal("Work revisit mutated the direct chain/producer or launched duplicate execution")
	}
	t7ReleaseAndJoin(t, ctx, runner)()
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, lane.homeDirectory, lane.workingDirectory, id, "requester-lineage-thread")
	_, err := support.WaitForObservation(30*time.Second, func() (api.Work, error) {
		return support.GetJSON[api.Work](t, workURL), nil
	}, func(item api.Work) bool { return support.WorkItemCustomerLocation(item) == "task:done" })
	if err != nil {
		t.Fatalf("revisited Work did not complete: %v", err)
	}
	assertRequesterDurableTokenPrivacy(t, fixture, ctx, id, environment["YOU_WORKER_SESSION_TOKEN"])
}

// The public CLI forwards credentials learned only at the controlled native
// provider edge. The running source and the child own independent routes and
// explicit Factory Sessions in the reusable process.
func TestRequesterDirectInvokeFromRunningCaller(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	parent := fixture.scenario(t, "requester-parent")
	child := fixture.scenario(t, "requester-child")
	defer parent.close(t)
	defer child.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := parent.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	parentID := scenarioScopedID(parent, "requester-source")
	parentPath := requesterExecutionPath(t, parent, parentID)
	start := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "invoke", "--execution", parentPath, "--async")
	if err := fixture.process.Execute(start.Input); err != nil {
		t.Fatalf("start requester: %v: %s", err, start.Stderr())
	}
	t19AwaitSignal(t, ctx, runner.started, "requester native command running")
	token := requesterSourceToken(t, runner, parentID)
	childID := scenarioScopedID(child, "requester-child")
	path := requesterExecutionPath(t, child, childID)
	parentEnv := requesterEnvironment(runner.Requests()[0].Env)
	assertRequesterEndpoint(t, parentEnv, fixture.baseURL)
	invoke := support.FakeInputs(ctx, []string{"you", "--json", "--remote", "worker-sessions", "invoke", "--execution", path})
	invoke.Input.WorkingDirectory = child.workingDirectory
	invoke.Input.Env = append(child.environment(), "YOU_SERVER="+parentEnv["YOU_SERVER"], "YOU_WORKER_SESSION_ID="+parentID, "YOU_WORKER_SESSION_TOKEN="+token)
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("attributed invoke: %v: %s", err, invoke.Stderr())
	}
	if child.providerRunner.CallCount() != 1 {
		t.Fatal("attributed child did not launch exactly once")
	}
	assertRequesterChild(t, fixture, child, ctx, childID, parentID, token)
	if strings.Contains(invoke.Stdout()+invoke.Stderr(), token) {
		t.Fatal("public invoke exposed the source credential")
	}
	for _, test := range []struct{ name, id, token string }{
		{"invalid", parentID, strings.Repeat("A", 43)},
		{"foreign", childID, token},
		{"partial", parentID, ""},
	} {
		assertRequesterRefusal(t, fixture, child, ctx, test.name, test.id, test.token)
	}
	stop := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "cancel", parentID)
	if err := fixture.process.Execute(stop.Input); err != nil {
		t.Fatalf("end requester: %v: %s", err, stop.Stderr())
	}
	t19AwaitSignal(t, ctx, runner.stopped, "requester native command ended")
	assertRequesterRefusal(t, fixture, child, ctx, "ended", parentID, token)
	if child.providerRunner.CallCount() != 1 || runner.CallCount() != 1 {
		t.Fatal("refused callers launched or changed a provider attempt")
	}
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.show", "rest/startWorkerSession")
}

// M7 requires a separate root lifetime: both hosts use the same isolated
// profile and real capture storage; only the native provider process is fake.
func TestRequesterContinuationAfterHostRestart(t *testing.T) {
	t.Parallel()
	root, parentDir, childDir := t.TempDir(), t.TempDir(), t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	parentRunner := &t7GatedProviderRunner{}
	parentRunner.reset()
	result := platformprocess.CommandResult{Stdout: directCodexSessionOutput("t7-thread-requester-child", t7ObservationReport)}
	childRunner := testutil.NewProviderCommandRunner(result, result, result, result)
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{
		{workingDirectory: parentDir, runner: parentRunner}, {workingDirectory: childDir, runner: childRunner},
	}}
	first := startContinuationRestartHost(t, root, host, home, route)
	fixture := &invokeContinuePackageFixture{process: first.process, baseURL: first.baseURL, hostDir: host}
	parent := &invokeContinueScenario{fixture: fixture, name: "requester-restart-parent", runNumber: 1, workingDirectory: parentDir, homeDirectory: home, providerRunner: parentRunner, session: fixture.openSession(t)}
	child := &invokeContinueScenario{fixture: fixture, name: "requester-restart-child", runNumber: 1, workingDirectory: childDir, homeDirectory: home, providerRunner: childRunner, session: fixture.openSession(t)}
	defer t7ReleaseAndJoin(t, t.Context(), parentRunner)()
	parentID, sourceID := scenarioScopedID(parent, "requester-source"), scenarioScopedID(child, "requester-child")
	start := t7RemoteCLIInputs(parent, t.Context(), first.baseURL, "invoke", "--execution", requesterExecutionPath(t, parent, parentID), "--async")
	if err := first.process.Execute(start.Input); err != nil {
		t.Fatal(err)
	}
	t19AwaitSignal(t, t.Context(), parentRunner.started, "restart caller running")
	parentToken := requesterSourceToken(t, parentRunner, parentID)
	invoke := t7RemoteCLIInputs(child, t.Context(), first.baseURL, "invoke", "--execution", requesterExecutionPath(t, child, sourceID))
	invoke.Input.Env = append(invoke.Input.Env, "YOU_WORKER_SESSION_ID="+parentID, "YOU_WORKER_SESSION_TOKEN="+parentToken)
	if err := first.process.Execute(invoke.Input); err != nil {
		t.Fatal(err)
	}
	assertRequesterChild(t, fixture, child, t.Context(), sourceID, parentID, parentToken)
	assertRequesterContinuations(t, fixture, parent, child, t.Context(), sourceID, parentID)
	before := requesterObservation(t, fixture, child, t.Context(), sourceID)
	awaitContinuationRestartLogs(t, first, home, childDir, scenarioScopedID(child, "requester-head"), "t7-thread-requester-child")
	parent.close(t)
	child.close(t)
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	fixture.process, fixture.baseURL = fresh.process, fresh.baseURL
	child.session = fixture.openSession(t)
	defer child.close(t)
	assertRequesterRestoredChain(t, fixture, child, sourceID, parentID, parentToken, before)
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.show", "cli/you.worker-sessions.continue", "rest/startWorkerSession")
}

// M7 retains real Factory-produced requester and Project Work facts across a
// root lifetime, rather than substituting direct invocation's parent metadata.
func TestRequesterProducedContinuationAfterHostRestart(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.continue", "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.list", "cli/you.worker-sessions.read", "cli/you.worker-sessions.show")
	for _, origin := range []string{"dispatch-output", "generated-batch"} {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()
			runRequesterProducedRestart(t, origin == "generated-batch")
		})
	}
}

func runRequesterProducedRestart(t *testing.T, batch bool) {
	root := t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	leadDir, laneDir := filepath.Join(root, "lead"), filepath.Join(root, "requester-factory-lane-restart")
	if err := os.MkdirAll(laneDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeRequesterFactoryLineage(t, leadDir, laneDir)
	output := "requester produced COMPLETE"
	if batch {
		writeRequesterBatchCompletion(t, leadDir)
		output = `{"completion":"COMPLETE","request":{"requestId":"restart-batch","type":"FACTORY_REQUEST_BATCH","works":[{"name":"restart-lane","workId":"restart-lane","workTypeName":"task","payload":"lane input","tags":{"project":"requester-project"}}]},"metadata":{"source":"forged-source","producingDispatchID":"forged-dispatch"}}`
	}
	producer := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: directCodexOutputWithoutSession(output)})
	runner := &t7GatedProviderRunner{}
	runner.reset()
	defer t7ReleaseAndJoin(t, t.Context(), runner)()
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: leadDir, runner: producer}, {workingDirectory: laneDir, runner: runner}}}
	first := startContinuationRestartHost(t, root, host, home, route)
	fixture := &invokeContinuePackageFixture{process: first.process, baseURL: first.baseURL, hostDir: host}
	lead := &invokeContinueScenario{fixture: fixture, name: "restart-lead", runNumber: 1, workingDirectory: leadDir, homeDirectory: home, providerRunner: producer}
	lane := &invokeContinueScenario{fixture: fixture, name: "restart-lane", runNumber: 1, workingDirectory: laneDir, homeDirectory: home, providerRunner: runner}
	opened := support.OpenFactorySessionAt(t, first.baseURL, leadDir)
	projectName := "requester-project"
	project := support.SubmitSessionWorkAt(t, first.baseURL, opened.Session.Id, api.SubmitWorkRequest{Name: &projectName, WorkTypeName: "project", Payload: "project input", Tags: &api.StringMap{"project": projectName}})
	if project.WorkId == nil {
		t.Fatal("Project Work has no exact ID")
	}
	support.SubmitSessionWorkAt(t, first.baseURL, opened.Session.Id, api.SubmitWorkRequest{WorkTypeName: "seed", Payload: "lead input", Tags: &api.StringMap{"project": projectName}})
	t19AwaitSignal(t, t.Context(), runner.started, "restart produced lane running")
	assertRequesterProducedLineage(t, fixture, lead, lane, t.Context(), opened.Session.Id, *project.WorkId)
	sourceEnv := requesterEnvironment(runner.Requests()[0].Env)
	assertRequesterProducedContinuation(t, fixture, lead, lane, t.Context(), opened.Session.Id)
	headEnv := requesterEnvironment(runner.Requests()[0].Env)
	before := requesterObservation(t, fixture, lane, t.Context(), sourceEnv["YOU_WORKER_SESSION_ID"])
	support.CloseFactorySessionAt(t, first.baseURL, opened.Session.Id)
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	fixture.process, fixture.baseURL = fresh.process, fresh.baseURL
	assertRequesterProducedRestoration(t, fixture, lane, runner, before, sourceEnv, headEnv)
	if producer.CallCount() != 1 {
		t.Fatal("reconstruction or direct revival redispatched the producer")
	}
}

func assertRequesterProducedRestoration(t *testing.T, fixture *invokeContinuePackageFixture, lane *invokeContinueScenario, runner *t7GatedProviderRunner, before api.WorkerSessionObservation, sourceEnv, headEnv map[string]string) {
	t.Helper()
	show := t7RemoteCLIInputs(lane, t.Context(), fixture.baseURL, "show", "--worker-session-id", before.WorkerSessionId, "--session", *before.Correlation.FactorySessionId)
	if err := fixture.process.Execute(show.Input); err != nil {
		t.Fatalf("restored produced source: %v: %s", err, show.Stderr())
	}
	var after api.WorkerSessionObservation
	decodeDirectWorkerSessionResult(t, show.Stdout(), &after)
	assertRequesterTokenAbsent(t, sourceEnv["YOU_WORKER_SESSION_TOKEN"], show.Stdout()+show.Stderr())
	assertRequesterTokenAbsent(t, headEnv["YOU_WORKER_SESSION_TOKEN"], show.Stdout()+show.Stderr())
	assertRequesterCopiedMetadata(t, before, after)
	if after.State != "COMPLETED" || after.ContinuationHeadWorkerSessionId == nil || *after.ContinuationHeadWorkerSessionId != headEnv["YOU_WORKER_SESSION_ID"] {
		t.Fatal("reconstruction lost the produced source's terminal chain head")
	}
	for index, env := range []map[string]string{sourceEnv, headEnv} {
		assertRequesterRefusal(t, fixture, lane, t.Context(), "old-produced-"+string(rune('a'+index)), env["YOU_WORKER_SESSION_ID"], env["YOU_WORKER_SESSION_TOKEN"])
		assertRequesterDurableTokenPrivacy(t, fixture, t.Context(), env["YOU_WORKER_SESSION_ID"], env["YOU_WORKER_SESSION_TOKEN"])
	}
	runner.reset()
	id := scenarioScopedID(lane, "produced-rebuilt")
	args := []string{"continue", before.WorkerSessionId, "--session", *before.Correlation.FactorySessionId, "--head", "--request-id", id + "-request", "--successor-worker-session-id", id, "--user-message", "produced restart follow-up", "--async"}
	request := t7RemoteCLIInputs(lane, t.Context(), fixture.baseURL, args...)
	if err := fixture.process.Execute(request.Input); err != nil {
		t.Fatalf("restored produced continuation: %v: %s", err, request.Stderr())
	}
	var admitted directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, request.Stdout(), &admitted)
	if admitted.SourceWorkerSessionID != headEnv["YOU_WORKER_SESSION_ID"] || admitted.SuccessorWorkerSessionID != id {
		t.Fatal("restored continuation did not resolve the exact original source head")
	}
	t19AwaitSignal(t, t.Context(), runner.started, "restored produced successor running")
	show = t7RemoteCLIInputs(lane, t.Context(), fixture.baseURL, "show", "--worker-session-id", id, "--session", *before.Correlation.FactorySessionId)
	if err := fixture.process.Execute(show.Input); err != nil {
		t.Fatalf("restored produced successor: %v: %s", err, show.Stderr())
	}
	var successor api.WorkerSessionObservation
	decodeDirectWorkerSessionResult(t, show.Stdout(), &successor)
	assertRequesterCopiedMetadata(t, before, successor)
	assertRequesterRestoredList(t, lane, successor)
	command := runner.Requests()[0]
	assertRequesterProducedResumeCommand(t, command, lane.workingDirectory)
	environment := requesterEnvironment(command.Env)
	assertRequesterTokenAbsent(t, environment["YOU_WORKER_SESSION_TOKEN"], show.Stdout()+show.Stderr())
	assertRequesterEndpoint(t, environment, fixture.baseURL)
	for _, previous := range []map[string]string{sourceEnv, headEnv} {
		assertRequesterSuccessorEnvironment(t, environment, id, before.Requester.WorkerSessionId, previous["YOU_WORKER_SESSION_TOKEN"])
		for _, key := range []string{"YOU_MESSAGE_TARGET_WORK_ID", "YOU_WORK_ID", "YOU_FACTORY_SESSION_ID"} {
			if environment[key] != previous[key] {
				t.Fatalf("reconstructed produced continuation changed %s", key)
			}
		}
	}
	t7ReleaseAndJoin(t, t.Context(), runner)()
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, lane.homeDirectory, lane.workingDirectory, id, "requester-lineage-thread")
	assertRequesterDurableTokenPrivacy(t, fixture, t.Context(), id, environment["YOU_WORKER_SESSION_TOKEN"])
	assertRequesterContinuationReplay(t, lane, t.Context(), args[:len(args)-1], headEnv["YOU_WORKER_SESSION_ID"], id, 1)
}

func assertRequesterProducedResumeCommand(t *testing.T, command platformprocess.CommandRequest, workingDirectory string) {
	t.Helper()
	args := strings.Join(command.Args, " ")
	if command.WorkDir != workingDirectory || !strings.Contains(args, "resume requester-lineage-thread") || !strings.Contains(args+string(command.Stdin), "produced restart follow-up") {
		t.Fatal("reconstruction replaced the exact provider continuation or follow-up")
	}
}

func assertRequesterRestoredList(t *testing.T, lane *invokeContinueScenario, expected api.WorkerSessionObservation) {
	t.Helper()
	args := []string{"list", "--scope", "all", "--history", "active"}
	seen := make(map[string]bool)
	for {
		input := t7RemoteCLIInputs(lane, t.Context(), lane.fixture.baseURL, args...)
		if err := lane.fixture.process.Execute(input.Input); err != nil {
			t.Fatalf("restored successor list: %v: %s", err, input.Stderr())
		}
		var page api.ListWorkerSessionsResponse
		decodeDirectWorkerSessionResult(t, input.Stdout(), &page)
		for _, row := range page.Sessions {
			if row.WorkerSessionId == expected.WorkerSessionId {
				assertRequesterCopiedMetadata(t, expected, row)
				return
			}
		}
		if page.PaginationContext == nil || page.PaginationContext.NextToken == nil || *page.PaginationContext.NextToken == "" || seen[*page.PaginationContext.NextToken] {
			t.Fatal("restored successor absent from public fleet or cursor repeated")
		}
		next := *page.PaginationContext.NextToken
		seen[next] = true
		args = []string{"list", "--scope", "all", "--history", "active", "--next-token", next}
	}
}

func assertRequesterRestoredChain(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, sourceID, parentID, parentToken string, before api.WorkerSessionObservation) {
	t.Helper()
	after := requesterObservation(t, fixture, child, t.Context(), sourceID)
	if !reflect.DeepEqual(before.Requester, after.Requester) || !reflect.DeepEqual(before.Correlation, after.Correlation) || !reflect.DeepEqual(before.Labels, after.Labels) || after.ContinuationHeadWorkerSessionId == nil || *after.ContinuationHeadWorkerSessionId != scenarioScopedID(child, "requester-head") {
		t.Fatal("reconstruction lost retained metadata or the exact chain head")
	}
	assertRequesterRefusal(t, fixture, child, t.Context(), "restart-parent", parentID, parentToken)
	for index, request := range child.providerRunner.Requests() {
		environment := requesterEnvironment(request.Env)
		assertRequesterRefusal(t, fixture, child, t.Context(), "restart-token-"+string(rune('a'+index)), environment["YOU_WORKER_SESSION_ID"], environment["YOU_WORKER_SESSION_TOKEN"])
		assertRequesterDurableTokenPrivacy(t, fixture, t.Context(), environment["YOU_WORKER_SESSION_ID"], environment["YOU_WORKER_SESSION_TOKEN"])
	}
	successorID := scenarioScopedID(child, "requester-rebuilt")
	input := t7RemoteCLIInputs(child, t.Context(), fixture.baseURL, "continue", sourceID, "--head", "--request-id", successorID+"-request", "--successor-worker-session-id", successorID, "--user-message", "requester rebuilt follow-up")
	if err := fixture.process.Execute(input.Input); err != nil {
		t.Fatalf("restored requester continuation: %v: %s", err, input.Stderr())
	}
	var continued directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, input.Stdout(), &continued)
	if continued.SourceWorkerSessionID != scenarioScopedID(child, "requester-head") || continued.State != "COMPLETED" || child.providerRunner.CallCount() != 4 {
		t.Fatal("reconstructed continuation did not advance the exact retained head once")
	}
	requests := child.providerRunner.Requests()
	environment := requesterEnvironment(requests[3].Env)
	assertRequesterEndpoint(t, environment, fixture.baseURL)
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, child.homeDirectory, child.workingDirectory, successorID, "t7-thread-requester-child")
	assertRequesterDurableTokenPrivacy(t, fixture, t.Context(), successorID, environment["YOU_WORKER_SESSION_TOKEN"])
	for _, request := range requests[:3] {
		assertRequesterSuccessorEnvironment(t, environment, successorID, parentID, requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"])
	}
	restored := requesterObservation(t, fixture, child, t.Context(), successorID)
	if !reflect.DeepEqual(before.Requester, restored.Requester) || !reflect.DeepEqual(before.Correlation, restored.Correlation) || !reflect.DeepEqual(before.Labels, restored.Labels) {
		t.Fatal("rebuilt successor lost retained requester metadata")
	}
}

func requesterSourceToken(t *testing.T, runner *t7GatedProviderRunner, parentID string) string {
	t.Helper()
	parentEnv := requesterEnvironment(runner.Requests()[0].Env)
	if parentEnv["YOU_WORKER_SESSION_ID"] != parentID || parentEnv["YOU_MESSAGE_TARGET"] != "" {
		t.Fatal("unattributed source identity was incorrect")
	}
	token := parentEnv["YOU_WORKER_SESSION_TOKEN"]
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != 32 {
		t.Fatal("source did not receive a 32-byte caller credential")
	}
	return token
}

func assertRequesterChild(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, childID, parentID, token string) {
	t.Helper()
	childEnv := requesterEnvironment(child.providerRunner.Requests()[0].Env)
	assertRequesterEndpoint(t, childEnv, fixture.baseURL)
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(childEnv["YOU_WORKER_SESSION_TOKEN"])
	if err != nil || len(decoded) != 32 {
		t.Fatal("child did not receive a fresh 32-byte caller credential")
	}
	if childEnv["YOU_WORKER_SESSION_ID"] != childID || childEnv["YOU_MESSAGE_TARGET"] != parentID || childEnv["YOU_WORKER_SESSION_TOKEN"] == token {
		t.Fatal("child did not receive its own fresh identity and exact caller target")
	}
	show := t7RemoteCLIInputs(child, ctx, fixture.baseURL, "show", "--worker-session-id", childID)
	if err := fixture.process.Execute(show.Input); err != nil {
		t.Fatal(err)
	}
	var observation api.WorkerSessionObservation
	if err := json.Unmarshal([]byte(show.Stdout()), &observation); err != nil {
		t.Fatal(err)
	}
	if observation.Requester == nil || observation.Requester.WorkerSessionId != parentID || observation.Labels == nil || !reflect.DeepEqual(*observation.Labels, []string{"parent:" + parentID}) {
		t.Fatalf("public child metadata disagrees with verified caller identity: requester=%+v labels=%v state=%s", observation.Requester, observation.Labels, observation.State)
	}
	if strings.Contains(show.Stdout()+show.Stderr(), token) {
		t.Fatal("public output exposed the source credential")
	}
}

func assertRequesterRefusal(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, name, callerID, token string) {
	t.Helper()
	before := child.providerRunner.CallCount()
	id := scenarioScopedID(child, "requester-refusal-"+name)
	path := requesterExecutionPath(t, child, id)
	input := t7RemoteCLIInputs(child, ctx, fixture.baseURL, "invoke", "--execution", path, "--async")
	input.Input.Env = append(input.Input.Env, "YOU_WORKER_SESSION_ID="+callerID)
	if token != "" {
		input.Input.Env = append(input.Input.Env, "YOU_WORKER_SESSION_TOKEN="+token)
	}
	if err := fixture.process.Execute(input.Input); err == nil {
		t.Fatal("invalid requester was admitted")
	}
	assertDirectWorkerSessionCLIError(t, input, "WORKER_SESSION_CALLER_INVALID")
	if child.providerRunner.CallCount() != before {
		t.Fatal("refused requester launched a provider")
	}
	if token != "" && strings.Contains(input.Stdout()+input.Stderr(), token) {
		t.Fatal("caller refusal disclosed credentials")
	}
}

// M6/M8: public continuation copies the attributed source's metadata rather
// than retargeting to its predecessor. Replaying the tuple cannot advance the
// chain or launch another provider; the original running caller stays intact.
func assertRequesterContinuations(t *testing.T, fixture *invokeContinuePackageFixture, parent, child *invokeContinueScenario, ctx context.Context, sourceID, parentID string) {
	t.Helper()
	source := requesterObservation(t, fixture, child, ctx, sourceID)
	if source.ProviderSession == nil {
		t.Fatal("attributed source did not retain its native provider identity")
	}
	host := invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}
	awaitContinuationRestartLogs(t, host, child.homeDirectory, child.workingDirectory, sourceID, source.ProviderSession.Id)
	parentBefore := requesterObservation(t, fixture, parent, ctx, parentID)
	previousID := sourceID
	previousToken := requesterEnvironment(child.providerRunner.Requests()[0].Env)["YOU_WORKER_SESSION_TOKEN"]
	for index, name := range []string{"first", "head"} {
		successorID := scenarioScopedID(child, "requester-"+name)
		args := []string{"continue", sourceID, "--request-id", successorID + "-request", "--successor-worker-session-id", successorID, "--user-message", "requester follow-up"}
		if index > 0 {
			args = append(args, "--head")
		}
		assertRequesterContinuationReplay(t, child, ctx, args, previousID, successorID, index+2)
		awaitContinuationRestartLogs(t, host, child.homeDirectory, child.workingDirectory, successorID, source.ProviderSession.Id)
		observation := requesterObservation(t, fixture, child, ctx, successorID)
		if !reflect.DeepEqual(observation.Requester, source.Requester) || !reflect.DeepEqual(observation.Correlation, source.Correlation) || !reflect.DeepEqual(observation.Labels, source.Labels) {
			t.Fatal("successor changed retained requester metadata")
		}
		command := child.providerRunner.Requests()[index+1]
		environment := requesterEnvironment(command.Env)
		assertRequesterEndpoint(t, environment, fixture.baseURL)
		assertRequesterSuccessorEnvironment(t, environment, successorID, parentID, previousToken)
		assertRequesterDurableTokenPrivacy(t, fixture, ctx, successorID, environment["YOU_WORKER_SESSION_TOKEN"])
		if !strings.Contains(strings.Join(command.Args, " "), "resume "+source.ProviderSession.Id) {
			t.Fatal("successor did not resume the exact captured provider session")
		}
		assertRequesterRefusal(t, fixture, child, ctx, "terminal-"+name, successorID, environment["YOU_WORKER_SESSION_TOKEN"])
		previousID, previousToken = successorID, environment["YOU_WORKER_SESSION_TOKEN"]
	}
	parentAfter := requesterObservation(t, fixture, parent, ctx, parentID)
	// Elapsed active time advances while the independent caller remains running.
	if parentBefore.DurationMillis == nil || parentAfter.DurationMillis == nil || *parentAfter.DurationMillis < *parentBefore.DurationMillis {
		t.Fatal("running caller lost its active-clock duration")
	}
	parentBefore.DurationMillis, parentAfter.DurationMillis = nil, nil
	if !reflect.DeepEqual(parentBefore, parentAfter) || parent.providerRunner.CallCount() != 1 {
		t.Fatalf("direct continuation mutated the independent running caller: before=%+v after=%+v", parentBefore, parentAfter)
	}
}

func assertRequesterContinuationReplay(t *testing.T, child *invokeContinueScenario, ctx context.Context, args []string, sourceID, successorID string, calls int) {
	t.Helper()
	for index := range 2 {
		input := t7RemoteCLIInputs(child, ctx, child.fixture.baseURL, args...)
		if index == 0 {
			input = support.FakeInputs(ctx, append([]string{"you", "--json", "worker-sessions"}, args...))
			input.Input.Env, input.Input.WorkingDirectory = child.environment(), child.workingDirectory
		}
		if err := child.fixture.process.Execute(input.Input); err != nil {
			t.Fatalf("attributed continuation: %v: %s", err, input.Stderr())
		}
		var result directWorkerSessionCLIResult
		decodeDirectWorkerSessionResult(t, input.Stdout(), &result)
		if result.SourceWorkerSessionID != sourceID || result.SuccessorWorkerSessionID != successorID || result.State != "COMPLETED" || child.providerRunner.CallCount() != calls {
			t.Fatalf("continuation/replay source=%s successor=%s state=%s calls=%d; want source=%s successor=%s COMPLETED calls=%d", result.SourceWorkerSessionID, result.SuccessorWorkerSessionID, result.State, child.providerRunner.CallCount(), sourceID, successorID, calls)
		}
	}
}

func assertRequesterSuccessorEnvironment(t *testing.T, environment map[string]string, id, requester, previousToken string) {
	t.Helper()
	token := environment["YOU_WORKER_SESSION_TOKEN"]
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != 32 || token == previousToken || environment["YOU_WORKER_SESSION_ID"] != id || environment["YOU_MESSAGE_TARGET"] != requester {
		t.Fatal("successor did not receive a fresh credential and its retained exact target")
	}
}

func requesterObservation(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, id string) api.WorkerSessionObservation {
	t.Helper()
	show := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "show", "--worker-session-id", id)
	if err := fixture.process.Execute(show.Input); err != nil {
		t.Fatal(err)
	}
	var observation api.WorkerSessionObservation
	decodeDirectWorkerSessionResult(t, show.Stdout(), &observation)
	for _, request := range scenario.providerRunner.Requests() {
		token := requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"]
		if token != "" && strings.Contains(show.Stdout()+show.Stderr(), token) {
			t.Fatal("public observation disclosed an execution credential")
		}
	}
	return observation
}

func requesterExecutionPath(t *testing.T, scenario *invokeContinueScenario, id string) string {
	t.Helper()
	path := filepath.Join(scenario.workingDirectory, id+".json")
	writeInvokeContinueJSON(t, path, invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-dispatch",
		workingDirectory: scenario.workingDirectory, userMessage: "requester controlled input",
	}))
	return path
}

func requesterEnvironment(environment []string) map[string]string {
	facts := make(map[string]string)
	for _, entry := range environment {
		if key, value, found := strings.Cut(entry, "="); found && strings.HasPrefix(key, "YOU_") {
			facts[key] = value
		}
	}
	return facts
}

func assertRequesterEndpoint(t *testing.T, environment map[string]string, endpoint string) {
	t.Helper()
	if environment["YOU_SERVER"] != endpoint {
		t.Fatalf("execution endpoint = %q, want bound host %q", environment["YOU_SERVER"], endpoint)
	}
}

// M10: both supported interruption modes preserve the admitted requester and
// issue a fresh execution credential. The requester remains an active peer.
func TestRequesterInterruptedSuccessor(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct{ name, mode string }{
		{"provider", "provider"}, {"recorded", "recorded"}, {"ack-input", "provider"}, {"ack-source", "recorded"},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			runRequesterInterruptedSuccessor(t, cell.name, cell.mode)
		})
	}
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.show", "cli/you.worker-sessions.list", "cli/you.worker-sessions.interrupt", "rest/interruptWorkerSession")
}

func runRequesterInterruptedSuccessor(t *testing.T, name, mode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "requester-interrupt-"+name)
	defer scenario.runner.releaseAll()
	if strings.HasPrefix(name, "ack-") {
		// The real journal commits, then the existing storage edge loses only
		// this operation's response. Admission must recover the stored metadata.
		scenario.ids.interruptRequest = "interrupt-" + name + "-" + scenario.ids.interruptRequest
	}
	ids := scenario.ids
	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestB, workerSessionID: ids.workerB, dispatchID: ids.dispatchB,
		factorySessionID: scenario.session.id, repository: scenario.repositoryB.path, workID: ids.workB, message: s8MessageB,
	})
	scenario.runner.waitStarted(t, scenario.repositoryB.path, s8InterruptCallBInitial)
	parentEnv := requesterEnvironment(scenario.runner.requests()[0].Env)
	parentBefore := requesterInterruptObservation(t, scenario, ids.workerB)
	callerEnv := append(append([]string(nil), scenario.env...), "YOU_WORKER_SESSION_ID="+ids.workerB, "YOU_WORKER_SESSION_TOKEN="+parentEnv["YOU_WORKER_SESSION_TOKEN"])
	invokeS8RemoteWorker(t, ctx, scenario.manager, callerEnv, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	source := requesterInterruptObservation(t, scenario, ids.workerA)
	sourceEnv := requesterEnvironment(scenario.runner.requests()[1].Env)
	assertRequesterSuccessorEnvironment(t, sourceEnv, ids.workerA, ids.workerB, parentEnv["YOU_WORKER_SESSION_TOKEN"])
	if source.Requester == nil || source.Requester.WorkerSessionId != ids.workerB || source.Labels == nil || !reflect.DeepEqual(*source.Labels, []string{"parent:" + ids.workerB}) {
		t.Fatal("interruption source did not retain its verified requester")
	}
	first := interruptRequesterSource(t, scenario, mode)
	assertRequesterInterruptReplay(t, scenario, mode, first)
	assertRequesterInterruptedMetadata(t, scenario, source, sourceEnv, parentBefore, parentEnv)
	assertRequesterInterruptCallerRefused(t, scenario, ids.workerA, sourceEnv["YOU_WORKER_SESSION_TOKEN"])
	scenario.runner.release(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	_ = replayS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.successor)
	assertRequesterInterruptCallerRefused(t, scenario, ids.successor, requesterEnvironment(scenario.runner.requests()[2].Env)["YOU_WORKER_SESSION_TOKEN"])
	assertS8WorkNotAdvanced(t, scenario.fixture, scenario.session.id, ids.workA, ids.workB)
	scenario.close(t)
}

func assertRequesterInterruptReplay(t *testing.T, scenario s8InterruptScenario, mode string, first s8InterruptResult) {
	t.Helper()
	ids := scenario.ids
	replay := postS8Interrupt(t, scenario.ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage, mode)
	if !reflect.DeepEqual(s8InterruptResultFromAPI(replay), first) {
		t.Fatal("attributed interrupt replay changed its admission snapshot")
	}
	otherMode := "provider"
	if mode == "provider" {
		otherMode = "recorded"
	}
	status, body, _, err := sendS8InterruptHTTP(scenario.ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage, otherMode)
	if err != nil || status != 409 || !strings.Contains(body, "WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT") || scenario.runner.CallCount() != 3 {
		t.Fatal("changed attributed interrupt replay was not refused before execution")
	}
}

func interruptRequesterSource(t *testing.T, scenario s8InterruptScenario, mode string) s8InterruptResult {
	t.Helper()
	ids := scenario.ids
	input := support.FakeInputs(scenario.ctx, []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "interrupt", ids.workerA,
		"--request-id", ids.interruptRequest, "--successor-worker-session-id", ids.successor,
		"--replacement-message", s8ReplacementMessage, "--resume-mode", mode, "--async"})
	input.Input.Env, input.Input.WorkingDirectory = scenario.env, scenario.repositoryA.path
	if err := scenario.manager.Execute(input.Input); err != nil {
		t.Fatal("attributed interruption failed")
	}
	var first s8InterruptResult
	decodeS8JSON(t, input.Stdout(), &first)
	assertS8InterruptAdmission(t, first, ids)
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	scenario.runner.assertOrder(t, "start:"+s8InterruptCallAInitial, "cancel:"+s8InterruptCallAInitial, "start:"+s8InterruptCallASuccessor)
	for _, request := range scenario.runner.requests() {
		token := requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"]
		if token != "" && strings.Contains(input.Stdout()+input.Stderr(), token) {
			t.Fatal("interruption response disclosed execution credentials")
		}
	}
	return first
}

func assertRequesterInterruptedMetadata(t *testing.T, scenario s8InterruptScenario, source api.WorkerSessionObservation, sourceEnv map[string]string, parentBefore api.WorkerSessionObservation, parentEnv map[string]string) {
	t.Helper()
	ids := scenario.ids
	successor := requesterInterruptObservation(t, scenario, ids.successor)
	if !reflect.DeepEqual(source.Requester, successor.Requester) || !reflect.DeepEqual(source.Correlation, successor.Correlation) || !reflect.DeepEqual(source.Labels, successor.Labels) {
		t.Fatal("interrupted successor changed admitted requester metadata")
	}
	assertInterruptModeLineage(t, scenario.ctx, scenario)
	requests := scenario.runner.requests()
	if len(requests) != 3 || scenario.runner.cancellationCount(s8InterruptCallBInitial) != 0 {
		t.Fatal("interruption replay launched a duplicate or stopped the requester")
	}
	environment := requesterEnvironment(requests[2].Env)
	assertRequesterSuccessorEnvironment(t, environment, ids.successor, ids.workerB, sourceEnv["YOU_WORKER_SESSION_TOKEN"])
	if environment["YOU_WORKER_SESSION_TOKEN"] == parentEnv["YOU_WORKER_SESSION_TOKEN"] || environment["YOU_WORK_ID"] != sourceEnv["YOU_WORK_ID"] || environment["YOU_FACTORY_SESSION_ID"] != sourceEnv["YOU_FACTORY_SESSION_ID"] {
		t.Fatal("successor reused requester authority or changed retained correlation")
	}
	parentAfter := requesterInterruptObservation(t, scenario, ids.workerB)
	if parentBefore.DurationMillis == nil || parentAfter.DurationMillis == nil || *parentAfter.DurationMillis < *parentBefore.DurationMillis {
		t.Fatal("active requester lost its elapsed duration")
	}
	parentBefore.DurationMillis, parentAfter.DurationMillis = nil, nil
	if !reflect.DeepEqual(parentBefore, parentAfter) {
		t.Fatal("interruption changed the active requester observation")
	}
	assertRequesterInterruptListedMetadata(t, scenario, successor)
}

func assertRequesterInterruptCallerRefused(t *testing.T, scenario s8InterruptScenario, id, token string) {
	t.Helper()
	invocation := s8RemoteWorkerInvocation{requestID: id + "-refused-request", workerSessionID: id + "-refused-child", dispatchID: id + "-refused-dispatch",
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: scenario.ids.workA, message: s8MessageA}
	path := s8ExecutionDocument(t, invocation)
	input := support.FakeInputs(scenario.ctx, []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "invoke", "--execution", path, "--async"})
	input.Input.Env = append(append([]string(nil), scenario.env...), "YOU_WORKER_SESSION_ID="+id, "YOU_WORKER_SESSION_TOKEN="+token)
	input.Input.WorkingDirectory = scenario.repositoryA.path
	if err := scenario.manager.Execute(input.Input); err == nil {
		t.Fatal("interrupted or terminal credential retained requester authority")
	}
	assertDirectWorkerSessionCLIError(t, input, "WORKER_SESSION_CALLER_INVALID")
	if strings.Contains(input.Stdout()+input.Stderr(), token) || scenario.runner.CallCount() != 3 {
		t.Fatal("retired credential refusal disclosed authority or launched a provider")
	}
}

func assertRequesterInterruptListedMetadata(t *testing.T, scenario s8InterruptScenario, expected api.WorkerSessionObservation) {
	t.Helper()
	args := []string{"--json", "worker-sessions", "list", "--scope", "direct"}
	seen := make(map[string]bool)
	for {
		input := executeS8RemoteCLI(t, scenario.ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL, args...)
		var page api.ListWorkerSessionsResponse
		decodeS8JSON(t, input.Stdout(), &page)
		for _, row := range page.Sessions {
			if row.WorkerSessionId != expected.WorkerSessionId {
				continue
			}
			if !reflect.DeepEqual(row.Requester, expected.Requester) || !reflect.DeepEqual(row.Correlation, expected.Correlation) || !reflect.DeepEqual(row.Labels, expected.Labels) {
				t.Fatal("interrupted successor show/list metadata disagreed")
			}
			return
		}
		if page.PaginationContext == nil || page.PaginationContext.NextToken == nil || *page.PaginationContext.NextToken == "" || seen[*page.PaginationContext.NextToken] {
			t.Fatal("interrupted successor absent from public list or cursor repeated")
		}
		next := *page.PaginationContext.NextToken
		seen[next] = true
		args = []string{"--json", "worker-sessions", "list", "--scope", "direct", "--next-token", next}
	}
}

func requesterInterruptObservation(t *testing.T, scenario s8InterruptScenario, id string) api.WorkerSessionObservation {
	t.Helper()
	input := support.FakeInputs(scenario.ctx, []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "show", "--worker-session-id", id})
	input.Input.Env, input.Input.WorkingDirectory = scenario.env, scenario.factoryDir
	if err := scenario.manager.Execute(input.Input); err != nil {
		t.Fatal("interrupted requester observation unavailable")
	}
	var observation api.WorkerSessionObservation
	decodeDirectWorkerSessionResult(t, input.Stdout(), &observation)
	for _, request := range scenario.runner.requests() {
		token := requesterEnvironment(request.Env)["YOU_WORKER_SESSION_TOKEN"]
		if token != "" && strings.Contains(input.Stdout()+input.Stderr(), token) {
			t.Fatal("interrupted requester observation disclosed execution credentials")
		}
	}
	return observation
}

// Public live activation retains host-owned packaged resolution and its lifecycle.
func TestRequesterPackagedLiveOpen(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "requester-packaged-live")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	document := map[string]any{"folderPath": scenario.workingDirectory, "factoryId": "@you/subagent", "requestId": scenarioScopedID(scenario, "packaged-open")}
	assertRequesterPackagedSelectionRefusals(t, fixture, scenario, ctx, document)
	opened := requesterPackagedOpen(t, fixture, ctx, document)
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	repeated := requesterPackagedOpen(t, fixture, ctx, document)
	if repeated.Session.Id != opened.Session.Id || scenario.providerRunner.CallCount() != 0 {
		t.Fatal("duplicate activation changed identity or dispatched before invocation")
	}
	args := map[string]any{"input": "packaged live request", "workingRoot": scenario.workingDirectory, "workerProvider": "codex", "workerModel": "gpt-5-codex"}
	status, body := t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/factory-sessions/"+opened.Session.Id+"/invocations", map[string]any{"requestId": scenarioScopedID(scenario, "packaged-invoke"), "args": args})
	if status != http.StatusOK || !strings.Contains(body, "packaged live output COMPLETE") {
		t.Fatalf("packaged invocation = %d %s", status, body)
	}
	var result api.InvocationResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil || result.Status != api.InvocationTerminalStatusCompleted {
		t.Fatalf("packaged invocation did not complete: %s", body)
	}
	if result.FailureReason != nil || strings.Contains(body, "failureReason") {
		t.Fatal("successful packaged invocation published a failure category")
	}
	requests := scenario.providerRunner.Requests()
	if len(requests) != 1 || requests[0].WorkDir != scenario.workingDirectory || !strings.Contains(string(requests[0].Stdin), "packaged live request") {
		t.Fatal("packaged invocation lost working root/input or duplicated provider launch")
	}
	functionalevidence.Covers(t, "rest/openFactorySession", "rest/invokeFactorySessionBySessionId", "rest/closeFactorySession")
}

// The host's invocation owner selects a provider category through production
// runtime wiring; HTTP and its shared decoder preserve that category.
func TestRequesterPackagedInvocationFailureReason(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "requester-packaged-auth-failure")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	opened := requesterPackagedOpen(t, fixture, ctx, map[string]any{
		"folderPath": scenario.workingDirectory, "factoryId": "@you/subagent", "requestId": scenarioScopedID(scenario, "failure-open"),
	})
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	status, body := t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/factory-sessions/"+opened.Session.Id+"/invocations", map[string]any{
		"requestId": scenarioScopedID(scenario, "failure-invoke"),
		"args":      map[string]any{"input": "controlled provider failure", "workingRoot": scenario.workingDirectory, "workerProvider": "codex", "workerModel": "gpt-5-codex"},
	})
	var result api.InvocationResponse
	if status != http.StatusOK || json.Unmarshal([]byte(body), &result) != nil || result.Status != api.InvocationTerminalStatusFailed {
		t.Fatalf("packaged failed invocation = %d %s", status, body)
	}
	if result.FailureReason == nil || *result.FailureReason != api.WorkFailureTypeAuthFailure {
		t.Fatal("public invocation lost the owner's authentication classification")
	}
	if result.RequestId == "" || result.TraceId == "" || scenario.providerRunner.CallCount() != 1 {
		t.Fatal("failed invocation lost correlation or retried a terminal authentication failure")
	}
	functionalevidence.Covers(t, "rest/openFactorySession", "rest/invokeFactorySessionBySessionId", "rest/closeFactorySession")
}

func requesterPackagedOpen(t *testing.T, fixture *invokeContinuePackageFixture, ctx context.Context, document map[string]any) api.OpenFactorySessionResponse {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/factory-sessions", document)
	var opened api.OpenFactorySessionResponse
	if status != http.StatusOK || json.Unmarshal([]byte(body), &opened) != nil || opened.Session == nil {
		t.Fatalf("packaged open = %d %s", status, body)
	}
	return opened
}

func assertRequesterPackagedSelectionRefusals(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, document map[string]any) {
	t.Helper()
	for _, selection := range []map[string]any{
		{"factoryId": " "}, {"factoryId": "@you/../subagent"}, {"factoryId": "@you/missing-retained-t4"},
		{"target": map[string]any{"kind": "default"}}, {"initNewFactory": true}, {"requestId": " "},
	} {
		candidate := make(map[string]any, len(document)+1)
		for key, value := range document {
			candidate[key] = value
		}
		for key, value := range selection {
			candidate[key] = value
		}
		status, body := t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/factory-sessions", candidate)
		if status != http.StatusBadRequest || !strings.Contains(body, `"code":"BAD_REQUEST"`) || scenario.providerRunner.CallCount() != 0 {
			t.Fatalf("invalid packaged selection was not refused before launch: %d %s", status, body)
		}
	}
}
