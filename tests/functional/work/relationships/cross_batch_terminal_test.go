package relationships

import (
	"errors"
	"fmt"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	crossBatchMixedCompleteName  = "mixed-complete"
	crossBatchMixedCompleteID    = "work-mixed-complete"
	crossBatchMixedFailedName    = "mixed-failed"
	crossBatchMixedFailedID      = "work-mixed-failed"
	crossBatchMixedDependentName = "mixed-dependent"
	crossBatchMixedDependentID   = "work-mixed-dependent"
)

// testCrossBatchDependsOnCompletedTargetReleasesAtAdmission proves that a
// target which already reached its required state releases a later batch
// immediately. It also checks that the admitted relation keeps the target ID.
func testCrossBatchDependsOnCompletedTargetReleasesAtAdmission(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()

	factoryDir := scaffoldCrossBatchFactory(t)
	baseURL := host.URL()
	session, closeSession := openSharedRelationshipSession(t, baseURL, factoryDir)

	executeCrossBatchSubmitForSessionOnServer(t, host.server, session.Id, crossBatchPrerequisiteBatchJSON())
	support.WaitForSessionTerminalStatus(t, baseURL, session.Id, 15*time.Second)

	executeCrossBatchSubmitForSessionOnServer(t, host.server, session.Id, crossBatchDependentBatchByIDJSON())
	support.WaitForSessionTerminalStatus(t, baseURL, session.Id, 15*time.Second)

	listed := listCrossBatchSessionWork(t, baseURL, session.Id)
	assertCrossBatchTerminalState(t, listed, crossBatchPrerequisiteID, "complete")
	assertCrossBatchTerminalState(t, listed, crossBatchDependentID, "complete")
	assertCrossBatchCanonicalDependency(t, listed, crossBatchDependentID, crossBatchPrerequisiteID)

	prerequisiteSequence, dependentSequence := crossBatchDispatchOrdering(
		t,
		support.GetFactoryEventsForSessionAt(t, baseURL, session.Id),
	)
	if dependentSequence <= prerequisiteSequence {
		t.Fatalf("dependent dispatch sequence = %d, want after completed target sequence %d", dependentSequence, prerequisiteSequence)
	}
	closeSession()
	runSharedHostReuseProbe(t, baseURL)
}

// testCrossBatchDependsOnFailedTargetCascadesAtAdmission proves that a later
// batch submitted against a failed target is admitted, cascades to failed, and
// never receives a worker dispatch on the shared host.
func testCrossBatchDependsOnFailedTargetCascadesAtAdmission(
	t *testing.T,
	host *sharedRelationshipHost,
) {
	t.Helper()

	factoryDir := scaffoldCrossBatchFactory(t)
	baseURL := host.URL()
	host.provider.register(t, factoryDir, sharedRelationshipFailureProvider(
		crossBatchFailedPrerequisiteID,
		errors.New("cross-batch target provider failure"),
	))
	session, closeSession := openSharedRelationshipSession(t, baseURL, factoryDir)

	executeCrossBatchSubmitForSessionOnServer(t, host.server, session.Id, crossBatchFailedPrerequisiteBatchJSON())
	support.WaitForSessionTerminalStatus(t, baseURL, session.Id, 15*time.Second)
	assertCrossBatchTerminalState(
		t,
		listCrossBatchSessionWork(t, baseURL, session.Id),
		crossBatchFailedPrerequisiteID,
		"failed",
	)

	executeCrossBatchSubmitForSessionOnServer(t, host.server, session.Id, crossBatchFailedDependentBatchJSON())
	support.WaitForSessionTerminalStatus(t, baseURL, session.Id, 15*time.Second)

	listed := listCrossBatchSessionWork(t, baseURL, session.Id)
	assertCrossBatchTerminalState(t, listed, crossBatchFailedPrerequisiteID, "failed")
	assertCrossBatchTerminalState(t, listed, crossBatchFailedDependentID, "failed")
	assertCrossBatchCanonicalDependency(t, listed, crossBatchFailedDependentID, crossBatchFailedPrerequisiteID)
	assertCrossBatchNoDispatchForWork(
		t,
		support.GetFactoryEventsForSessionAt(t, baseURL, session.Id),
		crossBatchFailedDependentID,
	)

	closeSession()
	runSharedHostReuseProbe(t, baseURL)
}

// testCrossBatchDependsOnMixedTerminalFanInCascades proves that a later batch
// with one complete and one failed target does not dispatch its dependent.
func testCrossBatchDependsOnMixedTerminalFanInCascades(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()

	factoryDir := scaffoldCrossBatchFactory(t)
	baseURL := host.URL()
	host.provider.register(t, factoryDir, sharedRelationshipFailureProvider(
		crossBatchMixedFailedID,
		errors.New("mixed terminal target provider failure"),
	))
	session, closeSession := openSharedRelationshipSession(t, baseURL, factoryDir)

	executeCrossBatchSubmitForSessionOnServer(t, host.server, session.Id, crossBatchMixedTargetsBatchJSON())
	support.WaitForSessionTerminalStatus(t, baseURL, session.Id, 15*time.Second)
	listed := listCrossBatchSessionWork(t, baseURL, session.Id)
	assertCrossBatchTerminalState(t, listed, crossBatchMixedCompleteID, "complete")
	assertCrossBatchTerminalState(t, listed, crossBatchMixedFailedID, "failed")

	executeCrossBatchSubmitForSessionOnServer(t, host.server, session.Id, crossBatchMixedDependentBatchJSON())
	support.WaitForSessionTerminalStatus(t, baseURL, session.Id, 15*time.Second)

	listed = listCrossBatchSessionWork(t, baseURL, session.Id)
	assertCrossBatchTerminalState(t, listed, crossBatchMixedDependentID, "failed")
	assertCrossBatchCanonicalDependency(t, listed, crossBatchMixedDependentID, crossBatchMixedCompleteID)
	assertCrossBatchCanonicalDependency(t, listed, crossBatchMixedDependentID, crossBatchMixedFailedID)
	assertCrossBatchNoDispatchForWork(t, support.GetFactoryEventsForSessionAt(t, baseURL, session.Id), crossBatchMixedDependentID)
	closeSession()
	runSharedHostReuseProbe(t, baseURL)
}

func crossBatchDependentBatchByIDJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-dependent-by-id",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Cross-batch dependent by ID"}
		}],
		"relations": [{
			"type": "DEPENDS_ON",
			"sourceWorkName": %q,
			"targetWorkId": %q
		}]
	}`, crossBatchDependentName, crossBatchDependentID, crossBatchDependentName, crossBatchPrerequisiteID)
}

const (
	crossBatchFailedPrerequisiteName = "failed-admission-prerequisite"
	crossBatchFailedPrerequisiteID   = "work-failed-admission-prerequisite"
	crossBatchFailedDependentName    = "failed-admission-dependent"
	crossBatchFailedDependentID      = "work-failed-admission-dependent"
)

func crossBatchFailedPrerequisiteBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-failed-prerequisite",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Cross-batch failed prerequisite"}
		}]
	}`, crossBatchFailedPrerequisiteName, crossBatchFailedPrerequisiteID)
}

func crossBatchFailedDependentBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-failed-dependent",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Cross-batch failed dependent"}
		}],
		"relations": [{
			"type": "DEPENDS_ON",
			"sourceWorkName": %q,
			"targetWorkName": %q
		}]
	}`, crossBatchFailedDependentName, crossBatchFailedDependentID,
		crossBatchFailedDependentName, crossBatchFailedPrerequisiteName)
}

func crossBatchMixedTargetsBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-mixed-targets",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{
				"name": %q,
				"workId": %q,
				"workTypeName": "task",
				"payload": {"title": "Mixed fan-in complete target"}
			},
			{
				"name": %q,
				"workId": %q,
				"workTypeName": "task",
				"payload": {"title": "Mixed fan-in failed target"}
			}
		]
	}`, crossBatchMixedCompleteName, crossBatchMixedCompleteID, crossBatchMixedFailedName, crossBatchMixedFailedID)
}

func crossBatchMixedDependentBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-mixed-dependent",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Mixed fan-in dependent"}
		}],
		"relations": [
			{
				"type": "DEPENDS_ON",
				"sourceWorkName": %q,
				"targetWorkName": %q
			},
			{
				"type": "DEPENDS_ON",
				"sourceWorkName": %q,
				"targetWorkName": %q
			}
		]
	}`, crossBatchMixedDependentName, crossBatchMixedDependentID,
		crossBatchMixedDependentName, crossBatchMixedCompleteName,
		crossBatchMixedDependentName, crossBatchMixedFailedName)
}

func assertCrossBatchTerminalState(t *testing.T, listed factoryapi.ListWorkResponse, workID, state string) {
	t.Helper()
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", state)) {
		for _, item := range listed.Results {
			if support.StringPointerValue(item.WorkId) == workID && item.State != nil {
				t.Fatalf("Work %q did not reach %q; public state=%q: %#v", workID, state, item.State.Name, listed)
			}
		}
		t.Fatalf("Work %q did not reach %q: %#v", workID, state, listed)
	}
}

func assertCrossBatchCanonicalDependency(t *testing.T, listed factoryapi.ListWorkResponse, sourceID, targetID string) {
	t.Helper()
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != sourceID {
			continue
		}
		if item.Relations == nil {
			t.Fatalf("Work %q has no relations, want dependency on %q", sourceID, targetID)
		}
		for _, relation := range *item.Relations {
			if relation.Type == factoryapi.RelationTypeDependsOn && support.StringPointerValue(relation.TargetWorkId) == targetID {
				return
			}
		}
		t.Fatalf("Work %q relations = %#v, want DEPENDS_ON target %q", sourceID, *item.Relations, targetID)
	}
	t.Fatalf("Work %q is missing from public listing: %#v", sourceID, listed)
}

func assertCrossBatchNoDispatchForWork(t *testing.T, events []factoryapi.FactoryEvent, workID string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch event for Work %q: %v", workID, err)
		}
		if dispatchRequestIncludesWork(payload, workID) {
			t.Fatalf("Work %q received dispatch at sequence %d", workID, event.Context.Sequence)
		}
	}
}
