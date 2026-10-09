// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package internal

import (
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	canonicalledger "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger"
	recordingsreplay "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay"

	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
	canonicalledgerwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
	replaywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/wire"
)

type stubLedger struct {
	events          []factorydefinitions.FactoryEvent
	subscribeErr    error
	subscribeScope  factorydefinitions.FactoryEventReconnectScope
	subscribeStream factorydefinitions.FactoryEventStream
}

func (ledger *stubLedger) CanonicalEvents() []factorydefinitions.FactoryEvent {
	out := make([]factorydefinitions.FactoryEvent, len(ledger.events))
	copy(out, ledger.events)
	return out
}

func (ledger *stubLedger) Subscribe(
	_ context.Context,
	_ *factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	ledger.subscribeScope = scope
	if ledger.subscribeErr != nil {
		return factorydefinitions.FactoryEventStream{}, ledger.subscribeErr
	}
	return ledger.subscribeStream, nil
}

func (ledger *stubLedger) StreamGenerationID() string { return "gen-1" }

func (ledger *stubLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (ledger *stubLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (ledger *stubLedger) AppendRecordedEvent(event factorydefinitions.FactoryEvent) {
	event.Context.Sequence = len(ledger.events)
	ledger.events = append(ledger.events, event)
}

func (ledger *stubLedger) AppendRecordedEventWithValidation(
	event factorydefinitions.FactoryEvent,
	validate func(factorydefinitions.FactoryEvent) error,
) (factorydefinitions.FactoryEvent, error) {
	event.Context.Sequence = len(ledger.events)
	if validate != nil {
		if err := validate(event); err != nil {
			return factorydefinitions.FactoryEvent{}, err
		}
	}
	ledger.events = append(ledger.events, event)
	return event, nil
}

func TestCanonicalOwnerPlainSlicesSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()
	ledger := &stubLedger{}
	assertAppendSubscribe(t, canonicalledgerwire.NewService(ledger), ledger)
}

func TestLifecycleOwnerPlainSlicesSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()
	assertRecordingLifecycle(t, recordinglifecyclewire.NewService(nil, nil, nil, staticRecordingClock{}))
}

func TestCombinedServiceProjectionPlainSlicesSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()
	projection := &plainReplayProjection{}
	root := NewCombinedService(nil, projection, nil, nil, nil, nil, unavailableHistoricalOwner{},
		staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil)
	assertProjectionQuery(t, root, projection)
}

func TestReplayOwnerPlainSlicesSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()
	snapshots := &plainOwnerSnapshots{byID: make(map[recordings.RecordingID]recordinglifecycle.Snapshot)}
	projection := &plainReplayProjection{}
	svc := replaywire.NewService(snapshots, projection, nil, nil)
	loaded := assertReplayLoadAndCompletion(t, svc, snapshots)
	assertReplayTypedFailures(t, svc, loaded)
	assertReplayDivergence(t, svc, loaded)
	assertReplayOrderedProgress(t, svc)
	if len(projection.events) != 2 || projection.events[1].Id != "state-1" {
		t.Fatalf("replay projection prefix = %#v, want both ordered state events", projection.events)
	}
}

func TestArtifactsOwnerPlainSlicesSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()
	snapshots := &plainOwnerSnapshots{byID: make(map[recordings.RecordingID]recordinglifecycle.Snapshot)}
	svc := artifactsexportwire.NewService(snapshots, nil)
	artifact := buildServicePortableArtifact(t, svc, snapshots)
	assertServicePortableRoundTrip(t, svc, artifact)
	assertServicePortableFailures(t, svc, artifact)
}

func TestLifecycleOwnerPreservesDetachedOutcomes(t *testing.T) {
	t.Parallel()

	lifecycle := recordinglifecyclewire.NewService(nil, nil, nil, staticRecordingClock{})
	scope := recordings.CanonicalEventScope{FactorySessionID: "lifecycle-adapter"}
	if _, err := lifecycle.StartRecording(recordings.StartRecordingRequest{}); err != nil {
		t.Fatalf("Begin disabled: %v", err)
	}
	if _, err := lifecycle.StartRecording(recordings.StartRecordingRequest{
		Enabled:     true,
		RecordingID: "lifecycle-adapter",
		Scope:       scope,
		Target:      recordings.RecordingTargetRequest{Artifact: "artifact://lifecycle-adapter"},
	}); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := lifecycle.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: "lifecycle-adapter",
		Event: recordings.CanonicalEvent{
			ID:         "lifecycle-adapter-event",
			Sequence:   0,
			Scope:      scope,
			Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "generation-1"},
			RecordedAt: time.Unix(1_700_000_600, 0).UTC(),
			Kind:       "WORK_REQUEST",
			Payload:    "{}",
		},
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if _, err := lifecycle.FlushRecording(recordings.FlushRecordingRequest{RecordingID: "lifecycle-adapter"}); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if _, err := lifecycle.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: "lifecycle-adapter"}); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if _, err := lifecycle.StopRecording(recordings.StopRecordingRequest{RecordingID: "lifecycle-adapter"}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	finished, err := lifecycle.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: "lifecycle-adapter",
		FinishedAt:  time.Unix(1_700_000_601, 0).UTC(),
	})
	if err != nil || finished.Status.State != recordings.RecordingFinalized {
		t.Fatalf("Finish = (%#v, %v), want finalized detached status", finished, err)
	}

	if _, err := lifecycle.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "lifecycle-failure",
		Artifact:    "artifact://lifecycle-failure",
		Scope:       scope,
	}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	failed, err := lifecycle.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: "lifecycle-failure",
		Failure: recordings.RecordingFailure{
			Code: "adapter-failure", Message: "adapter failure",
		},
	})
	if err != nil || failed.Status.State != recordings.RecordingFailed || len(failed.Status.Failures) != 1 {
		t.Fatalf("RecordFailure = (%#v, %v), want detached failed status", failed, err)
	}
	if _, err := lifecycle.StartRecording(recordings.StartRecordingRequest{
		Enabled: true, RecordingID: "missing-adapter-target", Scope: scope,
	}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("Begin missing target = %v, want ErrMissingRecordingTarget", err)
	}
}

func assertAppendSubscribe(t *testing.T, svc canonicalledger.Service, ledger *stubLedger) {
	t.Helper()
	assertOrderedAppend(t, svc, ledger)
	assertReconnectSubscription(t, svc, ledger)
}

func assertOrderedAppend(t *testing.T, svc canonicalledger.Service, ledger *stubLedger) {
	t.Helper()
	event := recordings.CanonicalEvent{
		ID:         "evt-1",
		Sequence:   7,
		Scope:      recordings.CanonicalEventScope{FactorySessionID: "session-1"},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Payload:    `{"work":"one"}`,
	}
	assertInvalidAppendsDoNotMutate(t, svc, ledger, event)
	accepted, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		t.Fatalf("Append valid event: %v", err)
	}
	if len(ledger.events) != 1 || ledger.events[0].Id != "evt-1" {
		t.Fatalf("Append did not delegate: %#v", ledger.events)
	}
	if ledger.events[0].Context.Sequence != 0 ||
		ledger.events[0].Context.SessionID == nil ||
		*ledger.events[0].Context.SessionID != "session-1" {
		t.Fatalf("Append canonical facts = %#v, want assigned sequence and session scope", ledger.events[0].Context)
	}
	if accepted.Event.Sequence != 0 ||
		accepted.Event.Cursor != (recordings.CanonicalEventCursor{
			StreamGenerationID: "gen-1",
			Sequence:           0,
		}) {
		t.Fatalf("Append accepted event = %#v, want Recordings-assigned ordering", accepted.Event)
	}
}

func assertInvalidAppendsDoNotMutate(
	t *testing.T,
	svc canonicalledger.Service,
	ledger *stubLedger,
	valid recordings.CanonicalEvent,
) {
	t.Helper()
	tests := map[string]func(*recordings.CanonicalEvent){
		"missing identity": func(event *recordings.CanonicalEvent) { event.ID = "" },
		"missing kind":     func(event *recordings.CanonicalEvent) { event.Kind = "" },
		"missing timestamp": func(event *recordings.CanonicalEvent) {
			event.RecordedAt = time.Time{}
		},
		"whitespace scope": func(event *recordings.CanonicalEvent) {
			event.Scope.FactorySessionID = "   "
		},
		"invalid payload": func(event *recordings.CanonicalEvent) {
			event.Payload = `{"incomplete":`
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			event := valid
			mutate(&event)
			if _, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event}); !errors.Is(
				err,
				recordings.ErrInvalidAppendEvent,
			) {
				t.Fatalf("Append invalid event error = %v, want ErrInvalidAppendEvent", err)
			}
			if len(ledger.events) != 0 {
				t.Fatalf("Append invalid event mutated ledger: %#v", ledger.events)
			}
		})
	}
}

func assertReconnectSubscription(t *testing.T, svc canonicalledger.Service, ledger *stubLedger) {
	t.Helper()
	assertSubscribeFailures(t, svc, ledger)
	first := assertScopedRetainedAndReconnect(t, svc, ledger)
	assertScopedLiveDelivery(t, svc, ledger, first.Event.Cursor)
	assertScopedDeliveryGap(t, svc, ledger)
}

func assertSubscribeFailures(t *testing.T, svc canonicalledger.Service, ledger *stubLedger) {
	t.Helper()
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "   "},
	}); !errors.Is(err, recordings.ErrInvalidSubscribeScope) {
		t.Fatalf("SubscribeFrom whitespace scope = %v, want ErrInvalidSubscribeScope", err)
	}

	ledger.subscribeErr = recordings.ErrReconnectCursorNotFound
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &recordings.CanonicalEventCursor{StreamGenerationID: "gen-1", Sequence: 0},
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	}); !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("SubscribeFrom stale cursor = %v, want ErrReconnectCursorExpired", err)
	}
	ledger.subscribeErr = nil
}

func assertScopedRetainedAndReconnect(
	t *testing.T,
	svc canonicalledger.Service,
	ledger *stubLedger,
) recordings.SubscriptionOutcome {
	t.Helper()
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 4, 0, "session-1"),
			scopedLegacyEvent("session-2/0", 5, 0, "session-2"),
			scopedLegacyEvent("session-1/1", 6, 1, "session-1"),
		},
	}
	subscribed, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom success path: %v", err)
	}
	first := subscribed.Subscription.Next(context.Background())
	if first.Kind != recordings.SubscriptionEvent || first.Event.ID != "session-1/0" ||
		first.Event.Sequence != 4 {
		t.Fatalf("SubscribeFrom first outcome = %#v, want global scoped event 4", first)
	}
	second := subscribed.Subscription.Next(context.Background())
	if second.Kind != recordings.SubscriptionEvent || second.Event.ID != "session-1/1" ||
		second.Event.Sequence != 6 {
		t.Fatalf("SubscribeFrom interleaved outcome = %#v, want global event 6", second)
	}

	cursor := first.Event.Cursor
	reconnected, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom reconnect: %v", err)
	}
	if outcome := reconnected.Subscription.Next(context.Background()); outcome.Kind != recordings.SubscriptionEvent ||
		outcome.Event.ID != "session-1/1" {
		t.Fatalf("SubscribeFrom reconnect outcome = %#v, want session-1/1", outcome)
	}
	return first
}

func assertScopedLiveDelivery(
	t *testing.T,
	svc canonicalledger.Service,
	ledger *stubLedger,
	cursor recordings.CanonicalEventCursor,
) {
	t.Helper()
	live := make(chan factorydefinitions.FactoryEvent, 2)
	live <- scopedLegacyEvent("session-2/1", 7, 1, "session-2")
	live <- scopedLegacyEvent("session-1/2", 8, 2, "session-1")
	ledger.subscribeStream.Events = live
	cursor.Sequence = 6
	liveSubscription, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom live: %v", err)
	}
	if outcome := liveSubscription.Subscription.Next(context.Background()); outcome.Kind != recordings.SubscriptionEvent ||
		outcome.Event.ID != "session-1/2" || outcome.Event.Sequence != 8 {
		t.Fatalf("SubscribeFrom interleaved live outcome = %#v, want global event 8", outcome)
	}
}

func assertScopedDeliveryGap(t *testing.T, svc canonicalledger.Service, ledger *stubLedger) {
	t.Helper()
	ledger.subscribeStream.Events = nil
	ledger.subscribeStream.History = []factorydefinitions.FactoryEvent{
		scopedLegacyEvent("session-1/0", 4, 0, "session-1"),
		scopedLegacyEvent("session-2/0", 5, 0, "session-2"),
		scopedLegacyEvent("session-1/2", 8, 2, "session-1"),
	}
	gapped, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom gap setup: %v", err)
	}
	_ = gapped.Subscription.Next(context.Background())
	gap := gapped.Subscription.Next(context.Background())
	if gap.Kind != recordings.SubscriptionGap || gap.Gap == nil ||
		gap.Gap.Cause != recordings.SubscriptionSequenceDiscontinuity ||
		gap.Gap.ExpectedSequence != 1 || gap.Gap.ObservedSequence != 2 ||
		gap.Gap.ReconnectFrom.Sequence != 4 {
		t.Fatalf("SubscribeFrom discontinuity = %#v, want explicit gap 1 -> 2", gap)
	}
}

func scopedLegacyEvent(
	id string,
	globalSequence int,
	sessionSequence int,
	sessionID string,
) factorydefinitions.FactoryEvent {
	return factorydefinitions.FactoryEvent{
		Id: id,
		Context: factorydefinitions.FactoryEventContext{
			Sequence:        globalSequence,
			SessionID:       &sessionID,
			SessionSequence: &sessionSequence,
		},
	}
}

func assertProjectionQuery(t *testing.T, svc recordings.Service, projection *plainReplayProjection) {
	t.Helper()
	historical, err := svc.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: "unavailable"},
	})
	var historicalErr *recordings.HistoricalRecordingQueryError
	if !errors.As(err, &historicalErr) ||
		historicalErr.Kind != recordings.HistoricalRecordingQueryErrorUnavailable ||
		historical.Recording.RecordingID != "" {
		t.Fatalf("QueryHistoricalRecording without capability = (%#v, %v), want unavailable typed error", historical, err)
	}
	if _, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		SelectedTick: -1,
	}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReconstructWorldState negative tick = %v, want ErrInvalidProjectionInput", err)
	}
	world, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       nil,
		SelectedTick: 0,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState success path: %v", err)
	}
	if _, err := svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: world.WorldState,
	}); err != nil {
		t.Fatalf("QuerySimpleDashboard: %v", err)
	}
	if _, err := svc.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{
		WorldState: world.WorldState,
	}); err != nil {
		t.Fatalf("QueryWorkstationRequests: %v", err)
	}
	if _, err := svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       "{",
		},
	}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("QuerySimpleDashboard invalid payload = %v, want ErrInvalidProjectionInput", err)
	}
	if _, err := svc.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{
		WorldState: recordings.WorldStateView{SchemaVersion: "unsupported", Payload: "{}"},
	}); !errors.Is(err, recordings.ErrUnsupportedProjectionView) {
		t.Fatalf("QueryWorkstationRequests unsupported view = %v, want ErrUnsupportedProjectionView", err)
	}
	assertReconnectReplayValidation(t, svc, projection)
}

func assertReconnectReplayValidation(t *testing.T, svc recordings.Service, projection *plainReplayProjection) {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-query"}
	history := []recordings.CanonicalEvent{
		canonicalProjectionEvent("query-0", 0, scope),
		canonicalProjectionEvent("query-2", 2, scope),
		canonicalProjectionEvent("query-4", 4, scope),
	}
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: history,
		Cursor: history[1].Cursor,
		Scope:  scope,
	}); err != nil {
		t.Fatalf("ValidateReconnectReplayFrom interleaved scoped history: %v", err)
	}
	if len(projection.reconnectEvents) != len(history) || projection.reconnectEvents[2].Id != string(history[2].ID) ||
		projection.reconnectCursor.AfterSequence == nil || *projection.reconnectCursor.AfterSequence != 2 ||
		projection.reconnectScope.SessionID != scope.FactorySessionID {
		t.Fatalf("reconnect collaborator request = (%#v, %#v, %#v), want ordered scoped history and cursor 2",
			projection.reconnectEvents, projection.reconnectCursor, projection.reconnectScope)
	}
	projection.reconnectErr = recordings.ErrReconnectCursorNotFound
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: history[1:],
		Cursor: history[0].Cursor,
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf(
			"ValidateReconnectReplayFrom continuation-only history = %v, want ErrReconnectCursorNotFound",
			err,
		)
	}
	projection.reconnectErr = nil
	malformed := append([]recordings.CanonicalEvent(nil), history...)
	malformed[1], malformed[2] = malformed[2], malformed[1]
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: malformed,
		Cursor: malformed[0].Cursor,
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrMalformedProjectionOrder) {
		t.Fatalf("ValidateReconnectReplayFrom malformed order = %v, want ErrMalformedProjectionOrder", err)
	}
	wrongScope := append([]recordings.CanonicalEvent(nil), history...)
	wrongScope[2].Scope = recordings.CanonicalEventScope{FactorySessionID: "other-session"}
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: wrongScope,
		Cursor: wrongScope[0].Cursor,
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrInvalidProjectionScope) {
		t.Fatalf("ValidateReconnectReplayFrom wrong scope = %v, want ErrInvalidProjectionScope", err)
	}
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: history,
		Cursor: recordings.CanonicalEventCursor{Sequence: 0},
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrMalformedProjectionOrder) {
		t.Fatalf("ValidateReconnectReplayFrom malformed cursor = %v, want ErrMalformedProjectionOrder", err)
	}
}

func canonicalProjectionEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:       recordings.CanonicalEventID(id),
		Sequence: sequence,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "gen-1",
			Sequence:           sequence,
		},
	}
}

func assertRecordingLifecycle(t *testing.T, svc recordinglifecycle.Service) {
	t.Helper()
	assertRecordingLifecycleBindingCollision(t, svc)
	assertRecordingLifecycleHappyPath(t, svc)
	assertRecordingLifecycleFlushFailure(t, svc)
}

func assertRecordingLifecycleHappyPath(t *testing.T, svc recordinglifecycle.Service) {
	t.Helper()
	if _, err := svc.BindRecording(recordings.BindRecordingRequest{}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("BindRecording empty artifact = %v, want ErrMissingRecordingTarget", err)
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recording"}
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:recording",
		Scope:    scope,
	})
	if err != nil || bound.Status.RecordingID == "" {
		t.Fatalf("BindRecording success = (%#v, %v)", bound, err)
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event: recordings.CanonicalEvent{
			ID: "wrong-scope", Kind: "WORK_REQUEST",
			Scope: recordings.CanonicalEventScope{FactorySessionID: "other-session"},
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "generation-1",
			},
		},
	}); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
		t.Fatalf("RecordRecordingEvent wrong scope = %v, want ErrInvalidRecordingEvent", err)
	}
	recorded, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event: recordings.CanonicalEvent{
			ID: "rec-evt-1", Kind: "WORK_REQUEST", Scope: scope,
			RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
			Payload:    "{}",
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "generation-1",
			},
		},
	})
	if err != nil || recorded.Status.AcceptedEvents != 1 {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	}); err != nil {
		t.Fatalf("FlushRecording: %v", err)
	}
	finished, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	})
	if err != nil || finished.Status.State != recordings.RecordingFinalized {
		t.Fatalf("FinishRecording: %v", err)
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event: recordings.CanonicalEvent{
			ID: "after-finish", Kind: "WORK_REQUEST", Scope: scope, Sequence: 1,
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "generation-1", Sequence: 1,
			},
		},
	}); !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("post-finish write = %v, want ErrRecordingWriteRejected", err)
	}
}

func assertRecordingLifecycleFlushFailure(t *testing.T, svc recordinglifecycle.Service) {
	t.Helper()
	boundFail, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "flush-fail",
		Artifact:    "artifact:fail",
	})
	if err != nil {
		t.Fatalf("BindRecording flush-fail: %v", err)
	}
	failed, err := svc.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: boundFail.Status.RecordingID,
		Failure: recordings.RecordingFailure{
			Code: "producer_failed", Message: "producer boom",
		},
	})
	if err != nil || failed.Status.State != recordings.RecordingFailed {
		t.Fatalf("RecordRecordingError: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: boundFail.Status.RecordingID,
	}); err != nil {
		t.Fatalf("FlushRecording after failure fact: %v", err)
	}
	status, err := svc.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: boundFail.Status.RecordingID,
	})
	if err != nil || status.Status.State != recordings.RecordingFailed ||
		len(status.Status.Failures) != 1 {
		t.Fatalf("QueryRecordingStatus = (%#v, %v)", status, err)
	}
	if _, err := svc.QueryRecordingStatus(recordings.RecordingStatusRequest{}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("QueryRecordingStatus missing = %v, want ErrMissingRecordingTarget", err)
	}
}

func assertRecordingLifecycleBindingCollision(t *testing.T, svc recordinglifecycle.Service) {
	t.Helper()
	assertGeneratedServiceRecordingIDDoesNotCollide(t, svc)
	request := recordings.BindRecordingRequest{
		RecordingID: "stable-recording",
		Artifact:    "artifact:stable",
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: "session-stable",
		},
	}
	bound, err := svc.BindRecording(request)
	if err != nil {
		t.Fatalf("BindRecording stable: %v", err)
	}
	event := recordings.CanonicalEvent{
		ID: "stable-event", Kind: "WORK_REQUEST", Scope: request.Scope,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(), Payload: "{}",
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-stable",
		},
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent stable: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	}); err != nil {
		t.Fatalf("FlushRecording stable: %v", err)
	}
	active := recordingLifecycleStatus(t, svc, request.RecordingID)
	assertServiceBindingCollision(t, svc, request, active, "active")

	producerErr := errors.New("preserve this failure")
	if _, err := svc.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: request.RecordingID,
		Failure: recordings.RecordingFailure{
			Code: "producer_failed", Message: "preserve this failure",
		},
		Cause: producerErr,
	}); err != nil {
		t.Fatalf("RecordRecordingError stable: %v", err)
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: request.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	}); !errors.Is(err, producerErr) {
		t.Fatalf("FinishRecording stable error = %v, want producer cause", err)
	}
	terminal := recordingLifecycleStatus(t, svc, request.RecordingID)
	if terminal.State != recordings.RecordingFailed || terminal.FinalizedAt == nil {
		t.Fatalf("terminal status = %#v, want finalized failed recording", terminal)
	}
	assertServiceBindingCollision(t, svc, request, terminal, "terminal")
}

func assertGeneratedServiceRecordingIDDoesNotCollide(t *testing.T, svc recordinglifecycle.Service) {
	t.Helper()
	explicit, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-1",
		Artifact:    "artifact:explicit",
	})
	if err != nil {
		t.Fatalf("BindRecording explicit generated-form ID: %v", err)
	}
	generated, err := svc.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:generated",
	})
	if err != nil {
		t.Fatalf("BindRecording generated ID: %v", err)
	}
	if generated.Status.RecordingID == explicit.Status.RecordingID {
		t.Fatalf("generated RecordingID %q replaced an existing binding", generated.Status.RecordingID)
	}
	got := recordingLifecycleStatus(t, svc, explicit.Status.RecordingID)
	if got.Artifact != explicit.Status.Artifact {
		t.Fatalf("explicit binding after generated bind = %#v, want unchanged", got)
	}
}

func assertServiceBindingCollision(
	t *testing.T,
	svc recordinglifecycle.Service,
	request recordings.BindRecordingRequest,
	want recordings.RecordingStatusFacts,
	phase string,
) {
	t.Helper()
	rebound, err := svc.BindRecording(request)
	if err != nil || !reflect.DeepEqual(rebound.Status, want) {
		t.Fatalf(
			"BindRecording identical %s = (%#v, %v), want unchanged %#v",
			phase,
			rebound.Status,
			err,
			want,
		)
	}
	conflict := request
	conflict.Artifact = "artifact:other"
	if _, err := svc.BindRecording(conflict); !errors.Is(
		err,
		recordings.ErrRecordingBindingConflict,
	) {
		t.Fatalf(
			"BindRecording conflicting %s = %v, want ErrRecordingBindingConflict",
			phase,
			err,
		)
	}
	got := recordingLifecycleStatus(t, svc, request.RecordingID)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"status after conflicting %s bind = %#v, want unchanged %#v",
			phase,
			got,
			want,
		)
	}
}

func recordingLifecycleStatus(
	t *testing.T,
	svc interface {
		QueryRecordingStatus(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error)
	},
	recordingID recordings.RecordingID,
) recordings.RecordingStatusFacts {
	t.Helper()
	result, err := svc.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus %q: %v", recordingID, err)
	}
	return result.Status
}

func assertReplayLoadAndCompletion(
	t *testing.T,
	svc recordingsreplay.Service,
	snapshots *plainOwnerSnapshots,
) recordings.ReplayRecordingFacts {
	t.Helper()
	if _, err := svc.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("LoadReplayRecording missing = %v, want ErrReplayRecordingNotFound", err)
	}
	const recordingID recordings.RecordingID = "recording-replay-service"
	snapshots.byID[recordingID] = recordinglifecycle.Snapshot{Status: recordings.RecordingStatusFacts{RecordingID: recordingID}}
	if _, err := svc.LoadReplayRecording(recordings.LoadReplayRecordingRequest{RecordingID: recordingID}); !errors.Is(err, recordings.ErrReplayRecordingNotFinalized) {
		t.Fatalf("LoadReplayRecording active = %v, want ErrReplayRecordingNotFinalized", err)
	}
	finishedAt := time.Unix(1_700_000_000, 0).UTC()
	snapshot := snapshots.byID[recordingID]
	snapshot.Status.FinalizedAt = &finishedAt
	snapshots.byID[recordingID] = snapshot
	loaded, err := svc.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording = %v", err)
	}
	planned, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan = %v", err)
	}
	observed, err := svc.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
	if err != nil || observed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("ObserveReplay = (%#v, %v)", observed, err)
	}
	return loaded.Recording
}

func assertReplayTypedFailures(
	t *testing.T,
	svc recordingsreplay.Service,
	recording recordings.ReplayRecordingFacts,
) {
	t.Helper()
	if _, err := svc.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: "missing",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("ObserveReplay missing = %v, want ErrReplayPlanNotFound", err)
	}
	if _, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: "unsupported",
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recording,
	}); !errors.Is(err, recordings.ErrUnsupportedReplayPlan) {
		t.Fatalf("CreateReplayPlan unsupported = %v, want ErrUnsupportedReplayPlan", err)
	}
	if _, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recordings.ReplayRecordingFacts{},
	}); !errors.Is(err, recordings.ErrCorruptReplayInput) {
		t.Fatalf("CreateReplayPlan corrupt = %v, want ErrCorruptReplayInput", err)
	}
}

func assertReplayDivergence(
	t *testing.T,
	svc recordingsreplay.Service,
	recording recordings.ReplayRecordingFacts,
) {
	t.Helper()
	expected := recordings.CanonicalEventCursor{
		StreamGenerationID: "expected-generation",
		Sequence:           1,
	}
	divergencePlan, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion:   recordings.ReplayPlanSchemaV1,
		Timing:          recordings.ReplayTimingOrderOnly,
		Recording:       recording,
		ExpectedThrough: &expected,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan divergence = %v", err)
	}
	diverged, err := svc.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: divergencePlan.Plan.Handle,
	})
	if err != nil || diverged.Observation.Kind != recordings.ReplayDiverged ||
		diverged.Observation.Divergence == nil {
		t.Fatalf("ObserveReplay divergence = (%#v, %v)", diverged, err)
	}
}

func assertReplayOrderedProgress(t *testing.T, svc recordingsreplay.Service) {
	t.Helper()
	events := []recordings.CanonicalEvent{
		replayStateEvent(0, `{"state":"RUNNING"}`),
		replayStateEvent(1, `{"state":"COMPLETED"}`),
	}
	expected := events[1].Cursor
	planned, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: "recording-progress",
			Events:      events,
		},
		ExpectedThrough: &expected,
		SelectedTick:    1,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan progress = %v", err)
	}
	progress, err := svc.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
	if err != nil || progress.Observation.Kind != recordings.ReplayProgress {
		t.Fatalf("ObserveReplay progress = (%#v, %v)", progress, err)
	}
	completed, err := svc.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
	if err != nil || completed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("ObserveReplay completed = (%#v, %v)", completed, err)
	}
}

func replayStateEvent(sequence recordings.CanonicalEventSequence, payload string) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:       recordings.CanonicalEventID("state-" + string(rune('0'+sequence))),
		Sequence: sequence,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-progress",
			Sequence:           sequence,
		},
		RecordedAt: time.Unix(1_700_000_000+int64(sequence), 0).UTC(),
		Kind:       "FACTORY_STATE_RESPONSE",
		Payload:    payload,
	}
}

// Snapshot doubles supply detached owner inputs without constructing a lifecycle.
// Nil embedded methods fail unexpected collaborator calls.
type plainOwnerSnapshots struct {
	recordinglifecycle.Service
	byID map[recordings.RecordingID]recordinglifecycle.Snapshot
}

func (snapshots *plainOwnerSnapshots) Snapshot(id recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	snapshot, ok := snapshots.byID[id]
	if !ok {
		return recordinglifecycle.Snapshot{}, recordings.ErrMissingRecordingTarget
	}
	return snapshot, nil
}

type plainReplayProjection struct {
	recordings.ProjectionService
	events          []recordings.FactoryEvent
	reconnectErr    error
	reconnectEvents []recordings.FactoryEvent
	reconnectCursor recordings.FactoryEventReconnectCursor
	reconnectScope  recordings.FactoryEventReconnectScope
}

func (projection *plainReplayProjection) ReconstructFactoryWorldState(events []recordings.FactoryEvent, tick int) (recordings.FactoryWorldState, error) {
	projection.events = append([]recordings.FactoryEvent(nil), events...)
	return recordings.FactoryWorldState{Tick: tick}, nil
}

func (*plainReplayProjection) SimpleDashboardRenderData(recordings.FactoryWorldState) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{}
}

func (*plainReplayProjection) ProjectWorkstationRequests(recordings.FactoryWorldState) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
}

func (projection *plainReplayProjection) ValidateReconnectReplay(events []recordings.FactoryEvent, cursor recordings.FactoryEventReconnectCursor, scope recordings.FactoryEventReconnectScope) error {
	projection.reconnectEvents = append([]recordings.FactoryEvent(nil), events...)
	projection.reconnectCursor = cursor
	projection.reconnectScope = scope
	return projection.reconnectErr
}
func buildServicePortableArtifact(t *testing.T, svc artifactsexport.Service, snapshots *plainOwnerSnapshots) recordings.PortableArtifact {
	t.Helper()
	const id recordings.RecordingID = "recording-export"
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-service-1"}
	snapshot := recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{RecordingID: id, Artifact: "artifact:export", Scope: scope},
		Events: []recordings.CanonicalEvent{{ID: "export-event", Kind: "WORK_REQUEST", Scope: scope,
			RecordedAt: time.Unix(1_700_000_000, 0).UTC(), Payload: "{}",
			Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "generation-export"}}},
	}
	snapshots.byID[id] = snapshot
	if _, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{RecordingID: id}); !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("BuildPortableArtifact active = %v", err)
	}
	finishedAt := time.Unix(1_700_000_001, 0).UTC()
	snapshot.Status.State = recordings.RecordingFinalized
	snapshot.Status.FinalizedAt = &finishedAt
	snapshots.byID[id] = snapshot
	built, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{RecordingID: id})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	return built.Artifact
}

func assertServicePortableRoundTrip(
	t *testing.T,
	svc artifactsexport.Service,
	artifact recordings.PortableArtifact,
) {
	t.Helper()
	if _, err := svc.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: artifact,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	summary, err := svc.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: artifact,
	})
	if err != nil || summary.Summary.Scope.FactorySessionID != "session-service-1" {
		t.Fatalf("SummarizePortableArtifact = (%#v, %v)", summary, err)
	}
	encoded, err := svc.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodePortableArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}
	decoded, err := svc.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: encoded.Payload,
	})
	if err != nil || decoded.Artifact.Summary.EventCount != 1 ||
		decoded.Artifact.Events[0].ID != artifact.Events[0].ID {
		t.Fatalf("DecodePortableArtifact = (%#v, %v)", decoded, err)
	}
}

func assertServicePortableFailures(
	t *testing.T,
	svc artifactsexport.Service,
	artifact recordings.PortableArtifact,
) {
	t.Helper()
	if _, err := svc.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{}); !errors.Is(err, recordings.ErrUnsupportedPortableArtifactSchema) {
		t.Fatalf("SummarizePortableArtifact empty = %v, want ErrUnsupportedPortableArtifactSchema", err)
	}
	if _, err := svc.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("DecodePortableArtifact empty = %v, want ErrInvalidPortableArtifact", err)
	}
	tampered := artifact
	tampered.Events = append([]recordings.CanonicalEvent{}, artifact.Events...)
	tampered.Events[0].Payload = `{"changed":true}`
	if _, err := svc.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: tampered,
	}); !errors.Is(err, recordings.ErrInvalidPortableArtifactIntegrity) {
		t.Fatalf("ValidatePortableArtifact tampered = %v", err)
	}
}

type plainPublicationFailure struct {
	destination string
	published   bool
	read        bool
}

func (publication *plainPublicationFailure) Publish(_ context.Context, destination string, _ []byte) error {
	publication.destination = destination
	publication.published = true
	return os.ErrPermission
}
func (publication *plainPublicationFailure) Read(_ context.Context, destination string) ([]byte, error) {
	publication.destination = destination
	publication.read = true
	return nil, os.ErrPermission
}

func TestArtifactsOwnerPortablePublicationFailures(t *testing.T) {
	t.Parallel()
	const id recordings.RecordingID = "recording-export-delegate"
	const destination recordings.RecordingArtifactReference = "artifact:export-delegate"
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-export-delegate"}
	finishedAt := time.Unix(1_700_000_001, 0).UTC()
	snapshots := &plainOwnerSnapshots{byID: map[recordings.RecordingID]recordinglifecycle.Snapshot{
		id: {Status: recordings.RecordingStatusFacts{RecordingID: id, Artifact: destination, Scope: scope,
			State: recordings.RecordingFinalized, FinalizedAt: &finishedAt},
			Events: []recordings.CanonicalEvent{{ID: "export-delegate-event", Kind: "WORK_REQUEST", Scope: scope,
				RecordedAt: time.Unix(1_700_000_000, 0).UTC(), Payload: "{}",
				Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "generation-export-delegate"}}}},
	}}
	publication := &plainPublicationFailure{}
	svc := artifactsexportwire.NewService(snapshots, publication)
	if _, err := svc.ExportPortableArtifact(context.Background(), recordings.ExportPortableArtifactRequest{RecordingID: id}); !errors.Is(err, recordings.ErrPortableArtifactExportFailed) {
		t.Fatalf("ExportPortableArtifact = %v, want ErrPortableArtifactExportFailed", err)
	}
	if !publication.published || publication.destination != string(destination) {
		t.Fatalf("publication = %#v, want export to selected artifact", publication)
	}
	if _, err := svc.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{RecordingID: id, Reference: destination}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("ReadPortableArtifact = %v, want ErrInvalidPortableArtifact", err)
	}
	if !publication.read || publication.destination != string(destination) {
		t.Fatalf("publication = %#v, want read of selected artifact", publication)
	}
}

func TestProjectionServiceDelegates(t *testing.T) {
	t.Parallel()
	projection := projectionquerywire.NewService()
	state, err := projection.ReconstructFactoryWorldState(nil, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	_ = projection.SimpleDashboardRenderData(state)
	_ = projection.ProjectWorkstationRequests(state)
	if err := projection.ValidateReconnectReplay(nil, factorydefinitions.FactoryEventReconnectCursor{}, factorydefinitions.FactoryEventReconnectScope{}); err != nil {
		t.Fatalf("ValidateReconnectReplay: %v", err)
	}
}

func TestNewReplayClockAndExecutionNilArtifact(t *testing.T) {
	t.Parallel()
	if got := NewReplayClock(nil); got != nil {
		t.Fatalf("NewReplayClock(nil) = %#v, want nil", got)
	}
	provider, runner, hooks, planner, err := NewReplayExecution(nil, nil, nil)
	if provider != nil || runner != nil || hooks != nil || planner != nil || err != nil {
		t.Fatalf("NewReplayExecution(nil) = (%v,%v,%v,%v,%v), want nils", provider, runner, hooks, planner, err)
	}
}

func TestCombinedServiceReadHelpersPreserveOwnerFailures(t *testing.T) {
	t.Parallel()

	svc := NewCombinedService(nil, nil, nil, nil, nil, nil, unavailableHistoricalOwner{}, staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(*combinedService)
	_, err := svc.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{})
	var historicalErr *recordings.HistoricalRecordingQueryError
	if !errors.As(err, &historicalErr) || historicalErr.Kind != recordings.HistoricalRecordingQueryErrorUnavailable {
		t.Fatalf("QueryHistoricalRecording without capability = %v, want unavailable typed error", err)
	}
	svc.replayByKey = nil
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{ArtifactID: "artifact-1"}); !errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("artifact with unavailable map = %v, want ErrInvalidReplayArtifact", err)
	}
}

func TestCombinedServiceReadHelpersRejectMalformedReplayCursor(t *testing.T) {
	t.Parallel()

	svc := NewCombinedService(nil, nil, nil, nil, nil, nil, unavailableHistoricalOwner{}, staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(*combinedService)
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
		Cursor: recordings.CanonicalEventCursor{Sequence: -1},
	}); !errors.Is(err, recordings.ErrMalformedProjectionOrder) {
		t.Fatalf("malformed reconnect cursor = %v, want ErrMalformedProjectionOrder", err)
	}
}

func TestCombinedServiceReadHelpersValidateEventScopeCursor(t *testing.T) {
	t.Parallel()

	events := []recordings.CanonicalEvent{
		{Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "stream-1", Sequence: 0}},
		{Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "stream-1", Sequence: 1}},
	}
	if copied, err := scopeEventPrefix(events, nil); err != nil || len(copied) != len(events) {
		t.Fatalf("scopeEventPrefix without cursor = %#v, %v, want copied history", copied, err)
	}
	if _, err := scopeEventPrefix(events, &recordings.CanonicalEventCursor{Sequence: -1, StreamGenerationID: "stream-1"}); !errors.Is(err, recordings.ErrInvalidReconnectCursor) {
		t.Fatalf("negative scope cursor = %v, want ErrInvalidReconnectCursor", err)
	}
	if _, err := scopeEventPrefix(events, &recordings.CanonicalEventCursor{Sequence: 0, StreamGenerationID: "other-stream"}); !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("foreign scope cursor = %v, want ErrReconnectCursorUnavailable", err)
	}
	if prefix, err := scopeEventPrefix(events, &events[0].Cursor); err != nil || len(prefix) != 1 {
		t.Fatalf("matching scope cursor = %#v, %v, want one-event prefix", prefix, err)
	}
	if _, err := scopeEventPrefix(events, &recordings.CanonicalEventCursor{Sequence: 8, StreamGenerationID: "stream-1"}); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("missing scope cursor = %v, want ErrReconnectCursorNotFound", err)
	}
	if cursorInEvents(events, recordings.CanonicalEventCursor{Sequence: -1, StreamGenerationID: "stream-1"}) {
		t.Fatal("negative cursor reported as present")
	}
	if !cursorInEvents(events, events[1].Cursor) {
		t.Fatal("matching cursor reported as absent")
	}
	if cursorInEvents(events, recordings.CanonicalEventCursor{Sequence: 9, StreamGenerationID: "stream-1"}) {
		t.Fatal("missing cursor reported as present")
	}
}

func TestCombinedServiceReadHelpersNormalizeRecordingClock(t *testing.T) {
	t.Parallel()
	want := time.Date(2026, 8, 26, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	zero := NewCombinedService(nil, nil, nil, nil, nil, nil, nil, staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(*combinedService)
	if got := zero.recordingFinishedAt(); !got.IsZero() {
		t.Fatalf("finished time from zero clock = %v, want zero", got)
	}
	explicit := NewCombinedService(nil, nil, nil, nil, nil, nil, nil, staticRecordingClock{at: want}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(*combinedService)
	if got := explicit.recordingFinishedAt(); !got.Equal(want.UTC()) || got.Location() != time.UTC {
		t.Fatalf("finished time = %v, want UTC %v", got, want.UTC())
	}
}

func TestCombinedServiceAbandonedScopeFinishesAtInjectedClock(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 3, 5, 34, 56, 789, time.FixedZone("recording", -7*60*60))
	finish := characterizeAbandonedRecording(t, staticRecordingClock{at: at}, errors.New("final writer failure"))
	if !finish.FinishedAt.Equal(at.UTC()) || finish.FinishedAt.Location() != time.UTC {
		t.Fatalf("abandoned finish timestamp = %v, want UTC %v", finish.FinishedAt, at.UTC())
	}
}

func TestCombinedServiceAbandonedScopeZeroClockPreservesOwnerRejection(t *testing.T) {
	t.Parallel()
	finish := characterizeAbandonedRecording(t, staticRecordingClock{}, recordings.ErrInvalidRecordingTerminalMetadata)
	if !finish.FinishedAt.IsZero() {
		t.Fatalf("abandoned finish timestamp = %v, want zero for lifecycle validation", finish.FinishedAt)
	}
}

// This completed-owner double observes root cleanup and forwarding only. Native
// lifecycle tests own terminal validation, final writes and durable status policy.
type abandonedScopeLifecycle struct {
	recordinglifecycle.Service
	cancel    context.CancelFunc
	started   recordings.StartRecordingRequest
	finishes  []recordings.FinishRecordingRequest
	finishErr error
}

func (owner *abandonedScopeLifecycle) StartRecording(request recordings.StartRecordingRequest) (recordings.StartRecordingResult, error) {
	owner.started = request
	owner.cancel()
	return recordings.StartRecordingResult{Enabled: true, Status: recordings.RecordingStatusFacts{
		RecordingID: request.RecordingID, Scope: request.Scope,
	}}, nil
}

func (owner *abandonedScopeLifecycle) FinishRecording(request recordings.FinishRecordingRequest) (recordings.FinishRecordingResult, error) {
	owner.finishes = append(owner.finishes, request)
	return recordings.FinishRecordingResult{}, owner.finishErr
}

func characterizeAbandonedRecording(t *testing.T, clock recordings.RecordingClock, finishErr error) recordings.FinishRecordingRequest {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	owner := &abandonedScopeLifecycle{cancel: cancel, finishErr: finishErr}
	root := NewCombinedService(nil, nil, owner, nil, nil, nil, nil, clock,
		logging.NoopLogger{}, nil, nil, nil, nil, nil).(*combinedService)
	request := recordings.BeginRecordingScopeRequest{
		Enabled:       true,
		Scope:         recordings.CanonicalEventScope{FactorySessionID: t.Name()},
		Target:        recordings.RecordingTargetRequest{Artifact: recordings.RecordingArtifactReference("artifact:" + t.Name())},
		FlushInterval: time.Hour,
	}
	result, err := root.BeginRecordingScope(ctx, request)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, finishErr) {
		t.Fatalf("BeginRecordingScope = %v, want cancellation and owner cause", err)
	}
	if !result.Scope.IsZero() || len(owner.finishes) != 1 {
		t.Fatalf("abandoned begin = %#v, finishes = %#v, want zero scope and one finish", result, owner.finishes)
	}
	if owner.started.RecordingID == "" || owner.finishes[0].RecordingID != owner.started.RecordingID ||
		owner.started.Scope != request.Scope || owner.started.Target != request.Target ||
		owner.started.FlushInterval != request.FlushInterval || !owner.started.Enabled {
		t.Fatalf("abandoned start/finish = (%#v, %#v), want selected identity and request", owner.started, owner.finishes[0])
	}
	// Root-local binding cleanup is distinct from the owner's persistence policy.
	if len(root.scopeByRef) != 0 {
		t.Fatalf("abandoned scope retained %d bindings", len(root.scopeByRef))
	}
	return owner.finishes[0]
}

func TestCombinedServiceResumeInputWithoutSource(t *testing.T) {
	t.Parallel()
	root := NewCombinedService(nil, nil, nil, nil, nil, nil, nil, staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(*combinedService)
	if _, err := root.LoadResumeInput(recordings.LoadResumeInputRequest{Path: "missing.json"}); !errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("LoadResumeInput without a replay source = %v, want ErrMissingReplayArtifact", err)
	}
}

func TestLifecycleOwnerRejectsDetachedInputs(t *testing.T) {
	t.Parallel()

	lifecycle := recordinglifecyclewire.NewService(nil, nil, nil, staticRecordingClock{})
	if result, err := lifecycle.StartRecording(recordings.StartRecordingRequest{}); err != nil || !reflect.DeepEqual(result, recordings.StartRecordingResult{}) {
		t.Fatalf("disabled lifecycle Begin = (%#v, %v), want zero result", result, err)
	}
	if _, err := lifecycle.StartRecording(recordings.StartRecordingRequest{Enabled: true}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("lifecycle Begin without target = %v, want INVALID_TARGET", err)
	}
	if _, err := lifecycle.BindRecording(recordings.BindRecordingRequest{RecordingID: "adapter-invalid"}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("lifecycle Bind without target = %v, want INVALID_TARGET", err)
	}
}

func TestLifecycleOwnerCompletesBeginStopFinish(t *testing.T) {
	t.Parallel()

	var persisted recordings.RecordingSnapshot
	lifecycle := recordinglifecyclewire.NewService(nil,
		func(_ string, snapshot recordings.RecordingSnapshot) error {
			persisted = snapshot
			return nil
		}, nil, staticRecordingClock{})
	begin, err := lifecycle.StartRecording(recordings.StartRecordingRequest{
		Enabled:     true,
		RecordingID: "adapter-begin",
		Target:      recordings.RecordingTargetRequest{Artifact: "artifact:adapter-begin"},
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "adapter-session"},
	})
	if err != nil || begin.Status.RecordingID != "adapter-begin" {
		t.Fatalf("lifecycle Begin success = (%#v, %v)", begin, err)
	}
	if _, err := lifecycle.StopRecording(recordings.StopRecordingRequest{RecordingID: begin.Status.RecordingID}); err != nil {
		t.Fatalf("lifecycle Stop after Begin: %v", err)
	}
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	finished, err := lifecycle.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: begin.Status.RecordingID,
		FinishedAt:  at,
	})
	if err != nil || finished.Status.State != recordings.RecordingFinalized {
		t.Fatalf("lifecycle Finish after Begin = (%#v, %v), want finalized", finished, err)
	}
	if finished.Status.FinalizedAt == nil || !finished.Status.FinalizedAt.Equal(at) ||
		persisted.Status.FinalizedAt == nil || !persisted.Status.FinalizedAt.Equal(at) ||
		string(persisted.Status.RecordingID) != string(begin.Status.RecordingID) {
		t.Fatalf("lifecycle terminal metadata = %#v, persisted = %#v, want %v", finished.Status, persisted.Status, at)
	}
}

func TestLifecycleOwnerBindsAndReportsFailure(t *testing.T) {
	t.Parallel()

	lifecycle := recordinglifecyclewire.NewService(nil, nil, nil, staticRecordingClock{})
	bound, err := lifecycle.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "adapter-bind",
		Artifact:    "artifact:adapter-bind",
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "adapter-session"},
	})
	if err != nil || bound.Status.RecordingID != "adapter-bind" {
		t.Fatalf("lifecycle Bind = (%#v, %v)", bound, err)
	}
	event := nativeLifecycleEvent()
	if _, err := lifecycle.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("lifecycle AppendEvent = %v", err)
	}
	invalidEvent := event
	invalidEvent.Payload = "not-json"
	if _, err := lifecycle.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       invalidEvent,
	}); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
		t.Fatalf("lifecycle AppendEvent invalid payload = %v, want INVALID_EVENT", err)
	}
	if _, err := lifecycle.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: bound.Status.RecordingID,
		Failure:     recordings.RecordingFailure{Code: "", Message: ""},
	}); !errors.Is(err, recordings.ErrInvalidRecordingFailure) {
		t.Fatalf("lifecycle RecordFailure without facts = %v, want INVALID_FAILURE", err)
	}
	if _, err := lifecycle.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: bound.Status.RecordingID,
		Failure: recordings.RecordingFailure{
			Code:    "adapter_failed",
			Message: "adapter failure",
		},
	}); err != nil {
		t.Fatalf("lifecycle RecordFailure = %v", err)
	}
	if flushed, err := lifecycle.FlushRecording(recordings.FlushRecordingRequest{RecordingID: bound.Status.RecordingID}); err != nil || flushed.Status.FlushedThrough == nil {
		t.Fatalf("lifecycle Flush = (%#v, %v), want flushed cursor", flushed, err)
	}
	if status, err := lifecycle.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: bound.Status.RecordingID}); err != nil || status.Status.State != recordings.RecordingFailed {
		t.Fatalf("lifecycle Status before Finish = (%#v, %v), want failed", status, err)
	}
}

func TestLifecycleOwnerRejectsTerminalAppendAfterFailedFinish(t *testing.T) {
	t.Parallel()

	lifecycle := recordinglifecyclewire.NewService(nil, nil, nil, staticRecordingClock{})
	bound, err := lifecycle.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "adapter-terminal",
		Artifact:    "artifact:adapter-terminal",
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "adapter-session"},
	})
	if err != nil {
		t.Fatalf("lifecycle Bind: %v", err)
	}
	event := nativeLifecycleEvent()
	cause := errors.New("adapter failure")
	if _, err := lifecycle.RecordRecordingEvent(recordings.RecordRecordingEventRequest{RecordingID: bound.Status.RecordingID, Event: event}); err != nil {
		t.Fatalf("lifecycle AppendEvent: %v", err)
	}
	if _, err := lifecycle.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: bound.Status.RecordingID,
		Failure:     recordings.RecordingFailure{Code: "adapter_failed", Message: "adapter failure"},
		Cause:       cause,
	}); err != nil {
		t.Fatalf("lifecycle RecordFailure: %v", err)
	}
	finished, err := lifecycle.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Date(2026, 8, 26, 12, 2, 0, 0, time.UTC),
	})
	if err == nil || !errors.Is(err, cause) || finished.Status.State != recordings.RecordingFailed {
		t.Fatalf("lifecycle Finish failed recording = (%#v, %v), want failed write outcome", finished, err)
	}
	if _, err := lifecycle.RecordRecordingEvent(recordings.RecordRecordingEventRequest{RecordingID: bound.Status.RecordingID, Event: event}); !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("lifecycle AppendEvent after Finish = %v, want TERMINAL", err)
	}
	if status, err := lifecycle.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: bound.Status.RecordingID}); err != nil || status.Status.FinalizedAt == nil {
		t.Fatalf("lifecycle Status after Finish = (%#v, %v), want finalized timestamp", status, err)
	}
}

func nativeLifecycleEvent() recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:       "adapter-event",
		Sequence: 0,
		Scope:    recordings.CanonicalEventScope{FactorySessionID: "adapter-session"},
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "adapter-generation",
			Sequence:           0,
		},
		RecordedAt: time.Date(2026, 8, 26, 12, 1, 0, 0, time.UTC),
		Kind:       "WORK_REQUEST",
		Payload:    "{}",
	}
}

func TestRuntimeLedgerRouterPublishesCallbacksAndRoutesOptionalProvenance(t *testing.T) {
	t.Parallel()

	var nilRouter *runtimeLedgerRouter
	nilRouter.AddEventRecorder(nil)
	nilRouter.AddEventTypeRecorder(nil)
	nilRouter.AppendRecordedEvent(factorydefinitions.FactoryEvent{})
	if _, err := nilRouter.AppendRecordedEventWithValidation(factorydefinitions.FactoryEvent{}, nil); err == nil {
		t.Fatal("nil router append returned nil error")
	}
	if nilRouter.SecretProvenanceForEvent(factorydefinitions.FactoryEvent{}) != nil || nilRouter.SecretProvenanceDuringAppend(factorydefinitions.FactoryEvent{}) != nil {
		t.Fatal("nil router returned provenance")
	}

	router := newRuntimeLedgerRouter(func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) })
	var recorded int
	var recordedTypes int
	router.AddEventRecorder(func(factorydefinitions.FactoryEvent) { recorded++ })
	router.AddEventTypeRecorder(func(factorydefinitions.FactoryEventType) { recordedTypes++ })
	event := factorydefinitions.FactoryEvent{
		Id:      "router-event",
		Type:    factorydefinitions.FactoryEventTypeWorkRequest,
		Payload: json.RawMessage(`{}`),
		Context: factorydefinitions.FactoryEventContext{EventTime: time.Date(2026, 8, 26, 12, 1, 0, 0, time.UTC)},
	}
	router.AppendRecordedEvent(event)
	if recorded != 1 || recordedTypes != 1 {
		t.Fatalf("router callbacks = (%d, %d), want one event and one type callback", recorded, recordedTypes)
	}
	if len(router.CanonicalEvents()) != 1 {
		t.Fatalf("router canonical events = %d, want one", len(router.CanonicalEvents()))
	}
	if _, err := router.AppendRecordedEventWithValidation(event, func(factorydefinitions.FactoryEvent) error { return nil }); err != nil {
		t.Fatalf("router validated append: %v", err)
	}
	if router.SecretProvenanceForEvent(event) != nil || router.SecretProvenanceDuringAppend(event) != nil {
		t.Fatal("router returned unexpected provenance for event without declared secrets")
	}
	if generation := router.StreamGenerationID(); generation != "recordings-root" {
		t.Fatalf("router stream generation = %q, want recordings-root", generation)
	}
}

var _ recordings.Service = (*combinedService)(nil)
var _ recordings.RuntimeScopeService = (*combinedService)(nil)

type unavailableHistoricalOwner struct{}

func (unavailableHistoricalOwner) QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
	return recordings.HistoricalRecordingQueryResult{}, &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorUnavailable}
}

func (owner unavailableHistoricalOwner) ReadHistoricalEvents(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
	return owner.QueryHistoricalRecording(request)
}

func (owner unavailableHistoricalOwner) DecodeHistoricalEvents(request recordings.HistoricalRecordingQueryRequest, _ []byte) (recordings.HistoricalRecordingQueryResult, error) {
	return owner.QueryHistoricalRecording(request)
}

type injectedCanonicalOwner struct {
	canonicalledger.Service
	err error
}

func (owner injectedCanonicalOwner) Append(recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error) {
	return recordings.AppendRecordedEventResult{}, owner.err
}

type injectedLifecycleOwner struct {
	recordinglifecycle.Service
	err error
}

func (owner injectedLifecycleOwner) StartRecording(recordings.StartRecordingRequest) (recordings.StartRecordingResult, error) {
	return recordings.StartRecordingResult{}, owner.err
}

type injectedArtifactsOwner struct {
	artifactsexport.Service
	err error
}

func (owner injectedArtifactsOwner) BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
	return recordings.BuildPortableArtifactResult{}, owner.err
}

type injectedReplayOwner struct {
	recordingsreplay.Service
	err error
}

func (owner injectedReplayOwner) CreateReplayPlan(recordings.CreateReplayPlanRequest) (recordings.CreateReplayPlanResult, error) {
	return recordings.CreateReplayPlanResult{}, owner.err
}

func TestCombinedServiceForwardsCompletedOwnerFailures(t *testing.T) {
	t.Parallel()
	canonicalErr := errors.New("injected canonical admission failure")
	lifecycleErr := errors.New("injected lifecycle selection failure")
	artifactsErr := errors.New("injected portable artifact failure")
	replayErr := errors.New("injected replay plan failure")
	service := NewCombinedService(nil, nil, injectedLifecycleOwner{err: lifecycleErr}, injectedArtifactsOwner{err: artifactsErr},
		injectedReplayOwner{err: replayErr}, injectedCanonicalOwner{err: canonicalErr}, unavailableHistoricalOwner{},
		staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil)
	_, err := service.Append(recordings.AppendRecordedEventRequest{})
	if !errors.Is(err, canonicalErr) {
		t.Fatalf("Append error = %v, want injected canonical cause", err)
	}
	_, err = service.StartRecording(recordings.StartRecordingRequest{})
	if !errors.Is(err, lifecycleErr) {
		t.Fatalf("StartRecording error = %v, want injected lifecycle cause", err)
	}
	_, err = service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{})
	if !errors.Is(err, artifactsErr) {
		t.Fatalf("BuildPortableArtifact error = %v, want injected artifacts cause", err)
	}
	_, err = service.CreateReplayPlan(recordings.CreateReplayPlanRequest{})
	if !errors.Is(err, replayErr) {
		t.Fatalf("CreateReplayPlan error = %v, want injected replay cause", err)
	}
	_, err = service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{})
	var historicalErr *recordings.HistoricalRecordingQueryError
	if !errors.As(err, &historicalErr) || historicalErr.Kind != recordings.HistoricalRecordingQueryErrorUnavailable {
		t.Fatalf("QueryHistoricalRecording error = %v, want owner unavailable cause", err)
	}
}
