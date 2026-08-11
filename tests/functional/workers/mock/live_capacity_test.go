package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const (
	liveCapacityResourceID         = "reviewers"
	liveCapacityResourceName       = "Reviewers"
	liveCapacityWorkType           = "review-task"
	liveCapacityWorker             = "capacity-worker"
	liveCapacityWorkstation        = "review"
	liveCapacityInitialWorkName    = "held-review"
	liveCapacityQueuedWorkName     = "queued-review"
	liveCapacitySecondQueuedName   = "second-queued-review"
	liveCapacityRaiseRequestID     = "capacity-raise-functional"
	liveCapacityBarrierCommand     = "functional-capacity-barrier"
	liveCapacityBarrierOutput      = "capacity barrier completed"
	liveCapacityTestTimeout        = 20 * time.Second
	liveCapacityJavaScriptWorkflow = `return (async function () {
  const results = await parallel([
        { prompt: "javascript capacity child one", label: "javascript-child-one", preset: "capacity-worker", resourceId: "reviewers" },
        { prompt: "javascript capacity child two", label: "javascript-child-two", preset: "capacity-worker", resourceId: "reviewers" },
  ]);
  return { results };
})();`
)

// TestLiveResourceCapacityIncreaseAdmitsWaitingMockDispatch proves the public
// live-capacity operation changes an already-running mock-worker Factory
// Session. One admitted dispatch stays active at the injected command edge;
// queued Work remains pending at capacity one, then a CLI capacity increase
// wakes another dispatch without replacing the session or interrupting the
// first one.
func TestLiveResourceCapacityIncreaseAdmitsWaitingMockDispatch(t *testing.T) {
	runner := newLiveCapacityBarrierRunner(1)
	dir := scaffoldLiveCapacityFactory(t, 1)
	server := startLiveCapacityServer(t, dir, runner)

	first := submitLiveCapacityWork(t, server.URL(), liveCapacityInitialWorkName)
	if first.WorkId == nil || *first.WorkId == "" {
		t.Fatalf("first submit response = %#v, want work id", first)
	}
	runner.waitForCall(t, 1)

	before := support.GetDefaultSession(t, server.URL())
	if before.Id == "" {
		t.Fatal("default Factory Session has no durable identity")
	}
	submitLiveCapacityWork(t, server.URL(), liveCapacityQueuedWorkName)
	submitLiveCapacityWork(t, server.URL(), liveCapacitySecondQueuedName)

	capacity := runLiveCapacityCLI(t, dir, server.URL(), liveCapacityResourceID, 2, 0, liveCapacityRaiseRequestID)
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
	afterRaise := support.GetDefaultSession(t, server.URL())
	if afterRaise.Id != before.Id {
		t.Fatalf("Factory Session id changed from %q to %q after live capacity raise", before.Id, afterRaise.Id)
	}

	close(runner.releaseBlocked)
	support.WaitForTerminalStatus(t, server.URL(), liveCapacityTestTimeout)

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	if len(dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want one admitted dispatch plus two queued dispatches; dispatches=%#v", len(dispatches), dispatches)
	}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("dispatch %q response = %#v, want accepted terminal response", dispatch.DispatchID, dispatch.Response)
		}
	}
	for _, event := range server.GetFactoryEvents(t) {
		if event.Type == factoryapi.FactoryEventTypeDispatchInterrupted {
			t.Fatalf("live capacity raise interrupted a dispatch: %#v", event)
		}
	}

	functionalevidence.Covers(t, "cli/you.session.resource.set")
}

// TestLiveResourceCapacityReductionPreservesActiveWork proves a safe live
// reduction updates the durable resource projection while an admitted mock
// dispatch remains in flight. The effective capacity may equal in-use work,
// but the active dispatch is neither interrupted nor restarted.
func TestLiveResourceCapacityReductionPreservesActiveWork(t *testing.T) {
	runner := newLiveCapacityBarrierRunner(1)
	dir := scaffoldLiveCapacityFactory(t, 3)
	server := startLiveCapacityServer(t, dir, runner)

	submitLiveCapacityWork(t, server.URL(), liveCapacityInitialWorkName)
	runner.waitForCall(t, 1)
	before := support.GetDefaultSession(t, server.URL())

	capacity := setLiveCapacityREST(t, server.URL(), "~default", liveCapacityResourceID, 1, 0, "capacity-lower-safe")
	if capacity.ResourceId != liveCapacityResourceID || capacity.PreviousCapacity != 3 ||
		capacity.RequestedCapacity != 1 || capacity.EffectiveCapacity != 1 ||
		capacity.InUseCount != 1 || capacity.AvailableCount != 0 || capacity.MinimumCapacity != 1 ||
		capacity.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("APPLIED") ||
		capacity.Revision != 1 || capacity.SessionId != before.Id {
		t.Fatalf("safe reduction response = %#v, want applied reviewers 3->1 at revision 1", capacity)
	}

	close(runner.releaseBlocked)
	support.WaitForTerminalStatus(t, server.URL(), liveCapacityTestTimeout)
	after := support.GetDefaultSession(t, server.URL())
	if after.Id != before.Id {
		t.Fatalf("Factory Session id changed from %q to %q after safe live capacity reduction", before.Id, after.Id)
	}
	assertLiveCapacityUsage(t, after, liveCapacityResourceID, 1, 1)
	assertNoLiveCapacityInterruptions(t, server.GetFactoryEvents(t))
}

// TestLiveResourceCapacityRejectsReductionBelowActiveUse proves an unsafe
// reduction is rejected before admission. The rejection emits no live-change
// events, leaves the revision and usage unchanged, and allows the already
// admitted mock dispatches to complete normally.
func TestLiveResourceCapacityRejectsReductionBelowActiveUse(t *testing.T) {
	runner := newLiveCapacityBarrierRunner(2)
	dir := scaffoldLiveCapacityFactory(t, 2)
	server := startLiveCapacityServer(t, dir, runner)

	submitLiveCapacityWork(t, server.URL(), liveCapacityInitialWorkName)
	submitLiveCapacityWork(t, server.URL(), liveCapacityQueuedWorkName)
	runner.waitForCall(t, 2)

	beforeEvents := server.GetFactoryEvents(t)
	before := support.GetDefaultSession(t, server.URL())
	if before.Runtime.Usage.Resources == nil {
		t.Fatal("active session has no resource usage projection")
	}

	errResponse := rejectLiveCapacityREST(t, server.URL(), "~default", liveCapacityResourceID, 1, 0, "capacity-lower-rejected")
	if errResponse.Code != factoryapi.ErrorResponseCodeRESOURCECAPACITYINUSE || errResponse.ResourceCapacity == nil {
		t.Fatalf("reduction rejection = %#v, want RESOURCE_CAPACITY_IN_USE details", errResponse)
	}
	details := errResponse.ResourceCapacity
	if details.ResourceId != liveCapacityResourceID || details.CurrentCapacity != 2 ||
		details.RequestedCapacity != 1 || details.InUseCount != 2 || details.AvailableCount != 0 ||
		details.MinimumCapacity != 2 {
		t.Fatalf("reduction rejection details = %#v, want current/requested/in-use/available/minimum 2/1/2/0/2", details)
	}

	afterRejectEvents := server.GetFactoryEvents(t)
	if len(afterRejectEvents) != len(beforeEvents) {
		t.Fatalf("event count changed from %d to %d for pre-admission rejection", len(beforeEvents), len(afterRejectEvents))
	}
	for index := range beforeEvents {
		if beforeEvents[index].Id != afterRejectEvents[index].Id {
			t.Fatalf("event %d changed across pre-admission rejection: before=%q after=%q", index, beforeEvents[index].Id, afterRejectEvents[index].Id)
		}
	}
	after := support.GetDefaultSession(t, server.URL())
	if after.Id != before.Id {
		t.Fatalf("Factory Session id changed from %q to %q after rejected live capacity reduction", before.Id, after.Id)
	}
	assertLiveCapacityUsage(t, after, liveCapacityResourceID, 2, 0)

	close(runner.releaseBlocked)
	support.WaitForTerminalStatus(t, server.URL(), liveCapacityTestTimeout)
	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want two admitted dispatches", len(dispatches))
	}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("dispatch %q response = %#v, want accepted terminal response", dispatch.DispatchID, dispatch.Response)
		}
	}
	assertNoLiveCapacityInterruptions(t, server.GetFactoryEvents(t))
}

// TestLiveResourceCapacityRecordingReplayAndCursor proves the public
// recording contract for an admitted capacity change: the request and success
// events are ordered and revision-correlated, an identical retry is replayed
// without appending history, different-body reuse is rejected, exact no-op and
// stale pre-admission requests append nothing, and retained history resumes
// after an acknowledged event cursor.
func TestLiveResourceCapacityRecordingReplayAndCursor(t *testing.T) {
	runner := newLiveCapacityBarrierRunner(0)
	dir := scaffoldLiveCapacityFactory(t, 1)
	server := startLiveCapacityServer(t, dir, runner)

	initialEvents := server.GetFactoryEvents(t)
	applied := setLiveCapacityREST(t, server.URL(), "~default", liveCapacityResourceID, 2, 0, "recorded-capacity-raise")
	if applied.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("APPLIED") ||
		applied.PreviousCapacity != 1 || applied.RequestedCapacity != 2 || applied.EffectiveCapacity != 2 ||
		applied.Revision != 1 {
		t.Fatalf("applied capacity response = %#v, want APPLIED revision 1", applied)
	}

	events := server.GetFactoryEvents(t)
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
	assertLiveCapacityReplayAndRejections(t, server, stableEvents, applied.ChangeId)

	if len(events) <= len(initialEvents) {
		t.Fatalf("recorded event count = %d, want request and success beyond initial %d", len(events), len(initialEvents))
	}
	cursorSequence := support.ReconnectSequenceForFactoryEvent(requestEvent)
	afterCursor := server.GetFactoryEventsAfter(t, support.FactoryEventReadCursor{
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
	server *support.FunctionalAPIServer,
	stableEvents []factoryapi.FactoryEvent,
	changeID string,
) {
	t.Helper()
	replayed := setLiveCapacityREST(t, server.URL(), "~default", liveCapacityResourceID, 2, 0, "recorded-capacity-raise")
	if replayed.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("REPLAYED") ||
		replayed.ChangeId != changeID || replayed.Revision != 1 || replayed.EffectiveCapacity != 2 {
		t.Fatalf("replayed capacity response = %#v, want REPLAYED original outcome", replayed)
	}
	assertLiveCapacityEventIDsUnchanged(t, stableEvents, server.GetFactoryEvents(t), "same-body replay")

	conflictStatus, conflictBody := postLiveCapacityREST(t, server.URL(), "~default", liveCapacityResourceID, 3, 0, "recorded-capacity-raise")
	if conflictStatus != http.StatusConflict {
		t.Fatalf("different-body request status = %d, want 409\n%s", conflictStatus, conflictBody)
	}
	var conflict factoryapi.ErrorResponse
	if err := json.Unmarshal(conflictBody, &conflict); err != nil {
		t.Fatalf("decode different-body conflict: %v\n%s", err, conflictBody)
	}
	if conflict.Code != factoryapi.ErrorResponseCodeBADREQUEST || !strings.Contains(conflict.Message, "different normalized body") {
		t.Fatalf("different-body conflict = %#v, want typed bad-request conflict", conflict)
	}
	assertLiveCapacityEventIDsUnchanged(t, stableEvents, server.GetFactoryEvents(t), "different-body conflict")

	noOpStatus, noOpBody := postLiveCapacityREST(t, server.URL(), "~default", liveCapacityResourceID, 2, 1, "recorded-capacity-noop")
	if noOpStatus != http.StatusConflict {
		t.Fatalf("exact no-op status = %d, want pre-admission conflict\n%s", noOpStatus, noOpBody)
	}
	var noOp factoryapi.ErrorResponse
	if err := json.Unmarshal(noOpBody, &noOp); err != nil {
		t.Fatalf("decode exact no-op response: %v\n%s", err, noOpBody)
	}
	if noOp.Code != factoryapi.ErrorResponseCodeBADREQUEST || !strings.Contains(noOp.Message, "would not alter") {
		t.Fatalf("exact no-op response = %#v, want typed pre-admission no-op", noOp)
	}
	assertLiveCapacityEventIDsUnchanged(t, stableEvents, server.GetFactoryEvents(t), "exact no-op")

	staleStatus, staleBody := postLiveCapacityREST(t, server.URL(), "~default", liveCapacityResourceID, 3, 0, "recorded-capacity-stale")
	if staleStatus != http.StatusConflict {
		t.Fatalf("stale request status = %d, want 409\n%s", staleStatus, staleBody)
	}
	var stale factoryapi.ErrorResponse
	if err := json.Unmarshal(staleBody, &stale); err != nil {
		t.Fatalf("decode stale revision response: %v\n%s", err, staleBody)
	}
	if stale.Code != factoryapi.ErrorResponseCodeREVISIONCONFLICT {
		t.Fatalf("stale revision response = %#v, want REVISION_CONFLICT", stale)
	}
	assertLiveCapacityEventIDsUnchanged(t, stableEvents, server.GetFactoryEvents(t), "stale revision")
}

// TestJavaScriptLiveResourceCapacityIncreaseWakesWaitingChildren proves the
// shared resource gate at the public JavaScript Factory Session boundary. One
// child is held at the injected mock-worker command edge, the second child
// waits on reviewers capacity one, and a live increase admits it in the same
// durable session with exactly two completed child dispatches.
func TestJavaScriptLiveResourceCapacityIncreaseWakesWaitingChildren(t *testing.T) {
	provider := newLiveCapacityJavaScriptProvider()
	dir := scaffoldLiveCapacityFactory(t, 1)
	support.WriteAgentConfig(t, dir, liveCapacityWorker, "---\n"+
		"type: MODEL_WORKER\n"+
		"---\n"+
		"Use the capacity worker for JavaScript children.\n")
	homeDir := writeLiveCapacityJavaScriptGlobalConfig(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env: append(os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Edges: serviceedges.Edges{ProviderOverride: provider},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startLiveCapacityJavaScriptWorkflow(t, server.URL(), liveCapacityJavaScriptWorkflow)
	if started.SessionId == "" {
		t.Fatal("JavaScript capacity workflow has no durable session ID")
	}
	provider.waitForCall(t, 1)
	before := readLiveCapacityDurableSession(t, server.URL(), started.SessionId)

	capacity := setLiveCapacityREST(t, server.URL(), started.SessionId, liveCapacityResourceID, 2, 0, "javascript-capacity-raise")
	if capacity.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("APPLIED") ||
		capacity.SessionId != started.SessionId || capacity.PreviousCapacity != 1 ||
		capacity.EffectiveCapacity != 2 || capacity.InUseCount != 1 || capacity.AvailableCount != 1 ||
		capacity.Revision != 1 {
		t.Fatalf("JavaScript capacity response = %#v, want applied reviewers 1->2 in same session", capacity)
	}
	provider.waitForCall(t, 2)
	afterRaise := readLiveCapacityDurableSession(t, server.URL(), started.SessionId)
	if afterRaise.SessionId != before.SessionId {
		t.Fatalf("JavaScript Factory Session id changed from %q to %q after live raise", before.SessionId, afterRaise.SessionId)
	}

	close(provider.releaseBlocked)
	waitForLiveCapacityDurableSessionTerminal(t, server.URL(), started.SessionId, liveCapacityTestTimeout)
	if provider.callCount() != 2 || provider.peakActive() != 2 {
		t.Fatalf("JavaScript provider calls=%d peakActive=%d, want exactly two calls with two concurrent effects", provider.callCount(), provider.peakActive())
	}
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf("JavaScript dispatch count = %d, want two resource-bound children", len(dispatches.Dispatches))
	}
	seenIDs := make(map[string]struct{}, len(dispatches.Dispatches))
	seenLabels := make(map[string]struct{}, len(dispatches.Dispatches))
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED || dispatch.Javascript == nil {
			t.Fatalf("JavaScript dispatch = %#v, want completed JavaScript projection", dispatch)
		}
		if _, duplicate := seenIDs[dispatch.Id]; duplicate || dispatch.Id == "" {
			t.Fatalf("JavaScript dispatch IDs contain duplicate or empty ID: %q", dispatch.Id)
		}
		seenIDs[dispatch.Id] = struct{}{}
		if dispatch.Label == nil || *dispatch.Label == "" {
			t.Fatalf("JavaScript dispatch %q has no public child label", dispatch.Id)
		}
		seenLabels[*dispatch.Label] = struct{}{}
	}
	for _, label := range []string{"javascript-child-one", "javascript-child-two"} {
		if _, ok := seenLabels[label]; !ok {
			t.Fatalf("JavaScript dispatch labels = %#v, missing %q", seenLabels, label)
		}
	}
}

func startLiveCapacityJavaScriptWorkflow(
	t *testing.T,
	serverURL, workflowSource string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	dialect := "you-workflow-v1"
	inlineSource := factoryapi.FactoryOrchestratorJavaScriptInlineSource{
		Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
		Inline:   workflowSource,
	}
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-live-capacity-workflow",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect:      &dialect,
				InlineSource: inlineSource,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal JavaScript capacity workflow: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/async"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build JavaScript capacity workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start JavaScript capacity workflow: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read JavaScript capacity workflow response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("start JavaScript capacity workflow status = %d, want 200\n%s", response.StatusCode, body)
	}
	var started factoryapi.FactorySessionExecutionResponse
	if err := json.Unmarshal(body, &started); err != nil {
		t.Fatalf("decode JavaScript capacity workflow response: %v\n%s", err, body)
	}
	return started
}

func readLiveCapacityDurableSession(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode JavaScript durable Factory Session: %v", err)
	}
	return session
}

func waitForLiveCapacityDurableSessionTerminal(
	t *testing.T,
	serverURL, sessionID string,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		session := readLiveCapacityDurableSession(t, serverURL, sessionID)
		switch session.Status {
		case factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
			factoryapi.FactorySessionDurableLifecycleStatusFailed,
			factoryapi.FactorySessionDurableLifecycleStatusCanceled,
			factoryapi.FactorySessionDurableLifecycleStatusTimedOut,
			factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
			factoryapi.FactorySessionDurableLifecycleStatusTerminated:
			return session
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for durable Factory Session %q to become terminal: last status %q", sessionID, session.Status)
		}
	}
}

func writeLiveCapacityJavaScriptGlobalConfig(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create JavaScript capacity global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {"workerModelProvider": "codex", "workerModel": "mock-capacity-model"},
  "workerPresets": [{"id": "capacity-worker", "modelProvider": "codex", "model": "mock-capacity-model"}]
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write JavaScript capacity global config: %v", err)
	}
	return homeDir
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

func startLiveCapacityServer(t *testing.T, dir string, runner *liveCapacityBarrierRunner) *support.FunctionalAPIServer {
	t.Helper()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		MockWorkersConfig: &workers.MockWorkersConfig{
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      liveCapacityWorker,
				WorkstationName: liveCapacityWorkstation,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: liveCapacityBarrierCommand,
				},
			}},
		},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: support.NewStaticSuccessCommandRunner(liveCapacityBarrierOutput),
			ScriptCommandRunner:   runner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	return server
}

func setLiveCapacityREST(
	t *testing.T,
	serverURL, sessionID, resourceID string,
	capacity, expectedRevision int,
	requestID string,
) factoryapi.FactorySessionResourceCapacityResponse {
	t.Helper()
	status, body := postLiveCapacityREST(t, serverURL, sessionID, resourceID, capacity, expectedRevision, requestID)
	if status != http.StatusOK {
		t.Fatalf("set resource capacity status = %d, want 200\n%s", status, body)
	}
	var response factoryapi.FactorySessionResourceCapacityResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode set resource capacity response: %v\n%s", err, body)
	}
	return response
}

func rejectLiveCapacityREST(
	t *testing.T,
	serverURL, sessionID, resourceID string,
	capacity, expectedRevision int,
	requestID string,
) factoryapi.ErrorResponse {
	t.Helper()
	status, body := postLiveCapacityREST(t, serverURL, sessionID, resourceID, capacity, expectedRevision, requestID)
	if status != http.StatusConflict {
		t.Fatalf("rejected resource capacity status = %d, want 409\n%s", status, body)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode rejected resource capacity response: %v\n%s", err, body)
	}
	return response
}

func postLiveCapacityREST(
	t *testing.T,
	serverURL, sessionID, resourceID string,
	capacity, expectedRevision int,
	requestID string,
) (int, []byte) {
	t.Helper()
	reason := "functional live resource capacity test"
	payload, err := json.Marshal(factoryapi.FactorySessionResourceCapacityRequest{
		Capacity:         capacity,
		ExpectedRevision: expectedRevision,
		Reason:           &reason,
		RequestId:        requestID,
	})
	if err != nil {
		t.Fatalf("marshal resource capacity request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + sessionID + "/resources/" + resourceID + "/capacity"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build resource capacity request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST resource capacity: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read resource capacity response: %v", err)
	}
	return response.StatusCode, body
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

func submitLiveCapacityWork(t *testing.T, serverURL, name string) factoryapi.SubmitWorkResponse {
	t.Helper()
	return support.SubmitDefaultSessionWork(t, serverURL, factoryapi.SubmitWorkRequest{
		Name:         stringPointer(name),
		WorkTypeName: liveCapacityWorkType,
		Payload:      map[string]any{"name": name},
	})
}

func runLiveCapacityCLI(
	t *testing.T,
	factoryDir, serverURL, resourceID string,
	capacity, expectedRevision int,
	requestID string,
) factoryapi.FactorySessionResourceCapacityResponse {
	t.Helper()
	process := support.BuildProcess(t, serviceedges.Edges{})
	if closer, ok := process.(interface{ Close(context.Context) error }); ok {
		t.Cleanup(func() {
			if err := closer.Close(context.Background()); err != nil {
				t.Errorf("close capacity CLI process: %v", err)
			}
		})
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--json",
		"--server", serverURL,
		"session", "resource", "set",
		resourceID, fmt.Sprintf("%d", capacity), "~default",
		"--request-id", requestID,
		"--expected-revision", fmt.Sprintf("%d", expectedRevision),
		"--reason", "raise functional throughput",
	})
	inputs.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("you session resource set: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var response factoryapi.FactorySessionResourceCapacityResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(inputs.Stdout())), &response); err != nil {
		t.Fatalf("decode you session resource set JSON: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	return response
}

func scaffoldLiveCapacityFactory(t *testing.T, capacity int) string {
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
			"name": liveCapacityWorker,
		}},
		"workstations": []map[string]any{{
			"name":      liveCapacityWorkstation,
			"type":      "MODEL_WORKSTATION",
			"worker":    liveCapacityWorker,
			"inputs":    []map[string]string{{"workType": liveCapacityWorkType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": liveCapacityWorkType, "state": "complete"}},
			"onFailure": []map[string]string{{"workType": liveCapacityWorkType, "state": "failed"}},
			"resources": []map[string]any{{"name": liveCapacityResourceName, "capacity": 1}},
		}},
	})
	support.WriteAgentConfig(t, dir, liveCapacityWorker, "---\n"+
		"type: SCRIPT_WORKER\n"+
		"command: authored-capacity-command\n"+
		"---\n"+
		"Run the capacity test work.\n")
	return dir
}

type liveCapacityJavaScriptProvider struct {
	mu             sync.Mutex
	calls          int
	active         int
	peak           int
	started        chan int
	releaseBlocked chan struct{}
}

func newLiveCapacityJavaScriptProvider() *liveCapacityJavaScriptProvider {
	return &liveCapacityJavaScriptProvider{
		started:        make(chan int, 8),
		releaseBlocked: make(chan struct{}),
	}
}

func (p *liveCapacityJavaScriptProvider) Infer(ctx context.Context, request workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.active++
	if p.active > p.peak {
		p.peak = p.active
	}
	p.mu.Unlock()
	p.started <- call
	defer func() {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}()
	if call <= 2 {
		select {
		case <-p.releaseBlocked:
		case <-ctx.Done():
			return workers.InferenceResponse{}, ctx.Err()
		}
	}
	label := "javascript-child-one"
	if strings.Contains(request.UserMessage, "two") {
		label = "javascript-child-two"
	}
	return workers.InferenceResponse{
		Content: fmt.Sprintf(`{"text":"live capacity complete","label":%q}`, label),
	}, nil
}

func (p *liveCapacityJavaScriptProvider) waitForCall(t *testing.T, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), liveCapacityTestTimeout)
	defer cancel()
	for {
		select {
		case call := <-p.started:
			if call >= want {
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for JavaScript provider call %d", want)
		}
	}
}

func (p *liveCapacityJavaScriptProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *liveCapacityJavaScriptProvider) peakActive() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak
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

func (r *liveCapacityBarrierRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
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
