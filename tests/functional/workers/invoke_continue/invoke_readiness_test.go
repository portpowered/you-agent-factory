package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

type t7ReadinessProvider struct {
	unavailable atomic.Bool
	integration *providerswire.ProgressingIntegration
}

// Each mode owns a distinct external registration. The injected probe reads
// an atomic readiness fact and never discovers or executes an integration.
type t7ReadinessBoundary struct {
	providers     map[providers.ID]*t7ReadinessProvider
	registrations providerswire.ProviderRegistrations
}

func newT7ReadinessBoundary() *t7ReadinessBoundary {
	b := &t7ReadinessBoundary{providers: make(map[providers.ID]*t7ReadinessProvider)}
	for _, mode := range []string{"local", "remote", "http"} {
		id := "t7-ready-" + mode
		integration := providerswire.ProgressingExternalIntegration(providerswire.Identity(id), "T7 recovered provider COMPLETE")
		b.providers[providers.ID(id)] = &t7ReadinessProvider{integration: integration}
		b.registrations = append(b.registrations, providerswire.Registration{
			Manifest: providerswire.Manifest{ID: id,
				ImplementationAvailability:   providerswire.ImplementationExternallySupplied,
				TechnicalSupportLevel:        providerswire.SupportProduction,
				MaximumExecutionCapabilities: providerswire.ExecutionCapabilities{PromptSubmission: true},
			}, Integration: integration,
		})
	}
	return b
}

func (b *t7ReadinessBoundary) probe(ctx context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
	if p := b.providers[descriptor.ID]; p != nil && p.unavailable.Load() {
		descriptor.Readiness = providers.ReadinessUnavailable
	}
	return descriptor, ctx.Err()
}

// F6-06/08 prove recovery without a reserved identity, then accepted replay
// before changed readiness, through the real Providers/Workers/Start owners.
func TestT7ReadinessRejectionRecoveryAndAcceptedReplay(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "rest/startWorkerSession", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, mode := range []string{"local", "remote", "http"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			t7ReadinessJourney(t, fixture, mode)
		})
	}
}

func t7ReadinessJourney(t *testing.T, fixture *invokeContinuePackageFixture, mode string) {
	t.Helper()
	scenario := fixture.scenario(t, "t7-ready-"+mode)
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	provider := fixture.readiness.providers[providers.ID(scenario.name)]
	before := provider.integration.Stats()
	provider.unavailable.Store(true)
	defer provider.unavailable.Store(false)
	id := scenarioScopedID(scenario, "readiness-session")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
		workingDirectory: scenario.workingDirectory, userMessage: "readiness controlled input",
	})
	execution := document["execution"].(map[string]any)
	execution["dispatch"].(map[string]any)["execution"] = map[string]any{"requestId": id + "-request"}
	for _, key := range []string{"runnerId", "executorProvider", "modelProvider"} {
		execution[key] = scenario.name
	}
	path := filepath.Join(scenario.workingDirectory, "readiness.json")
	writeInvokeContinueJSON(t, path, document)
	t7AssertUnavailableInvoke(t, ctx, fixture, scenario, mode, path, document)
	for _, suffix := range []string{"", "/logs"} {
		status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+suffix, nil)
		if status != http.StatusNotFound {
			t.Fatalf("unavailable request opened capture: %d %s", status, body)
		}
	}
	if provider.integration.Stats().InvokeCalls != before.InvokeCalls || scenario.providerRunner.CallCount() != 0 {
		t.Fatal("preflight executed an external provider")
	}
	provider.unavailable.Store(false)
	join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(join.Input); err != nil {
		t.Fatalf("readiness recovery = %v: %s", err, join.Stderr())
	}
	if !strings.Contains(join.Stdout(), id) || !strings.Contains(join.Stdout(), `"state":"COMPLETED"`) || !strings.Contains(join.Stdout(), "T7 recovered provider COMPLETE") {
		t.Fatalf("recovered identity/output = %s", join.Stdout())
	}
	provider.unavailable.Store(true)
	t7AssertHTTPReplay(t, ctx, fixture.baseURL, id, document)
	status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(body, "T7 recovered provider COMPLETE") || !strings.Contains(body, `"health":"COMPLETE"`) {
		t.Fatalf("replayed capture = %d %s", status, body)
	}
	document["requestId"], document["workerSessionId"] = id+"-new-request", id+"-new-session"
	status, body = t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/worker-sessions", document)
	if status != http.StatusServiceUnavailable || provider.integration.Stats().InvokeCalls != before.InvokeCalls+1 {
		t.Fatalf("new admission bypassed changed readiness or replay reexecuted: %d %s", status, body)
	}
}

func t7AssertUnavailableInvoke(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, mode, path string, document map[string]any) {
	t.Helper()
	if mode == "http" {
		status, body := t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/worker-sessions", document)
		if status != http.StatusServiceUnavailable || !strings.Contains(body, "WORKER_SESSION_ADMISSION_FAILED") {
			t.Fatalf("unavailable Start = %d %s", status, body)
		}
	} else {
		args := []string{"you", "--json"}
		if mode == "remote" {
			args = append(args, "--remote", "--server", fixture.baseURL)
		}
		args = append(args, "worker-sessions", "invoke", "--execution", path)
		inputs := support.FakeInputs(ctx, args)
		inputs.Input.Env, inputs.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
		if err := fixture.process.Execute(inputs.Input); err == nil || inputs.Stdout() != "" {
			t.Fatalf("unavailable invoke = %v stdout=%s", err, inputs.Stdout())
		}
		assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_ADMISSION_FAILED")
	}
}

func appendRequesterIndependenceScenarios(rootDir string, setup *invokeContinueScenarioSetup, late *invokeContinueResettableProviderCommandRunner) error {
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-mcp-failure-late", late, late, nil, nil, nil, late.Reset); err != nil {
		return err
	}
	peer := &t7GatedProviderRunner{}
	peer.reset()
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-independent-peer", peer, peer, nil, nil, nil, peer.reset); err != nil {
		return err
	}
	for _, mode := range []string{"attributed", "unattributed"} {
		result := platformprocess.CommandResult{Stdout: directCodexSessionOutput("independent-thread", "Independent Factory COMPLETE")}
		runner := newInvokeContinueResettableProviderCommandRunner(result, result)
		if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-independent-"+mode, runner, runner, nil, nil, nil, runner.Reset); err != nil {
			return err
		}
	}
	return nil
}

// M12 compares the same public Factory invocation with and without a verified
// requester. Direct revival then runs alongside the still occupied caller.
// Both flows use the existing root process and immutable command routes.
func TestRequesterFactoryIndependence(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.run", "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.continue", "cli/you.worker-sessions.show")
	fixture := ensureInvokeContinuePackageFixture(t)
	peer := fixture.scenario(t, "requester-independent-peer")
	defer peer.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := peer.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	peerID := scenarioScopedID(peer, "occupied-peer")
	start := t7RemoteCLIInputs(peer, ctx, fixture.baseURL, "invoke", "--execution", requesterExecutionPath(t, peer, peerID), "--async")
	if err := fixture.process.Execute(start.Input); err != nil {
		t.Fatal(err)
	}
	t19AwaitSignal(t, ctx, runner.started, "independence peer admitted")
	token := requesterSourceToken(t, runner, peerID)
	before := requesterObservation(t, fixture, peer, ctx, peerID)
	peerEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, peer.session.id)
	attributed := runRequesterIndependentFactory(t, fixture, ctx, "attributed", peerID, token)
	unattributed := runRequesterIndependentFactory(t, fixture, ctx, "unattributed", "", "")
	if !reflect.DeepEqual(attributed, unattributed) {
		t.Fatalf("requester changed canonical Factory behavior:\nattributed=%s\nunattributed=%s", attributed, unattributed)
	}
	after := requesterObservation(t, fixture, peer, ctx, peerID)
	before.DurationMillis, after.DurationMillis = nil, nil
	if !reflect.DeepEqual(before, after) || runner.CallCount() != 1 ||
		!reflect.DeepEqual(peerEvents, support.GetFactoryEventsForSessionAt(t, fixture.baseURL, peer.session.id)) {
		t.Fatal("Factory invocation/continuation changed the occupied peer or its canonical events")
	}
}

func runRequesterIndependentFactory(t *testing.T, fixture *invokeContinuePackageFixture, ctx context.Context, mode, callerID, token string) string {
	t.Helper()
	scenario := fixture.scenario(t, "requester-independent-"+mode)
	defer scenario.close(t)
	opened := requesterPackagedOpen(t, fixture, ctx, map[string]any{
		"folderPath": scenario.workingDirectory, "factoryId": "@you/subagent", "requestId": scenarioScopedID(scenario, "independence-open"),
	})
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	prior := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, opened.Session.Id)
	input := support.FakeInputs(ctx, []string{"you", "--remote", "--server", fixture.baseURL, "--session", opened.Session.Id,
		"run", "--factory", testutil.MustRepoPath(t, "packages/packaged-factories/generated/factories/subagent/factory.json"),
		"--worker-provider", "codex", "--worker-model", "opaque-factory-model", "--working-root", scenario.workingDirectory, "independence input"})
	input.Input.Env, input.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
	if callerID != "" {
		input.Input.Env = append(input.Input.Env, "YOU_WORKER_SESSION_ID="+callerID, "YOU_WORKER_SESSION_TOKEN="+token)
	}
	if err := fixture.process.Execute(input.Input); err != nil || !strings.Contains(input.Stdout(), "Independent Factory COMPLETE") {
		t.Fatalf("independent Factory invocation: %v: %s", err, input.Stderr())
	}
	requests := scenario.providerRunner.Requests()
	if len(requests) != 1 || requests[0].Command != "codex" || !strings.Contains(string(requests[0].Stdin), "independence input") {
		t.Fatal("Factory invocation changed native command selection, input or admission count")
	}
	environment := requesterEnvironment(requests[0].Env)
	source := requesterObservation(t, fixture, scenario, ctx, environment["YOU_WORKER_SESSION_ID"])
	if environment["YOU_MESSAGE_TARGET"] != callerID || (source.Requester == nil) != (callerID == "") {
		t.Fatal("comparison did not exercise attributed and unattributed Factory admissions")
	}
	if callerID != "" && source.Requester.WorkerSessionId != callerID {
		t.Fatal("Factory admission selected another requester")
	}
	if !requesterCorrelationAgrees(source, environment["YOU_WORK_ID"], opened.Session.Id) {
		t.Fatal("Factory source correlation disagrees with its command environment")
	}
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, scenario.homeDirectory, scenario.workingDirectory, source.WorkerSessionId, "independent-thread")
	snapshot := requesterCanonicalFactorySnapshot(t, fixture, opened.Session.Id)
	assertRequesterIndependentContinuation(t, fixture, scenario, ctx, source, environment, snapshot)
	return normalizeRequesterFactorySnapshot(t, snapshot, len(prior), scenario.workingDirectory)
}

type requesterFactorySnapshot struct {
	Events []api.FactoryEvent
	Work   api.ListWorkResponse
}

func requesterCanonicalFactorySnapshot(t *testing.T, fixture *invokeContinuePackageFixture, sessionID string) requesterFactorySnapshot {
	t.Helper()
	base := fixture.baseURL + "/factory-sessions/" + sessionID
	return requesterFactorySnapshot{
		Events: support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID),
		Work:   support.GetJSON[api.ListWorkResponse](t, base+"/work"),
	}
}

func assertRequesterIndependentContinuation(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, ctx context.Context, source api.WorkerSessionObservation, environment map[string]string, snapshot requesterFactorySnapshot) {
	t.Helper()
	id := scenarioScopedID(scenario, "independent-successor")
	input := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "continue", source.WorkerSessionId, "--session", *source.Correlation.FactorySessionId,
		"--head", "--request-id", id+"-request", "--successor-worker-session-id", id, "--user-message", "independent follow-up")
	if err := fixture.process.Execute(input.Input); err != nil {
		t.Fatalf("independent direct continuation: %v: %s", err, input.Stderr())
	}
	successor := requesterObservation(t, fixture, scenario, ctx, id)
	assertRequesterCopiedMetadata(t, source, successor)
	requests := scenario.providerRunner.Requests()
	if len(requests) != 2 || !strings.Contains(strings.Join(requests[1].Args, " "), "independent-thread") || !successor.Direct {
		t.Fatal("Factory source continuation did not directly resume its exact provider reference once")
	}
	next := requesterEnvironment(requests[1].Env)
	assertRequesterSuccessorEnvironment(t, next, id, environment["YOU_MESSAGE_TARGET"], environment["YOU_WORKER_SESSION_TOKEN"])
	after := requesterObservation(t, fixture, scenario, ctx, source.WorkerSessionId)
	if after.SuccessorWorkerSessionId == nil || *after.SuccessorWorkerSessionId != id ||
		after.ContinuationHeadWorkerSessionId == nil || *after.ContinuationHeadWorkerSessionId != id {
		t.Fatal("direct continuation lost the exact source-to-head link")
	}
	after.SuccessorWorkerSessionId, after.ContinuationHeadWorkerSessionId = source.SuccessorWorkerSessionId, source.ContinuationHeadWorkerSessionId
	if !reflect.DeepEqual(after, source) {
		t.Fatal("direct continuation mutated its Factory source attempts")
	}
	if !reflect.DeepEqual(snapshot, requesterCanonicalFactorySnapshot(t, fixture, *source.Correlation.FactorySessionId)) {
		t.Fatal("direct continuation changed canonical Work, dispatches or ordered Factory events")
	}
}

func normalizeRequesterFactorySnapshot(t *testing.T, snapshot requesterFactorySnapshot, prior int, directory string) string {
	t.Helper()
	snapshot.Events = snapshot.Events[prior:]
	wantTypes := []api.FactoryEventType{"WORK_REQUEST", "DISPATCH_REQUEST", "DISPATCH_WORKER_SESSION_ASSOCIATION", "MODEL_REQUEST", "MODEL_RESPONSE", "AGENT_RUN_RESPONSE", "DISPATCH_RESPONSE", "WORK_STATE_CHANGE"}
	if len(snapshot.Events) != len(wantTypes) || len(snapshot.Work.Results) != 1 {
		t.Fatalf("Factory invocation added/lost canonical events or Work: events=%d work=%d", len(snapshot.Events), len(snapshot.Work.Results))
	}
	item := snapshot.Work.Results[0]
	if item.Name != "work-1" || support.WorkItemCustomerLocation(item) != "task:complete" || item.Payload != "independence input" {
		t.Fatal("requester changed the ordinary Work name, terminal state or payload")
	}
	for index, event := range snapshot.Events {
		if event.Type != wantTypes[index] {
			t.Fatal("Factory invocation changed canonical dispatch/result ordering")
		}
		if index > 0 && event.Context.Sequence <= snapshot.Events[index-1].Context.Sequence {
			t.Fatal("Factory events lost append order")
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	identities := make(map[string]string)
	value = normalizeRequesterFactoryValue(value, "", directory, identities)
	data, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Preserve every payload/relationship and event position. Only execution
// identities, host paths, clocks/global tick offsets and additive nonsecret
// Worker Session metadata differ legitimately between equivalent scenarios.
func normalizeRequesterFactoryValue(value any, key, directory string, identities map[string]string) any {
	switch node := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(node))
		for name := range node {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			switch name {
			case "eventTime", "durationMillis", "sequence", "sessionSequence", "tick", "sessionMetadata":
				delete(node, name)
			default:
				node[name] = normalizeRequesterFactoryValue(node[name], name, directory, identities)
			}
		}
	case []any:
		for index := range node {
			node[index] = normalizeRequesterFactoryValue(node[index], key, directory, identities)
		}
	case string:
		if key == "replayKey" || key == "modelRequestId" || key == "agentRunId" {
			return normalizeRequesterDerivedIdentity(node, identities)
		}
		switch key {
		case "id", "sessionId", "workId", "workIds", "relatedWorkIds", "requestId", "traceId", "traceIds", "dispatchId", "workerSessionId", "attemptId", "currentChainingTraceId", "previousChainingTraceIds":
			if node != "" {
				if identities[node] == "" {
					identities[node] = fmt.Sprintf("identity-%d", len(identities)+1)
				}
				return identities[node]
			}
		}
		return strings.ReplaceAll(node, directory, "<workspace>")
	}
	return value
}

func normalizeRequesterDerivedIdentity(value string, identities map[string]string) string {
	keys := make([]string, 0, len(identities))
	for identity := range identities {
		keys = append(keys, identity)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, identity := range keys {
		value = strings.ReplaceAll(value, identity, identities[identity])
	}
	return value
}
