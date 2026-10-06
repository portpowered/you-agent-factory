package customer_journeys_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	fscp03ObservationTimeout = 15 * time.Second
	fscp03InlineWorkflowKind = "INLINE_WORKFLOW"
)

// TestFactorySessionLifecycleAndIsolation verifies durable execution controls
// and live response isolation through the public Factory Session boundary.
func TestFactorySessionLifecycleAndIsolation(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"durable identity, concurrency, and failed-start recovery", runFSCP03DurableIdentityScenario},
		{"durable controls and timeout branches", runFSCP03DurableControlScenario},
		{"live work and response isolation", runFSCP03LiveIsolationScenario},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			acquireExecutionFixtureSlot(t)
			scenario.run(t)
		})
	}
}

func runFSCP03DurableIdentityScenario(t *testing.T) {
	t.Helper()

	factoryDir := scaffoldFSCP03ProbeFactory(t)
	home := t.TempDir()
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		BrowserOpener:         func(context.Context, string) error { return nil },
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("fscp03 durable COMPLETE"),
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	fscp03ExecuteHelp(t, process, factoryDir, home)
	canonical := openFSCP03Execution(t, process.FactorySessions())
	selections := fscp03RuntimeSelections(factoryDir, home)

	runFSCP03SequentialDurableIdentity(t, canonical, selections)
	runFSCP03DurableDirection(t, canonical, selections)
	runFSCP03ConcurrentDurableIdentity(t, factoryDir, home)
}

func runFSCP03DurableControlScenario(t *testing.T) {
	t.Helper()

	factoryDir := scaffoldFSCP03ProbeFactory(t)
	home := t.TempDir()
	controlRunner := newFSCP03ControlRunner()
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		BrowserOpener:         func(context.Context, string) error { return nil },
		ProviderCommandRunner: controlRunner,
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	fscp03ExecuteHelp(t, process, factoryDir, home)
	canonical := openFSCP03Execution(t, process.FactorySessions())
	selections := fscp03RuntimeSelections(factoryDir, home)

	runFSCP03Cancel(t, canonical, selections, controlRunner)
	runFSCP03Terminate(t, canonical, selections, controlRunner)
	runFSCP03Close(t, canonical, selections, factoryDir)
	runFSCP03TimeoutBranches(t, canonical, selections, controlRunner)
}

func runFSCP03LiveIsolationScenario(t *testing.T) {
	t.Helper()

	firstDir := scaffoldFSCP03ProbeFactory(t)
	secondDir := scaffoldFSCP03ProbeFactory(t)
	runner := newFSCP03LiveRunner()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                firstDir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	baseURL := server.URL()
	first := support.GetDefaultSession(t, baseURL)
	if first.Runtime.StreamIdentity == nil || strings.TrimSpace(first.Id) == "" {
		t.Fatalf("default live session = %#v, want session and stream identity", first)
	}
	opened := support.OpenFactorySessionAt(t, baseURL, secondDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("second live session = %#v, want session and stream identity", opened.Session)
	}
	secondResponse := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(opened.Session.Id),
	)
	second, err := secondResponse.AsFactorySession()
	if err != nil {
		t.Fatalf("decode second live Factory Session: %v", err)
	}
	if second.Runtime.StreamIdentity == nil {
		t.Fatalf("second live session = %#v, want full session stream identity", second)
	}
	if first.Id == second.Id || first.Runtime.StreamIdentity.LogicalSessionKeyID == second.Runtime.StreamIdentity.LogicalSessionKeyID ||
		first.Runtime.StreamIdentity.StreamGenerationID == second.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf("live stream identities first=%#v second=%#v, want distinct session/logical/generation identities", first.Runtime.StreamIdentity, second.Runtime.StreamIdentity)
	}
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, second.Id) })

	runFSCP03LiveInvocations(t, baseURL, first.Id, second.Id, runner)
}

func runFSCP03LiveInvocations(t *testing.T, baseURL, firstID, secondID string, runner *fscp03LiveRunner) {
	t.Helper()
	firstResponse, err := postFSCP03Invocation(t.Context(), baseURL, factorysessions.DefaultSessionID, "fscp03 live first")
	if err != nil {
		t.Fatalf("first live invocation error = %v", err)
	}
	secondInvocation, err := postFSCP03Invocation(t.Context(), baseURL, secondID, "fscp03 live second")
	if err != nil {
		t.Fatalf("second live invocation error = %v", err)
	}
	assertFSCP03HTTPInvocation(t, firstResponse)
	assertFSCP03HTTPInvocation(t, secondInvocation)
	if firstResponse.RequestId == secondInvocation.RequestId || firstResponse.TraceId == secondInvocation.TraceId {
		t.Fatalf("sequential invocation identities first=(%q,%q) second=(%q,%q), want distinct", firstResponse.RequestId, firstResponse.TraceId, secondInvocation.RequestId, secondInvocation.TraceId)
	}
	assertFSCP03LiveFactoryEvents(t, baseURL, firstID, secondID)
	assertFSCP03LiveResponseEvents(t, baseURL, firstID, secondID)

	firstStream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, factorysessions.DefaultSessionID))
	secondStream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, secondID))
	runner.Hold()
	concurrent := make(chan fscp03HTTPInvocationOutcome, 2)
	go func() {
		response, invokeErr := postFSCP03Invocation(t.Context(), baseURL, factorysessions.DefaultSessionID, "fscp03 live concurrent first")
		concurrent <- fscp03HTTPInvocationOutcome{response: response, err: invokeErr}
	}()
	go func() {
		response, invokeErr := postFSCP03Invocation(t.Context(), baseURL, secondID, "fscp03 live concurrent second")
		concurrent <- fscp03HTTPInvocationOutcome{response: response, err: invokeErr}
	}()
	if err := runner.WaitStarted(t.Context(), 2); err != nil {
		t.Fatalf("concurrent live invocations did not overlap at provider edge: %v", err)
	}
	runner.Release()
	var concurrentResponses []factoryapi.InvocationResponse
	for range 2 {
		select {
		case outcome := <-concurrent:
			if outcome.err != nil {
				t.Fatalf("concurrent live invocation error = %v", outcome.err)
			}
			assertFSCP03HTTPInvocation(t, outcome.response)
			concurrentResponses = append(concurrentResponses, outcome.response)
		case <-time.After(fscp03ObservationTimeout):
			t.Fatal("concurrent live invocations did not complete after provider release")
		}
	}
	if len(concurrentResponses) != 2 || concurrentResponses[0].RequestId == concurrentResponses[1].RequestId || concurrentResponses[0].TraceId == concurrentResponses[1].TraceId {
		t.Fatalf("concurrent invocation responses = %#v, want distinct request/trace identities", concurrentResponses)
	}
	firstFrames := collectFSCP03ResponseFrames(t, firstStream, firstID)
	secondFrames := collectFSCP03ResponseFrames(t, secondStream, secondID)
	assertFSCP03DisjointHTTPResponseFrames(t, firstFrames, secondFrames)
}

type fscp03StartOutcome struct {
	result factorysessions.SessionStartResult
	err    error
}

type fscp03Process interface {
	Execute(root.Input) error
}

func fscp03ExecuteHelp(t *testing.T, process fscp03Process, factoryDir, home string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--help"})
	inputs.Input.Env = isolatedEnvironment(home)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(--help) error = %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout(), "Usage:") {
		t.Fatalf("Process.Execute(--help) stdout = %q, want public usage", inputs.Stdout())
	}
}

func openFSCP03Execution(t *testing.T, capability interface{ FactorySessions() any }) factorysessions.Service {
	t.Helper()
	if capability == nil {
		t.Fatal("root process returned no FactorySessions capability")
	}
	canonical, ok := capability.FactorySessions().(factorysessions.Service)
	if !ok || canonical == nil {
		t.Fatalf("FactorySessions type = %T, want factorysessions.Service", capability.FactorySessions())
	}
	return canonical
}

func fscp03RuntimeSelections(factoryDir, home string) *factorysessions.SessionRuntimeSelection {
	return &factorysessions.SessionRuntimeSelection{
		SystemConfigHome: home,
		ExecutionBaseDir: factoryDir,
		LogPolicy:        factorysessions.SessionArtifactPolicyDisabled,
		MetricsPolicy:    factorysessions.SessionArtifactPolicyDisabled,
	}
}

func fscp03DurableStartRequest(selections *factorysessions.SessionRuntimeSelection, requestID string, source factorysessions.Source) factorysessions.SessionStartRequest {
	return factorysessions.SessionStartRequest{
		Mode:             factorysessions.SessionOperationModeDurable,
		FolderPath:       selections.ExecutionBaseDir,
		Persistence:      factorysessions.PersistencePolicyDisabled,
		Correlation:      factorysessions.SessionOperationCorrelation{RequestID: requestID},
		Source:           source,
		Synchronous:      true,
		RuntimeSelection: selections,
	}
}

func fscp03StartSynchronous(
	t *testing.T,
	canonical factorysessions.Service,
	selections *factorysessions.SessionRuntimeSelection,
	requestID string,
	source factorysessions.Source,
) factorysessions.SessionStartResult {
	t.Helper()
	started, err := canonical.Start(t.Context(), fscp03DurableStartRequest(selections, requestID, source))
	if err != nil {
		t.Fatalf("canonical durable Start(%s) error = %v", requestID, err)
	}
	return started
}

func fscp03StartBlockedAsync(
	t *testing.T,
	canonical factorysessions.Service,
	selections *factorysessions.SessionRuntimeSelection,
	startedEvents <-chan struct{},
	requestID string,
) factorysessions.SessionStartResult {
	t.Helper()
	request := fscp03DurableStartRequest(selections, requestID, fscp03ChildSource(requestID))
	request.Synchronous = false
	started, err := canonical.Start(t.Context(), request)
	if err != nil {
		t.Fatalf("blocked canonical Start(%s) error = %v", requestID, err)
	}
	if started.Status != string(factorysessions.LifecycleStatusRunning) || started.Async == nil {
		t.Fatalf("blocked Start(%s) = %#v, want RUNNING async result", requestID, started)
	}
	select {
	case <-startedEvents:
	case <-time.After(fscp03ObservationTimeout):
		t.Fatalf("blocked Start(%s) did not reach provider", requestID)
	}
	return started
}

func assertFSCP03SuccessfulStart(t *testing.T, started factorysessions.SessionStartResult) {
	t.Helper()
	if strings.TrimSpace(started.SessionID) == "" || started.Mode != factorysessions.SessionOperationModeDurable ||
		started.Status != string(factorysessions.LifecycleStatusSucceeded) || started.Sync == nil ||
		started.Sync.SyncOutcome != factorysessions.SyncOutcome("COMPLETED") || started.Sync.TimedOut {
		t.Fatalf("successful start = %#v, want durable SUCCEEDED/COMPLETED", started)
	}
}

func assertFSCP03DurableLineage(t *testing.T, canonical factorysessions.Service, sessionID string) {
	t.Helper()
	view, err := canonical.Get(t.Context(), factorysessions.SessionGetRequest{
		SessionID: sessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
	})
	if err != nil {
		t.Fatalf("canonical Get(%s) error = %v", sessionID, err)
	}
	if view.Session.SessionID != sessionID || view.Session.Mode != factorysessions.SessionOperationModeDurable ||
		view.Session.Status != string(factorysessions.LifecycleStatusSucceeded) || view.Session.OrchestratorKind != "JAVASCRIPT" ||
		view.Session.SourceRef != "inline" || view.Session.ResultStatus != string(factorysessions.ResultStatusFinal) {
		t.Fatalf("canonical view = %#v, want stable durable success projection", view.Session)
	}
	result, err := canonical.ReadResult(t.Context(), factorysessions.SessionResultReadRequest{
		SessionID: sessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Request:   factorysessions.ResultRequest{Mode: factorysessions.ResultModeFinal},
	})
	if err != nil {
		t.Fatalf("canonical ReadResult(%s) error = %v", sessionID, err)
	}
	if result.SessionID != sessionID || result.Mode != factorysessions.SessionOperationModeDurable ||
		result.Status != string(factorysessions.ResultStatusFinal) || result.Durable == nil ||
		result.Durable.SessionStatus != factorysessions.LifecycleStatusSucceeded || len(result.Durable.PrimaryResult) == 0 {
		t.Fatalf("canonical result = %#v, want final durable result lineage", result)
	}
	assertFSCP03DurableDispatchResponses(t, canonical, sessionID)
}

func assertFSCP03DurableDispatchResponses(t *testing.T, canonical factorysessions.Service, sessionID string) {
	t.Helper()
	dispatches, err := canonical.QueryDispatches(t.Context(), factorysessions.DispatchQueryRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("canonical QueryDispatches(%s) error = %v", sessionID, err)
	}
	if len(dispatches.Dispatches) == 0 {
		t.Fatalf("canonical dispatches(%s) = %#v, want child dispatch", sessionID, dispatches)
	}
	for _, dispatch := range dispatches.Dispatches {
		if strings.TrimSpace(dispatch.ID) == "" || dispatch.Attempt < 1 || dispatch.Status != factorysessions.DispatchStatus("COMPLETED") {
			t.Fatalf("dispatch for %s = %#v, want session-scoped completed attempt", sessionID, dispatch)
		}
	}
	subscription, err := canonical.SubscribeResponses(t.Context(), factorysessions.SessionResponseSubscriptionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("canonical SubscribeResponses(%s) error = %v", sessionID, err)
	}
	if subscription.Cursor == nil {
		t.Fatal("canonical response subscription returned nil cursor")
	}
	events, err := subscription.Cursor.Drain()
	subscription.Cursor.Detach()
	if err != nil {
		t.Fatalf("canonical response cursor Drain(%s) error = %v", sessionID, err)
	}
	if len(events) == 0 {
		t.Fatalf("canonical response events(%s) = none, want child observations", sessionID)
	}
	assertFSCP03ResponseEvents(t, sessionID, events)
}

func assertFSCP03DisjointDurableObservations(t *testing.T, canonical factorysessions.Service, firstID, secondID string) {
	t.Helper()
	first, err := canonical.SubscribeResponses(t.Context(), factorysessions.SessionResponseSubscriptionRequest{SessionID: firstID})
	if err != nil {
		t.Fatalf("subscribe first durable response cursor: %v", err)
	}
	second, err := canonical.SubscribeResponses(t.Context(), factorysessions.SessionResponseSubscriptionRequest{SessionID: secondID})
	if err != nil {
		t.Fatalf("subscribe second durable response cursor: %v", err)
	}
	firstEvents, err := first.Cursor.Drain()
	if err != nil {
		t.Fatalf("drain first durable response cursor: %v", err)
	}
	secondEvents, err := second.Cursor.Drain()
	if err != nil {
		t.Fatalf("drain second durable response cursor: %v", err)
	}
	first.Cursor.Detach()
	second.Cursor.Detach()
	assertFSCP03ResponseEvents(t, firstID, firstEvents)
	assertFSCP03ResponseEvents(t, secondID, secondEvents)
	firstDispatches, err := canonical.QueryDispatches(t.Context(), factorysessions.DispatchQueryRequest{SessionID: firstID})
	if err != nil {
		t.Fatalf("query first durable dispatches: %v", err)
	}
	secondDispatches, err := canonical.QueryDispatches(t.Context(), factorysessions.DispatchQueryRequest{SessionID: secondID})
	if err != nil {
		t.Fatalf("query second durable dispatches: %v", err)
	}
	assertFSCP03ResponseEventsUseSessionDispatches(t, firstID, firstEvents, firstDispatches.Dispatches)
	assertFSCP03ResponseEventsUseSessionDispatches(t, secondID, secondEvents, secondDispatches.Dispatches)
	firstEventIDs := make(map[string]struct{}, len(firstEvents))
	for _, event := range firstEvents {
		firstEventIDs[event.EventID] = struct{}{}
	}
	for _, event := range secondEvents {
		if _, shared := firstEventIDs[event.EventID]; shared {
			t.Fatalf("response event identity %q crossed durable sessions", event.EventID)
		}
	}
}

func assertFSCP03ResponseEventsUseSessionDispatches(
	t *testing.T,
	sessionID string,
	events []factorysessions.FactoryResponseEvent,
	dispatches []factorysessions.DispatchSummary,
) {
	t.Helper()
	dispatchIDs := make(map[string]struct{}, len(dispatches))
	for _, dispatch := range dispatches {
		dispatchIDs[dispatch.ID] = struct{}{}
		dispatchIDs[sessionID+"/"+dispatch.ID] = struct{}{}
	}
	for _, event := range events {
		if _, ok := dispatchIDs[event.DispatchID]; !ok {
			t.Fatalf("response event %q in session %q references dispatch %q outside its session query", event.EventID, sessionID, event.DispatchID)
		}
	}
}

func assertFSCP03DurableStatus(t *testing.T, canonical factorysessions.Service, sessionID string, want factorysessions.LifecycleStatus) {
	t.Helper()
	view, err := canonical.Get(t.Context(), factorysessions.SessionGetRequest{SessionID: sessionID, Mode: factorysessions.SessionOperationModeDurable})
	if err != nil {
		t.Fatalf("canonical Get(%s) error = %v", sessionID, err)
	}
	if view.Session.Status != string(want) {
		t.Fatalf("session %s status = %q, want %q", sessionID, view.Session.Status, want)
	}
}

func assertFSCP03DurableTerminalStatus(t *testing.T, canonical factorysessions.Service, sessionID string) {
	t.Helper()
	view, err := canonical.Get(t.Context(), factorysessions.SessionGetRequest{SessionID: sessionID, Mode: factorysessions.SessionOperationModeDurable})
	if err != nil {
		t.Fatalf("canonical Get(%s) error = %v", sessionID, err)
	}
	if view.Session.Status != string(factorysessions.LifecycleStatusTerminated) && view.Session.Status != string(factorysessions.LifecycleStatusCanceled) {
		t.Fatalf("session %s status = %q, want terminal TERMINATED or CANCELED after typed terminate", sessionID, view.Session.Status)
	}
}

func assertFSCP03ResponseEvents(t *testing.T, sessionID string, events []factorysessions.FactoryResponseEvent) {
	t.Helper()
	seen := make(map[string]struct{}, len(events))
	var previous int64
	for _, event := range events {
		if event.FactorySessionID != sessionID {
			t.Fatalf("response event %q session id = %q, want %q", event.EventID, event.FactorySessionID, sessionID)
		}
		if strings.TrimSpace(event.EventID) == "" || event.Sequence <= previous {
			t.Fatalf("response event = %#v, want non-empty increasing identity", event)
		}
		if _, duplicate := seen[event.EventID]; duplicate {
			t.Fatalf("response event identity %q repeated in session %q", event.EventID, sessionID)
		}
		seen[event.EventID] = struct{}{}
		if event.DispatchID == "" {
			t.Fatalf("response event = %#v, want session-scoped dispatch identity", event)
		}
		previous = event.Sequence
	}
}

func fscp03InlineSource(source string) factorysessions.Source {
	return factorysessions.Source{
		Kind: fscp03InlineWorkflowKind,
		InlineWorkflow: &factorysessions.InlineWorkflowSource{
			Dialect:      "you-workflow-v1",
			InlineSource: source,
		},
	}
}
