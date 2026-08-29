package relationships

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSharedServerRelationships keeps one customer-hosted application process
// alive while independent relationship scenarios execute in isolated Factory
// Sessions. The remaining eligible top-level cases retain their process-scoped
// edges until the next migration story; the two built-CLI cases remain
// top-level for their executable-boundary proof.
func TestSharedServerRelationships(t *testing.T) {
	t.Parallel()

	hostFactoryDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_simple_dir"))
	fanInGate := support.NewMockWorkerGate(t)
	crossBatchGate := support.NewMockWorkerGate(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: hostFactoryDir,
		MockWorkersConfig: &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{
			{
				ID:              "shared-cross-batch-failed-prerequisite",
				WorkstationName: dependencyFinishWorkstation,
				WorkInputs: []workers.MockWorkInputSelector{{
					WorkID: crossBatchFailedPrerequisiteID,
				}},
				RunType: workers.MockWorkerRunTypeReject,
			},
			{
				ID:              "shared-dependency-failed-prerequisite",
				WorkstationName: dependencyFinishWorkstation,
				WorkInputs: []workers.MockWorkInputSelector{{
					WorkID: sharedDependencyFailurePrerequisiteID,
				}},
				RunType: workers.MockWorkerRunTypeReject,
			},
			{
				ID:              "partial-fan-in-second-prerequisite",
				WorkstationName: dependencyFinishWorkstation,
				WorkInputs: []workers.MockWorkInputSelector{{
					WorkID: "task-prerequisite-b",
				}},
				RunType:    workers.MockWorkerRunTypeAccept,
				GateConfig: fanInGate.Config(15 * time.Second),
			},
			{
				ID:              "active-cross-batch-prerequisite",
				WorkstationName: dependencyFinishWorkstation,
				WorkInputs: []workers.MockWorkInputSelector{{
					WorkID: crossBatchPrerequisiteID,
				}},
				RunType:    workers.MockWorkerRunTypeAccept,
				GateConfig: crossBatchGate.Config(15 * time.Second),
			},
		}},
		WaitForServiceModeRuntime: true,
	})

	tests := []struct {
		name string
		run  func(*testing.T, string)
	}{
		{name: "MultiOutputFanoutPreservesSourceNameOnDownstreamWork", run: testMultiOutputFanoutPreservesSourceNameOnDownstreamWork},
		{name: "MultiOutputNameAvailableOnDownstreamTask", run: testMultiOutputNameAvailableOnDownstreamTask},
		{name: "ReviewerFanoutPreservesSharedNameDownstream", run: testReviewerFanoutPreservesSharedNameDownstream},
		{name: "DocReviewerPNGFanoutPreservesSharedNameDownstream", run: testDocReviewerPNGFanoutPreservesSharedNameDownstream},
		{name: "NtoNTypeMatchingCompletesEveryAuthoredBranch", run: testNtoNTypeMatchingCompletesEveryAuthoredBranch},
		{name: "DependentWorkWaitsForPrerequisiteTargetState", run: testDependentWorkWaitsForPrerequisiteTargetState},
		{name: "WorkWithoutDependsOnRelationsDispatchesNormally", run: testWorkWithoutDependsOnRelationsDispatchesNormally},
		{name: "DependentWorkBlockedUntilPrerequisiteArchived", run: testDependentWorkBlockedUntilPrerequisiteArchived},
		{name: "DependentWorkBlockedDuringPrerequisiteProcessing", run: testDependentWorkBlockedDuringPrerequisiteProcessing},
		{name: "DependentWorkAndPrerequisiteBothReachArchived", run: testDependentWorkAndPrerequisiteBothReachArchived},
		{name: "FanInReleasesOnlyAfterEveryPrerequisite", run: func(t *testing.T, baseURL string) {
			testFanInReleasesOnlyAfterEveryPrerequisite(t, baseURL, fanInGate)
		}},
		{name: "CrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion", run: func(t *testing.T, _ string) {
			testCrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion(t, server, crossBatchGate)
		}},
		{name: "CrossBatchDependsOnFailedTargetCascadesAtAdmission", run: func(t *testing.T, _ string) {
			testCrossBatchDependsOnFailedTargetCascadesAtAdmission(t, server)
		}},
		{name: "DependentWorkDoesNotDispatchAfterPrerequisiteFailure", run: func(t *testing.T, baseURL string) {
			testDependentWorkDoesNotDispatchAfterPrerequisiteFailure(t, baseURL)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t, server.URL())
		})
	}
}

func runSharedRelationshipFactoryToCompletion(
	t *testing.T,
	baseURL string,
	factoryDir string,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	return runSharedRelationshipFactoryToCompletionMode(t, baseURL, factoryDir, timeout, false)
}

func runSharedRelationshipFactoryToCompletionAndClose(
	t *testing.T,
	baseURL string,
	factoryDir string,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	return runSharedRelationshipFactoryToCompletionMode(t, baseURL, factoryDir, timeout, true)
}

func runSharedRelationshipFactoryToCompletionMode(
	t *testing.T,
	baseURL string,
	factoryDir string,
	timeout time.Duration,
	closeAfterObservation bool,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()

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
