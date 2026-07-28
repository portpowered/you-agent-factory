package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func newDurableResponseEventsService(t *testing.T) *JavaScriptRuntimeService {
	t.Helper()

	var next atomic.Uint64
	streams, err := responsestreamwire.NewService(func() string {
		return fmt.Sprintf("response-event-%d", next.Add(1))
	}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{})
	service.responseStreams = streams
	service.generateResponseEventID = func() string {
		return fmt.Sprintf("response-event-%d", next.Add(1))
	}
	service.liveChildInvocation = func(publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
		return &progressCapturingChildExecutor{publisher: publisher}, nil
	}
	return service
}

type progressCapturingChildExecutor struct {
	publisher workers.ProgressPublisher
}

func (e *progressCapturingChildExecutor) Execute(context.Context, workers.InvocationInput) (workers.InvocationResult, error) {
	return workers.InvocationResult{}, nil
}

func seedResponseEventSession(t *testing.T, service *JavaScriptRuntimeService, sessionID string) *runtimeSessionState {
	t.Helper()

	state := &runtimeSessionState{
		session: SessionReadResult{SessionID: sessionID},
	}
	service.mu.Lock()
	service.sessions[sessionID] = state
	service.mu.Unlock()
	return state
}

func validMessageDeltaDraft(dispatchID string) responseevents.Draft {
	payload, _ := json.Marshal(responseevents.MessageDeltaPayload{
		ContentBlockIndex: 0,
		ContentBlockKind:  responseevents.ContentBlockText,
		TextDelta:         "hello",
	})
	return responseevents.Draft{
		RunID:      dispatchID,
		DispatchID: dispatchID,
		ItemID:     "message-1",
		Kind:       responseevents.KindMessage,
		Phase:      responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider:        "cursor",
			NativeEventType: "content_block_delta",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationDelta,
			Fidelity:        responseevents.FidelityLossless,
		},
		Payload: payload,
	}
}

func TestJavaScriptRuntimeService_SubscribeResponseEvents_PublishesAndStreamsChildProgress(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	sessionID := "dur-sess-response-events"
	state := seedResponseEventSession(t, service, sessionID)

	publisher := service.sessionProgressPublisher(sessionID, state)
	publisher(workers.ProgressFragment{
		DispatchID:     "dispatch-1",
		CanonicalDraft: validMessageDeltaDraft("dispatch-1"),
	})

	cursor, err := service.SubscribeResponseEvents(context.Background(), sessionID, factorysessions.ResponseEventSubscriptionRequest{
		SessionID: sessionID,
		Kinds:     []factorysessions.ResponseEventKind{factorysessions.ResponseEventKindMessage},
	})
	if err != nil {
		t.Fatalf("SubscribeResponseEvents: %v", err)
	}
	defer cursor.Detach()

	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("cursor.Next: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one MESSAGE delta", events)
	}
	if events[0].Kind != responseevents.KindMessage || events[0].DispatchID != "dispatch-1" {
		t.Fatalf("event = %#v, want MESSAGE dispatch-1", events[0])
	}
}

func TestJavaScriptRuntimeService_SubscribeResponseEvents_RejectsMissingStore(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	seedResponseEventSession(t, service, "dur-sess-empty")

	_, err := service.SubscribeResponseEvents(context.Background(), "dur-sess-empty", factorysessions.ResponseEventSubscriptionRequest{
		SessionID: "dur-sess-empty",
	})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("SubscribeResponseEvents error = %v, want ErrSessionNotFound", err)
	}
}

func TestJavaScriptRuntimeService_SubscribeResponseEvents_RejectsInvalidCursor(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	sessionID := "dur-sess-invalid-cursor"
	state := seedResponseEventSession(t, service, sessionID)
	publisher := service.sessionProgressPublisher(sessionID, state)
	publisher(workers.ProgressFragment{CanonicalDraft: validMessageDeltaDraft("dispatch-1")})

	_, err := service.SubscribeResponseEvents(context.Background(), sessionID, factorysessions.ResponseEventSubscriptionRequest{
		SessionID:     sessionID,
		AfterSequence: -1,
	})
	if !errors.Is(err, factorysessions.ErrInvalidResponseEventCursor) {
		t.Fatalf("SubscribeResponseEvents error = %v, want ErrInvalidResponseEventCursor", err)
	}
}

func TestJavaScriptRuntimeService_LiveChildExecutor_UsesProgressPublisher(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	sessionID := "dur-sess-child"
	state := seedResponseEventSession(t, service, sessionID)

	executor := service.liveChildExecutor(sessionID, state)
	child, ok := executor.(*progressCapturingChildExecutor)
	if !ok || child.publisher == nil {
		t.Fatalf("executor = %#v, want progress-aware child executor", executor)
	}
}

func TestJavaScriptRuntimeService_LiveChildExecutor_UsesConductorBeforeSessionRegistration(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	executor := service.liveChildExecutor("dur-sess-starting", nil)
	child, ok := executor.(*progressCapturingChildExecutor)
	if !ok {
		t.Fatalf("executor = %#v, want live child executor", executor)
	}
	if child.publisher != nil {
		t.Fatalf("publisher = %#v, want nil before durable session registration", child.publisher)
	}
}

func TestJavaScriptRuntimeService_SessionProgressPublisher_IgnoresNonDraftFragments(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	sessionID := "dur-sess-ignore"
	state := seedResponseEventSession(t, service, sessionID)

	publisher := service.sessionProgressPublisher(sessionID, state)
	publisher(workers.ProgressFragment{DispatchID: "dispatch-1", Kind: workers.ProgressFragmentKind})
	publisher(workers.ProgressFragment{CanonicalDraft: "not-a-draft"})

	if state.responseEvents == nil {
		t.Fatal("response event store was not initialized")
	}
	if accounting := state.responseEvents.RetentionAccounting(); accounting.EventCount != 0 {
		t.Fatalf("retained events = %#v, want none for ignored fragments", accounting)
	}
}

func TestJavaScriptRuntimeService_SubscribeResponseEvents_RequiresRuntime(t *testing.T) {
	t.Parallel()

	var service *JavaScriptRuntimeService
	_, err := service.SubscribeResponseEvents(context.Background(), "dur-sess-1", factorysessions.ResponseEventSubscriptionRequest{})
	if !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("SubscribeResponseEvents error = %v, want ErrRuntimeNotAvailable", err)
	}
}

func TestEnsureSessionResponseEvents_RequiresIDGenerator(t *testing.T) {
	t.Parallel()

	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{})
	state := &runtimeSessionState{session: SessionReadResult{SessionID: "dur-sess-missing-id"}}
	if err := service.ensureSessionResponseEvents("dur-sess-missing-id", state); err == nil {
		t.Fatal("ensureSessionResponseEvents succeeded without ID generator")
	}
}
