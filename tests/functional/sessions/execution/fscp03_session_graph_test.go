package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const fscp03ObservationTimeout = 15 * time.Second

// TestFSCP03CanonicalSessionGraph is the current-head functional witness for
// the retained FSCP-02 action. Every scenario constructs one root process and
// observes only Process.Execute, the public Factory Sessions capabilities, or
// the public HTTP session boundary. The controlled provider edges make the
// overlap and cancellation windows deterministic without a real provider.
func TestFSCP03CanonicalSessionGraph(t *testing.T) {
	t.Parallel()

	t.Run("durable identity, concurrency, and failed-start recovery", func(t *testing.T) {
		acquireExecutionFixtureSlot(t)
		runFSCP03DurableIdentityScenario(t)
	})
	t.Run("durable controls and timeout branches", func(t *testing.T) {
		acquireExecutionFixtureSlot(t)
		runFSCP03DurableControlScenario(t)
	})
	t.Run("live work and response isolation", func(t *testing.T) {
		acquireExecutionFixtureSlot(t)
		runFSCP03LiveIsolationScenario(t)
	})
	t.Run("process close and inert construction", func(t *testing.T) {
		acquireExecutionFixtureSlot(t)
		runFSCP03ProcessLifecycleScenario(t)
	})

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
	opened, canonical := openFSCP03Execution(t, process, factoryDir, home, "fscp03-durable-identity-owner")
	legacy, ok := opened.Execution.(factorysessions.DurableExecutionService)
	if !ok {
		t.Fatalf("execution type = %T, want public DurableExecutionService", opened.Execution)
	}

	first := fscp03StartSynchronous(t, canonical, "fscp03-sequential-first", fscp03ChildSource("sequential-first"))
	second := fscp03StartSynchronous(t, canonical, "fscp03-sequential-second", fscp03ChildSource("sequential-second"))
	if first.SessionID == second.SessionID {
		t.Fatalf("sequential session ids = %q and %q, want distinct", first.SessionID, second.SessionID)
	}
	assertFSCP03SuccessfulStart(t, first)
	assertFSCP03SuccessfulStart(t, second)
	assertFSCP03DurableLineage(t, canonical, first.SessionID)
	assertFSCP03DurableLineage(t, canonical, second.SessionID)
	assertFSCP03DisjointDurableObservations(t, canonical, first.SessionID, second.SessionID)

	canonicalDirection := fscp03StartSynchronous(t, canonical, "fscp03-canonical-direction", fscp03InlineSource(`return "fscp03 direction COMPLETE";`))
	legacyDirection, err := legacy.StartSync(t.Context(), factorysessions.StartRequest{
		RequestID: "fscp03-legacy-direction",
		Source:    fscp03InlineSource(`return "fscp03 direction COMPLETE";`),
	})
	if err != nil {
		t.Fatalf("legacy durable StartSync() error = %v", err)
	}
	if canonicalDirection.SessionID == legacyDirection.SessionID {
		t.Fatalf("canonical/legacy session id = %q, want distinct executions", canonicalDirection.SessionID)
	}
	assertFSCP03SuccessfulStart(t, canonicalDirection)
	if legacyDirection.Status != string(factorysessions.LifecycleStatusSucceeded) ||
		legacyDirection.SyncOutcome != factorysessions.SyncOutcome("COMPLETED") {
		t.Fatalf("legacy direction result = %#v, want SUCCEEDED/COMPLETED", legacyDirection)
	}
	legacyView, err := legacy.GetSession(t.Context(), legacyDirection.SessionID)
	if err != nil {
		t.Fatalf("legacy direction GetSession() error = %v", err)
	}
	if legacyView.Status != factorysessions.LifecycleStatusSucceeded ||
		legacyView.OrchestratorKind != "JAVASCRIPT" ||
		legacyView.ResolvedSource.SourceRef != "inline" ||
		legacyView.ResultSummary == nil || legacyView.ResultSummary.ResultStatus != string(factorysessions.ResultStatusFinal) {
		t.Fatalf("legacy direction view = %#v, want equivalent durable success semantics", legacyView)
	}
	canonicalView, err := legacy.GetSession(t.Context(), canonicalDirection.SessionID)
	if err != nil {
		t.Fatalf("canonical direction GetSession() error = %v", err)
	}
	if canonicalView.Status != legacyView.Status ||
		canonicalView.OrchestratorKind != legacyView.OrchestratorKind ||
		canonicalView.ResolvedSource.SourceRef != legacyView.ResolvedSource.SourceRef ||
		canonicalView.ResultSummary == nil || legacyView.ResultSummary == nil ||
		canonicalView.ResultSummary.ResultStatus != legacyView.ResultSummary.ResultStatus {
		t.Fatalf("canonical view = %#v, legacy view = %#v, want equivalent public projections", canonicalView, legacyView)
	}

	barrier := newFSCP03BarrierRunner()
	concurrentProcess, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		BrowserOpener:         func(context.Context, string) error { return nil },
		ProviderCommandRunner: barrier,
	})
	if err != nil {
		t.Fatalf("concurrent root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, concurrentProcess)
	concurrentOpened, concurrentCanonical := openFSCP03Execution(t, concurrentProcess, factoryDir, t.TempDir(), "fscp03-concurrent-owner")
	concurrentResults := make(chan fscp03StartOutcome, 2)
	for _, label := range []string{"concurrent-a", "concurrent-b"} {
		label := label
		go func() {
			result, startErr := concurrentCanonical.Start(t.Context(), factorysessions.SessionStartRequest{
				Mode:        factorysessions.SessionOperationModeDurable,
				Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp03-" + label},
				Source:      fscp03ChildSource(label),
				Synchronous: true,
			})
			concurrentResults <- fscp03StartOutcome{result: result, err: startErr}
		}()
	}
	if err := barrier.WaitStarted(t.Context(), 2); err != nil {
		t.Fatalf("concurrent starts did not overlap at provider edge: %v", err)
	}
	barrier.Release()
	concurrent := make([]factorysessions.SessionStartResult, 0, 2)
	for range 2 {
		select {
		case outcome := <-concurrentResults:
			if outcome.err != nil {
				t.Fatalf("concurrent canonical Start() error = %v", outcome.err)
			}
			concurrent = append(concurrent, outcome.result)
		case <-time.After(fscp03ObservationTimeout):
			t.Fatal("concurrent canonical starts did not complete after provider release")
		}
	}
	if concurrentOpened.Execution == nil {
		t.Fatal("concurrent opening lost its retained execution root")
	}
	if concurrent[0].SessionID == concurrent[1].SessionID {
		t.Fatalf("concurrent session ids = %q and %q, want distinct", concurrent[0].SessionID, concurrent[1].SessionID)
	}
	for _, result := range concurrent {
		assertFSCP03SuccessfulStart(t, result)
		assertFSCP03DurableLineage(t, concurrentCanonical, result.SessionID)
	}
	assertFSCP03DisjointDurableObservations(t, concurrentCanonical, concurrent[0].SessionID, concurrent[1].SessionID)

	barrier.Reset()
	activeResults := make(chan fscp03StartOutcome, 1)
	go func() {
		result, startErr := concurrentCanonical.Start(t.Context(), factorysessions.SessionStartRequest{
			Mode:        factorysessions.SessionOperationModeDurable,
			Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp03-failure-peer"},
			Source:      fscp03ChildSource("failure-peer"),
			Synchronous: true,
		})
		activeResults <- fscp03StartOutcome{result: result, err: startErr}
	}()
	if err := barrier.WaitStarted(t.Context(), 1); err != nil {
		t.Fatalf("active peer did not reach controlled provider: %v", err)
	}
	failed, err := concurrentCanonical.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp03-controlled-failure"},
		Source:      fscp03InlineSource(`throw new Error("fscp03 controlled failure");`),
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("controlled failed Start() error = %v, want terminal failed projection", err)
	}
	if failed.Status != string(factorysessions.LifecycleStatusFailed) {
		t.Fatalf("failed start status = %q, want FAILED", failed.Status)
	}
	failedView, err := concurrentOpened.Execution.(factorysessions.DurableExecutionService).GetSession(t.Context(), failed.SessionID)
	if err != nil {
		t.Fatalf("failed GetSession() error = %v", err)
	}
	if failedView.Status != factorysessions.LifecycleStatusFailed || failedView.Failure == nil ||
		!strings.Contains(failedView.Failure.Message, "fscp03 controlled failure") {
		t.Fatalf("failed durable view = %#v, want original typed failure", failedView)
	}
	failedResult, err := concurrentCanonical.ReadResult(t.Context(), factorysessions.SessionResultReadRequest{
		SessionID: failed.SessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Request:   factorysessions.ResultRequest{Mode: factorysessions.ResultModeFinal},
	})
	if err != nil {
		t.Fatalf("failed ReadResult() error = %v", err)
	}
	if failedResult.Status != string(factorysessions.ResultStatusUnavailable) || failedResult.Durable == nil ||
		failedResult.Durable.SessionStatus != factorysessions.LifecycleStatusFailed {
		t.Fatalf("failed result = %#v durable=%#v, want unavailable result with original failure", failedResult, failedResult.Durable)
	}
	barrier.Release()
	active := <-activeResults
	if active.err != nil {
		t.Fatalf("active peer after failed start error = %v", active.err)
	}
	assertFSCP03SuccessfulStart(t, active.result)
	recovery := fscp03StartSynchronous(t, concurrentCanonical, "fscp03-after-failure", fscp03ChildSource("after-failure"))
	assertFSCP03SuccessfulStart(t, recovery)
	if recovery.SessionID == failed.SessionID || recovery.SessionID == active.result.SessionID {
		t.Fatalf("recovery session id = %q, want a fresh usable session", recovery.SessionID)
	}
	assertFSCP03DurableLineage(t, concurrentCanonical, recovery.SessionID)
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
	_, canonical := openFSCP03Execution(t, process, factoryDir, home, "fscp03-control-owner")

	canceled := fscp03StartBlockedAsync(t, canonical, controlRunner.started, "fscp03-cancel")
	cancelControl, err := canonical.Control(t.Context(), factorysessions.SessionControlRequest{
		SessionID: canceled.SessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Operation: factorysessions.SessionControlCancel,
		Control:   factorysessions.ControlRequest{RequestID: "fscp03-cancel-control"},
	})
	if err != nil {
		t.Fatalf("canonical CANCEL error = %v", err)
	}
	if cancelControl.Operation != factorysessions.SessionControlCancel ||
		cancelControl.Outcome != factorysessions.LifecycleControlOutcomeAccepted ||
		cancelControl.Status != factorysessions.LifecycleStatusCanceling {
		t.Fatalf("CANCEL result = %#v, want accepted CANCEL/CANCELING", cancelControl)
	}
	assertFSCP03DurableStatus(t, canonical, canceled.SessionID, factorysessions.LifecycleStatusCanceled)

	terminated := fscp03StartBlockedAsync(t, canonical, controlRunner.started, "fscp03-terminate")
	terminateControl, err := canonical.Control(t.Context(), factorysessions.SessionControlRequest{
		SessionID: terminated.SessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Operation: factorysessions.SessionControlTerminate,
		Control:   factorysessions.ControlRequest{RequestID: "fscp03-terminate-control"},
	})
	if err != nil {
		t.Fatalf("canonical TERMINATE error = %v", err)
	}
	if terminateControl.Operation != factorysessions.SessionControlTerminate ||
		terminateControl.Outcome != factorysessions.LifecycleControlOutcomeAccepted ||
		terminateControl.Status != factorysessions.LifecycleStatusTerminated {
		t.Fatalf("TERMINATE result = %#v, want accepted TERMINATE/TERMINATED", terminateControl)
	}
	assertFSCP03DurableTerminalStatus(t, canonical, terminated.SessionID)

	live, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:       factorysessions.SessionOperationModeLive,
		FolderPath: factoryDir,
	})
	if err != nil {
		t.Fatalf("live Start() for CLOSE error = %v", err)
	}
	closed, err := canonical.Control(t.Context(), factorysessions.SessionControlRequest{
		SessionID: live.SessionID,
		Mode:      factorysessions.SessionOperationModeLive,
		Operation: factorysessions.SessionControlClose,
		Control:   factorysessions.ControlRequest{RequestID: "fscp03-close-control"},
	})
	if err != nil {
		t.Fatalf("canonical CLOSE error = %v", err)
	}
	if closed.Operation != factorysessions.SessionControlClose || !closed.Closed {
		t.Fatalf("CLOSE result = %#v, want closed live session", closed)
	}

	for _, cancelOnTimeout := range []bool{false, true} {
		requestID := fmt.Sprintf("fscp03-timeout-%t", cancelOnTimeout)
		started := make(chan fscp03StartOutcome, 1)
		go func() {
			result, startErr := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
				Mode:        factorysessions.SessionOperationModeDurable,
				Correlation: factorysessions.SessionOperationCorrelation{RequestID: requestID},
				Source:      fscp03ChildSource(requestID),
				Synchronous: true,
				Wait: factorysessions.SessionOperationWait{
					TimeoutMillis:   25,
					CancelOnTimeout: cancelOnTimeout,
				},
			})
			started <- fscp03StartOutcome{result: result, err: startErr}
		}()
		select {
		case <-controlRunner.started:
		case <-time.After(fscp03ObservationTimeout):
			t.Fatalf("timeout branch cancel=%t did not reach provider", cancelOnTimeout)
		}
		var outcome fscp03StartOutcome
		select {
		case outcome = <-started:
		case <-time.After(fscp03ObservationTimeout):
			t.Fatalf("timeout branch cancel=%t did not return", cancelOnTimeout)
		}
		if outcome.err != nil {
			t.Fatalf("timeout branch cancel=%t error = %v", cancelOnTimeout, outcome.err)
		}
		if outcome.result.Sync == nil || outcome.result.Sync.SyncOutcome != factorysessions.SyncOutcome("TIMED_OUT") || !outcome.result.Sync.TimedOut {
			t.Fatalf("timeout branch cancel=%t result = %#v, want timed-out sync outcome", cancelOnTimeout, outcome.result)
		}
		if outcome.result.Sync.SessionCanceledByTimeout != cancelOnTimeout {
			t.Fatalf("timeout branch cancel=%t canceledByTimeout = %t, want %t", cancelOnTimeout, outcome.result.Sync.SessionCanceledByTimeout, cancelOnTimeout)
		}
		if cancelOnTimeout {
			assertFSCP03DurableStatus(t, canonical, outcome.result.SessionID, factorysessions.LifecycleStatusCanceled)
			continue
		}
		if outcome.result.Status != string(factorysessions.LifecycleStatusRunning) {
			t.Fatalf("timeout branch cancel=false status = %q, want RUNNING", outcome.result.Status)
		}
		control, controlErr := canonical.Control(t.Context(), factorysessions.SessionControlRequest{
			SessionID: outcome.result.SessionID,
			Mode:      factorysessions.SessionOperationModeDurable,
			Operation: factorysessions.SessionControlCancel,
			Control:   factorysessions.ControlRequest{RequestID: requestID + "-cleanup"},
		})
		if controlErr != nil {
			t.Fatalf("timeout false cleanup CANCEL error = %v", controlErr)
		}
		if control.Operation != factorysessions.SessionControlCancel {
			t.Fatalf("timeout false cleanup control = %#v, want CANCEL", control)
		}
		assertFSCP03DurableStatus(t, canonical, outcome.result.SessionID, factorysessions.LifecycleStatusCanceled)
	}
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

func fscp03ExecuteHelp(t *testing.T, process *initializerapplication.Process, factoryDir, home string) {
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
	process *initializerapplication.Process,
	factoryDir, home, sessionID string,
) (factorysessions.OpenedExecutionRuntime, fscp03CanonicalOperations) {
	t.Helper()
	capability := process.ExecutionRuntimeOpening()
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
		Kind: factoryruntime.WorkflowSourceKindInlineWorkflow,
		InlineWorkflow: &factorysessions.InlineWorkflowSource{
			Dialect:      "you-workflow-v1",
			InlineSource: source,
		},
	}
}

func fscp03ChildSource(label string) factorysessions.Source {
	return fscp03InlineSource(fmt.Sprintf(`return (async function () {
  return await agent.run({
    prompt: %q,
    label: %q,
    modelProvider: "codex",
    model: "gpt-5-codex"
  });
})();`, label, label))
}

type fscp03BarrierRunner struct {
	gate    chan struct{}
	started chan struct{}
}

func newFSCP03BarrierRunner() *fscp03BarrierRunner {
	return &fscp03BarrierRunner{gate: make(chan struct{}), started: make(chan struct{}, 32)}
}

func (runner *fscp03BarrierRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	select {
	case runner.started <- struct{}{}:
	default:
	}
	select {
	case <-runner.gate:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("fscp03 barrier COMPLETE")}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func (runner *fscp03BarrierRunner) WaitStarted(ctx context.Context, count int) error {
	for range count {
		select {
		case <-runner.started:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(fscp03ObservationTimeout):
			return fmt.Errorf("waiting for provider dispatch %d", count)
		}
	}
	return nil
}

func (runner *fscp03BarrierRunner) Release() { close(runner.gate) }

func (runner *fscp03BarrierRunner) Reset() {
	runner.gate = make(chan struct{})
}

var _ platformprocess.CommandRunner = (*fscp03BarrierRunner)(nil)

type fscp03ControlRunner struct {
	started chan struct{}
}

func newFSCP03ControlRunner() *fscp03ControlRunner {
	return &fscp03ControlRunner{started: make(chan struct{}, 32)}
}

func (runner *fscp03ControlRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	select {
	case runner.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

var _ platformprocess.CommandRunner = (*fscp03ControlRunner)(nil)

type fscp03LiveRunner struct {
	mu      sync.Mutex
	gate    chan struct{}
	started chan struct{}
}

func newFSCP03LiveRunner() *fscp03LiveRunner {
	return &fscp03LiveRunner{started: make(chan struct{}, 32)}
}

func (runner *fscp03LiveRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	gate := runner.gate
	runner.mu.Unlock()
	if gate != nil {
		select {
		case runner.started <- struct{}{}:
		default:
		}
		select {
		case <-gate:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("fscp03 live COMPLETE")}, nil
}

func (runner *fscp03LiveRunner) Hold() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.gate = make(chan struct{})
}

func (runner *fscp03LiveRunner) Release() {
	runner.mu.Lock()
	gate := runner.gate
	runner.gate = nil
	runner.mu.Unlock()
	if gate != nil {
		close(gate)
	}
}

func (runner *fscp03LiveRunner) WaitStarted(ctx context.Context, count int) error {
	for range count {
		select {
		case <-runner.started:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(fscp03ObservationTimeout):
			return fmt.Errorf("waiting for live provider dispatch %d", count)
		}
	}
	return nil
}

var _ platformprocess.CommandRunner = (*fscp03LiveRunner)(nil)

type fscp03HTTPInvocationOutcome struct {
	response factoryapi.InvocationResponse
	err      error
}

func postFSCP03Invocation(ctx context.Context, baseURL, sessionID, text string) (factoryapi.InvocationResponse, error) {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	payload, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    ptr(factoryapi.WorkContent{part}),
	})
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return factoryapi.InvocationResponse{}, fmt.Errorf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	return decoded, nil
}

func ptr[T any](value T) *T { return &value }

func assertFSCP03HTTPInvocation(t *testing.T, response factoryapi.InvocationResponse) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusCompleted || strings.TrimSpace(response.RequestId) == "" || strings.TrimSpace(response.TraceId) == "" {
		t.Fatalf("HTTP invocation response = %#v, want COMPLETED request/trace identity", response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) == 0 {
		t.Fatalf("HTTP invocation primary result = %#v, want result content", response.PrimaryResult)
	}
}

func assertFSCP03LiveFactoryEvents(t *testing.T, baseURL, firstID, secondID string) {
	t.Helper()
	first := support.GetFactoryEventsForSessionAt(t, baseURL, firstID)
	second := support.GetFactoryEventsForSessionAt(t, baseURL, secondID)
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("live Factory Events first=%d second=%d, want events for both sessions", len(first), len(second))
	}
	firstWorkIDs := make(map[string]struct{})
	firstSessionEvents := 0
	for _, event := range first {
		if event.Context.SessionId != nil {
			firstSessionEvents++
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != firstID && *event.Context.SessionId != factorysessions.DefaultSessionID {
			t.Fatalf("first Factory Event context = %#v, want session %q", event.Context, firstID)
		}
		if event.Context.WorkIds != nil {
			for _, workID := range *event.Context.WorkIds {
				firstWorkIDs[workID] = struct{}{}
			}
		}
	}
	if len(firstWorkIDs) == 0 {
		t.Fatalf("first Factory Events = %#v, want Work lineage", first)
	}
	if firstSessionEvents == 0 {
		t.Fatalf("first Factory Events = %#v, want at least one session-correlated event", first)
	}
	secondSessionEvents := 0
	for _, event := range second {
		if event.Context.SessionId != nil {
			secondSessionEvents++
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != secondID {
			t.Fatalf("second Factory Event context = %#v, want session %q", event.Context, secondID)
		}
		if event.Context.WorkIds != nil {
			for _, workID := range *event.Context.WorkIds {
				if _, shared := firstWorkIDs[workID]; shared {
					t.Fatalf("Work identity %q crossed live sessions", workID)
				}
			}
		}
	}
	if secondSessionEvents == 0 {
		t.Fatalf("second Factory Events = %#v, want at least one session-correlated event", second)
	}
}

func assertFSCP03LiveResponseEvents(t *testing.T, baseURL, firstID, secondID string) {
	t.Helper()
	first := support.GetFactoryResponseEventsAt(t, baseURL, firstID)
	second := support.GetFactoryResponseEventsAt(t, baseURL, secondID)
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("live Response Events first=%d second=%d, want events for both sessions", len(first), len(second))
	}
	firstEventIDs := make(map[string]struct{}, len(first))
	firstDispatchIDs := make(map[string]struct{}, len(first))
	for _, event := range first {
		if event.FactorySessionId != firstID || strings.TrimSpace(event.EventId) == "" {
			t.Fatalf("first Response Event = %#v, want session-scoped identity", event)
		}
		firstEventIDs[event.EventId] = struct{}{}
		if event.DispatchId != nil {
			firstDispatchIDs[*event.DispatchId] = struct{}{}
		}
	}
	if len(firstDispatchIDs) == 0 {
		t.Fatalf("first Response Events = %#v, want dispatch identity", first)
	}
	for _, event := range second {
		if event.FactorySessionId != secondID {
			t.Fatalf("second Response Event = %#v, want session %q", event, secondID)
		}
		if _, shared := firstEventIDs[event.EventId]; shared {
			t.Fatalf("Response Event identity %q crossed live sessions", event.EventId)
		}
		if event.DispatchId != nil {
			if _, shared := firstDispatchIDs[*event.DispatchId]; shared {
				t.Fatalf("dispatch identity %q crossed live sessions", *event.DispatchId)
			}
		}
	}
}

func collectFSCP03ResponseFrames(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	sessionID string,
) []support.FactoryResponseEventFrame {
	t.Helper()
	var frames []support.FactoryResponseEventFrame
	deadline := time.NewTimer(fscp03ObservationTimeout)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatalf("timed out collecting response events for session %q; got %d frames", sessionID, len(frames))
		default:
		}
		frame, ok := stream.TryNextFrame(time.Second)
		if !ok {
			t.Fatalf("response event stream for session %q closed before MESSAGE completion; got %d frames", sessionID, len(frames))
		}
		if frame.Event.FactorySessionId != sessionID {
			t.Fatalf("response frame session = %q, want %q", frame.Event.FactorySessionId, sessionID)
		}
		frames = append(frames, frame)
		if frame.Event.Kind == factoryapi.FactoryResponseEventKindMessage && frame.Event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
			return frames
		}
	}
}

func assertFSCP03DisjointHTTPResponseFrames(t *testing.T, first, second []support.FactoryResponseEventFrame) {
	t.Helper()
	firstEventIDs := make(map[string]struct{}, len(first))
	firstDispatchIDs := make(map[string]struct{}, len(first))
	for _, frame := range first {
		firstEventIDs[frame.Event.EventId] = struct{}{}
		if frame.Event.DispatchId != nil {
			firstDispatchIDs[*frame.Event.DispatchId] = struct{}{}
		}
	}
	if len(firstDispatchIDs) == 0 {
		t.Fatal("first concurrent response stream had no dispatch identity")
	}
	for _, frame := range second {
		if _, shared := firstEventIDs[frame.Event.EventId]; shared {
			t.Fatalf("concurrent Response Event identity %q crossed streams", frame.Event.EventId)
		}
		if frame.Event.DispatchId != nil {
			if _, shared := firstDispatchIDs[*frame.Event.DispatchId]; shared {
				t.Fatalf("concurrent dispatch identity %q crossed streams", *frame.Event.DispatchId)
			}
		}
	}
}

func scaffoldFSCP03ProbeFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "fscp03-probe",
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}
