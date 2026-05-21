package runtime_api

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	factoryboundary "github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type thinEventSmokeHarness struct {
	harness    *testutil.ServiceTestHarness
	provider   *blockingFunctionalInferenceProvider
	recordPath string
}

type thinEventSmokeActiveSnapshot struct {
	events          []factoryapi.FactoryEvent
	requestEvent    factoryapi.FactoryEvent
	requestPayload  factoryapi.InferenceRequestEventPayload
	dispatchID      string
	dispatchReqIdx  int
	requestEventIdx int
}

type thinEventSmokeFinalSnapshot struct {
	liveEvents            []factoryapi.FactoryEvent
	artifact              *interfaces.ReplayArtifact
	responsePayload       factoryapi.InferenceResponseEventPayload
	finalResponseEventIdx int
	finalState            interfaces.FactoryWorldState
}

func newThinEventSmokeHarness(t *testing.T) thinEventSmokeHarness {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	recordPath := filepath.Join(t.TempDir(), "thin-event-reducer-views.replay.json")
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-thin-event-reducers",
		WorkTypeID: "task",
		TraceID:    "trace-thin-event-reducers",
		Payload:    []byte(`{"title":"reconstruct thin reducer views"}`),
	})
	provider := newBlockingFunctionalInferenceProvider(
		thinReducerInferenceResponse("sess-thin-dispatch-1", "Step one done. COMPLETE"),
		thinReducerInferenceResponse("sess-thin-dispatch-2", "Step two done. COMPLETE"),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithRecordPath(recordPath),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	return thinEventSmokeHarness{harness: h, provider: provider, recordPath: recordPath}
}

func captureThinEventSmokeActiveSnapshot(
	t *testing.T,
	smoke thinEventSmokeHarness,
) thinEventSmokeActiveSnapshot {
	t.Helper()

	smoke.provider.WaitForFirstCall(t, 5*time.Second)
	activeEvents := waitForFunctionalInferenceRequestSnapshot(t, smoke.harness, 5*time.Second)
	requestEventIdx := indexOfFunctionalEventType(activeEvents, factoryapi.FactoryEventTypeInferenceRequest, 0)
	if requestEventIdx < 0 {
		t.Fatalf("active events missing inference request: %v", functionalEventTypes(activeEvents))
	}
	if indexOfFunctionalEventType(activeEvents, factoryapi.FactoryEventTypeInferenceResponse, 0) >= 0 {
		t.Fatalf("active events already contained inference response: %v", functionalEventTypes(activeEvents))
	}
	requestEvent := activeEvents[requestEventIdx]
	requestPayload, err := requestEvent.Payload.AsInferenceRequestEventPayload()
	if err != nil {
		t.Fatalf("decode active inference request payload: %v", err)
	}
	dispatchID := stringValueFromFunctionalPtr(requestEvent.Context.DispatchId)
	if dispatchID == "" {
		t.Fatalf("active inference request missing context.dispatchId: %#v", requestEvent.Context)
	}
	dispatchReqIdx := indexOfFunctionalDispatchEvent(activeEvents, factoryapi.FactoryEventTypeDispatchRequest, dispatchID)
	if dispatchReqIdx < 0 || dispatchReqIdx > requestEventIdx {
		t.Fatalf("active events = %v, want dispatch request before inference request for %s", functionalEventTypes(activeEvents), dispatchID)
	}
	return thinEventSmokeActiveSnapshot{
		events:          activeEvents,
		requestEvent:    requestEvent,
		requestPayload:  requestPayload,
		dispatchID:      dispatchID,
		dispatchReqIdx:  dispatchReqIdx,
		requestEventIdx: requestEventIdx,
	}
}

func assertThinEventSmokeActiveSnapshot(t *testing.T, active thinEventSmokeActiveSnapshot) {
	t.Helper()

	assertRawThinDispatchRequestEvent(t, active.events[active.dispatchReqIdx])
	assertRawInferenceEventUsesContextDispatchIdentity(t, active.requestEvent, active.requestPayload.InferenceRequestId)

	activeState, err := projections.ReconstructFactoryWorldState(active.events, active.requestEvent.Context.Tick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick %d: %v", active.requestEvent.Context.Tick, err)
	}
	activeDispatch, ok := activeState.ActiveDispatches[active.dispatchID]
	if !ok {
		t.Fatalf("active dispatches = %#v, want %q", activeState.ActiveDispatches, active.dispatchID)
	}
	if len(activeState.CompletedDispatches) != 0 {
		t.Fatalf("active completed dispatches = %#v, want none before inference response", activeState.CompletedDispatches)
	}
	activeAttempt := activeState.InferenceAttemptsByDispatchID[active.dispatchID][active.requestPayload.InferenceRequestId]
	if activeAttempt.InferenceRequestID != active.requestPayload.InferenceRequestId || activeAttempt.Response != "" {
		t.Fatalf("active inference attempt = %#v, want pending request without response", activeAttempt)
	}
	if activeAttempt.Prompt == "" || activeAttempt.RequestTime.IsZero() || activeAttempt.TransitionID != activeDispatch.TransitionID {
		t.Fatalf("active inference attempt = %#v, want prompt, request time, and matching transition", activeAttempt)
	}
	assertThinEventSmokeActiveViews(t, activeState, active.dispatchID)
}

func assertThinEventSmokeActiveViews(
	t *testing.T,
	activeState interfaces.FactoryWorldState,
	dispatchID string,
) {
	t.Helper()

	activeView := projections.BuildFactoryWorldView(activeState)
	if activeView.Runtime.InFlightDispatchCount != 1 {
		t.Fatalf("active world view in-flight dispatch count = %d, want 1", activeView.Runtime.InFlightDispatchCount)
	}
	activeRequestView := workstationRequestViewByDispatchID(
		t,
		factoryboundary.BuildFactoryWorldWorkstationRequestProjectionSlice(activeState),
		dispatchID,
	)
	if activeRequestView.Response != nil {
		t.Fatalf("active workstation request response = %#v, want nil before inference response", activeRequestView.Response)
	}
	assertRuntimeAPIProjectionOmitsInferenceFields(
		t,
		activeRequestView.Request,
		[]string{"requestTime", "prompt", "provider", "model", "workingDirectory", "worktree", "requestMetadata"},
	)
}

func loadThinEventSmokeFinalSnapshot(
	t *testing.T,
	smoke thinEventSmokeHarness,
	active thinEventSmokeActiveSnapshot,
) thinEventSmokeFinalSnapshot {
	t.Helper()

	liveEvents, err := smoke.harness.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	artifact := testutil.LoadReplayArtifact(t, smoke.recordPath)
	assertInferenceEventsRecordedInArtifact(t, liveEvents, artifact.Events)
	responseEventIdx := indexOfFunctionalInferenceResponseForRequest(liveEvents, active.dispatchID, active.requestPayload.InferenceRequestId)
	if responseEventIdx < 0 {
		t.Fatalf("live events = %v, want inference response for dispatch %s request %s", functionalEventTypes(liveEvents), active.dispatchID, active.requestPayload.InferenceRequestId)
	}
	responsePayload, err := liveEvents[responseEventIdx].Payload.AsInferenceResponseEventPayload()
	if err != nil {
		t.Fatalf("decode final inference response payload: %v", err)
	}
	finalState, err := projections.ReconstructFactoryWorldState(artifact.Events, support.LastFactoryEventTick(artifact.Events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState final tick: %v", err)
	}
	return thinEventSmokeFinalSnapshot{
		liveEvents:            liveEvents,
		artifact:              artifact,
		responsePayload:       responsePayload,
		finalResponseEventIdx: responseEventIdx,
		finalState:            finalState,
	}
}

func assertThinEventSmokeFinalSnapshot(
	t *testing.T,
	active thinEventSmokeActiveSnapshot,
	final thinEventSmokeFinalSnapshot,
) {
	t.Helper()

	assertRawInferenceEventUsesContextDispatchIdentity(
		t,
		final.liveEvents[final.finalResponseEventIdx],
		final.responsePayload.InferenceRequestId,
	)
	finalDispatchResponseIdx := indexOfFunctionalDispatchEventAfter(
		final.liveEvents,
		factoryapi.FactoryEventTypeDispatchResponse,
		active.dispatchID,
		final.finalResponseEventIdx+1,
	)
	if finalDispatchResponseIdx < 0 {
		t.Fatalf("live events = %v, want dispatch response after inference response for %s", functionalEventTypes(final.liveEvents), active.dispatchID)
	}
	assertRawThinDispatchResponseEvent(t, final.liveEvents[finalDispatchResponseIdx])
	assertThinEventSmokeFinalState(t, active, final.finalState)
}

func assertThinEventSmokeFinalState(
	t *testing.T,
	active thinEventSmokeActiveSnapshot,
	finalState interfaces.FactoryWorldState,
) {
	t.Helper()

	if len(finalState.CompletedDispatches) < 2 {
		t.Fatalf("final completed dispatches = %#v, want both model-worker dispatches", finalState.CompletedDispatches)
	}
	finalAttempt := finalState.InferenceAttemptsByDispatchID[active.dispatchID][active.requestPayload.InferenceRequestId]
	if finalAttempt.Response != "Step one done. COMPLETE" || finalAttempt.ProviderSession == nil || finalAttempt.ProviderSession.ID != "sess-thin-dispatch-1" {
		t.Fatalf("final inference attempt = %#v, want recorded response and provider session", finalAttempt)
	}
	if finalAttempt.Diagnostics == nil || finalAttempt.Diagnostics.Provider == nil {
		t.Fatalf("final inference attempt diagnostics = %#v, want provider diagnostics", finalAttempt.Diagnostics)
	}
	if finalAttempt.Prompt == "" || finalAttempt.RequestTime.IsZero() {
		t.Fatalf("final inference attempt = %#v, want prompt and request time", finalAttempt)
	}
	completion := completedFunctionalDispatchByID(t, finalState.CompletedDispatches, active.dispatchID)
	if completion.ProviderSession == nil || completion.ProviderSession.ID != "sess-thin-dispatch-1" || completion.Diagnostics == nil || completion.Diagnostics.Provider == nil {
		t.Fatalf("completed dispatch = %#v, want provider session and diagnostics derived from inference response", completion)
	}
	providerSession := functionalProviderSessionByDispatchID(t, finalState.ProviderSessions, active.dispatchID)
	if providerSession.ProviderSession.ID != "sess-thin-dispatch-1" {
		t.Fatalf("provider session view = %#v, want sess-thin-dispatch-1", providerSession)
	}
	assertThinEventSmokeFinalViews(t, active.dispatchID, finalState)
}

func assertThinEventSmokeFinalViews(
	t *testing.T,
	dispatchID string,
	finalState interfaces.FactoryWorldState,
) {
	t.Helper()

	finalView := projections.BuildFactoryWorldView(finalState)
	if !worldViewDispatchHistoryContainsTrace(finalView, dispatchID, "trace-thin-event-reducers") {
		t.Fatalf("dispatch history = %#v, want dispatch %q for trace-thin-event-reducers", finalView.Runtime.Session.DispatchHistory, dispatchID)
	}
	if len(finalView.Runtime.Session.ProviderSessions) == 0 {
		t.Fatalf("provider sessions = %#v, want provider-attempt rows", finalView.Runtime.Session.ProviderSessions)
	}
	completedRequestView := workstationRequestViewByDispatchID(
		t,
		factoryboundary.BuildFactoryWorldWorkstationRequestProjectionSlice(finalState),
		dispatchID,
	)
	assertRuntimeAPIProjectionOmitsInferenceFields(
		t,
		completedRequestView.Request,
		[]string{"requestMetadata", "requestTime", "prompt", "provider", "model", "workingDirectory", "worktree"},
	)
	if completedRequestView.Response == nil {
		t.Fatalf("completed workstation request response = %#v, want omitted dispatch-level inference detail", completedRequestView.Response)
	}
	assertRuntimeAPIProjectionOmitsInferenceFields(
		t,
		completedRequestView.Response,
		[]string{"responseText", "providerSession", "diagnostics", "responseMetadata", "errorClass"},
	)
	completedAttempt := finalState.InferenceAttemptsByDispatchID[dispatchID]
	if len(completedAttempt) != 1 {
		t.Fatalf("completed inference attempts = %#v, want one attempt", completedAttempt)
	}
	for _, attempt := range completedAttempt {
		if attempt.Prompt == "" || attempt.Response == "" {
			t.Fatalf("completed inference attempt = %#v, want attempt-owned prompt/request/response detail", attempt)
		}
	}
}

type blockingFunctionalInferenceProvider struct {
	responses        []interfaces.InferenceResponse
	firstCallStarted chan interfaces.ProviderInferenceRequest
	releaseFirst     chan struct{}
	releaseOnce      sync.Once
	mu               sync.Mutex
	index            int
}

var _ workers.Provider = (*blockingFunctionalInferenceProvider)(nil)

func newBlockingFunctionalInferenceProvider(
	responses ...interfaces.InferenceResponse,
) *blockingFunctionalInferenceProvider {
	return &blockingFunctionalInferenceProvider{
		responses:        responses,
		firstCallStarted: make(chan interfaces.ProviderInferenceRequest, 1),
		releaseFirst:     make(chan struct{}),
	}
}

func (p *blockingFunctionalInferenceProvider) Infer(ctx context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	index := p.index
	p.index++
	p.mu.Unlock()

	if index == 0 {
		select {
		case p.firstCallStarted <- req:
		default:
		}
		select {
		case <-p.releaseFirst:
		case <-ctx.Done():
			return interfaces.InferenceResponse{}, ctx.Err()
		}
	}

	if index < len(p.responses) {
		return p.responses[index], nil
	}
	return interfaces.InferenceResponse{Content: "default mock response"}, nil
}

func (p *blockingFunctionalInferenceProvider) WaitForFirstCall(t *testing.T, timeout time.Duration) interfaces.ProviderInferenceRequest {
	t.Helper()

	select {
	case req := <-p.firstCallStarted:
		return req
	case <-time.After(timeout):
		t.Fatalf("timed out waiting %s for first provider call", timeout)
	}
	return interfaces.ProviderInferenceRequest{}
}

func (p *blockingFunctionalInferenceProvider) ReleaseFirst() {
	p.releaseOnce.Do(func() {
		close(p.releaseFirst)
	})
}

func thinReducerInferenceResponse(sessionID string, content string) interfaces.InferenceResponse {
	return interfaces.InferenceResponse{
		Content: content,
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       sessionID,
		},
		Diagnostics: &interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{
				Provider: "codex",
				Model:    "gpt-5.4",
				RequestMetadata: map[string]string{
					"prompt_source": "factory-renderer",
				},
			},
		},
	}
}

func waitForFunctionalInferenceRequestSnapshot(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	timeout time.Duration,
) []factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		events, err := h.GetFactoryEvents(context.Background())
		if err != nil {
			t.Fatalf("GetFactoryEvents while waiting for inference request: %v", err)
		}
		if indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeDispatchRequest, 0) >= 0 &&
			indexOfFunctionalEventType(events, factoryapi.FactoryEventTypeInferenceRequest, 0) >= 0 {
			return events
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting %s for dispatch and inference request events; saw %v", timeout, functionalEventTypes(events))
		}
		<-ticker.C
	}
}

func waitForFunctionalHarnessCompletion(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	errCh <-chan error,
	cancel context.CancelFunc,
	timeout time.Duration,
) {
	t.Helper()

	select {
	case <-h.WaitToComplete():
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("factory run exited before completion: %v", err)
		}
	case <-time.After(timeout):
		t.Fatalf("timed out waiting %s for functional harness completion", timeout)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("factory run error: %v", err)
		}
	case <-time.After(timeout):
		t.Fatalf("timed out waiting %s for background run to exit", timeout)
	}
}

func workstationRequestViewByDispatchID(
	t *testing.T,
	slice factoryapi.FactoryWorldWorkstationRequestProjectionSlice,
	dispatchID string,
) factoryapi.FactoryWorldWorkstationRequestView {
	t.Helper()

	if slice.WorkstationRequestsByDispatchId == nil {
		t.Fatalf("workstation request slice missing projection map for dispatch %q", dispatchID)
	}
	view, ok := (*slice.WorkstationRequestsByDispatchId)[dispatchID]
	if !ok {
		t.Fatalf("workstation request slice = %#v, want dispatch %q", slice.WorkstationRequestsByDispatchId, dispatchID)
	}
	return view
}

func completedFunctionalDispatchByID(
	t *testing.T,
	completions []interfaces.FactoryWorldDispatchCompletion,
	dispatchID string,
) interfaces.FactoryWorldDispatchCompletion {
	t.Helper()

	for _, completion := range completions {
		if completion.DispatchID == dispatchID {
			return completion
		}
	}
	t.Fatalf("completed dispatches = %#v, want dispatch %q", completions, dispatchID)
	return interfaces.FactoryWorldDispatchCompletion{}
}

func functionalProviderSessionByDispatchID(
	t *testing.T,
	sessions []interfaces.FactoryWorldProviderSessionRecord,
	dispatchID string,
) interfaces.FactoryWorldProviderSessionRecord {
	t.Helper()

	for _, session := range sessions {
		if session.DispatchID == dispatchID {
			return session
		}
	}
	t.Fatalf("provider sessions = %#v, want dispatch %q", sessions, dispatchID)
	return interfaces.FactoryWorldProviderSessionRecord{}
}

func sliceValue[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}

func mapValue[K comparable, V any](values *map[K]V) map[K]V {
	if values == nil {
		return nil
	}
	return *values
}

func worldViewDispatchHistoryContainsTrace(view interfaces.FactoryWorldView, dispatchID, traceID string) bool {
	for _, dispatch := range view.Runtime.Session.DispatchHistory {
		if dispatch.DispatchID != dispatchID {
			continue
		}
		for _, candidate := range dispatch.TraceIDs {
			if candidate == traceID {
				return true
			}
		}
	}
	return false
}
