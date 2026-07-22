package stream_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/stream"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

var streamTestClock = platformclock.Real{}
var streamResponseEventIdentity atomic.Uint64

func streamResponseEventID() string {
	return fmt.Sprintf("stream-response-event-%d", streamResponseEventIdentity.Add(1))
}

func newTestResponseStream() *factorysessions.SessionResponseStream {
	return factorysessions.NewSessionResponseStream(streamTestClock)
}

func newTestResponseStreamRegistry() *factorysessions.ResponseStreamRegistry {
	return factorysessions.NewResponseStreamRegistry(newTestResponseStream, streamTestClock)
}

func newTestManager(host stream.Host) *stream.Manager {
	return stream.NewManager(host, newTestResponseStreamRegistry())
}

type streamTestHost struct {
	session     *factorysessions.LiveSession
	streams     *factorysessions.SessionResponseStreamSet
	published   int
	compactions int
	degraded    int
}

func responseFragment(dispatchID, payload string) workers.ProgressFragment {
	return workers.ProgressFragment{
		DispatchID: dispatchID,
		Kind:       workers.ResponseFragmentKind,
		Type:       "TEXT_DELTA",
		Payload:    payload,
	}
}

func progressFragment(dispatchID, payload string) workers.ProgressFragment {
	return workers.ProgressFragment{
		DispatchID: dispatchID,
		Kind:       workers.ProgressFragmentKind,
		Type:       "PROGRESS",
		Payload:    payload,
	}
}

func completedFragment(dispatchID string) workers.ProgressFragment {
	return workers.ProgressFragment{DispatchID: dispatchID, Kind: workers.CompletedFragmentKind}
}

func failedFragment(dispatchID, payload string) workers.ProgressFragment {
	return workers.ProgressFragment{
		DispatchID: dispatchID,
		Kind:       workers.FailedFragmentKind,
		Payload:    payload,
	}
}

func canonicalDraftFragment(dispatchID string, draft any) workers.ProgressFragment {
	return workers.ProgressFragment{DispatchID: dispatchID, CanonicalDraft: draft}
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
		h.streams = factorysessions.NewSessionResponseStreamSetWithFactory(newTestResponseStream, streamTestClock)
	}
	return h.streams
}

func (h *streamTestHost) NewResponseStream() *factorysessions.SessionResponseStream {
	return newTestResponseStream()
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
	manager := newTestManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-stream")
	publisher(responseFragment("dispatch-1", "alpha"))
	publisher(progressFragment("dispatch-1", "beta"))

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
		streamTestClock,
		streamResponseEventID,
		streamResponseEventID,
	)
	host := &streamTestHost{session: session}
	manager := newTestManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)(session.ID)
	publisher(responseFragment("dispatch-1", "alpha"))

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

func TestManager_CursorInvocationPublishesStructuredSnapshotAndUnresolvedToolGap(t *testing.T) {
	session := factorysessions.NewLiveSession(
		"sess-cursor-structured", "/factory", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory", streamTestClock, streamResponseEventID, streamResponseEventID,
	)
	host := &streamTestHost{session: session}
	publisher := newTestManager(host).InferenceProgressPublisherFactory(nil)(session.ID)
	// Cursor decoding belongs to Workers and is covered owner-locally by
	// TestResponseEventDecoder_MapsCursorInitializationAndAssistantFixture and
	// TestResponseEventDecoder_ReconnectReportsEveryStillUnresolvedToolAsGap. This
	// Factory Sessions invariant starts at the public Worker draft contract.
	for _, draft := range cursorStructuredDrafts(t) {
		publisher(canonicalDraftFragment(draft.DispatchID, draft))
	}

	events := session.ResponseEvents.Events()
	if len(events) != 5 {
		t.Fatalf("response events = %#v, want session, delta, tool, gap, and snapshot", events)
	}
	assertCursorStructuredInvocationEvents(t, events)
	if host.published != 0 {
		t.Fatalf("legacy fragment publications = %d, want structured Cursor path to bypass compatibility fragments", host.published)
	}
}

func cursorStructuredDrafts(t *testing.T) []responseevents.Draft {
	t.Helper()
	const dispatchID = "dispatch-cursor-live"
	provenance := responseevents.Provenance{
		Provider:        "cursor",
		Delivery:        responseevents.DeliveryNativeStream,
		Fidelity:        responseevents.FidelityNormalized,
		Representation:  responseevents.RepresentationNotification,
		NativeEventType: "fixture",
	}
	draft := func(kind responseevents.Kind, phase responseevents.Phase, itemID string, payload any) responseevents.Draft {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal response draft: %v", err)
		}
		return responseevents.Draft{
			RunID: "cursor-session-live", DispatchID: dispatchID, ItemID: itemID,
			Kind: kind, Phase: phase, Provenance: provenance, Payload: encoded,
			ProviderSessionRef: "cursor-session-live",
		}
	}
	return []responseevents.Draft{
		draft(responseevents.KindSession, responseevents.PhaseStarted, "cursor-session-live",
			responseevents.SessionPayload{Status: "started"}),
		draft(responseevents.KindMessage, responseevents.PhaseDelta, "cursor-message-1",
			responseevents.MessageDeltaPayload{
				ContentBlockIndex: 0, ContentBlockKind: responseevents.ContentBlockText, TextDelta: "draft answer",
			}),
		draft(responseevents.KindTool, responseevents.PhaseStarted, "cursor-tool/call-live-1",
			responseevents.ToolPayload{ToolCallID: "call-live-1", ToolName: "read", Status: "started"}),
		draft(responseevents.KindStreamGap, responseevents.PhaseUpdated, "cursor-gap/call-live-1",
			responseevents.StreamGapPayload{
				AffectedItemID: "cursor-tool/call-live-1", ToolCallID: "call-live-1", Reason: "tool outcome was not observed",
			}),
		draft(responseevents.KindMessage, responseevents.PhaseCompleted, "cursor-message-final",
			responseevents.MessagePayload{
				Role: "assistant",
				ContentBlocks: []responseevents.ContentBlock{{
					Kind: responseevents.ContentBlockText, Text: "authoritative answer",
				}},
			}),
	}
}

func assertCursorStructuredInvocationEvents(t *testing.T, events []responseevents.FactoryResponseEvent) {
	t.Helper()
	want := []struct {
		kind  responseevents.Kind
		phase responseevents.Phase
	}{
		{responseevents.KindSession, responseevents.PhaseStarted},
		{responseevents.KindMessage, responseevents.PhaseDelta},
		{responseevents.KindTool, responseevents.PhaseStarted},
		{responseevents.KindStreamGap, responseevents.PhaseUpdated},
		{responseevents.KindMessage, responseevents.PhaseCompleted},
	}
	for index, expected := range want {
		event := events[index]
		if event.Kind != expected.kind || event.Phase != expected.phase {
			t.Fatalf("events[%d] = %s/%s, want %s/%s", index, event.Kind, event.Phase, expected.kind, expected.phase)
		}
		if event.DispatchID != "dispatch-cursor-live" || event.Provenance.Provider != "cursor" {
			t.Fatalf("events[%d] correlation = %#v, want live Cursor dispatch", index, event)
		}
	}
	assertCursorUnresolvedToolGap(t, events[3])
	assertCursorAuthoritativeSnapshot(t, events[4])
}

func assertCursorUnresolvedToolGap(t *testing.T, event responseevents.FactoryResponseEvent) {
	t.Helper()
	var gap responseevents.StreamGapPayload
	if err := json.Unmarshal(event.Payload, &gap); err != nil {
		t.Fatalf("decode gap: %v", err)
	}
	if gap.AffectedItemID != "cursor-tool/call-live-1" || gap.ToolCallID != "call-live-1" {
		t.Fatalf("gap = %#v, want unresolved call-live-1", gap)
	}
}

func assertCursorAuthoritativeSnapshot(t *testing.T, event responseevents.FactoryResponseEvent) {
	t.Helper()
	var snapshot responseevents.MessagePayload
	if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snapshot.ContentBlocks) != 1 || snapshot.ContentBlocks[0].Text != "authoritative answer" {
		t.Fatalf("snapshot = %#v, want authoritative terminal result", snapshot)
	}
}

func TestManager_PublishesProviderCanonicalDraftWithoutLegacyRemapping(t *testing.T) {
	t.Parallel()

	session := factorysessions.NewLiveSession(
		"sess-native-draft", "/factory", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory", streamTestClock, streamResponseEventID, streamResponseEventID,
	)
	host := &streamTestHost{session: session}
	publisher := newTestManager(host).InferenceProgressPublisherFactory(nil)(session.ID)
	payload, err := json.Marshal(responseevents.MessageDeltaPayload{
		ContentBlockIndex: 2, ContentBlockKind: responseevents.ContentBlockText, TextDelta: "native delta",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	publisher(canonicalDraftFragment("dispatch-native", responseevents.Draft{
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

func TestManager_SuppressesLegacyTerminalAfterProviderCanonicalDrafts(t *testing.T) {
	t.Parallel()

	session := factorysessions.NewLiveSession(
		"sess-adapter-terminal", "/factory", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory", streamTestClock, streamResponseEventID, streamResponseEventID,
	)
	publisher := newTestManager(&streamTestHost{session: session}).InferenceProgressPublisherFactory(nil)(session.ID)
	payload, err := json.Marshal(responseevents.MessagePayload{
		Role: "assistant", ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: "final answer"}},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	draft := responseevents.Draft{
		RunID: "run-1", DispatchID: "dispatch-1", Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted,
		Provenance: responseevents.Provenance{
			Provider: "opencode", NativeEventType: "text", Delivery: responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationSnapshot, Fidelity: responseevents.FidelityNormalized,
		},
		Payload: payload, ItemID: "message-1", ProviderSessionRef: "session-1",
	}
	publisher(canonicalDraftFragment(draft.DispatchID, draft))
	terminal := completedFragment(draft.DispatchID)
	terminal.CanonicalEventAlreadyPublished = true
	publisher(terminal)

	events := session.ResponseEvents.Events()
	if len(events) != 1 || events[0].Kind != responseevents.KindMessage || events[0].Phase != responseevents.PhaseCompleted {
		t.Fatalf("canonical events = %#v", events)
	}
	if events[0].Provenance != draft.Provenance || events[0].ItemID != draft.ItemID || events[0].ProviderSessionRef != draft.ProviderSessionRef {
		t.Fatalf("adapter semantics were not preserved: %#v", events[0])
	}
}

func TestManager_NativeFailureSuppressesLegacyMarkersSecondCanonicalProjection(t *testing.T) {
	t.Parallel()
	session := factorysessions.NewLiveSession(
		"sess-native-failure", "/factory", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory", streamTestClock, streamResponseEventID, streamResponseEventID,
	)
	host := &streamTestHost{session: session}
	publisher := newTestManager(host).InferenceProgressPublisherFactory(nil)(session.ID)
	payload, err := json.Marshal(responseevents.ErrorPayload{Code: "codex_turn_failed", Message: "Codex is temporarily unavailable.", Retryable: true})
	if err != nil {
		t.Fatal(err)
	}
	publisher(canonicalDraftFragment("dispatch-native-failure", responseevents.Draft{
		RunID: "dispatch-native-failure", DispatchID: "dispatch-native-failure",
		Kind: responseevents.KindError, Phase: responseevents.PhaseFailed,
		Provenance: responseevents.Provenance{
			Provider: "codex", NativeEventType: "turn.failed", Delivery: responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationNotification, Fidelity: responseevents.FidelityNormalized,
		},
		Payload: payload,
	}))
	marker := failedFragment("dispatch-native-failure", "Codex is temporarily unavailable.")
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
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory", streamTestClock, streamResponseEventID, streamResponseEventID,
	)
	host := &streamTestHost{session: session}
	publisher := newTestManager(host).InferenceProgressPublisherFactory(nil)(session.ID)
	publisher(canonicalDraftFragment("dispatch-invalid-type", "not-a-draft"))
	publisher(canonicalDraftFragment("dispatch-invalid-shape", responseevents.Draft{
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
	manager := newTestManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-close-all")
	publisher(responseFragment("dispatch-1", "alpha"))
	manager.CloseAll(host.session)

	_, err := manager.Subscribe("sess-close-all", "dispatch-1", 0)
	if err != responsestream.ErrSubscriptionClosed {
		t.Fatalf("Subscribe after close all = %v, want %v", err, responsestream.ErrSubscriptionClosed)
	}
}

func TestManager_CloseDispatch_ReleasesOneDispatchStream(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-close-dispatch"},
	}
	manager := newTestManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-close-dispatch")
	publisher(responseFragment("dispatch-1", "alpha"))

	if !manager.CloseDispatch(host.session, "dispatch-1") {
		t.Fatal("CloseDispatch = false, want true")
	}
}

func TestManager_DispatchCompletionObserverFactory_ClosesDispatchOnCompletion(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-observer"},
	}
	manager := newTestManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-observer")
	publisher(responseFragment("dispatch-1", "alpha"))

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
	manager := newTestManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-fragments")
	publisher(responseFragment("dispatch-1", "response"))
	publisher(completedFragment("dispatch-1"))
	publisher(failedFragment("dispatch-1", "failed"))

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
