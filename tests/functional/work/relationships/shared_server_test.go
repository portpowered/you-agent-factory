package relationships

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// sharedRelationshipHost owns the one root-built application process used by
// every eligible relationship scenario. Provider behavior is registered by
// fixture directory so parallel Factory Sessions cannot consume one another's
// response sequence.
type sharedRelationshipHost struct {
	server   *support.FunctionalAPIServer
	provider *sharedRelationshipProviderRouter
}

func (host *sharedRelationshipHost) URL() string {
	if host == nil || host.server == nil {
		return ""
	}
	return host.server.URL()
}

// sharedRelationshipProviderRouter keeps the controlled provider edge
// process-scoped while selecting one immutable scenario route per fixture.
// Unregistered fixtures receive an accepted result for the ordinary fan-out
// and dependency cases.
type sharedRelationshipProviderRouter struct {
	testutil.NativeProvider
	mu     sync.RWMutex
	routes map[string]providers.Service
}

func newSharedRelationshipProviderRouter() *sharedRelationshipProviderRouter {
	return &sharedRelationshipProviderRouter{routes: make(map[string]providers.Service)}
}

func (router *sharedRelationshipProviderRouter) register(
	t testing.TB,
	factoryDir string,
	provider providers.Service,
) {
	t.Helper()
	if provider == nil {
		t.Fatal("shared relationship provider route is nil")
	}
	key := sharedRelationshipPathKey(factoryDir)
	if key == "" {
		t.Fatal("shared relationship provider route has an empty Factory directory")
	}
	router.mu.Lock()
	if _, exists := router.routes[key]; exists {
		router.mu.Unlock()
		t.Fatalf("shared relationship provider route %q is already registered", key)
	}
	router.routes[key] = provider
	router.mu.Unlock()
	t.Cleanup(func() {
		if err := router.unregister(factoryDir); err != nil {
			t.Errorf("unregister shared relationship provider route %q: %v", factoryDir, err)
		}
	})
}

func (router *sharedRelationshipProviderRouter) unregister(factoryDir string) error {
	key := sharedRelationshipPathKey(factoryDir)
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[key]; !exists {
		return fmt.Errorf("route is not registered")
	}
	delete(router.routes, key)
	return nil
}

func (router *sharedRelationshipProviderRouter) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	key := sharedRelationshipPathKey(request.FactoryDirectory)
	if key == "" {
		key = sharedRelationshipPathKey(request.WorkingDirectory)
	}
	router.mu.RLock()
	provider := router.routes[key]
	router.mu.RUnlock()
	if provider != nil {
		return provider.Execute(ctx, request)
	}
	return sharedRelationshipAcceptedResult(), nil
}

func (router *sharedRelationshipProviderRouter) routeCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.routes)
}

func sharedRelationshipAcceptedResult() providers.ExecuteResult {
	return providers.ExecuteResult{
		Content: "<COMPLETE>",
	}
}

func sharedRelationshipPathKey(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

// relationshipProviderGate is a deterministic provider-edge barrier. Arrival
// is observable before the attempt waits, and release or session termination
// always unblocks the attempt without a polling sleep.
type relationshipProviderGate struct {
	targetWorkID  string
	arrived       chan struct{}
	release       chan struct{}
	cancelled     chan struct{}
	arrivedOnce   sync.Once
	releaseOnce   sync.Once
	cancelledOnce sync.Once
	mu            sync.Mutex
	callCount     int
	lastRequest   providers.ExecuteRequest
}

func newRelationshipProviderGate(t testing.TB, targetWorkID string) *relationshipProviderGate {
	t.Helper()
	if strings.TrimSpace(targetWorkID) == "" {
		t.Fatal("relationship provider gate target Work ID is empty")
	}
	gate := &relationshipProviderGate{
		targetWorkID: targetWorkID,
		arrived:      make(chan struct{}),
		release:      make(chan struct{}),
		cancelled:    make(chan struct{}),
	}
	t.Cleanup(gate.Release)
	return gate
}

func (gate *relationshipProviderGate) WaitForArrival(t testing.TB, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-gate.arrived:
	case <-timer.C:
		gate.mu.Lock()
		callCount := gate.callCount
		lastRequest := gate.lastRequest.Clone()
		gate.mu.Unlock()
		t.Fatalf(
			"timed out waiting for provider gate arrival for Work %q after %d calls; last request=%#v",
			gate.targetWorkID,
			callCount,
			lastRequest,
		)
	}
}

func (gate *relationshipProviderGate) WaitForCancellation(t testing.TB, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-gate.cancelled:
	case <-timer.C:
		t.Fatalf("timed out waiting for provider gate cancellation for Work %q", gate.targetWorkID)
	}
}

func (gate *relationshipProviderGate) Release() {
	if gate == nil {
		return
	}
	gate.releaseOnce.Do(func() { close(gate.release) })
}

func (gate *relationshipProviderGate) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	gate.mu.Lock()
	gate.callCount++
	gate.lastRequest = request.Clone()
	gate.mu.Unlock()
	if request.TransitionID != dependencyFinishWorkstation || !sharedRelationshipRequestIncludesWork(request, gate.targetWorkID) {
		return sharedRelationshipAcceptedResult(), nil
	}
	gate.arrivedOnce.Do(func() { close(gate.arrived) })
	select {
	case <-gate.release:
		return sharedRelationshipAcceptedResult(), nil
	case <-ctx.Done():
		gate.cancelledOnce.Do(func() { close(gate.cancelled) })
		return providers.ExecuteResult{}, ctx.Err()
	}
}

func sharedRelationshipRequestIncludesWork(request providers.ExecuteRequest, workID string) bool {
	for _, candidate := range request.Correlation.WorkIDs {
		if candidate == workID {
			return true
		}
	}
	return support.FirstInputWorkID(request.InputTokens) == workID
}

func sharedRelationshipGateProvider(gate *relationshipProviderGate) providers.Service {
	provider := testutil.NativeProvider{}
	provider.ExecuteFunc = gate.Execute
	return provider
}

func sharedRelationshipFailureProvider(targetWorkID string, failure error) providers.Service {
	provider := testutil.NativeProvider{}
	provider.ExecuteFunc = func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
		if request.TransitionID == dependencyFinishWorkstation && sharedRelationshipRequestIncludesWork(request, targetWorkID) {
			return providers.ExecuteResult{}, failure
		}
		return sharedRelationshipAcceptedResult(), nil
	}
	return provider
}

// TestSharedServerRelationships keeps one customer-hosted application process
// alive while independent relationship scenarios execute in isolated Factory
// Sessions. The two built-CLI cases remain top-level for their executable-
// boundary proof.
func TestSharedServerRelationships(t *testing.T) {
	t.Parallel()

	hostFactoryDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_simple_dir"))
	providerRouter := newSharedRelationshipProviderRouter()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: hostFactoryDir,
		Edges: serviceedges.Edges{
			ProviderOverride:    providerRouter,
			ScriptCommandRunner: support.NewStaticSuccessCommandRunner("COMPLETE"),
		},
		WaitForServiceModeRuntime: true,
	})
	host := &sharedRelationshipHost{server: server, provider: providerRouter}
	t.Cleanup(func() {
		if got := providerRouter.routeCount(); got != 0 {
			t.Errorf("shared relationship provider routes after child cleanup = %d, want zero", got)
		}
	})

	tests := []struct {
		name string
		run  func(*testing.T, *sharedRelationshipHost)
	}{
		{name: "MultiOutputFanoutPreservesSourceNameOnDownstreamWork", run: func(t *testing.T, host *sharedRelationshipHost) {
			testMultiOutputFanoutPreservesSourceNameOnDownstreamWork(t, host)
		}},
		{name: "MultiOutputNameAvailableOnDownstreamTask", run: func(t *testing.T, host *sharedRelationshipHost) {
			testMultiOutputNameAvailableOnDownstreamTask(t, host)
		}},
		{name: "ReviewerFanoutPreservesSharedNameDownstream", run: func(t *testing.T, host *sharedRelationshipHost) {
			testReviewerFanoutPreservesSharedNameDownstream(t, host)
		}},
		{name: "DocReviewerPNGFanoutPreservesSharedNameDownstream", run: func(t *testing.T, host *sharedRelationshipHost) {
			testDocReviewerPNGFanoutPreservesSharedNameDownstream(t, host)
		}},
		{name: "NtoNTypeMatchingCompletesEveryAuthoredBranch", run: func(t *testing.T, host *sharedRelationshipHost) {
			testNtoNTypeMatchingCompletesEveryAuthoredBranch(t, host)
		}},
		{name: "DependentWorkWaitsForPrerequisiteTargetState", run: func(t *testing.T, host *sharedRelationshipHost) {
			testDependentWorkWaitsForPrerequisiteTargetState(t, host)
		}},
		{name: "WorkWithoutDependsOnRelationsDispatchesNormally", run: func(t *testing.T, host *sharedRelationshipHost) {
			testWorkWithoutDependsOnRelationsDispatchesNormally(t, host)
		}},
		{name: "DependentWorkBlockedUntilPrerequisiteArchived", run: func(t *testing.T, host *sharedRelationshipHost) {
			testDependentWorkBlockedUntilPrerequisiteArchived(t, host)
		}},
		{name: "DependentWorkBlockedDuringPrerequisiteProcessing", run: func(t *testing.T, host *sharedRelationshipHost) {
			testDependentWorkBlockedDuringPrerequisiteProcessing(t, host)
		}},
		{name: "DependentWorkAndPrerequisiteBothReachArchived", run: func(t *testing.T, host *sharedRelationshipHost) {
			testDependentWorkAndPrerequisiteBothReachArchived(t, host)
		}},
		{name: "FanInReleasesOnlyAfterEveryPrerequisite", run: func(t *testing.T, host *sharedRelationshipHost) {
			fanInGate := newRelationshipProviderGate(t, "task-prerequisite-b")
			testFanInReleasesOnlyAfterEveryPrerequisite(t, host, fanInGate)
		}},
		{name: "CrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion", run: func(t *testing.T, host *sharedRelationshipHost) {
			crossBatchGate := newRelationshipProviderGate(t, crossBatchPrerequisiteID)
			testCrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion(t, host, crossBatchGate)
		}},
		{name: "CrossBatchDependsOnFailedTargetCascadesAtAdmission", run: func(t *testing.T, host *sharedRelationshipHost) {
			testCrossBatchDependsOnFailedTargetCascadesAtAdmission(t, host)
		}},
		{name: "DependentWorkDoesNotDispatchAfterPrerequisiteFailure", run: func(t *testing.T, host *sharedRelationshipHost) {
			testDependentWorkDoesNotDispatchAfterPrerequisiteFailure(t, host)
		}},
		{name: "CrossBatchDependsOnRejectsCrossSessionTargetAtomically", run: func(t *testing.T, host *sharedRelationshipHost) {
			testCrossBatchDependsOnRejectsCrossSessionTargetAtomically(t, host)
		}},
		{name: "CrossBatchDependsOnCompletedTargetReleasesAtAdmission", run: func(t *testing.T, host *sharedRelationshipHost) {
			testCrossBatchDependsOnCompletedTargetReleasesAtAdmission(t, host)
		}},
		{name: "CrossBatchDependsOnMixedTerminalFanInCascades", run: func(t *testing.T, host *sharedRelationshipHost) {
			testCrossBatchDependsOnMixedTerminalFanInCascades(t, host)
		}},
		{name: "DispatchPreservesSubmittedWorkPayloadTagsAndType", run: func(t *testing.T, host *sharedRelationshipHost) {
			testDispatchPreservesSubmittedWorkPayloadTagsAndType(t, host)
		}},
		{name: "RejectionFeedbackSurfacesOnExecutorRetry", run: func(t *testing.T, host *sharedRelationshipHost) {
			testRejectionFeedbackSurfacesOnExecutorRetry(t, host)
		}},
		{name: "ParentAndDependsOnLineageSurviveOnChildDispatch", run: func(t *testing.T, host *sharedRelationshipHost) {
			testParentAndDependsOnLineageSurviveOnChildDispatch(t, host)
		}},
		{name: "DependentWorkFailsWhenDirectPrerequisiteFails", run: func(t *testing.T, host *sharedRelationshipHost) {
			testDependentWorkFailsWhenDirectPrerequisiteFails(t, host)
		}},
		{name: "TransitiveDependencyFailureCascadesToFailedTerminals", run: func(t *testing.T, host *sharedRelationshipHost) {
			testTransitiveDependencyFailureCascadesToFailedTerminals(t, host)
		}},
		{name: "CompletedPrerequisiteIsNotCascadedWhenDependentFails", run: func(t *testing.T, host *sharedRelationshipHost) {
			testCompletedPrerequisiteIsNotCascadedWhenDependentFails(t, host)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t, host)
		})
	}
	t.Run("LifecycleCleanupProbes", func(t *testing.T) {
		runSharedRelationshipLifecycleProbes(t, host)
	})
}

func runSharedRelationshipLifecycleProbes(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()

	t.Run("CancellationReleasesGatedAttempt", func(t *testing.T) {
		dir := scaffoldCrossBatchFactory(t)
		gate := newRelationshipProviderGate(t, crossBatchPrerequisiteID)
		host.provider.register(t, dir, sharedRelationshipGateProvider(gate))
		session, closeSession := openSharedRelationshipSession(t, host.URL(), dir)

		executeCrossBatchSubmitForSessionOnServer(t, host.server, session.Id, crossBatchPrerequisiteBatchJSON())
		gate.WaitForArrival(t, 15*time.Second)
		support.TerminateFactorySessionAt(t, host.URL(), session.Id)
		gate.WaitForCancellation(t, 15*time.Second)
		support.WaitForSessionStopped(t, host.URL(), session.Id, 15*time.Second)
		closeSession()
		runSharedHostReuseProbe(t, host.URL())
	})

	t.Run("BoundedObservationTimeoutPreservesDiagnostic", func(t *testing.T) {
		dir := scaffoldCrossBatchFactory(t)
		gate := newRelationshipProviderGate(t, crossBatchPrerequisiteID)
		host.provider.register(t, dir, sharedRelationshipGateProvider(gate))
		session, closeSession := openSharedRelationshipSession(t, host.URL(), dir)

		executeCrossBatchSubmitForSessionOnServer(t, host.server, session.Id, crossBatchPrerequisiteBatchJSON())
		gate.WaitForArrival(t, 15*time.Second)
		lastListed, err := support.WaitForObservation(
			100*time.Millisecond,
			func() (factoryapi.ListWorkResponse, error) {
				return listRelationshipSessionWork(t, host.URL(), session.Id), nil
			},
			func(listed factoryapi.ListWorkResponse) bool {
				return support.HasWorkAtCustomerState(listed, crossBatchPrerequisiteID, support.WorkCustomerLocation("task", "complete"))
			},
		)
		if err == nil || !strings.Contains(err.Error(), "timed out after") {
			t.Fatalf("bounded relationship observation error = %v, want timeout diagnostic", err)
		}
		if !support.HasWorkAtCustomerState(lastListed, crossBatchPrerequisiteID, support.WorkCustomerLocation("task", "processing")) {
			t.Fatalf("timed-out observation changed gated Work to an unexpected state: %#v", lastListed.Results)
		}
		closeSession()
		gate.WaitForCancellation(t, 15*time.Second)
		runSharedHostReuseProbe(t, host.URL())
	})

	t.Run("EarlyReturnRetainsPrimaryDiagnostic", func(t *testing.T) {
		primary := errors.New("synthetic relationship assertion failure")
		if err := runSharedRelationshipEarlyReturn(t, host, primary); !errors.Is(err, primary) {
			t.Fatalf("early-return error = %v, want primary diagnostic %v", err, primary)
		}
		runSharedHostReuseProbe(t, host.URL())
	})
}

func runSharedRelationshipEarlyReturn(t *testing.T, host *sharedRelationshipHost, primary error) error {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_simple_dir"))
	workID := "shared-host-early-return"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     workID,
		Payload:    []byte("shared host early return"),
	})
	_, closeSession := openSharedRelationshipSession(t, host.URL(), dir)
	defer closeSession()
	return primary
}

func runSharedRelationshipFactoryToCompletion(
	t *testing.T,
	host *sharedRelationshipHost,
	factoryDir string,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	return runSharedRelationshipFactoryToCompletionMode(t, host, factoryDir, timeout, false)
}

func runSharedRelationshipFactoryToCompletionAndClose(
	t *testing.T,
	host *sharedRelationshipHost,
	factoryDir string,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	return runSharedRelationshipFactoryToCompletionMode(t, host, factoryDir, timeout, true)
}

func runSharedRelationshipFactoryToCompletionMode(
	t *testing.T,
	host *sharedRelationshipHost,
	factoryDir string,
	timeout time.Duration,
	closeAfterObservation bool,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()

	baseURL := host.URL()
	session, closeSession := openSharedRelationshipSession(t, baseURL, factoryDir)
	sessionID := session.Id
	support.WaitForSessionTerminalStatus(t, baseURL, sessionID, timeout)

	sessionResponse := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	latestSession, err := sessionResponse.AsFactorySession()
	if err != nil {
		t.Fatalf("decode shared-host Factory Session %q: %v", sessionID, err)
	}
	listed := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	if closeAfterObservation {
		closeSession()
	}
	return latestSession, listed, events
}

func openSharedRelationshipSession(
	t *testing.T,
	baseURL string,
	factoryDir string,
) (factoryapi.FactorySessionSummary, func()) {
	t.Helper()

	opened := support.OpenFactorySessionAt(t, baseURL, factoryDir)
	if opened.Session == nil {
		t.Fatalf("shared-host open response missing session: %#v", opened)
	}
	session := *opened.Session
	closed := false
	closeSession := func() {
		if closed {
			return
		}
		closed = true
		support.CloseFactorySessionAt(t, baseURL, session.Id)
	}
	t.Cleanup(closeSession)
	return session, closeSession
}

func runSharedHostReuseProbe(t *testing.T, baseURL string) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_simple_dir"))
	workID := "shared-host-reuse-probe"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     workID,
		Payload:    []byte("shared host reuse probe"),
	})

	session, closeSession := openSharedRelationshipSession(t, baseURL, dir)
	support.WaitForSessionTerminalStatus(t, baseURL, session.Id, 10*time.Second)
	listed := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(session.Id)+"/work",
	)
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("shared-host reuse probe Work %q did not complete: %#v", workID, listed.Results)
	}
	closeSession()
}
