package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/wire"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
)

func newResponseServiceTestGateway(t *testing.T, host *openTestHost) *factorysessionservice.Service {
	t.Helper()
	responseService, err := responsestreamwire.NewService(func() string { return "response-event-test-id" })
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	registry, err := responseService.NewStreamRegistry(serviceTestClock)
	if err != nil {
		t.Fatalf("construct response-stream registry: %v", err)
	}
	return factorysessionservice.NewWithResponseService(host, host, host, registry, nil, nil, responseService)
}

func TestService_GetFactorySessionSyncPreflight_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	session := &livesession.LiveSession{ID: "sess-preflight"}
	host := &openTestHost{requireSession: session}
	gateway := newServiceTestGateway(host)

	response, err := gateway.GetFactorySessionSyncPreflight(context.Background(), "sess-preflight", nil, nil)
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight: %v", err)
	}
	if response.Reason != factorysessions.SyncPreflightReasonOK {
		t.Fatalf("reason = %q, want ok", response.Reason)
	}
	if response.BackendScopeID == nil || *response.BackendScopeID != "runtime-test" {
		t.Fatalf("backend scope ID = %#v, want runtime-test", response.BackendScopeID)
	}
}

func TestService_SubscribeFactoryResponseEvents_UsesExactSessionWithoutDefaultFallback(t *testing.T) {
	t.Parallel()

	gateway := newServiceTestGateway(&openTestHost{
		sessions: map[string]*livesession.LiveSession{
			factorysessions.DefaultSessionID: {
				ID:             factorysessions.DefaultSessionID,
				ResponseEvents: responseeventstore.NewSessionResponseEventStore(factorysessions.DefaultSessionID, serviceTestClock, func() string { return "response-event-test-id" }),
			},
		},
	})

	_, err := gateway.SubscribeFactoryResponseEvents(
		context.Background(),
		factorysessions.ResponseEventSubscriptionRequest{SessionID: "session-missing"},
	)
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("SubscribeFactoryResponseEvents error = %v, want ErrSessionNotFound", err)
	}
}

func TestService_SubscribeFactoryResponseEvents_RequiresInjectedResponseOwner(t *testing.T) {
	t.Parallel()

	sessionID := "session-without-response-owner"
	gateway := newServiceTestGateway(&openTestHost{
		sessions: map[string]*livesession.LiveSession{
			sessionID: {
				ID: sessionID,
				ResponseEvents: responseeventstore.NewSessionResponseEventStore(
					sessionID,
					serviceTestClock,
					func() string { return "response-event-test-id" },
				),
			},
		},
	})

	_, err := gateway.SubscribeFactoryResponseEvents(
		context.Background(),
		factorysessions.ResponseEventSubscriptionRequest{SessionID: sessionID},
	)
	if !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("SubscribeFactoryResponseEvents error = %v, want ErrRuntimeNotAvailable", err)
	}
}

func TestService_SubscribeFactoryResponseEvents_DelegatesReconnectPolicyToPrivateService(t *testing.T) {
	t.Parallel()
	responseService, err := responsestreamwire.NewService(func() string { return "response-event-outer" })
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store, err := responseService.NewEventStore("session-1", serviceTestClock)
	if err != nil {
		t.Fatalf("construct event store: %v", err)
	}
	first, err := responseService.Publish(store, responseevents.FactoryResponseEvent{
		RunID: "run-1", Kind: responseevents.KindMessage, Phase: responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider: "test", NativeEventType: "delta", Delivery: responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationDelta, Fidelity: responseevents.FidelityLossless,
		},
		Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"first"}`),
	})
	if err != nil {
		t.Fatalf("publish first: %v", err)
	}
	secondInput := first
	secondInput.Payload = json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"second"}`)
	second, err := responseService.Publish(store, secondInput)
	if err != nil {
		t.Fatalf("publish second: %v", err)
	}
	host := &openTestHost{sessions: map[string]*livesession.LiveSession{
		"session-1": {ID: "session-1", ResponseEvents: store},
	}}
	gateway := newResponseServiceTestGateway(t, host)
	cursor, err := gateway.SubscribeFactoryResponseEvents(context.Background(), factorysessions.ResponseEventSubscriptionRequest{
		SessionID: "session-1", AfterSequence: first.Sequence,
	})
	if err != nil {
		t.Fatalf("SubscribeFactoryResponseEvents: %v", err)
	}
	defer cursor.Detach()
	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != second.Sequence {
		t.Fatalf("events = %#v, want only sequence %d", events, second.Sequence)
	}
	_, err = gateway.SubscribeFactoryResponseEvents(context.Background(), factorysessions.ResponseEventSubscriptionRequest{
		SessionID: "session-1", AfterSequence: -1,
	})
	if !errors.Is(err, factorysessions.ErrInvalidResponseEventCursor) {
		t.Fatalf("invalid cursor error = %v, want ErrInvalidResponseEventCursor", err)
	}
}

func TestService_SubscribeFactoryResponseEvents_PreservesFilterSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()
	responseService, err := responsestreamwire.NewService(func() string { return "response-event-filter" })
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store, err := responseService.NewEventStore("session-filter", serviceTestClock)
	if err != nil {
		t.Fatalf("construct event store: %v", err)
	}

	message, err := responseService.Publish(store, responseevents.FactoryResponseEvent{
		DispatchID: "dispatch-1",
		RunID:      "run-1",
		Kind:       responseevents.KindMessage,
		Phase:      responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider: "test", NativeEventType: "delta", Delivery: responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationDelta, Fidelity: responseevents.FidelityLossless,
		},
		Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"keep"}`),
	})
	if err != nil {
		t.Fatalf("publish message: %v", err)
	}
	_, err = responseService.Publish(store, responseevents.FactoryResponseEvent{
		DispatchID: "dispatch-1",
		RunID:      "run-1",
		Kind:       responseevents.KindReasoning,
		Phase:      responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider: "test", NativeEventType: "delta", Delivery: responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationDelta, Fidelity: responseevents.FidelityLossless,
		},
		Payload: json.RawMessage(`{"summaryDelta":"skip"}`),
	})
	if err != nil {
		t.Fatalf("publish reasoning: %v", err)
	}

	host := &openTestHost{sessions: map[string]*livesession.LiveSession{
		"session-filter": {ID: "session-filter", ResponseEvents: store},
	}}
	gateway := newResponseServiceTestGateway(t, host)

	cursor, err := gateway.SubscribeFactoryResponseEvents(context.Background(), factorysessions.ResponseEventSubscriptionRequest{
		SessionID: "session-filter",
		Kinds:     []factorysessions.ResponseEventKind{factorysessions.ResponseEventKindMessage},
	})
	if err != nil {
		t.Fatalf("SubscribeFactoryResponseEvents filter success: %v", err)
	}
	defer cursor.Detach()
	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != message.Sequence || events[0].Kind != responseevents.KindMessage {
		t.Fatalf("filtered events = %#v, want only message sequence %d", events, message.Sequence)
	}

	_, err = gateway.SubscribeFactoryResponseEvents(context.Background(), factorysessions.ResponseEventSubscriptionRequest{
		SessionID: "session-filter",
		Kinds:     []factorysessions.ResponseEventKind{factorysessions.ResponseEventKind("not-a-supported-kind")},
	})
	if !errors.Is(err, factorysessions.ErrInvalidResponseEventFilter) {
		t.Fatalf("invalid filter error = %v, want ErrInvalidResponseEventFilter", err)
	}
	if errors.Is(err, factorysessions.ErrInvalidResponseEventCursor) {
		t.Fatal("invalid filter must stay distinct from invalid cursor")
	}
}

func TestService_GetFactorySessionSyncPreflight_RejectsDurableSessionID(t *testing.T) {
	t.Parallel()

	gateway := newServiceTestGateway(&openTestHost{})

	response, err := gateway.GetFactorySessionSyncPreflight(context.Background(), "dur-sess-js-run-n-001", nil, nil)
	if err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight: %v", err)
	}
	if response.Reason != factorysessions.SyncPreflightReasonSessionNotFound {
		t.Fatalf("reason = %q, want session_not_found", response.Reason)
	}
}

func TestService_GetFactorySessionResult_RejectsNonJavaScriptFactory(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		requireSession: &livesession.LiveSession{ID: "sess-petri"},
	}
	gateway := newServiceTestGateway(host)

	_, err := gateway.GetFactorySessionResult(context.Background(), "sess-petri")
	if err == nil {
		t.Fatal("GetFactorySessionResult = nil, want result unavailable")
	}
}

func TestService_GetFactorySessionPartialResult_RejectsNonJavaScriptFactory(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		requireSession: &livesession.LiveSession{ID: "sess-petri"},
	}
	gateway := newServiceTestGateway(host)

	_, err := gateway.GetFactorySessionPartialResult(context.Background(), "sess-petri")
	if err == nil {
		t.Fatal("GetFactorySessionPartialResult = nil, want result unavailable")
	}
}

func TestService_ListFactorySessions_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		sessionIDs: []string{"~default"},
		sessions: map[string]*livesession.LiveSession{
			"~default": {ID: "~default", IsDefault: true},
		},
	}
	gateway := newServiceTestGateway(host)

	response, err := gateway.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(response) != 1 || response[0].Context.Session.ID != "~default" {
		t.Fatalf("sessions = %#v, want one default session", response)
	}
}

func TestService_GetFactorySession_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		requireSession: &livesession.LiveSession{ID: "sess-get"},
	}
	gateway := newServiceTestGateway(host)

	session, err := gateway.GetFactorySession(context.Background(), "sess-get")
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if session.Context.Session.ID != "sess-get" {
		t.Fatalf("session id = %q, want sess-get", session.Context.Session.ID)
	}
}

func TestService_ListFactorySessions_RequiresGateway(t *testing.T) {
	t.Parallel()

	var gateway *factorysessionservice.Service
	_, err := gateway.ListFactorySessions(context.Background())
	if err == nil {
		t.Fatal("ListFactorySessions = nil, want gateway required")
	}
}

func TestService_OpenFactorySession_MapsInitNewFactoryHint(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		discoverErr: sessionvalidation.New(
			factorysessions.ValidationReasonNotRunnable,
			"folderPath",
			errors.New("no runnable targets"),
		),
	}
	gateway := newServiceTestGateway(host)
	validateOnly := true

	response, err := gateway.OpenFactorySession(context.Background(), factorysessions.OpenRequest{
		FolderPath:   t.TempDir(),
		ValidateOnly: validateOnly,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if response == nil || !response.InitsNewFactory {
		t.Fatalf("initsNewFactory = %#v, want true", response.InitsNewFactory)
	}
}
