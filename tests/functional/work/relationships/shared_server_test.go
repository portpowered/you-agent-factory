package relationships

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSharedServerRelationships keeps one customer-hosted application process
// alive while independent relationship scenarios execute in isolated Factory
// Sessions. Scenarios requiring invocation-specific provider behavior remain
// top-level tests with their own process-scoped edges.
func TestSharedServerRelationships(t *testing.T) {
	t.Parallel()

	hostFactoryDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_simple_dir"))
	fanInGate := support.NewMockWorkerGate(t)
	crossBatchGate := support.NewMockWorkerGate(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: hostFactoryDir,
		MockWorkersConfig: &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{
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
	t.Helper()

	opened := support.OpenFactorySessionAt(t, baseURL, factoryDir)
	sessionID := opened.Session.Id
	t.Cleanup(func() {
		support.CloseFactorySessionAt(t, baseURL, sessionID)
	})
	support.WaitForSessionTerminalStatus(t, baseURL, sessionID, timeout)

	sessionResponse := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := sessionResponse.AsFactorySession()
	if err != nil {
		t.Fatalf("decode shared-host Factory Session %q: %v", sessionID, err)
	}
	listed := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	return session, listed, events
}
