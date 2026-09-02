package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	submitcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/submit"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	liveCapacityResourceID            = "reviewers"
	liveCapacityResourceName          = "Reviewers"
	liveCapacityWorkType              = "review-task"
	liveCapacityWorker                = "capacity-worker"
	liveCapacityWorkstation           = "review"
	liveCapacityInitialWorkName       = "held-review"
	liveCapacityQueuedWorkName        = "queued-review"
	liveCapacitySecondQueuedName      = "second-queued-review"
	liveCapacityRaiseRequestID        = "capacity-raise-functional"
	liveCapacityBarrierCommand        = "functional-capacity-barrier"
	liveCapacityBarrierOutput         = "capacity barrier completed"
	liveCapacityJavaScriptWorker      = "javascript-capacity-worker"
	liveCapacityJavaScriptWorkstation = "javascript-capacity-review"
	liveCapacityTestTimeout           = 20 * time.Second
	liveCapacityJavaScriptWorkflow    = `return (async function () {
  const results = await parallel([
        { prompt: "javascript capacity child one", label: "javascript-child-one", preset: "javascript-capacity-worker", resourceId: "reviewers" },
        { prompt: "javascript capacity child two", label: "javascript-child-two", preset: "javascript-capacity-worker", resourceId: "reviewers" },
  ]);
  return { results };
})();`
)

// testLiveResourceCapacityIncreaseAdmitsWaitingMockDispatch proves the public
// live-capacity operation changes an already-running mock-worker Factory
// Session. One admitted dispatch stays active at the injected command edge;
// queued Work remains pending at capacity one, then a CLI capacity increase
// wakes another dispatch without replacing the session or interrupting the
// first one.
func testLiveResourceCapacityIncreaseAdmitsWaitingMockDispatch(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	runner := newLiveCapacityBarrierRunner(1)
	dir := scaffoldLiveCapacityFactory(t, 1)
	fixture.useCommandRunnersFor(t, dir, nil, runner)
	session := fixture.openSession(t, dir)
	defer session.closeAndAssertGone(t)

	first := submitLiveCapacityWork(t, fixture, dir, session.id, liveCapacityInitialWorkName)
	if first.WorkId == nil || *first.WorkId == "" {
		t.Fatalf("first submit response = %#v, want work id", first)
	}
	runner.waitForCall(t, 1)

	before := session.current(t)
	if before.Id == "" {
		t.Fatal("default Factory Session has no durable identity")
	}
	submitLiveCapacityWork(t, fixture, dir, session.id, liveCapacityQueuedWorkName)
	submitLiveCapacityWork(t, fixture, dir, session.id, liveCapacitySecondQueuedName)

	capacity := runLiveCapacityCLI(t, fixture, dir, session.id, liveCapacityResourceID, 2, 0, liveCapacityRaiseRequestID, "raise functional throughput")
	if capacity.ResourceId != liveCapacityResourceID || capacity.EffectiveCapacity != 2 ||
		capacity.PreviousCapacity != 1 || capacity.RequestedCapacity != 2 ||
		capacity.InUseCount != 1 || capacity.AvailableCount != 1 ||
		capacity.MinimumCapacity != 1 || capacity.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("APPLIED") ||
		capacity.Revision != 1 || capacity.SessionId != before.Id {
		t.Fatalf("capacity response = %#v, want applied reviewers 1->2 at revision 1", capacity)
	}

	// The second invocation is the observable wake-up edge. It must begin while
	// the first command is still held, proving the live mutation reached the
	// shared admission gate instead of restarting or draining the session.
	runner.waitForCall(t, 2)
	afterRaise := session.current(t)
	if afterRaise.Id != before.Id {
		t.Fatalf("Factory Session id changed from %q to %q after live capacity raise", before.Id, afterRaise.Id)
	}

	close(runner.releaseBlocked)
	support.WaitForSessionTerminalStatus(t, fixture.server.URL(), session.id, liveCapacityTestTimeout)

	events := session.events(t)
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want one admitted dispatch plus two queued dispatches; dispatches=%#v", len(dispatches), dispatches)
	}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("dispatch %q response = %#v, want accepted terminal response", dispatch.DispatchID, dispatch.Response)
		}
	}
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchInterrupted {
			t.Fatalf("live capacity raise interrupted a dispatch: %#v", event)
		}
	}

}

// testLiveResourceCapacityReductionPreservesActiveWork proves a safe live
// reduction updates the durable resource projection while an admitted mock
// dispatch remains in flight. The effective capacity may equal in-use work,
// but the active dispatch is neither interrupted nor restarted.
func testLiveResourceCapacityReductionPreservesActiveWork(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	runner := newLiveCapacityBarrierRunner(1)
	dir := scaffoldLiveCapacityFactory(t, 3)
	fixture.useCommandRunnersFor(t, dir, nil, runner)
	session := fixture.openSession(t, dir)
	defer session.closeAndAssertGone(t)

	submitLiveCapacityWork(t, fixture, dir, session.id, liveCapacityInitialWorkName)
	runner.waitForCall(t, 1)
	before := session.current(t)

	capacity := runLiveCapacityCLI(t, fixture, dir, session.id, liveCapacityResourceID, 1, 0, "capacity-lower-safe", "lower capacity safely")
	if capacity.ResourceId != liveCapacityResourceID || capacity.PreviousCapacity != 3 ||
		capacity.RequestedCapacity != 1 || capacity.EffectiveCapacity != 1 ||
		capacity.InUseCount != 1 || capacity.AvailableCount != 0 || capacity.MinimumCapacity != 1 ||
		capacity.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("APPLIED") ||
		capacity.Revision != 1 || capacity.SessionId != before.Id {
		t.Fatalf("safe reduction response = %#v, want applied reviewers 3->1 at revision 1", capacity)
	}

	close(runner.releaseBlocked)
	support.WaitForSessionTerminalStatus(t, fixture.server.URL(), session.id, liveCapacityTestTimeout)
	after := session.current(t)
	if after.Id != before.Id {
		t.Fatalf("Factory Session id changed from %q to %q after safe live capacity reduction", before.Id, after.Id)
	}
	assertLiveCapacityUsage(t, after, liveCapacityResourceID, 1, 1)
	assertNoLiveCapacityInterruptions(t, session.events(t))
}

// testLiveResourceCapacityRejectsReductionBelowActiveUse proves an unsafe
// reduction is rejected before admission. The rejection emits no live-change
// events, leaves the revision and usage unchanged, and allows the already
// admitted mock dispatches to complete normally.
func testLiveResourceCapacityRejectsReductionBelowActiveUse(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	runner := newLiveCapacityBarrierRunner(2)
	dir := scaffoldLiveCapacityFactory(t, 2)
	fixture.useCommandRunnersFor(t, dir, nil, runner)
	session := fixture.openSession(t, dir)
	defer session.closeAndAssertGone(t)

	submitLiveCapacityWork(t, fixture, dir, session.id, liveCapacityInitialWorkName)
	submitLiveCapacityWork(t, fixture, dir, session.id, liveCapacityQueuedWorkName)
	runner.waitForCall(t, 2)

	beforeEvents := session.events(t)
	before := session.current(t)
	if before.Runtime.Usage.Resources == nil {
		t.Fatal("active session has no resource usage projection")
	}

	errResponse := rejectLiveCapacityCLI(t, fixture, dir, session.id, liveCapacityResourceID, 1, 0, "capacity-lower-rejected", "reject unsafe capacity reduction")
	if errResponse.Code != factoryapi.ErrorResponseCodeRESOURCECAPACITYINUSE || errResponse.ResourceCapacity == nil {
		t.Fatalf("reduction rejection = %#v, want RESOURCE_CAPACITY_IN_USE details", errResponse)
	}
	details := errResponse.ResourceCapacity
	if details.ResourceId != liveCapacityResourceID || details.CurrentCapacity != 2 ||
		details.RequestedCapacity != 1 || details.InUseCount != 2 || details.AvailableCount != 0 ||
		details.MinimumCapacity != 2 {
		t.Fatalf("reduction rejection details = %#v, want current/requested/in-use/available/minimum 2/1/2/0/2", details)
	}

	afterRejectEvents := session.events(t)
	if len(afterRejectEvents) != len(beforeEvents) {
		t.Fatalf("event count changed from %d to %d for pre-admission rejection", len(beforeEvents), len(afterRejectEvents))
	}
	for index := range beforeEvents {
		if beforeEvents[index].Id != afterRejectEvents[index].Id {
			t.Fatalf("event %d changed across pre-admission rejection: before=%q after=%q", index, beforeEvents[index].Id, afterRejectEvents[index].Id)
		}
	}
	after := session.current(t)
	if after.Id != before.Id {
		t.Fatalf("Factory Session id changed from %q to %q after rejected live capacity reduction", before.Id, after.Id)
	}
	assertLiveCapacityUsage(t, after, liveCapacityResourceID, 2, 0)

	close(runner.releaseBlocked)
	support.WaitForSessionTerminalStatus(t, fixture.server.URL(), session.id, liveCapacityTestTimeout)
	events := session.events(t)
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want two admitted dispatches", len(dispatches))
	}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("dispatch %q response = %#v, want accepted terminal response", dispatch.DispatchID, dispatch.Response)
		}
	}
	assertNoLiveCapacityInterruptions(t, events)
}

// testLiveResourceCapacityRecordingReplayAndCursor proves the public
// recording contract for an admitted capacity change: the request and success
// events are ordered and revision-correlated, an identical retry is replayed
// without appending history, different-body reuse is rejected, exact no-op and
// stale pre-admission requests append nothing, and retained history resumes
// after an acknowledged event cursor.
func testLiveResourceCapacityRecordingReplayAndCursor(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	runner := newLiveCapacityBarrierRunner(0)
	dir := scaffoldLiveCapacityFactory(t, 1)
	fixture.useCommandRunnersFor(t, dir, nil, runner)
	session := fixture.openSession(t, dir)
	defer session.closeAndAssertGone(t)

	initialEvents := session.events(t)
	applied := runLiveCapacityCLI(t, fixture, dir, session.id, liveCapacityResourceID, 2, 0, "recorded-capacity-raise", "raise capacity for recording")
	if applied.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("APPLIED") ||
		applied.PreviousCapacity != 1 || applied.RequestedCapacity != 2 || applied.EffectiveCapacity != 2 ||
		applied.Revision != 1 {
		t.Fatalf("applied capacity response = %#v, want APPLIED revision 1", applied)
	}

	events := session.events(t)
	requestIndex, requestEvent := findLiveCapacityEvent(t, events, factoryapi.FactoryEventTypeFactoryChangeRequest, "recorded-capacity-raise")
	changeIndex, changeEvent := findLiveCapacityEvent(t, events, factoryapi.FactoryEventTypeFactoryChange, "recorded-capacity-raise")
	if changeIndex <= requestIndex {
		t.Fatalf("FactoryChange index %d did not follow request index %d", changeIndex, requestIndex)
	}
	if requestEvent.Context.SessionId == nil || changeEvent.Context.SessionId == nil ||
		*requestEvent.Context.SessionId != *changeEvent.Context.SessionId {
		t.Fatalf("live-change session correlation request=%#v success=%#v", requestEvent.Context, changeEvent.Context)
	}
	requestPayload, err := requestEvent.Payload.AsFactoryChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("decode FACTORY_CHANGE_REQUEST payload: %v", err)
	}
	if requestPayload.ExpectedRevision != 0 || requestPayload.ChangeId == "" || requestPayload.TargetId != liveCapacityResourceID {
		t.Fatalf("request payload = %#v, want revision 0 and reviewers target", requestPayload)
	}
	changePayload, err := changeEvent.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode FACTORY_CHANGE payload: %v", err)
	}
	if changePayload.PreviousRevision == nil || *changePayload.PreviousRevision != 0 ||
		changePayload.NewRevision == nil || *changePayload.NewRevision != 1 ||
		changePayload.EffectiveSequence == nil || *changePayload.EffectiveSequence != changeEvent.Context.Sequence ||
		changePayload.ResourceCapacity == nil {
		t.Fatalf("success payload = %#v, want revision 0->1 with detached capacity accounting", changePayload)
	}
	accounting := changePayload.ResourceCapacity
	if accounting.ResourceId != liveCapacityResourceID || accounting.PreviousCapacity != 1 ||
		accounting.RequestedCapacity != 2 || accounting.EffectiveCapacity != 2 || accounting.InUseCount != 0 ||
		accounting.AvailableCount != 2 || accounting.MinimumCapacity != 0 ||
		accounting.Outcome != factoryapi.FactoryResourceCapacityChangeOutcome("APPLIED") {
		t.Fatalf("success capacity accounting = %#v, want detached applied 1->2 accounting", accounting)
	}

	stableEvents := append([]factoryapi.FactoryEvent(nil), events...)
	assertLiveCapacityReplayAndRejections(t, fixture, dir, session.id, stableEvents, applied.ChangeId)

	if len(events) <= len(initialEvents) {
		t.Fatalf("recorded event count = %d, want request and success beyond initial %d", len(events), len(initialEvents))
	}
	cursorSequence := support.ReconnectSequenceForFactoryEvent(requestEvent)
	afterCursor := session.eventsAfter(t, support.FactoryEventReadCursor{
		AfterEventID:  requestEvent.Id,
		AfterSequence: &cursorSequence,
	})
	wantAfterCursor := events[requestIndex+1:]
	if len(afterCursor) != len(wantAfterCursor) {
		t.Fatalf("cursor replay count = %d, want %d after request event", len(afterCursor), len(wantAfterCursor))
	}
	for index := range wantAfterCursor {
		if afterCursor[index].Id != wantAfterCursor[index].Id {
			t.Fatalf("cursor replay event %d = %q, want %q", index, afterCursor[index].Id, wantAfterCursor[index].Id)
		}
	}
}

func assertLiveCapacityReplayAndRejections(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
	factoryDir string,
	sessionID string,
	stableEvents []factoryapi.FactoryEvent,
	changeID string,
) {
	t.Helper()
	replayed := runLiveCapacityCLI(t, fixture, factoryDir, sessionID, liveCapacityResourceID, 2, 0, "recorded-capacity-raise", "raise capacity for recording")
	if replayed.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("REPLAYED") ||
		replayed.ChangeId != changeID || replayed.Revision != 1 || replayed.EffectiveCapacity != 2 {
		t.Fatalf("replayed capacity response = %#v, want REPLAYED original outcome", replayed)
	}
	assertLiveCapacityEventIDsUnchanged(t, stableEvents, fixtureSessionEvents(t, fixture, sessionID), "same-body replay")

	conflict := rejectLiveCapacityCLI(t, fixture, factoryDir, sessionID, liveCapacityResourceID, 3, 0, "recorded-capacity-raise", "reuse request id with different body")
	if conflict.Code != factoryapi.ErrorResponseCodeREQUESTCONFLICT ||
		conflict.Family != factoryapi.ErrorFamilyConflict ||
		!strings.Contains(conflict.Message, "different normalized body") {
		t.Fatalf("different-body conflict = %#v, want typed request conflict", conflict)
	}
	assertLiveCapacityEventIDsUnchanged(t, stableEvents, fixtureSessionEvents(t, fixture, sessionID), "different-body conflict")

	noOp := runLiveCapacityCLI(t, fixture, factoryDir, sessionID, liveCapacityResourceID, 2, 1, "recorded-capacity-noop", "repeat current capacity for recording")
	if noOp.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("NO_OP") ||
		noOp.PreviousCapacity != 2 || noOp.RequestedCapacity != 2 ||
		noOp.EffectiveCapacity != 2 || noOp.Revision != 1 {
		t.Fatalf("exact no-op response = %#v, want typed NO_OP at revision 1", noOp)
	}
	assertLiveCapacityEventIDsUnchanged(t, stableEvents, fixtureSessionEvents(t, fixture, sessionID), "exact no-op")

	stale := rejectLiveCapacityCLI(t, fixture, factoryDir, sessionID, liveCapacityResourceID, 3, 0, "recorded-capacity-stale", "submit stale capacity revision")
	if stale.Code != factoryapi.ErrorResponseCodeREVISIONCONFLICT {
		t.Fatalf("stale revision response = %#v, want REVISION_CONFLICT", stale)
	}
	assertLiveCapacityEventIDsUnchanged(t, stableEvents, fixtureSessionEvents(t, fixture, sessionID), "stale revision")

	notFound := rejectLiveCapacityCLI(t, fixture, factoryDir, sessionID, "missing-resource", 2, 1, "recorded-capacity-not-found", "submit capacity for an unknown resource")
	if notFound.Code != factoryapi.ErrorResponseCodeNOTFOUND || notFound.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("unknown resource response = %#v, want typed NOT_FOUND", notFound)
	}
	assertLiveCapacityEventIDsUnchanged(t, stableEvents, fixtureSessionEvents(t, fixture, sessionID), "unknown resource")
}

func fixtureSessionEvents(t testing.TB, fixture *sharedWorkersMockFixture, sessionID string) []factoryapi.FactoryEvent {
	t.Helper()
	return support.GetFactoryEventsForSessionAt(t, fixture.server.URL(), sessionID)
}

func findLiveCapacityEvent(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	typeName factoryapi.FactoryEventType,
	requestID string,
) (int, factoryapi.FactoryEvent) {
	t.Helper()
	for index, event := range events {
		if event.Type == typeName && support.StringPointerValue(event.Context.RequestId) == requestID {
			return index, event
		}
	}
	t.Fatalf("event history has no %s for request %q", typeName, requestID)
	return -1, factoryapi.FactoryEvent{}
}

func assertLiveCapacityEventIDsUnchanged(
	t *testing.T,
	want, got []factoryapi.FactoryEvent,
	operation string,
) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event count after %s = %d, want unchanged %d", operation, len(got), len(want))
	}
	for index := range want {
		if got[index].Id != want[index].Id {
			t.Fatalf("event %d after %s = %q, want unchanged %q", index, operation, got[index].Id, want[index].Id)
		}
	}
}

func assertLiveCapacityUsage(t *testing.T, session factoryapi.FactorySession, name string, total, available int) {
	t.Helper()
	for _, usage := range session.Runtime.Usage.Resources {
		if usage.Name == name {
			if usage.Total != total || usage.Available != available {
				t.Fatalf("resource %q usage = %#v, want total=%d available=%d", name, usage, total, available)
			}
			return
		}
	}
	t.Fatalf("session resource usage missing %q: %#v", name, session.Runtime.Usage.Resources)
}

func assertNoLiveCapacityInterruptions(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchInterrupted {
			t.Fatalf("live capacity change interrupted a dispatch: %#v", event)
		}
	}
}

func submitLiveCapacityWork(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
	factoryDir, sessionID, name string,
) factoryapi.SubmitWorkResponse {
	t.Helper()

	payloadPath := filepath.Join(t.TempDir(), "live-capacity-work.json")
	payload, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		t.Fatalf("marshal live capacity Work payload: %v", err)
	}
	if err := os.WriteFile(payloadPath, payload, 0o600); err != nil {
		t.Fatalf("write live capacity Work payload: %v", err)
	}

	inputs, err := fixture.executeCLI(t, factoryDir,
		"--json",
		"--server", fixture.server.URL(),
		"submit",
		"--session", sessionID,
		"--name", name,
		"--work-type-name", liveCapacityWorkType,
		"--payload", payloadPath,
	)
	if err != nil {
		t.Fatalf("you submit live capacity Work: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var confirmation submitcli.SubmitSuccessResult
	if err := json.Unmarshal(bytes.TrimSpace([]byte(inputs.Stdout())), &confirmation); err != nil {
		t.Fatalf("decode you submit live capacity Work JSON: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	return factoryapi.SubmitWorkResponse{
		Accepted:     true,
		Name:         stringPointer(confirmation.Name),
		SessionId:    stringPointer(confirmation.SessionID),
		TraceId:      confirmation.TraceID,
		WorkId:       confirmation.WorkID,
		WorkTypeName: stringPointer(confirmation.WorkTypeName),
	}
}

func runLiveCapacityCLI(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
	factoryDir, sessionID, resourceID string,
	capacity, expectedRevision int,
	requestID, reason string,
) factoryapi.FactorySessionResourceCapacityResponse {
	t.Helper()
	inputs, err := fixture.executeCLI(t, factoryDir,
		"--json",
		"--server", fixture.server.URL(),
		"session", "resource", "set",
		resourceID, fmt.Sprintf("%d", capacity), sessionID,
		"--request-id", requestID,
		"--expected-revision", fmt.Sprintf("%d", expectedRevision),
		"--reason", reason,
	)
	if err != nil {
		t.Fatalf("you session resource set: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var response factoryapi.FactorySessionResourceCapacityResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(inputs.Stdout())), &response); err != nil {
		t.Fatalf("decode you session resource set JSON: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	return response
}

func rejectLiveCapacityCLI(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
	factoryDir, sessionID, resourceID string,
	capacity, expectedRevision int,
	requestID, reason string,
) factoryapi.ErrorResponse {
	t.Helper()
	inputs, err := fixture.executeCLI(t, factoryDir,
		"--json",
		"--server", fixture.server.URL(),
		"session", "resource", "set",
		resourceID, fmt.Sprintf("%d", capacity), sessionID,
		"--request-id", requestID,
		"--expected-revision", fmt.Sprintf("%d", expectedRevision),
		"--reason", reason,
	)
	if err == nil {
		t.Fatalf("you session resource set %s unexpectedly succeeded\nstdout:\n%s", requestID, inputs.Stdout())
	}
	var rejected *sessioncli.ResourceCapacityRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("you session resource set %s error = %T (%v), want typed capacity rejection\nstderr:\n%s", requestID, err, err, inputs.Stderr())
	}
	return rejected.Response
}

func scaffoldLiveCapacityFactory(t *testing.T, capacity int) string {
	t.Helper()
	return scaffoldLiveCapacityFactoryWithWorker(
		t,
		capacity,
		liveCapacityWorker,
		liveCapacityWorkstation,
		"---\n"+
			"type: SCRIPT_WORKER\n"+
			"command: authored-capacity-command\n"+
			"---\n"+
			"Run the capacity test work.\n",
	)
}

func scaffoldLiveCapacityJavaScriptFactory(t *testing.T) string {
	t.Helper()
	return scaffoldLiveCapacityFactoryWithWorker(
		t,
		1,
		liveCapacityJavaScriptWorker,
		liveCapacityJavaScriptWorkstation,
		"---\n"+
			"type: MODEL_WORKER\n"+
			"---\n"+
			"Use the capacity worker for JavaScript children.\n",
	)
}

func scaffoldLiveCapacityFactoryWithWorker(
	t *testing.T,
	capacity int,
	workerName, workstationName, agentConfig string,
) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "live-capacity-functional",
		"resources": []map[string]any{{
			"id":       liveCapacityResourceID,
			"name":     liveCapacityResourceName,
			"capacity": capacity,
		}},
		"workTypes": []map[string]any{{
			"name": liveCapacityWorkType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": workerName,
		}},
		"workstations": []map[string]any{{
			"name":      workstationName,
			"type":      "MODEL_WORKSTATION",
			"worker":    workerName,
			"inputs":    []map[string]string{{"workType": liveCapacityWorkType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": liveCapacityWorkType, "state": "complete"}},
			"onFailure": []map[string]string{{"workType": liveCapacityWorkType, "state": "failed"}},
			"resources": []map[string]any{{"name": liveCapacityResourceName, "capacity": 1}},
		}},
	})
	support.WriteAgentConfig(t, dir, workerName, agentConfig)
	return dir
}

type liveCapacityBarrierRunner struct {
	mu             sync.Mutex
	calls          int
	blockedCalls   int
	started        chan int
	releaseBlocked chan struct{}
}

func newLiveCapacityBarrierRunner(blockedCalls int) *liveCapacityBarrierRunner {
	return &liveCapacityBarrierRunner{
		blockedCalls:   blockedCalls,
		started:        make(chan int, 16),
		releaseBlocked: make(chan struct{}),
	}
}

func (r *liveCapacityBarrierRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	r.started <- call
	if call <= r.blockedCalls {
		select {
		case <-r.releaseBlocked:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return platformprocess.CommandResult{Stdout: []byte(liveCapacityBarrierOutput)}, nil
}

func (r *liveCapacityBarrierRunner) waitForCall(t *testing.T, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), liveCapacityTestTimeout)
	defer cancel()
	for {
		select {
		case call := <-r.started:
			if call >= want {
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for command barrier call %d", want)
		}
	}
}

func stringPointer(value string) *string { return &value }

var _ platformprocess.CommandRunner = (*liveCapacityBarrierRunner)(nil)
