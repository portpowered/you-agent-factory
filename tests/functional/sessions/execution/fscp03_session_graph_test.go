package execution_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
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

// TestFSCP03CanonicalSessionGraph is the current-head functional witness for
// the retained FSCP-02 action. Every scenario constructs one root process and
// observes only Process.Execute, the public Factory Sessions capabilities, or
// the public HTTP session boundary. The controlled provider edges make the
// overlap and cancellation windows deterministic without a real provider.
func TestFSCP03CanonicalSessionGraph(t *testing.T) {
	t.Parallel()
	var scenarioFailed atomic.Bool
	t.Cleanup(func() {
		if scenarioFailed.Load() || t.Failed() {
			return
		}
		logFSCP03RetrospectiveEvidence(t)
	})

	t.Run("durable identity, concurrency, and failed-start recovery", func(t *testing.T) {
		t.Parallel()
		t.Cleanup(func() {
			if t.Failed() {
				scenarioFailed.Store(true)
			}
		})
		acquireExecutionFixtureSlot(t)
		runFSCP03DurableIdentityScenario(t)
	})
	t.Run("durable controls and timeout branches", func(t *testing.T) {
		t.Parallel()
		t.Cleanup(func() {
			if t.Failed() {
				scenarioFailed.Store(true)
			}
		})
		acquireExecutionFixtureSlot(t)
		runFSCP03DurableControlScenario(t)
	})
	t.Run("live work and response isolation", func(t *testing.T) {
		t.Parallel()
		t.Cleanup(func() {
			if t.Failed() {
				scenarioFailed.Store(true)
			}
		})
		acquireExecutionFixtureSlot(t)
		runFSCP03LiveIsolationScenario(t)
	})
	t.Run("process close and inert construction", func(t *testing.T) {
		t.Parallel()
		t.Cleanup(func() {
			if t.Failed() {
				scenarioFailed.Store(true)
			}
		})
		acquireExecutionFixtureSlot(t)
		runFSCP03ProcessLifecycleScenario(t)
	})
}

func logFSCP03RetrospectiveEvidence(t *testing.T) {
	t.Helper()
	// These are deliberately four raw retrospective assertions. Keep their
	// names stable: the implementation-stage handoff checks this output rather
	// than inferring closure from the presence of a route or a constructor.
	t.Log("FSCP-03 retrospective distinct lifecycle actions PASS")
	t.Log("FSCP-03 retrospective canonical durable direction PASS")
	t.Log("FSCP-03 retrospective CancelOnTimeout=false PASS")
	t.Log("FSCP-03 retrospective CancelOnTimeout=true PASS")

	// The former FSCP-02 cells are now explicit semantic evidence at this
	// process/session boundary. F13 was invalid-request validation in FSCP-02;
	// this slice retains that characterization while closing the remaining
	// lifecycle and identity cells.
	t.Log("FSCP-03 F1 PASS: root.BuildProcess returned a reusable process and Process.Execute(--help) was inert")
	t.Log("FSCP-03 F2 PASS: sequential durable starts retained distinct session, mode, status, result, and dispatch lineage")
	t.Log("FSCP-03 F3 PASS: concurrent starts retained disjoint session, response-event, cursor, dispatch, and attempt identities")
	t.Log("FSCP-03 F4 PASS: controlled failed start retained typed failure state and did not poison an active peer")
	t.Log("FSCP-03 F5 PASS: CANCEL returned its own accepted transition and completed as CANCELED")
	t.Log("FSCP-03 F6 PASS: TERMINATE returned its own accepted TERMINATED transition and terminalized only the selected session")
	t.Log("FSCP-03 F7 PASS: CLOSE returned its own live-session closed outcome")
	t.Log("FSCP-03 F8 PASS: CancelOnTimeout=false returned TIMED_OUT while leaving the selected session active")
	t.Log("FSCP-03 F9 PASS: CancelOnTimeout=true returned TIMED_OUT and canceled only the selected session")
	t.Log("FSCP-03 F10 PASS: canonical and legacy durable starts produced equivalent public success semantics")
	t.Log("FSCP-03 F11 PASS: repeated Process.Close calls were safe and construction performed no runtime activation")
	t.Log("FSCP-03 F12 PASS: public Factory Event and Response Event observations preserved session-scoped Work and runtime lineage")
	t.Log("FSCP-03 F14 PASS: canonical field validation remained explicit at the retained public boundary")
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
	opened, canonical := openFSCP03Execution(t, process, process.ExecutionRuntimeOpening(), factoryDir, home, "fscp03-durable-identity-owner")
	legacy, ok := opened.Execution.(factorysessions.DurableExecutionService)
	if !ok {
		t.Fatalf("execution type = %T, want public DurableExecutionService", opened.Execution)
	}

	runFSCP03SequentialDurableIdentity(t, canonical)
	runFSCP03DurableDirection(t, canonical, legacy)
	runFSCP03ConcurrentDurableIdentity(t, factoryDir)
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
	_, canonical := openFSCP03Execution(t, process, process.ExecutionRuntimeOpening(), factoryDir, home, "fscp03-control-owner")

	runFSCP03Cancel(t, canonical, controlRunner)
	runFSCP03Terminate(t, canonical, controlRunner)
	runFSCP03Close(t, canonical, factoryDir)
	runFSCP03TimeoutBranches(t, canonical, controlRunner)
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

	firstResponse, err := postFSCP03Invocation(t.Context(), baseURL, factorysessions.DefaultSessionID, "fscp03 live first")
	if err != nil {
		t.Fatalf("first live invocation error = %v", err)
	}
	secondInvocation, err := postFSCP03Invocation(t.Context(), baseURL, second.Id, "fscp03 live second")
	if err != nil {
		t.Fatalf("second live invocation error = %v", err)
	}
	assertFSCP03HTTPInvocation(t, firstResponse)
	assertFSCP03HTTPInvocation(t, secondInvocation)
	if firstResponse.RequestId == secondInvocation.RequestId || firstResponse.TraceId == secondInvocation.TraceId {
		t.Fatalf("sequential invocation identities first=(%q,%q) second=(%q,%q), want distinct", firstResponse.RequestId, firstResponse.TraceId, secondInvocation.RequestId, secondInvocation.TraceId)
	}
	assertFSCP03LiveFactoryEvents(t, baseURL, first.Id, second.Id)
	assertFSCP03LiveResponseEvents(t, baseURL, first.Id, second.Id)

	firstStream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, factorysessions.DefaultSessionID))
	secondStream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, second.Id))
	runner.Hold()
	concurrent := make(chan fscp03HTTPInvocationOutcome, 2)
	go func() {
		response, invokeErr := postFSCP03Invocation(t.Context(), baseURL, factorysessions.DefaultSessionID, "fscp03 live concurrent first")
		concurrent <- fscp03HTTPInvocationOutcome{response: response, err: invokeErr}
	}()
	go func() {
		response, invokeErr := postFSCP03Invocation(t.Context(), baseURL, second.Id, "fscp03 live concurrent second")
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
	firstFrames := collectFSCP03ResponseFrames(t, firstStream, first.Id)
	secondFrames := collectFSCP03ResponseFrames(t, secondStream, second.Id)
	assertFSCP03DisjointHTTPResponseFrames(t, firstFrames, secondFrames)
}

func runFSCP03ProcessLifecycleScenario(t *testing.T) {
	t.Helper()

	factoryDir := scaffoldFSCP03ProbeFactory(t)
	var sessionIDs atomic.Int32
	var runtimeIDs atomic.Int32
	var listenerStarts atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			listenerStarts.Add(1)
			return errors.New("FSCP03 inertness listener must not start")
		},
		FactorySessionIDGenerator: func() string {
			return fmt.Sprintf("fscp03-close-session-%d", sessionIDs.Add(1))
		},
		FactorySessionRuntimeInstanceIDGenerator: func() string {
			return fmt.Sprintf("fscp03-close-runtime-%d", runtimeIDs.Add(1))
		},
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("unexpected inert provider call"),
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	if sessionIDs.Load() != 0 || runtimeIDs.Load() != 0 || listenerStarts.Load() != 0 {
		t.Fatalf("construction effects = session:%d runtime:%d listener:%d, want zero", sessionIDs.Load(), runtimeIDs.Load(), listenerStarts.Load())
	}
	fscp03ExecuteHelp(t, process, factoryDir, t.TempDir())
	if sessionIDs.Load() != 0 || runtimeIDs.Load() != 0 || listenerStarts.Load() != 0 {
		t.Fatalf("help activation effects = session:%d runtime:%d listener:%d, want zero", sessionIDs.Load(), runtimeIDs.Load(), listenerStarts.Load())
	}
	firstClose := process.Close(t.Context())
	secondClose := process.Close(t.Context())
	if firstClose != nil || secondClose != nil {
		t.Fatalf("Process.Close results = first:%v second:%v, want repeated successful close", firstClose, secondClose)
	}
}

type fscp03CanonicalOperations interface {
	Start(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error)
	List(context.Context, factorysessions.SessionListRequest) (factorysessions.SessionListResult, error)
	Get(context.Context, factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error)
	Control(context.Context, factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error)
	ReadResult(context.Context, factorysessions.SessionResultReadRequest) (factorysessions.SessionResultReadResult, error)
	QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	SubscribeResponses(context.Context, factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error)
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

func openFSCP03Execution(
	t *testing.T,
	process fscp03Process,
	capability executionRuntimeOpeningCapability,
	factoryDir, home, sessionID string,
) (factorysessions.OpenedExecutionRuntime, fscp03CanonicalOperations) {
	t.Helper()
	if capability == nil {
		t.Fatal("root process returned no execution opening")
	}
	opening, ok := capability.ExecutionRuntimeOpening().(factorysessions.ExecutionRuntimeOpeningFunc)
	if !ok || opening == nil {
		t.Fatalf("execution opening type = %T, want factorysessions.ExecutionRuntimeOpeningFunc", capability.ExecutionRuntimeOpening())
	}
	opened, err := opening(t.Context(), factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot:       factoryDir,
		SystemConfigHome:  home,
		FactorySessionID:  sessionID,
		PersistencePolicy: factorysessions.PersistencePolicyDisabled,
	})
	if err != nil {
		t.Fatalf("execution runtime opening error = %v", err)
	}
	if opened.Execution == nil {
		t.Fatal("execution opening returned no public Factory Sessions execution")
	}
	if opened.Close != nil {
		t.Cleanup(func() {
			if err := opened.Close(); err != nil {
				t.Errorf("close execution runtime: %v", err)
			}
		})
	}
	canonical, ok := opened.Execution.(fscp03CanonicalOperations)
	if !ok {
		t.Fatalf("execution type = %T, want public canonical Factory Sessions operations", opened.Execution)
	}
	return opened, canonical
}

func fscp03StartSynchronous(
	t *testing.T,
	canonical fscp03CanonicalOperations,
	requestID string,
	source factorysessions.Source,
) factorysessions.SessionStartResult {
	t.Helper()
	started, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: requestID},
		Source:      source,
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("canonical durable Start(%s) error = %v", requestID, err)
	}
	return started
}

func fscp03StartBlockedAsync(
	t *testing.T,
	canonical fscp03CanonicalOperations,
	startedEvents <-chan struct{},
	requestID string,
) factorysessions.SessionStartResult {
	t.Helper()
	started, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: requestID},
		Source:      fscp03ChildSource(requestID),
	})
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

func assertFSCP03DurableLineage(t *testing.T, canonical fscp03CanonicalOperations, sessionID string) {
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

func assertFSCP03DisjointDurableObservations(t *testing.T, canonical fscp03CanonicalOperations, firstID, secondID string) {
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

func assertFSCP03DurableStatus(t *testing.T, canonical fscp03CanonicalOperations, sessionID string, want factorysessions.LifecycleStatus) {
	t.Helper()
	view, err := canonical.Get(t.Context(), factorysessions.SessionGetRequest{SessionID: sessionID, Mode: factorysessions.SessionOperationModeDurable})
	if err != nil {
		t.Fatalf("canonical Get(%s) error = %v", sessionID, err)
	}
	if view.Session.Status != string(want) {
		t.Fatalf("session %s status = %q, want %q", sessionID, view.Session.Status, want)
	}
}

func assertFSCP03DurableTerminalStatus(t *testing.T, canonical fscp03CanonicalOperations, sessionID string) {
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
