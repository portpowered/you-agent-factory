package service_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

type streamGatewayHost struct {
	openTestHost
	streams *factorysessions.SessionResponseStreamSet
}

func (h *streamGatewayHost) ResponseStreams(_ *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if h.streams == nil {
		h.streams = factorysessions.NewSessionResponseStreamSetWithFactory(factorysessions.NewSessionResponseStream)
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

func TestService_SubscribeSessionResponseStream_DelegatesThroughStreamManager(t *testing.T) {
	t.Parallel()

	host := &streamGatewayHost{
		openTestHost: openTestHost{
			requireSession: &factorysessions.LiveSession{ID: "sess-stream"},
		},
	}
	gateway := factorysessionservice.New(host)
	publisher := gateway.InferenceProgressPublisherFactory(nil)("sess-stream")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))

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
	gateway := factorysessionservice.New(host)
	publisher := gateway.InferenceProgressPublisherFactory(nil)("sess-close")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))

	gateway.CloseSessionResponseStreams(host.requireSession)

	_, err := gateway.SubscribeSessionResponseStream("sess-close", "dispatch-1", 0)
	if err != responsestream.ErrSubscriptionClosed {
		t.Fatalf("Subscribe after close = %v, want %v", err, responsestream.ErrSubscriptionClosed)
	}
}
