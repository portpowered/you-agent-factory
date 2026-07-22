package service_test

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type streamGatewayHost struct {
	openTestHost
	streams    *factorysessions.SessionResponseStreamSet
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

func (h *streamGatewayHost) ResponseStreams(_ *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if h.streams == nil {
		h.streams = factorysessions.NewSessionResponseStreamSetWithFactory(newServiceTestResponseStream, serviceTestClock)
	}
	return h.streams
}

func (h *streamGatewayHost) CloseResponseStreams(_ *factorysessions.LiveSession) {
	if h.streams != nil {
		h.streams.Close()
	}
}

func (h *streamGatewayHost) CloseResponseStreamDispatch(_ *factorysessions.LiveSession, dispatchID string) bool {
	if h.streams == nil {
		return false
	}
	return h.streams.CloseDispatch(dispatchID)
}

func (h *streamGatewayHost) JavaScriptCheckpointStore(_ *factorysessions.LiveSession) factoryruntime.JavaScriptCheckpointStore {
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
			requireSession: &factorysessions.LiveSession{ID: "sess-stream"},
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
			requireSession: &factorysessions.LiveSession{ID: "sess-close"},
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
			requireSession: &factorysessions.LiveSession{ID: "sess-dispatch-ids"},
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
			requireSession: &factorysessions.LiveSession{ID: "sess-checkpoint"},
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
			requireSession: &factorysessions.LiveSession{ID: "sess-dispatch-close"},
			sessions: map[string]*factorysessions.LiveSession{
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
