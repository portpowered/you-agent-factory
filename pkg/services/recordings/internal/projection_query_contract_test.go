package internal

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestProjectionQueries_AreEquivalentForRetainedAndReplayedCanonicalFacts(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := NewService(ledger, NewProjectionService())
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-query"}
	retained := appendProjectionFacts(t, svc, scope)

	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: ledger.StreamGenerationID(),
		History:            ledger.CanonicalEvents(),
	}
	replayed := collectProjectionFacts(t, svc, scope, len(retained))
	if len(replayed) != 2 || replayed[0].Sequence != 0 || replayed[1].Sequence != 2 {
		t.Fatalf("scoped replay order = %#v, want global positions 0 and 2", replayed)
	}

	retainedView := reconstructProjectionView(t, svc, scope, retained)
	replayedView := reconstructProjectionView(t, svc, scope, replayed)
	if retainedView != replayedView {
		t.Fatalf("retained view != replayed view:\nretained=%#v\nreplayed=%#v", retainedView, replayedView)
	}

	retainedDashboard, err := svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: retainedView,
	})
	if err != nil {
		t.Fatalf("QuerySimpleDashboard retained: %v", err)
	}
	replayedDashboard, err := svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: replayedView,
	})
	if err != nil {
		t.Fatalf("QuerySimpleDashboard replayed: %v", err)
	}
	if !reflect.DeepEqual(retainedDashboard, replayedDashboard) {
		t.Fatalf("retained dashboard != replayed dashboard")
	}

	retainedDashboard.Data.PlaceTokenCounts = map[string]int{"caller-only": 1}
	again, err := svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: retainedView,
	})
	if err != nil {
		t.Fatalf("QuerySimpleDashboard detached check: %v", err)
	}
	if again.Data.PlaceTokenCounts["caller-only"] != 0 {
		t.Fatalf("caller mutation leaked into later query: %#v", again.Data.PlaceTokenCounts)
	}

	assertScopedReplayEquivalent(t, svc, scope, replayed, replayedView)
	assertScopedPortableArtifact(t, svc, scope, replayed)
}

func TestProjectionQueries_RejectInvalidScopeOrderAndView(t *testing.T) {
	t.Parallel()

	svc := NewService(&stubLedger{}, NewProjectionService())
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-query"}
	first := canonicalProjectionFact("event-1", 0, scope)

	wrongScope := first
	wrongScope.Scope.FactorySessionID = "other-session"
	if _, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope: scope, Events: []recordings.CanonicalEvent{wrongScope},
	}); !errors.Is(err, recordings.ErrInvalidProjectionScope) {
		t.Fatalf("wrong scope error = %v, want ErrInvalidProjectionScope", err)
	}

	duplicate := canonicalProjectionFact("event-2", 0, scope)
	if _, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope: scope, Events: []recordings.CanonicalEvent{first, duplicate},
	}); !errors.Is(err, recordings.ErrMalformedProjectionOrder) {
		t.Fatalf("duplicate order error = %v, want ErrMalformedProjectionOrder", err)
	}

	if _, err := svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: recordings.WorldStateView{SchemaVersion: "future"},
	}); !errors.Is(err, recordings.ErrUnsupportedProjectionView) {
		t.Fatalf("unsupported view error = %v, want ErrUnsupportedProjectionView", err)
	}
}

func appendProjectionFacts(
	t *testing.T,
	svc recordings.Service,
	scope recordings.CanonicalEventScope,
) []recordings.CanonicalEvent {
	t.Helper()
	facts := []recordings.CanonicalEvent{
		canonicalProjectionFact("event-1", 0, scope),
		canonicalProjectionFact("event-2", 1, scope),
	}
	first, err := svc.Append(recordings.AppendRecordedEventRequest{Event: facts[0]})
	if err != nil {
		t.Fatalf("Append first projection fact: %v", err)
	}
	facts[0] = first.Event
	otherScope := recordings.CanonicalEventScope{FactorySessionID: "session-other"}
	if _, err := svc.Append(recordings.AppendRecordedEventRequest{
		Event: canonicalProjectionFact("other-event", 0, otherScope),
	}); err != nil {
		t.Fatalf("Append interleaved projection fact: %v", err)
	}
	second, err := svc.Append(recordings.AppendRecordedEventRequest{Event: facts[1]})
	if err != nil {
		t.Fatalf("Append second projection fact: %v", err)
	}
	facts[1] = second.Event
	return facts
}

func collectProjectionFacts(
	t *testing.T,
	svc recordings.Service,
	scope recordings.CanonicalEventScope,
	count int,
) []recordings.CanonicalEvent {
	t.Helper()
	result, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{Scope: scope})
	if err != nil {
		t.Fatalf("SubscribeFrom replay history: %v", err)
	}
	facts := make([]recordings.CanonicalEvent, 0, count)
	for range count {
		outcome := result.Subscription.Next(context.Background())
		if outcome.Kind != recordings.SubscriptionEvent {
			t.Fatalf("subscription outcome = %#v, want event", outcome)
		}
		facts = append(facts, outcome.Event)
	}
	return facts
}

func reconstructProjectionView(
	t *testing.T,
	svc recordings.Service,
	scope recordings.CanonicalEventScope,
	events []recordings.CanonicalEvent,
) recordings.WorldStateView {
	t.Helper()
	result, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope: scope, Events: events, SelectedTick: 4,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState: %v", err)
	}
	return result.WorldState
}

func canonicalProjectionFact(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(id),
		Sequence:    sequence,
		FactoryTick: int(sequence) + 1,
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "gen-1",
			Sequence:           sequence,
		},
		Kind:    recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunResponse),
		Payload: `{}`,
		RecordedAt: time.Unix(
			1_700_000_000+int64(sequence),
			0,
		).UTC(),
	}
}

func assertScopedReplayEquivalent(
	t *testing.T,
	svc recordings.Service,
	scope recordings.CanonicalEventScope,
	events []recordings.CanonicalEvent,
	want recordings.WorldStateView,
) {
	t.Helper()
	expected := events[len(events)-1].Cursor
	planned, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: "recording-interleaved-replay",
			Scope:       scope,
			Events:      events,
		},
		ExpectedThrough: &expected,
		SelectedTick:    want.SelectedTick,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan interleaved scope: %v", err)
	}
	var observed recordings.ObserveReplayResult
	for range events {
		observed, err = svc.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
		if err != nil {
			t.Fatalf("ObserveReplay interleaved scope: %v", err)
		}
	}
	if observed.Observation.Kind != recordings.ReplayCompleted ||
		observed.Observation.WorldState != want {
		t.Fatalf("scoped replay observation = %#v, want completed equivalent view", observed.Observation)
	}
}

func assertScopedPortableArtifact(
	t *testing.T,
	svc recordings.Service,
	scope recordings.CanonicalEventScope,
	events []recordings.CanonicalEvent,
) {
	t.Helper()
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-interleaved-export",
		Artifact:    "artifact:interleaved-export",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording interleaved export: %v", err)
	}
	for _, event := range events {
		if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID,
			Event:       event,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent interleaved export: %v", err)
		}
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording interleaved export: %v", err)
	}
	built, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact interleaved scope: %v", err)
	}
	if _, err := svc.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: built.Artifact,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact interleaved scope: %v", err)
	}
	if len(built.Artifact.Events) != len(events) ||
		built.Artifact.Events[0].Sequence != events[0].Sequence ||
		built.Artifact.Events[len(events)-1].Sequence != events[len(events)-1].Sequence {
		t.Fatalf("portable scoped order = %#v, want preserved global order", built.Artifact.Events)
	}
}
