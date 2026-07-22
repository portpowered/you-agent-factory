package runtime_api

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type thinEventSmokeHarness struct {
	server     *support.FunctionalAPIServer
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
	session         factoryapi.FactorySession
}

type thinEventSmokeFinalSnapshot struct {
	liveEvents            []factoryapi.FactoryEvent
	responsePayload       factoryapi.InferenceResponseEventPayload
	finalResponseEventIdx int
	session               factoryapi.FactorySession
	work                  factoryapi.ListWorkResponse
}

func newThinEventSmokeHarness(t *testing.T) thinEventSmokeHarness {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	recordPath := filepath.Join(t.TempDir(), "thin-event-reducer-views.replay.json")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-thin-event-reducers",
		WorkTypeID: "task",
		TraceID:    "trace-thin-event-reducers",
		Payload:    []byte(`{"title":"reconstruct thin reducer views"}`),
	})
	provider := newBlockingFunctionalInferenceProvider(
		thinReducerInferenceResponse("sess-thin-dispatch-1", "Step one done. COMPLETE"),
		thinReducerInferenceResponse("sess-thin-dispatch-2", "Step two done. COMPLETE"),
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", recordPath},
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	return thinEventSmokeHarness{server: server, provider: provider, recordPath: recordPath}
}

func captureThinEventSmokeActiveSnapshot(
	t *testing.T,
	smoke thinEventSmokeHarness,
) thinEventSmokeActiveSnapshot {
	t.Helper()

	smoke.provider.WaitForFirstCall(t, 5*time.Second)
	activeEvents := waitForFunctionalInferenceRequestSnapshot(t, smoke.server, 5*time.Second)
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
		session:         support.GetDefaultSession(t, smoke.server.URL()),
	}
}

func assertThinEventSmokeActiveSnapshot(t *testing.T, active thinEventSmokeActiveSnapshot) {
	t.Helper()

	assertRawThinDispatchRequestEvent(t, active.events[active.dispatchReqIdx])
	assertRawInferenceEventUsesContextDispatchIdentity(t, active.requestEvent, active.requestPayload.InferenceRequestId)
	if active.session.Runtime.Progress.Categories.Processing != 1 ||
		active.session.Runtime.Progress.Categories.Terminal != 0 {
		t.Fatalf(
			"active Factory Session categories = %#v, want processing=1 terminal=0",
			active.session.Runtime.Progress.Categories,
		)
	}
}

func loadThinEventSmokeFinalSnapshot(
	t *testing.T,
	smoke thinEventSmokeHarness,
	active thinEventSmokeActiveSnapshot,
) thinEventSmokeFinalSnapshot {
	t.Helper()

	liveEvents := smoke.server.GetFactoryEvents(t)
	session := support.GetDefaultSession(t, smoke.server.URL())
	work := support.ListDefaultSessionWork(t, smoke.server.URL())
	smoke.server.Stop(t)
	artifact := testutil.LoadReplayArtifact(t, smoke.recordPath)
	generatedArtifactEvents := testutil.GeneratedFactoryEvents(t, artifact.Events)
	assertInferenceEventsRecordedInArtifact(t, liveEvents, generatedArtifactEvents)
	responseEventIdx := indexOfFunctionalInferenceResponseForRequest(liveEvents, active.dispatchID, active.requestPayload.InferenceRequestId)
	if responseEventIdx < 0 {
		t.Fatalf("live events = %v, want inference response for dispatch %s request %s", functionalEventTypes(liveEvents), active.dispatchID, active.requestPayload.InferenceRequestId)
	}
	responsePayload, err := liveEvents[responseEventIdx].Payload.AsInferenceResponseEventPayload()
	if err != nil {
		t.Fatalf("decode final inference response payload: %v", err)
	}
	return thinEventSmokeFinalSnapshot{
		liveEvents:            liveEvents,
		responsePayload:       responsePayload,
		finalResponseEventIdx: responseEventIdx,
		session:               session,
		work:                  work,
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
	if final.responsePayload.ProviderSession == nil ||
		stringValueFromFunctionalPtr(final.responsePayload.ProviderSession.Id) != "sess-thin-dispatch-1" {
		t.Fatalf("public INFERENCE_RESPONSE provider session = %#v, want sess-thin-dispatch-1", final.responsePayload.ProviderSession)
	}
	if final.responsePayload.Diagnostics == nil || final.responsePayload.Diagnostics.Provider == nil {
		t.Fatalf("public INFERENCE_RESPONSE diagnostics = %#v, want provider diagnostics", final.responsePayload.Diagnostics)
	}
	if final.session.Runtime.Progress.Categories.Processing != 0 ||
		final.session.Runtime.Progress.Categories.Terminal != 1 {
		t.Fatalf(
			"final Factory Session categories = %#v, want processing=0 terminal=1",
			final.session.Runtime.Progress.Categories,
		)
	}
	if len(final.work.Results) != 1 ||
		stringValueFromFunctionalPtr(final.work.Results[0].TraceId) != "trace-thin-event-reducers" {
		t.Fatalf("public Work listing = %#v, want completed thin-event trace", final.work.Results)
	}
}

type blockingFunctionalInferenceProvider struct {
	responses        []workerexecution.InferenceResponse
	firstCallStarted chan workerexecution.ProviderInferenceRequest
	releaseFirst     chan struct{}
	releaseOnce      sync.Once
	mu               sync.Mutex
	index            int
}

var _ workerprovider.Provider = (*blockingFunctionalInferenceProvider)(nil)

func newBlockingFunctionalInferenceProvider(
	responses ...workerexecution.InferenceResponse,
) *blockingFunctionalInferenceProvider {
	return &blockingFunctionalInferenceProvider{
		responses:        responses,
		firstCallStarted: make(chan workerexecution.ProviderInferenceRequest, 1),
		releaseFirst:     make(chan struct{}),
	}
}

func (p *blockingFunctionalInferenceProvider) Infer(ctx context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
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
			return workerexecution.InferenceResponse{}, ctx.Err()
		}
	}

	if index < len(p.responses) {
		return p.responses[index], nil
	}
	return workerexecution.InferenceResponse{Content: "default mock response"}, nil
}

func (p *blockingFunctionalInferenceProvider) WaitForFirstCall(t *testing.T, timeout time.Duration) workerexecution.ProviderInferenceRequest {
	t.Helper()

	select {
	case req := <-p.firstCallStarted:
		return req
	case <-time.After(timeout):
		t.Fatalf("timed out waiting %s for first provider call", timeout)
	}
	return workerexecution.ProviderInferenceRequest{}
}

func (p *blockingFunctionalInferenceProvider) ReleaseFirst() {
	p.releaseOnce.Do(func() {
		close(p.releaseFirst)
	})
}

func thinReducerInferenceResponse(sessionID string, content string) workerexecution.InferenceResponse {
	return workerexecution.InferenceResponse{
		Content: content,
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       sessionID,
		},
		Diagnostics: &workerexecution.WorkDiagnostics{
			Provider: &workerexecution.ProviderDiagnostic{
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
	server *support.FunctionalAPIServer,
	timeout time.Duration,
) []factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		events := server.GetFactoryEvents(t)
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
	server *support.FunctionalAPIServer,
	timeout time.Duration,
) {
	t.Helper()

	support.WaitForTerminalStatus(t, server.URL(), timeout)
}
