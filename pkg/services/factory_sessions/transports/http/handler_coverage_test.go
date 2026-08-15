package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type sessionEventAPIFake struct {
	subscribe func(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error)
	probe     func(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) error
}

func (f sessionEventAPIFake) SubscribeFactoryEventsForSession(
	ctx context.Context,
	sessionID string,
	cursor *factorydefinitions.FactoryEventReconnectCursor,
) (*factorydefinitions.FactoryEventStream, error) {
	return f.subscribe(ctx, sessionID, cursor)
}

func (f sessionEventAPIFake) ProbeFactoryEventsForSession(
	ctx context.Context,
	sessionID string,
	cursor *factorydefinitions.FactoryEventReconnectCursor,
) error {
	if f.probe == nil {
		return nil
	}
	return f.probe(ctx, sessionID, cursor)
}

// TestHandlerUnavailableBranchesStayOwnedBySessions exercises the retained
// session/current-factory/event routes without constructing a runtime graph.
func TestHandlerUnavailableBranchesStayOwnedBySessions(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Dependencies{}, zap.NewNop())
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	var sessionID factoryapi.SessionID = "session-alpha"
	var dispatchID factoryapi.DispatchID = "dispatch-alpha"
	var artifactID factoryapi.ArtifactID = "artifact-alpha"

	tests := []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, r *http.Request) { handler.ValidateFactory(w, r) },
		func(w http.ResponseWriter, r *http.Request) { handler.PreviewFactory(w, r) },
		func(w http.ResponseWriter, r *http.Request) { handler.GetFactorySessionResult(w, r, sessionID) },
		func(w http.ResponseWriter, r *http.Request) { handler.GetFactorySessionPartialResult(w, r, sessionID) },
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactorySessionDispatch(w, r, sessionID, dispatchID)
		},
		func(w http.ResponseWriter, r *http.Request) { handler.ListFactorySessionArtifacts(w, r, sessionID) },
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactorySessionArtifact(w, r, sessionID, artifactID)
		},
		func(w http.ResponseWriter, r *http.Request) { handler.GetCurrentFactoryBySessionId(w, r, sessionID) },
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetCurrentFactoryWorkstationPromptTemplateContractBySessionId(w, r, sessionID, "worker")
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.ValidateCurrentFactoryWorkstationPromptTemplateBySessionId(w, r, sessionID, "worker")
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.InvokeFactorySessionBySessionId(w, r, sessionID)
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactorySessionResults(w, r, sessionID, factoryapi.GetFactorySessionResultsParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.ListFactorySessionDispatches(w, r, sessionID, factoryapi.ListFactorySessionDispatchesParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetEventsBySessionId(w, r, sessionID, factoryapi.GetEventsBySessionIdParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactoryResponseEventsBySessionId(w, r, sessionID, factoryapi.GetFactoryResponseEventsBySessionIdParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.GetFactorySessionSyncPreflightBySessionId(w, r, sessionID, factoryapi.GetFactorySessionSyncPreflightBySessionIdParams{})
		},
	}
	for _, call := range tests {
		call(httptest.NewRecorder(), request)
	}
	unsupported := httptest.NewRequest(http.MethodPost, "/", nil)
	unsupported.Header.Set("Content-Type", "text/plain")
	handler.OpenFactorySession(httptest.NewRecorder(), unsupported)
}

func TestGetEventsBySessionIdJSONRecoveryProbeUsesSessionEventAPI(t *testing.T) {
	t.Parallel()

	var probedSessionID string
	var probedCursor *factorydefinitions.FactoryEventReconnectCursor
	handler := NewHandler(Dependencies{
		SessionEvents: sessionEventAPIFake{
			probe: func(_ context.Context, sessionID string, cursor *factorydefinitions.FactoryEventReconnectCursor) error {
				probedSessionID, probedCursor = sessionID, cursor
				return nil
			},
		},
	}, zap.NewNop())
	afterSequence := factoryapi.AfterSequence(7)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha/events", nil)
	request.Header.Set("Accept", "application/json")

	handler.GetEventsBySessionId(
		recorder,
		request,
		factoryapi.SessionID("session-alpha"),
		factoryapi.GetEventsBySessionIdParams{AfterSequence: &afterSequence},
	)

	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response = %d %q, want JSON recovery response", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	if probedSessionID != "session-alpha" || probedCursor == nil || probedCursor.AfterSequence == nil || *probedCursor.AfterSequence != 7 {
		t.Fatalf("probe = session %q cursor %#v, want session-alpha after_sequence 7", probedSessionID, probedCursor)
	}
}

func TestGetEventsBySessionIdProjectsProviderContinuation(t *testing.T) {
	continuation := (&providers.SessionMetadata{
		Provider: string(providers.IDCodex),
		Kind:     providers.SessionIDKind,
		ID:       "session-from-event",
	}).ContinuationRef()
	payload, err := json.Marshal(workers.InferenceResponseEventPayload{
		Outcome:      workers.InferenceOutcomeSucceeded,
		Continuation: continuation,
	})
	if err != nil {
		t.Fatalf("marshal inference response payload: %v", err)
	}

	closed := make(chan factorydefinitions.FactoryEvent)
	close(closed)
	handler := NewHandler(Dependencies{
		SessionEvents: sessionEventAPIFake{
			subscribe: func(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
				return &factorydefinitions.FactoryEventStream{
					History: []factorydefinitions.FactoryEvent{{
						Id:            "factory-event/inference-response/1",
						Payload:       payload,
						SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
						Type:          factorydefinitions.FactoryEventTypeInferenceResponse,
					}},
					Events: closed,
				}, nil
			},
		},
	}, zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha/events", nil)

	handler.GetEventsBySessionId(
		recorder,
		request,
		factoryapi.SessionID("session-alpha"),
		factoryapi.GetEventsBySessionIdParams{},
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want 200", recorder.Code)
	}
	line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(recorder.Body.String()), "data:"))
	var public factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(line), &public); err != nil {
		t.Fatalf("decode public event: %v; body=%q", err, recorder.Body.String())
	}
	decoded, err := public.Payload.AsInferenceResponseEventPayload()
	if err != nil {
		t.Fatalf("decode public inference response payload: %v", err)
	}
	if decoded.ProviderSession == nil || decoded.ProviderSession.Id == nil || *decoded.ProviderSession.Id != "session-from-event" {
		t.Fatalf("provider session = %#v, want projected session identity", decoded.ProviderSession)
	}
	if strings.Contains(line, `"continuation"`) {
		t.Fatalf("public event retained opaque continuation: %s", line)
	}
}
