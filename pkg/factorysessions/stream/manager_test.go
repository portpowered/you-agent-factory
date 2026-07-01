package stream_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/factorysessions/stream"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

type streamTestHost struct {
	session       *factorysessions.LiveSession
	streams       *factorysessions.SessionResponseStreamSet
	published     int
	compactions   int
	degraded      int
	checkpoint    *factorysessions.JavaScriptCheckpointStore
}

func (h *streamTestHost) RequireSession(_ string) (*factorysessions.LiveSession, error) {
	if h.session == nil {
		return nil, errMissingSession
	}
	return h.session, nil
}

func (h *streamTestHost) GetLiveSession(_ string) *factorysessions.LiveSession {
	return h.session
}

func (h *streamTestHost) ResponseStreams(_ *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if h.streams == nil {
		h.streams = factorysessions.NewSessionResponseStreamSetWithFactory(factorysessions.NewSessionResponseStream)
	}
	return h.streams
}

func (h *streamTestHost) NewResponseStream() *factorysessions.SessionResponseStream {
	return factorysessions.NewSessionResponseStream()
}

func (h *streamTestHost) CloseResponseStreams(_ *factorysessions.LiveSession) {
	if h.streams != nil {
		h.streams.Close()
	}
}

func (h *streamTestHost) CloseResponseStreamDispatch(_ *factorysessions.LiveSession, dispatchID string) bool {
	if h.streams == nil {
		return false
	}
	return h.streams.CloseDispatch(dispatchID)
}

func (h *streamTestHost) JavaScriptCheckpointStore(_ *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	if h.checkpoint == nil {
		h.checkpoint = factorysessions.NewJavaScriptCheckpointStore()
	}
	return h.checkpoint
}

func (h *streamTestHost) ObserveResponseStreamPublished(_ *factorysessions.LiveSession, _ string, _ responsestream.Event) {
	h.published++
}

func (h *streamTestHost) ObserveResponseStreamCompaction(
	_ *factorysessions.LiveSession,
	_ string,
	_ string,
	_ responsestream.CompactionSummary,
) {
	h.compactions++
}

func (h *streamTestHost) ObserveResponseStreamDegraded(
	_ *factorysessions.LiveSession,
	_ string,
	_ string,
	_ string,
	_ *zap.Logger,
	_ error,
) {
	h.degraded++
}

var errMissingSession = errors.New("missing session")

func TestManager_SubscribeAndPublishInferenceProgress(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-stream"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-stream")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))
	publisher(workerprovider.ProgressFragment("dispatch-1", nil, "beta"))

	dispatchIDs, err := manager.DispatchIDs("sess-stream")
	if err != nil {
		t.Fatalf("DispatchIDs: %v", err)
	}
	if len(dispatchIDs) != 1 || dispatchIDs[0] != "dispatch-1" {
		t.Fatalf("dispatch IDs = %#v, want [dispatch-1]", dispatchIDs)
	}

	subscription, err := manager.Subscribe("sess-stream", "dispatch-1", 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(initial.Events) != 2 {
		t.Fatalf("event count = %d, want 2", len(initial.Events))
	}
	if host.published != 2 {
		t.Fatalf("published observations = %d, want 2", host.published)
	}
}

func TestManager_CloseAllPreventsSubscription(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-close-all"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-close-all")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))
	manager.CloseAll(host.session)

	_, err := manager.Subscribe("sess-close-all", "dispatch-1", 0)
	if err != responsestream.ErrSubscriptionClosed {
		t.Fatalf("Subscribe after close all = %v, want %v", err, responsestream.ErrSubscriptionClosed)
	}
}

func TestManager_JavaScriptCheckpointStoreIsSessionOwned(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-checkpoint"},
	}
	manager := stream.NewManager(host)

	first := manager.JavaScriptCheckpointStore(host.session)
	second := manager.JavaScriptCheckpointStore(host.session)
	if first == nil || second == nil || first != second {
		t.Fatalf("checkpoint store = (%p, %p), want same session-owned instance", first, second)
	}
}
