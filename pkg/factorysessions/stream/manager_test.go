package stream_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/factorysessions/stream"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

type streamTestHost struct {
	session     *factorysessions.LiveSession
	streams     *factorysessions.SessionResponseStreamSet
	published   int
	compactions int
	degraded    int
	checkpoint  *factorysessions.JavaScriptCheckpointStore
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

func TestManager_PublishesCanonicalResponseEventsToSessionStore(t *testing.T) {
	t.Parallel()

	session := factorysessions.NewLiveSession(
		"sess-canonical",
		"/factory",
		"/workspace",
		"/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		nil,
		false,
		"factory",
	)
	host := &streamTestHost{session: session}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)(session.ID)
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))

	events := session.ResponseEvents.Events()
	if len(events) != 1 {
		t.Fatalf("canonical event count = %d, want 1", len(events))
	}
	if events[0].FactorySessionID != factorysessions.CanonicalFactorySessionID(session) {
		t.Fatalf("factorySessionId = %q, want %q", events[0].FactorySessionID, factorysessions.CanonicalFactorySessionID(session))
	}
	if events[0].DispatchID != "dispatch-1" {
		t.Fatalf("dispatchId = %q, want dispatch-1", events[0].DispatchID)
	}
}

func TestManager_PublishesProviderCanonicalDraftWithoutLegacyRemapping(t *testing.T) {
	t.Parallel()

	session := factorysessions.NewLiveSession(
		"sess-native-draft", "/factory", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory",
	)
	host := &streamTestHost{session: session}
	publisher := stream.NewManager(host).InferenceProgressPublisherFactory(nil)(session.ID)
	payload, err := json.Marshal(responseevents.MessageDeltaPayload{
		ContentBlockIndex: 2, ContentBlockKind: responseevents.ContentBlockText, TextDelta: "native delta",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	publisher(workerprovider.CanonicalDraftFragment("dispatch-native", responseevents.Draft{
		RunID: "dispatch-native", DispatchID: "dispatch-native", ItemID: "claude-message-7",
		Kind: responseevents.KindMessage, Phase: responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider: "claude", NativeEventType: "content_block_delta",
			Delivery: responseevents.DeliveryNativeStream, Representation: responseevents.RepresentationDelta,
			Fidelity: responseevents.FidelityLossless,
		},
		Payload: payload,
	}))

	events := session.ResponseEvents.Events()
	if len(events) != 1 || events[0].ItemID != "claude-message-7" || events[0].Provenance.Delivery != responseevents.DeliveryNativeStream {
		t.Fatalf("canonical events = %#v, want unchanged provider-native identity and provenance", events)
	}
	if events[0].Sequence != 1 || events[0].EventID == "" || events[0].RecordedAt.IsZero() {
		t.Fatalf("publication metadata = %#v, want session-owned identity, sequence, and time", events[0])
	}
	if host.published != 0 {
		t.Fatalf("legacy response-stream publications = %d, want no lossy remapping", host.published)
	}
}

func TestManager_NativeFailureSuppressesLegacyMarkersSecondCanonicalProjection(t *testing.T) {
	t.Parallel()
	session := factorysessions.NewLiveSession(
		"sess-native-failure", "/factory", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory",
	)
	host := &streamTestHost{session: session}
	publisher := stream.NewManager(host).InferenceProgressPublisherFactory(nil)(session.ID)
	payload, err := json.Marshal(responseevents.ErrorPayload{Code: "codex_turn_failed", Message: "Codex is temporarily unavailable.", Retryable: true})
	if err != nil {
		t.Fatal(err)
	}
	publisher(workerprovider.CanonicalDraftFragment("dispatch-native-failure", responseevents.Draft{
		RunID: "dispatch-native-failure", DispatchID: "dispatch-native-failure",
		Kind: responseevents.KindError, Phase: responseevents.PhaseFailed,
		Provenance: responseevents.Provenance{
			Provider: "codex", NativeEventType: "turn.failed", Delivery: responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationNotification, Fidelity: responseevents.FidelityNormalized,
		},
		Payload: payload,
	}))
	marker := workerprovider.FailedFragment("dispatch-native-failure", nil, "Codex is temporarily unavailable.")
	marker.CanonicalEventAlreadyPublished = true
	publisher(marker)

	events := session.ResponseEvents.Events()
	if len(events) != 1 || events[0].Kind != responseevents.KindError || events[0].Provenance.NativeEventType != "turn.failed" {
		t.Fatalf("canonical events = %#v, want one native terminal failure", events)
	}
	if host.published != 1 {
		t.Fatalf("legacy response-stream publications = %d, want terminal marker retained", host.published)
	}
}

func TestManager_RejectsInvalidProviderCanonicalDraftsWithoutLegacyFallback(t *testing.T) {
	t.Parallel()
	session := factorysessions.NewLiveSession(
		"sess-invalid-native", "/factory", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory",
	)
	host := &streamTestHost{session: session}
	publisher := stream.NewManager(host).InferenceProgressPublisherFactory(nil)(session.ID)
	publisher(workerprovider.CanonicalDraftFragment("dispatch-invalid-type", "not-a-draft"))
	publisher(workerprovider.CanonicalDraftFragment("dispatch-invalid-shape", responseevents.Draft{
		DispatchID: "dispatch-invalid-shape", Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted,
	}))

	if host.degraded != 2 {
		t.Fatalf("degraded publications = %d, want both invalid drafts rejected", host.degraded)
	}
	if events := session.ResponseEvents.Events(); len(events) != 0 {
		t.Fatalf("canonical events = %#v, want no lossy fallback", events)
	}
	if host.published != 0 {
		t.Fatalf("legacy publications = %d, want no invalid draft remapping", host.published)
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

func TestManager_CloseDispatch_ReleasesOneDispatchStream(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-close-dispatch"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-close-dispatch")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))

	if !manager.CloseDispatch(host.session, "dispatch-1") {
		t.Fatal("CloseDispatch = false, want true")
	}
}

func TestManager_DispatchCompletionObserverFactory_ClosesDispatchOnCompletion(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-observer"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-observer")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))

	observer := manager.DispatchCompletionObserverFactory()("sess-observer")
	observer("dispatch-1")

	subscription, err := manager.Subscribe("sess-observer", "dispatch-1", 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(initial.Events) != 1 {
		t.Fatalf("events = %d, want one buffered event after dispatch close", len(initial.Events))
	}
}

func TestManager_InferenceProgressPublisher_NormalizesFragmentKinds(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-fragments"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-fragments")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "response"))
	publisher(workerprovider.CompletedFragment("dispatch-1", nil))
	publisher(workerprovider.FailedFragment("dispatch-1", nil, "failed"))

	subscription, err := manager.Subscribe("sess-fragments", "dispatch-1", 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(initial.Events) != 3 {
		t.Fatalf("event count = %d, want 3", len(initial.Events))
	}
}
