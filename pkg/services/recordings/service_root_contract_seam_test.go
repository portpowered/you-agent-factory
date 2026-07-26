package recordings_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestServiceRootContract_FakeImplementsAndExercisesSeam(t *testing.T) {
	t.Parallel()

	fake := &peerRootServiceFake{
		streamGenerationID: "generation-empty",
		subscribeErr:       recordings.ErrReconnectCursorNotFound,
	}

	var service recordings.Service = fake
	ctx := context.Background()

	_, err := service.SubscribeFrom(ctx, recordings.SubscribeRequest{})
	if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("SubscribeFrom error = %v, want ErrReconnectCursorNotFound", err)
	}

	state, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{})
	if err != nil {
		t.Fatalf("ReconstructWorldState: %v", err)
	}
	if state.WorldState.SchemaVersion != recordings.WorldStateViewSchemaV1 {
		t.Fatalf("ReconstructWorldState = %#v, want detached V1 world state", state)
	}

	dashboard, err := service.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: state.WorldState,
	})
	if err != nil || dashboard.Data.InFlightDispatchCount != 0 {
		t.Fatalf("QuerySimpleDashboard = %#v, error = %v", dashboard, err)
	}

	if _, err := service.BindRecording(recordings.BindRecordingRequest{}); !errors.Is(
		err,
		recordings.ErrMissingRecordingTarget,
	) {
		t.Fatalf("BindRecording error = %v, want ErrMissingRecordingTarget", err)
	}
}

func TestAppendSubscribeRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	fake := &peerRootServiceFake{streamGenerationID: "generation-append-subscribe"}
	var service recordings.Service = fake
	ctx := context.Background()

	first := rootAppendEvent("event-1", "WORK_REQUEST")
	second := rootAppendEvent("event-2", "WORK_STATE_CHANGE")
	assertFakeInvalidAppendsDoNotMutate(t, service, fake, first)
	acceptedFirst, err := service.Append(recordings.AppendRecordedEventRequest{Event: first})
	if err != nil {
		t.Fatalf("Append first valid event: %v", err)
	}
	if _, err := service.Append(recordings.AppendRecordedEventRequest{Event: second}); err != nil {
		t.Fatalf("Append second valid event: %v", err)
	}

	cursor := acceptedFirst.Event.Cursor
	result, err := service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom success path: %v", err)
	}
	outcome := result.Subscription.Next(ctx)
	if outcome.Kind != recordings.SubscriptionEvent || outcome.Event.ID != "event-2" {
		t.Fatalf("SubscribeFrom outcome = %#v, want ordered event-2", outcome)
	}

	stale := recordings.CanonicalEventCursor{
		StreamGenerationID: "generation-append-subscribe",
		Sequence:           99,
	}
	_, err = service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &stale,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("stale cursor error = %v, want ErrReconnectCursorExpired", err)
	}

	invalid := recordings.CanonicalEventCursor{Sequence: 0}
	_, err = service.SubscribeFrom(ctx, recordings.SubscribeRequest{Cursor: &invalid})
	if !errors.Is(err, recordings.ErrInvalidReconnectCursor) {
		t.Fatalf("invalid cursor error = %v, want ErrInvalidReconnectCursor", err)
	}

	unavailable := recordings.CanonicalEventCursor{
		StreamGenerationID: "replaced-generation",
		Sequence:           0,
	}
	_, err = service.SubscribeFrom(ctx, recordings.SubscribeRequest{Cursor: &unavailable})
	if !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("unavailable cursor error = %v, want ErrReconnectCursorUnavailable", err)
	}

	_, err = service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "   "},
	})
	if !errors.Is(err, recordings.ErrInvalidSubscribeScope) {
		t.Fatalf("invalid scope error = %v, want ErrInvalidSubscribeScope", err)
	}
	if errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("invalid scope must remain distinct from ErrReconnectCursorNotFound")
	}

	gap := recordings.EventSubscription((&peerEventSubscription{outcomes: []recordings.SubscriptionOutcome{{
		Kind: recordings.SubscriptionGap,
		Gap: &recordings.SubscriptionGapFacts{
			Cause:            recordings.SubscriptionSequenceDiscontinuity,
			ExpectedSequence: 2,
			ObservedSequence: 4,
			ReconnectFrom:    cursor,
		},
	}}}).Next).Next(ctx)
	if gap.Kind != recordings.SubscriptionGap || gap.Gap == nil ||
		gap.Gap.ReconnectFrom != cursor {
		t.Fatalf("slow-subscriber outcome = %#v, want explicit reconnectable gap", gap)
	}
}

func assertFakeInvalidAppendsDoNotMutate(
	t *testing.T,
	service recordings.Service,
	fake *peerRootServiceFake,
	valid recordings.CanonicalEvent,
) {
	t.Helper()
	invalid := []recordings.CanonicalEvent{
		func() recordings.CanonicalEvent { event := valid; event.ID = ""; return event }(),
		func() recordings.CanonicalEvent { event := valid; event.Kind = ""; return event }(),
		func() recordings.CanonicalEvent { event := valid; event.RecordedAt = time.Time{}; return event }(),
		func() recordings.CanonicalEvent {
			event := valid
			event.Scope.FactorySessionID = "   "
			return event
		}(),
		func() recordings.CanonicalEvent {
			event := valid
			event.Payload = `{"incomplete":`
			return event
		}(),
	}
	for _, event := range invalid {
		if _, err := service.Append(recordings.AppendRecordedEventRequest{Event: event}); !errors.Is(
			err,
			recordings.ErrInvalidAppendEvent,
		) {
			t.Fatalf("Append invalid event error = %v, want ErrInvalidAppendEvent", err)
		}
		if len(fake.events) != 0 {
			t.Fatalf("Append invalid event mutated fake history: %#v", fake.events)
		}
	}
}

func rootAppendEvent(id string, kind recordings.CanonicalEventKind) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:         recordings.CanonicalEventID(id),
		Scope:      recordings.CanonicalEventScope{FactorySessionID: "session-1"},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       kind,
		Payload:    `{}`,
	}
}

func TestProjectionQueryRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	fake := &peerRootServiceFake{
		dashboardData: recordings.SimpleDashboardRenderData{
			InFlightDispatchCount: 2,
		},
		workstationRequests: recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{},
		validateReplayErr:   recordings.ErrReconnectCursorNotFound,
	}
	var service recordings.Service = fake

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}
	events := []recordings.CanonicalEvent{
		projectionEvent("event-1", 0, scope, "WORK_REQUEST"),
		projectionEvent("event-2", 1, scope, "WORK_STATE_CHANGE"),
	}
	world, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope:        scope,
		Events:       events,
		SelectedTick: 7,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState success path: %v", err)
	}
	if world.WorldState.SelectedTick != 7 ||
		world.WorldState.SchemaVersion != recordings.WorldStateViewSchemaV1 {
		t.Fatalf("ReconstructWorldState view = %#v, want detached tick 7 V1 view", world.WorldState)
	}

	dashboard, err := service.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: world.WorldState,
	})
	if err != nil {
		t.Fatalf("QuerySimpleDashboard: %v", err)
	}
	if dashboard.Data.InFlightDispatchCount != 2 {
		t.Fatalf("QuerySimpleDashboard = %#v, want InFlightDispatchCount 2", dashboard.Data)
	}

	workstation, err := service.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{
		WorldState: world.WorldState,
	})
	if err != nil {
		t.Fatalf("QueryWorkstationRequests: %v", err)
	}
	if workstation.Projection.WorkstationRequestsByDispatchId != nil &&
		len(*workstation.Projection.WorkstationRequestsByDispatchId) != 0 {
		t.Fatalf("QueryWorkstationRequests = %#v, want empty detached projection", workstation.Projection)
	}

	if err := service.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: nil,
		Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "generation-1"},
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("invalid reconnect validation error = %v, want ErrReconnectCursorNotFound", err)
	}

	_, err = service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       events,
		SelectedTick: -1,
	})
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("malformed projection input error = %v, want ErrInvalidProjectionInput", err)
	}
	if errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("malformed projection input must remain distinct from ErrReconnectCursorNotFound")
	}
}

func projectionEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
	kind recordings.CanonicalEventKind,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(id),
		Sequence:    sequence,
		FactoryTick: 7,
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-1",
			Sequence:           sequence,
		},
		Kind:    kind,
		Payload: `{}`,
	}
}
