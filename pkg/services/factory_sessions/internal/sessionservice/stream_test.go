package service_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	apiserver "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type streamGatewayHost struct {
	openTestHost
	streams    *responsestream.StreamSet
	checkpoint factoryruntime.JavaScriptCheckpointStore
}

func responseFragment(dispatchID, payload string) workers.ProgressFragment {
	return workers.ProgressFragment{
		DispatchID: dispatchID,
		Kind:       workers.ResponseFragmentKind,
		Type:       "TEXT_DELTA",
		Payload:    payload,
	}
}

func (h *streamGatewayHost) ResponseStreams(_ *livesession.LiveSession) *responsestream.StreamSet {
	if h.streams == nil {
		h.streams = responsestream.NewStreamSetWithFactory(newServiceTestResponseStream, serviceTestClock)
	}
	return h.streams
}

func (h *streamGatewayHost) CloseResponseStreams(_ *livesession.LiveSession) {
	if h.streams != nil {
		h.streams.Close()
	}
}

func (h *streamGatewayHost) CloseResponseStreamDispatch(_ *livesession.LiveSession, dispatchID string) bool {
	if h.streams == nil {
		return false
	}
	return h.streams.CloseDispatch(dispatchID)
}

func (h *streamGatewayHost) JavaScriptCheckpointStore(_ *livesession.LiveSession) factoryruntime.JavaScriptCheckpointStore {
	if h.checkpoint == nil {
		h.checkpoint = streamCheckpointStore{}
	}
	return h.checkpoint
}

type streamCheckpointStore struct{}

func (streamCheckpointStore) Put(interfaces.JavaScriptCheckpointRecord) {}
func (streamCheckpointStore) List() []interfaces.JavaScriptCheckpointRecord {
	return nil
}
func (streamCheckpointStore) Get(string) (interfaces.JavaScriptCheckpointRecord, bool) {
	return interfaces.JavaScriptCheckpointRecord{}, false
}

var _ factoryruntime.JavaScriptCheckpointStore = streamCheckpointStore{}

func TestService_SubscribeSessionResponseStream_DelegatesThroughStreamManager(t *testing.T) {
	t.Parallel()

	host := &streamGatewayHost{
		openTestHost: openTestHost{
			requireSession: &livesession.LiveSession{ID: "sess-stream"},
		},
	}
	gateway := newServiceTestGateway(host)
	publisher := gateway.InferenceProgressPublisherFactory(nil)("sess-stream")
	publisher(responseFragment("dispatch-1", "alpha"))

	subscription, err := gateway.SubscribeSessionResponseStream("sess-stream", "dispatch-1", 0)
	if err != nil {
		t.Fatalf("SubscribeSessionResponseStream: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(initial.Events) != 1 || initial.Events[0].Payload != "alpha" {
		t.Fatalf("events = %#v, want one alpha fragment", initial.Events)
	}
}

func TestService_CloseSessionResponseStreams_ReleasesDispatchStreams(t *testing.T) {
	t.Parallel()

	host := &streamGatewayHost{
		openTestHost: openTestHost{
			requireSession: &livesession.LiveSession{ID: "sess-close"},
		},
	}
	gateway := newServiceTestGateway(host)
	publisher := gateway.InferenceProgressPublisherFactory(nil)("sess-close")
	publisher(responseFragment("dispatch-1", "alpha"))

	gateway.CloseSessionResponseStreams(host.requireSession)

	_, err := gateway.SubscribeSessionResponseStream("sess-close", "dispatch-1", 0)
	if err != responsestream.ErrSubscriptionClosed {
		t.Fatalf("Subscribe after close = %v, want %v", err, responsestream.ErrSubscriptionClosed)
	}
}

func TestService_SessionResponseStreamDispatchIDs_ReturnsActiveDispatches(t *testing.T) {
	t.Parallel()

	host := &streamGatewayHost{
		openTestHost: openTestHost{
			requireSession: &livesession.LiveSession{ID: "sess-dispatch-ids"},
		},
	}
	gateway := newServiceTestGateway(host)
	publisher := gateway.InferenceProgressPublisherFactory(nil)("sess-dispatch-ids")
	publisher(responseFragment("dispatch-1", "alpha"))

	dispatchIDs, err := gateway.SessionResponseStreamDispatchIDs("sess-dispatch-ids")
	if err != nil {
		t.Fatalf("SessionResponseStreamDispatchIDs: %v", err)
	}
	if len(dispatchIDs) != 1 || dispatchIDs[0] != "dispatch-1" {
		t.Fatalf("dispatch IDs = %#v, want [dispatch-1]", dispatchIDs)
	}
}

func TestService_JavaScriptCheckpointStore_ReturnsSessionOwnedStore(t *testing.T) {
	t.Parallel()

	host := &streamGatewayHost{
		openTestHost: openTestHost{
			requireSession: &livesession.LiveSession{ID: "sess-checkpoint"},
		},
	}
	gateway := newServiceTestGateway(host)

	store := gateway.JavaScriptCheckpointStore(host.requireSession)
	if store == nil {
		t.Fatal("JavaScriptCheckpointStore = nil, want session-owned store")
	}
}

func TestService_DispatchCompletionObserverFactory_ClosesDispatchStream(t *testing.T) {
	t.Parallel()

	host := &streamGatewayHost{
		openTestHost: openTestHost{
			requireSession: &livesession.LiveSession{ID: "sess-dispatch-close"},
			sessions: map[string]*livesession.LiveSession{
				"sess-dispatch-close": {ID: "sess-dispatch-close"},
			},
		},
	}
	gateway := newServiceTestGateway(host)
	publisher := gateway.InferenceProgressPublisherFactory(nil)("sess-dispatch-close")
	publisher(responseFragment("dispatch-1", "alpha"))

	observer := gateway.DispatchCompletionObserverFactory()("sess-dispatch-close")
	observer("dispatch-1")

	subscription, err := gateway.SubscribeSessionResponseStream("sess-dispatch-close", "dispatch-1", 0)
	if err != nil {
		t.Fatalf("SubscribeSessionResponseStream: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(initial.Events) != 1 {
		t.Fatalf("events = %d, want buffered dispatch stream to remain readable", len(initial.Events))
	}
}

func TestService_LegacyEventSSEHTTP_UsesResolvedCanonicalIDForExactAndDefault(t *testing.T) {
	t.Parallel()

	const (
		exactSessionID   = "session-http-exact-001"
		defaultSessionID = factorysessions.DefaultSessionID
		defaultCanonical = "3c1d4c6b-0d6a-4e8f-b0c0-9e5a2bb1d8aa"
	)

	event, err := interfaces.NewFactoryEvent(factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeWorkRequest,
		Id:            "session-http-legacy-event-001",
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, 8, 31, 17, 0, 0, 0, time.UTC),
			Sequence:  0,
		},
	})
	if err != nil {
		t.Fatalf("convert HTTP event fixture: %v", err)
	}

	events := make(chan interfaces.FactoryEvent)
	close(events)
	sessions := map[string]*livesession.LiveSession{
		exactSessionID: {
			ID:                      exactSessionID,
			RuntimeFactorySessionID: exactSessionID,
			RuntimeEventSessionID:   exactSessionID,
		},
		defaultSessionID: {
			ID:                      defaultSessionID,
			RuntimeFactorySessionID: defaultCanonical,
			RuntimeEventSessionID:   defaultSessionID,
		},
	}
	factory := &gatewayLifecycleFactory{
		subscribeFactoryFn: func(
			_ context.Context,
			_ *interfaces.FactoryEventReconnectCursor,
			scope interfaces.FactoryEventReconnectScope,
		) (*interfaces.FactoryEventStream, error) {
			if _, ok := sessions[scope.SessionID]; !ok {
				t.Fatalf("event scope session = %q, want an independently resolved live session", scope.SessionID)
			}
			return &interfaces.FactoryEventStream{
				// The legacy runtime intentionally omits identity metadata. The
				// Factory Sessions gateway must supply only the resolved identity.
				History: []interfaces.FactoryEvent{event},
				Events:  events,
			}, nil
		},
	}
	host := &httpEventGatewayHost{
		lifecycleGatewayHost: lifecycleGatewayHost{
			openTestHost: openTestHost{},
			factory:      factory,
		},
		sessions: sessions,
	}
	server := newSessionServiceEventHTTPServer(t, newServiceTestGateway(host))

	for _, test := range []struct {
		name      string
		selector  string
		canonical string
	}{
		{name: "exact selector", selector: exactSessionID, canonical: exactSessionID},
		{name: "default selector", selector: defaultSessionID, canonical: defaultCanonical},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := getSessionServiceEventStream(t, server.URL, test.selector)
			defer response.Body.Close()

			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", response.StatusCode, readSessionServiceHTTPBody(t, response))
			}
			if got := response.Header.Get(recordingshttp.SessionEventStreamFactorySessionHeader); got != test.canonical {
				t.Fatalf("resolved Factory Session header = %q, want %q", got, test.canonical)
			}
			if got := response.Header.Get(recordingshttp.SessionEventStreamRetainedCountHeader); got != "1" {
				t.Fatalf("retained event count header = %q, want 1", got)
			}

			streamed := readSessionServiceSSEEvent(t, response.Body)
			if streamed.Id != event.Id || streamed.Type != factoryapi.FactoryEventTypeWorkRequest {
				t.Fatalf("streamed event = %#v, want unchanged event %q/%q", streamed, event.Id, factoryapi.FactoryEventTypeWorkRequest)
			}
		})
	}
}

func TestService_LegacyEventSSEHTTP_PreservesTypedFailuresWithoutIdentity(t *testing.T) {
	t.Parallel()

	const unknownSessionID = "session-http-unknown-001"
	subscriptionFailure := errors.New("legacy reconnect cursor is stale")
	for _, test := range []struct {
		name       string
		selector   string
		host       *lifecycleGatewayHost
		wantStatus int
		wantCode   factoryapi.ErrorResponseCode
		wantBody   string
	}{
		{
			name:     "unknown session",
			selector: unknownSessionID,
			host: &lifecycleGatewayHost{
				openTestHost:      openTestHost{requireSessionE: factorysessions.ErrSessionNotFound},
				sessionFactoryErr: factorysessions.ErrSessionNotFound,
			},
			wantStatus: http.StatusNotFound,
			wantCode:   factoryapi.ErrorResponseCodeNOTFOUND,
			wantBody:   "factory session not found",
		},
		{
			name:     "subscription failure",
			selector: "session-http-failure-001",
			host: &lifecycleGatewayHost{
				openTestHost: openTestHost{
					requireSession: &livesession.LiveSession{ID: "session-http-failure-001"},
				},
				factory: &gatewayLifecycleFactory{
					subscribeFactoryFn: func(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
						return nil, subscriptionFailure
					},
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   factoryapi.ErrorResponseCodeINTERNALERROR,
			wantBody:   "failed to subscribe to factory events",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := newSessionServiceEventHTTPServer(t, newServiceTestGateway(test.host))
			response := getSessionServiceEventStream(t, server.URL, test.selector)
			defer response.Body.Close()

			if response.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.StatusCode, test.wantStatus, readSessionServiceHTTPBody(t, response))
			}
			if _, present := response.Header[http.CanonicalHeaderKey(recordingshttp.SessionEventStreamFactorySessionHeader)]; present {
				t.Fatalf("resolved Factory Session header = %q on failed response, want omitted", response.Header.Get(recordingshttp.SessionEventStreamFactorySessionHeader))
			}

			var errorResponse factoryapi.ErrorResponse
			if err := json.NewDecoder(response.Body).Decode(&errorResponse); err != nil {
				t.Fatalf("decode typed error response: %v", err)
			}
			if errorResponse.Code != test.wantCode || !strings.Contains(errorResponse.Message, test.wantBody) {
				t.Fatalf("error response = %#v, want code %q and message containing %q", errorResponse, test.wantCode, test.wantBody)
			}
		})
	}
}

type httpEventGatewayHost struct {
	lifecycleGatewayHost
	sessions map[string]*livesession.LiveSession
}

func (h *httpEventGatewayHost) RequireSession(sessionID string) (*livesession.LiveSession, error) {
	session, ok := h.sessions[sessionID]
	if !ok {
		return nil, factorysessions.ErrSessionNotFound
	}
	return session, nil
}

func newSessionServiceEventHTTPServer(t *testing.T, gateway *factorysessionservice.Service) *httptest.Server {
	t.Helper()
	logger := zap.NewNop()
	server := apiserver.NewServerWithRecordings(
		recordingshttp.NewLegacyAdapterWithLive(nil, nil, gateway),
		factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{}, logger),
		nil, nil, nil, nil, logger,
	)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return httpServer
}

func getSessionServiceEventStream(t *testing.T, serverURL, sessionID string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, serverURL+"/factory-sessions/"+sessionID+"/events", nil)
	if err != nil {
		t.Fatalf("new Factory Event request: %v", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("GET Factory Event stream: %v", err)
	}
	return response
}

func readSessionServiceSSEEvent(t *testing.T, body io.Reader) factoryapi.FactoryEvent {
	t.Helper()
	reader := bufio.NewReader(body)
	var data string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE frame: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
	}
	if data == "" {
		t.Fatal("SSE frame did not contain a data payload")
	}

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		t.Fatalf("decode SSE event: %v", err)
	}
	return event
}

func readSessionServiceHTTPBody(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read HTTP response body: %v", err)
	}
	return string(body)
}
