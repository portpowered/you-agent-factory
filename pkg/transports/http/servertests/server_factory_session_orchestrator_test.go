package apiserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

func TestFactorySessionsAPI_ResponseEventsRouteStreamsGeneratedContractEnvelope(t *testing.T) {
	store := responseeventstore.NewSessionResponseEventStore("session-beta")
	want := publishIntegrationResponseProgress(t, store, "integration-retained")
	store.Complete()
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET response-events status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	reader := bufio.NewReader(rec.Body)
	idLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read SSE id: %v", err)
	}
	dataLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read SSE data: %v", err)
	}
	separator, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read SSE separator: %v", err)
	}
	if strings.TrimSpace(idLine) != "id: 1" || strings.TrimSpace(separator) != "" || !strings.HasPrefix(dataLine, "data: ") {
		t.Fatalf("SSE framing = %q%q%q, want one id and one data line", idLine, dataLine, separator)
	}
	var got responseevents.FactoryResponseEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(dataLine, "data: "))), &got); err != nil {
		t.Fatalf("decode response-event SSE data: %v", err)
	}
	if got.EventID != want.EventID || got.FactorySessionID != "session-beta" {
		t.Fatalf("response event = %#v, want event %q for session-beta", got, want.EventID)
	}
	if err := responseevents.ValidateEvent(got); err != nil {
		t.Fatalf("response event violates public schema semantics: %v", err)
	}
}

func TestFactorySessionsAPI_ResponseEventsRouteSignalsStaleReconnectFirst(t *testing.T) {
	store, err := responseeventstore.NewSessionResponseEventStoreWithLimits(
		"session-beta",
		responseeventstore.RetentionLimits{MaxEvents: 2, MaxBytes: 1 << 20},
	)
	if err != nil {
		t.Fatalf("new response-event store: %v", err)
	}
	publishIntegrationResponseProgress(t, store, "dropped-1")
	publishIntegrationResponseProgress(t, store, "dropped-2")
	wantFirst := publishIntegrationResponseProgress(t, store, "retained-3")
	wantSecond := publishIntegrationResponseProgress(t, store, "retained-4")
	store.Complete()
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events?after_sequence=1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET stale response-events status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	reader := bufio.NewReader(rec.Body)
	gap := readIntegrationResponseEvent(t, reader)
	if gap.Kind != responseevents.KindStreamGap {
		t.Fatalf("first stale response event kind = %q, want STREAM_GAP", gap.Kind)
	}
	var gapPayload responseevents.StreamGapPayload
	if err := json.Unmarshal(gap.Payload, &gapPayload); err != nil {
		t.Fatalf("decode STREAM_GAP payload: %v", err)
	}
	if gapPayload.FromSequence != 2 || gapPayload.ToSequence != 2 || gapPayload.FirstAvailableSequence != 3 {
		t.Fatalf("STREAM_GAP payload = %#v, want unavailable 2..2 and first available 3", gapPayload)
	}
	gotFirst := readIntegrationResponseEvent(t, reader)
	gotSecond := readIntegrationResponseEvent(t, reader)
	if gotFirst.EventID != wantFirst.EventID || gotSecond.EventID != wantSecond.EventID {
		t.Fatalf("stale catch-up = [%q %q], want [%q %q]", gotFirst.EventID, gotSecond.EventID, wantFirst.EventID, wantSecond.EventID)
	}
}

type integrationResponseEventClock struct {
	now time.Time
}

func (c *integrationResponseEventClock) Now() time.Time {
	return c.now
}

func TestFactorySessionsAPI_ResponseEventsRouteReturnsTypedExpiredOutcome(t *testing.T) {
	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	clock := &integrationResponseEventClock{now: start}
	store := responseeventstore.NewSessionResponseEventStoreWithClock("session-beta", clock)
	publishIntegrationResponseProgress(t, store, "completed")
	store.Complete()
	clock.now = start.Add(responseeventstore.CompletedStreamRetentionWindow)
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("GET expired response-events status = %d, want 410: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode expired response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeRESPONSEEVENTSTREAMEXPIRED || response.Family != factoryapi.ErrorFamilyGone {
		t.Fatalf("expired response = %#v, want typed RESPONSE_EVENT_STREAM_EXPIRED/GONE", response)
	}
	if got := rec.Header().Get("Content-Type"); strings.Contains(got, "text/event-stream") {
		t.Fatalf("expired response Content-Type = %q, want JSON before SSE headers", got)
	}
}

func publishIntegrationResponseProgress(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	label string,
) responseevents.FactoryResponseEvent {
	t.Helper()
	payload, err := json.Marshal(responseevents.ProgressPayload{Label: label})
	if err != nil {
		t.Fatalf("marshal response-event payload: %v", err)
	}
	event, err := store.Publish(responseevents.FactoryResponseEvent{
		FactorySessionID: store.FactorySessionID(),
		Kind:             responseevents.KindProgress,
		Phase:            responseevents.PhaseUpdated,
		Provenance: responseevents.Provenance{
			Provider:        "test-provider",
			NativeEventType: "progress",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        responseevents.FidelityLossless,
		},
		RunID:   "run-integration",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("publish response event: %v", err)
	}
	return event
}

func readIntegrationResponseEvent(t *testing.T, reader *bufio.Reader) responseevents.FactoryResponseEvent {
	t.Helper()
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read SSE id: %v", err)
	}
	dataLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read SSE data: %v", err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read SSE separator: %v", err)
	}
	var event responseevents.FactoryResponseEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(dataLine, "data: "))), &event); err != nil {
		t.Fatalf("decode response-event SSE data: %v", err)
	}
	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("response event violates public schema semantics: %v", err)
	}
	return event
}

func newMockFactorySessionTestServer(f *testutil.MockFactory) *api.Server {
	logger, _ := zap.NewDevelopment()
	return api.NewServer(f, 8080, logger)
}

func stringPointerForFactorySessionTest(value string) *string {
	return &value
}

func TestFactorySessionsAPI_GetFactorySession(t *testing.T) {
	phase := "review"
	backendScopeID := "backend-scope-live"
	logicalSessionKeyID := "/workspace/root::named::beta"
	streamGenerationID := "stream-gen-live-001"
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
			Target: factoryapi.FactorySessionTargetRef{
				Kind: factoryapi.FactorySessionTargetRefKindNamed,
				Name: stringPointerForFactorySessionTest("beta"),
			},
			Runtime: factoryapi.FactorySessionRuntime{
				OrchestratorKind: factoryapi.JAVASCRIPT,
				StreamIdentity: &factoryapi.FactorySessionStreamIdentity{
					BackendScopeID:      backendScopeID,
					LogicalSessionKeyID: logicalSessionKeyID,
					FactorySessionID:    "session-beta",
					StreamGenerationID:  streamGenerationID,
				},
				Status: factoryapi.FactorySessionStatusIDLE,
				Progress: factoryapi.FactorySessionProgress{
					FactoryState:  "UNKNOWN",
					Categories:    factoryapi.StatusCategories{},
					InFlightCount: 0,
					TotalTokens:   0,
				},
				Usage: factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}},
				Lifecycle: factoryapi.FactorySessionLifecycle{
					StartedAt: time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
				},
				Javascript: &factoryapi.FactorySessionJavaScriptProjection{
					Phase:               &phase,
					Phases:              []string{"plan", "review"},
					ScriptStatus:        factoryapi.FactorySessionJavaScriptScriptStatusIDLE,
					ChildDispatchCounts: factoryapi.FactorySessionJavaScriptChildDispatchCounts{},
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/session-beta status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.FactorySession
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode factory session response: %v", err)
	}
	if response.Runtime.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("orchestrator kind = %q, want JAVASCRIPT", response.Runtime.OrchestratorKind)
	}
	if response.Runtime.StreamIdentity == nil {
		t.Fatal("streamIdentity = nil, want populated identity")
	}
	if response.Runtime.StreamIdentity.BackendScopeID != backendScopeID {
		t.Fatalf("streamIdentity.backendScopeID = %q, want %q", response.Runtime.StreamIdentity.BackendScopeID, backendScopeID)
	}
	if response.Runtime.StreamIdentity.FactorySessionID != "session-beta" {
		t.Fatalf("streamIdentity.factorySessionID = %q, want session-beta", response.Runtime.StreamIdentity.FactorySessionID)
	}
	if response.Runtime.StreamIdentity.StreamGenerationID != streamGenerationID {
		t.Fatalf("streamIdentity.streamGenerationID = %q, want %q", response.Runtime.StreamIdentity.StreamGenerationID, streamGenerationID)
	}
	if response.Runtime.Javascript == nil || response.Runtime.Javascript.Phase == nil || *response.Runtime.Javascript.Phase != "review" {
		t.Fatalf("javascript projection = %#v", response.Runtime.Javascript)
	}
}

func TestFactorySessionsAPI_GetFactorySessionSyncPreflight(t *testing.T) {
	backendScopeID := "backend-scope-test"
	logicalSessionKeyID := "/workspace/root::default::"
	factorySessionID := "~default"
	streamGenerationID := "backend-scope-test::~default"
	afterEventID := "factory-event/initial-structure/0"
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySessionSyncPreflight: factoryapi.FactorySessionSyncPreflightResponse{
			RequestedSessionId:  "~default",
			ReasonCode:          factoryapi.Ok,
			CheckpointReusable:  true,
			BackendScopeId:      &backendScopeID,
			LogicalSessionKeyId: &logicalSessionKeyID,
			FactorySessionId:    &factorySessionID,
			StreamGenerationId:  &streamGenerationID,
			ReconnectCursor: factoryapi.FactorySessionSyncPreflightReconnectCursor{
				Provided:                 true,
				ValidForStreamGeneration: true,
				AfterEventId:             &afterEventID,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/sync-preflight?after_event_id="+afterEventID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/~default/sync-preflight status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.FactorySessionSyncPreflightResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode sync preflight response: %v", err)
	}
	if response.ReasonCode != factoryapi.Ok {
		t.Fatalf("reasonCode = %q, want %q", response.ReasonCode, factoryapi.Ok)
	}
	if !response.CheckpointReusable || !response.ReconnectCursor.ValidForStreamGeneration {
		t.Fatalf("response = %#v, want reusable valid cursor", response)
	}
	if response.FactorySessionId == nil || *response.FactorySessionId != factorySessionID {
		t.Fatalf("factorySessionId = %#v, want %q", response.FactorySessionId, factorySessionID)
	}
}

func TestFactorySessionsAPI_GetFactorySessionSyncPreflight_StaleCursorReturnsTypedOutcome(t *testing.T) {
	backendScopeID := "backend-scope-test"
	logicalSessionKeyID := "/workspace/root::default::"
	factorySessionID := "~default"
	streamGenerationID := "backend-scope-test::~default"
	afterEventID := "factory-event/missing-preflight-cursor"
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySessionSyncPreflight: factoryapi.FactorySessionSyncPreflightResponse{
			RequestedSessionId:  "~default",
			ReasonCode:          factoryapi.CursorStale,
			CheckpointReusable:  false,
			BackendScopeId:      &backendScopeID,
			LogicalSessionKeyId: &logicalSessionKeyID,
			FactorySessionId:    &factorySessionID,
			StreamGenerationId:  &streamGenerationID,
			ReconnectCursor: factoryapi.FactorySessionSyncPreflightReconnectCursor{
				Provided:                 true,
				ValidForStreamGeneration: false,
				AfterEventId:             &afterEventID,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/sync-preflight?after_event_id="+afterEventID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET stale sync preflight status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.FactorySessionSyncPreflightResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode stale sync preflight response: %v", err)
	}
	if response.ReasonCode != factoryapi.CursorStale {
		t.Fatalf("reasonCode = %q, want %q", response.ReasonCode, factoryapi.CursorStale)
	}
	if response.CheckpointReusable || response.ReconnectCursor.ValidForStreamGeneration {
		t.Fatalf("response = %#v, want non-reusable stale cursor outcome", response)
	}
}

func TestFactorySessionsAPI_GetFactorySessionSyncPreflight_MissingSessionReturnsTypedOutcome(t *testing.T) {
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySessionSyncPreflight: factoryapi.FactorySessionSyncPreflightResponse{
			RequestedSessionId: "~default",
			ReasonCode:         factoryapi.SessionNotFound,
			CheckpointReusable: false,
			ReconnectCursor: factoryapi.FactorySessionSyncPreflightReconnectCursor{
				Provided:                 false,
				ValidForStreamGeneration: false,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/live-session-missing-001/sync-preflight", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET missing-session sync preflight status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.FactorySessionSyncPreflightResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode missing-session sync preflight response: %v", err)
	}
	if response.ReasonCode != factoryapi.SessionNotFound {
		t.Fatalf("reasonCode = %q, want %q", response.ReasonCode, factoryapi.SessionNotFound)
	}
	if response.CheckpointReusable {
		t.Fatal("checkpointReusable = true, want false")
	}
	if response.BackendScopeId != nil || response.FactorySessionId != nil || response.StreamGenerationId != nil {
		t.Fatalf("identity fields = %#v, want nil for missing session", response)
	}
}

func TestFactorySessionsAPI_GetFactorySessionResult_OmitsRawCheckpointBody(t *testing.T) {
	hash := "sha256:checkpoint-body"
	size := int64(128)
	checkpoints := []factoryapi.FactorySessionJavaScriptCheckpointRef{{
		Id:    "ckpt-1",
		Label: stringPointerForFactorySessionTest("after-plan"),
		ArtifactRef: &factoryapi.FactoryArtifactRef{
			Id:          "artifact-ckpt-1",
			Kind:        factoryapi.FactoryArtifactKindCHECKPOINT,
			Visibility:  factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			ContentHash: &hash,
			SizeBytes:   &size,
		},
	}}
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySessionLiveResult: factoryapi.FactorySessionLiveResult{
			SessionId: "session-js",
			Status:    factoryapi.FactorySessionStatusIDLE,
			ResultArtifactRef: &factoryapi.FactoryArtifactRef{
				Id:         "artifact-ckpt-1",
				Kind:       factoryapi.FactoryArtifactKindCHECKPOINT,
				Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			},
			CheckpointRefs: &checkpoints,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-js/result", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/session-js/result status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"rawBody", "storagePath", "vmState", "/tmp/checkpoints"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("session result leaked %q: %s", forbidden, body)
		}
	}
	var response factoryapi.FactorySessionLiveResult
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&response); err != nil {
		t.Fatalf("decode session result: %v", err)
	}
	if response.ResultArtifactRef == nil || response.ResultArtifactRef.Visibility != factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT {
		t.Fatalf("result artifact ref = %#v", response.ResultArtifactRef)
	}
}

func TestFactorySessionsAPI_GetFactorySessionPartialResult_ReturnsCheckpointRefs(t *testing.T) {
	phase := "review"
	hash := "sha256:checkpoint-body"
	checkpoints := []factoryapi.FactorySessionJavaScriptCheckpointRef{{
		Id: "ckpt-1",
		ArtifactRef: &factoryapi.FactoryArtifactRef{
			Id:          "artifact-ckpt-1",
			Kind:        factoryapi.FactoryArtifactKindCHECKPOINT,
			Visibility:  factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			ContentHash: &hash,
		},
	}}
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		FactorySessionPartialResult: factoryapi.FactorySessionPartialResult{
			SessionId: "session-js",
			Phase:     phase,
			PartialResultArtifactRef: &factoryapi.FactoryArtifactRef{
				Id:         "artifact-ckpt-1",
				Kind:       factoryapi.FactoryArtifactKindCHECKPOINT,
				Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			},
			CheckpointRefs: &checkpoints,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-js/partial-result", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions/session-js/partial-result status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.FactorySessionPartialResult
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode partial result: %v", err)
	}
	if response.Phase != phase {
		t.Fatalf("phase = %q, want %q", response.Phase, phase)
	}
	if response.PartialResultArtifactRef == nil {
		t.Fatal("expected partial result artifact ref")
	}
}

func TestFactorySessionsAPI_GetFactorySessionResult_NotFoundForUnavailableSession(t *testing.T) {
	srv := newMockFactorySessionTestServer(&testutil.MockFactory{
		GetFactorySessionResultErr: apisurface.ErrFactorySessionResultUnavailable,
	})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-petri/result", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

const (
	responseEventSlowConsumerFloodCount      = 200
	responseEventSlowConsumerPublishBudget   = 3 * time.Second
	responseEventSlowConsumerScenarioTimeout = 10 * time.Second
)

type gatedSSEResponseReader struct {
	src          io.Reader
	mu           sync.Mutex
	blocked      bool
	blockedReads atomic.Int64
}

func newGatedSSEResponseReader(src io.Reader) *gatedSSEResponseReader {
	return &gatedSSEResponseReader{src: src}
}

func (r *gatedSSEResponseReader) block() {
	r.mu.Lock()
	r.blocked = true
	r.mu.Unlock()
}

func (r *gatedSSEResponseReader) release() {
	r.mu.Lock()
	r.blocked = false
	r.mu.Unlock()
}

func (r *gatedSSEResponseReader) Read(p []byte) (int, error) {
	for {
		r.mu.Lock()
		blocked := r.blocked
		r.mu.Unlock()
		if !blocked {
			return r.src.Read(p)
		}
		r.blockedReads.Add(1)
		time.Sleep(1 * time.Millisecond)
	}
}

// TestFactoryResponseEventsBySessionID_SlowConsumerFloodDoesNotBlockPublication
// proves the public SSE response-event surface keeps store publication
// non-blocking while a client deliberately stops draining.
func TestFactoryResponseEventsBySessionID_SlowConsumerFloodDoesNotBlockPublication(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping response-event SSE slow-consumer fixture in short mode")
	}

	store, err := responseeventstore.NewSessionResponseEventStoreWithLimits(
		"session-beta",
		responseeventstore.RetentionLimits{MaxEvents: 16, MaxBytes: 64 * 1024},
	)
	if err != nil {
		t.Fatalf("new response-event store: %v", err)
	}

	publishIntegrationResponseProgress(t, store, "seed")

	srv := newMockFactorySessionTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	scenarioCtx, cancelScenario := context.WithTimeout(context.Background(), responseEventSlowConsumerScenarioTimeout)
	defer cancelScenario()

	request, err := http.NewRequestWithContext(
		scenarioCtx,
		http.MethodGet,
		httpServer.URL+"/factory-sessions/session-beta/response-events",
		nil,
	)
	if err != nil {
		t.Fatalf("new response-event request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open response-event stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("response-event status = %d, want 200", response.StatusCode)
	}

	gated := newGatedSSEResponseReader(response.Body)
	gated.block()

	floodDone := make(chan error, 1)
	go func() {
		publishStarted := time.Now()
		if _, err := store.Publish(responseEventRunCompletedInput("run-completed")); err != nil {
			floodDone <- err
			return
		}
		for i := 0; i < responseEventSlowConsumerFloodCount; i++ {
			if _, err := store.Publish(responseEventDeltaPublishInput(i)); err != nil {
				floodDone <- err
				return
			}
		}
		if elapsed := time.Since(publishStarted); elapsed > responseEventSlowConsumerPublishBudget {
			floodDone <- fmt.Errorf("publish elapsed = %v, want <= %v", elapsed, responseEventSlowConsumerPublishBudget)
			return
		}
		store.Complete()
		floodDone <- nil
	}()

	select {
	case err := <-floodDone:
		if err != nil {
			t.Fatalf("publish flood while SSE consumer lagged: %v", err)
		}
	case <-time.After(responseEventSlowConsumerPublishBudget):
		t.Fatalf("timed out waiting for publish flood within %v while SSE consumer was blocked", responseEventSlowConsumerPublishBudget)
	}

	wantFinalRun, ok := retainedResponseRunCompleted(t, store)
	if !ok {
		t.Fatal("terminal RUN completed event evicted under flood pressure")
	}

	gated.release()
	reader := bufio.NewReader(gated)
	drainCtx, cancelDrain := context.WithTimeout(scenarioCtx, 3*time.Second)
	defer cancelDrain()
	if err := drainResponseEventSSEUntilClosed(drainCtx, reader, wantFinalRun.EventID); err != nil {
		t.Fatalf("drain SSE after releasing slow consumer: %v", err)
	}
}

func retainedResponseRunCompleted(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
) (responseevents.FactoryResponseEvent, bool) {
	t.Helper()
	for _, event := range store.Events() {
		if event.Kind == responseevents.KindRun && event.Phase == responseevents.PhaseCompleted {
			return event, true
		}
	}
	return responseevents.FactoryResponseEvent{}, false
}

func drainResponseEventSSEUntilClosed(ctx context.Context, reader *bufio.Reader, wantFinalRunEventID string) error {
	sawFinalRun := false
	for {
		if err := ctx.Err(); err != nil {
			if sawFinalRun {
				return nil
			}
			return fmt.Errorf("timed out before terminal RUN event %q was observed", wantFinalRunEventID)
		}
		event, err := tryReadSSEFactoryResponseEvent(reader)
		if err == io.EOF {
			if !sawFinalRun {
				return fmt.Errorf("SSE stream closed before terminal RUN event %q", wantFinalRunEventID)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if event.EventID == wantFinalRunEventID {
			sawFinalRun = true
		}
	}
}

func tryReadSSEFactoryResponseEvent(reader *bufio.Reader) (responseevents.FactoryResponseEvent, error) {
	var idLine, dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return responseevents.FactoryResponseEvent{}, err
		}
		line = trimSSELine(line)
		if line == "" {
			break
		}
		switch {
		case hasSSEPrefix(line, "id: "):
			idLine = trimSSEPrefix(line, "id: ")
		case hasSSEPrefix(line, "data: "):
			dataLine = trimSSEPrefix(line, "data: ")
		default:
			return responseevents.FactoryResponseEvent{}, fmt.Errorf("unexpected response-event SSE line %q", line)
		}
	}
	if idLine == "" || dataLine == "" {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE message id=%q data=%q, want exactly one of each", idLine, dataLine)
	}
	var event responseevents.FactoryResponseEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("decode response-event SSE data: %w", err)
	}
	if idLine != fmt.Sprint(event.Sequence) {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE id = %q, want event sequence %d", idLine, event.Sequence)
	}
	if err := responseevents.ValidateEvent(event); err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE response event is schema-invalid: %w", err)
	}
	return event, nil
}

func trimSSELine(line string) string {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line
}

func hasSSEPrefix(line, prefix string) bool {
	return len(line) >= len(prefix) && line[:len(prefix)] == prefix
}

func trimSSEPrefix(line, prefix string) string {
	return line[len(prefix):]
}

func responseEventRunCompletedInput(status string) responseevents.FactoryResponseEvent {
	payload, _ := json.Marshal(responseevents.RunPayload{Status: status})
	return responseevents.FactoryResponseEvent{
		FactorySessionID: "session-beta",
		Kind:             responseevents.KindRun,
		Phase:            responseevents.PhaseCompleted,
		Provenance: responseevents.Provenance{
			Provider:        "test-provider",
			NativeEventType: "run",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        responseevents.FidelityLossless,
		},
		RunID:   "run-test",
		Payload: payload,
	}
}

func responseEventDeltaPublishInput(index int) responseevents.FactoryResponseEvent {
	payload, _ := json.Marshal(responseevents.MessageDeltaPayload{
		ContentBlockIndex: 0,
		ContentBlockKind:  responseevents.ContentBlockText,
		TextDelta:         "delta-" + strconv.Itoa(index),
	})
	return responseevents.FactoryResponseEvent{
		FactorySessionID: "session-beta",
		Kind:             responseevents.KindMessage,
		Phase:            responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider:        "test-provider",
			NativeEventType: "message.delta",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        responseevents.FidelityLossless,
		},
		RunID:   "run-test",
		Payload: payload,
	}
}
