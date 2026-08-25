package internal

import (
	"context"
	"errors"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordingsreplay "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay"
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

type scopedQueryFixture struct {
	root        recordings.Service
	ref         recordings.RecordingScopeRef
	recordingID recordings.RecordingID
	eventScope  recordings.CanonicalEventScope
	events      []recordings.CanonicalEvent
	exported    recordings.ExportPortableArtifactScopeResult
}

type openedScopedQuery struct {
	ref   recordings.RecordingScopeRef
	scope recordings.CanonicalEventScope
}

func TestRecordingScopeReplayProjectionInspectionAndArtifactQueries(t *testing.T) {
	t.Parallel()

	fixture := newFinalizedQueryFixture(t)
	assertHistoricalReplayFacts(t, fixture)
	assertHistoricalSubscription(t, fixture)
	assertHistoricalProjections(t, fixture)
	assertHistoricalReplayPlan(t, fixture)
	assertHistoricalArtifacts(t, fixture)
}

func newFinalizedQueryFixture(t *testing.T) *scopedQueryFixture {
	t.Helper()
	root := newScopedQueryRoot(t)
	eventScope := recordings.CanonicalEventScope{FactorySessionID: "history-scope"}
	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-history-scope",
		Artifact:    recordings.RecordingArtifactReference(filepath.Join(t.TempDir(), "history.json")),
		Scope:       eventScope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	events := []recordings.CanonicalEvent{
		scopedScopeEvent("history-event-1", 0, eventScope),
		scopedScopeEvent("history-event-2", 1, eventScope),
	}
	for index, event := range events {
		if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID,
			Event:       event,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent[%d]: %v", index, err)
		}
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	opened, err := root.OpenRecordingScope(context.Background(), recordings.OpenRecordingScopeRequest{
		RecordingID: bound.Status.RecordingID,
		Scope:       eventScope,
	})
	if err != nil {
		t.Fatalf("OpenRecordingScope: %v", err)
	}
	if opened.Scope.IsZero() || opened.Status.State != recordings.RecordingFinalized {
		t.Fatalf("opened scope = %#v, want finalized opaque scope", opened)
	}
	return &scopedQueryFixture{
		root:        root,
		ref:         opened.Scope,
		recordingID: bound.Status.RecordingID,
		eventScope:  eventScope,
		events:      events,
	}
}

func assertHistoricalReplayFacts(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	loaded, err := fixture.root.LoadReplayRecordingScope(context.Background(), recordings.LoadReplayRecordingScopeRequest{
		Scope: fixture.ref,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecordingScope: %v", err)
	}
	if len(loaded.Recording.Events) != len(fixture.events) ||
		loaded.Recording.Events[0].ID != fixture.events[0].ID ||
		loaded.Recording.Events[1].Cursor != fixture.events[1].Cursor {
		t.Fatalf("loaded replay = %#v, want detached ordered history", loaded.Recording)
	}
	loaded.Recording.Events[0].Payload = `{"mutated":true}`
	loadedAgain, err := fixture.root.LoadReplayRecordingScope(context.Background(), recordings.LoadReplayRecordingScopeRequest{
		Scope: fixture.ref,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecordingScope after detached mutation: %v", err)
	}
	if loadedAgain.Recording.Events[0].Payload == `{"mutated":true}` {
		t.Fatal("scope replay returned mutable lifecycle-owned event data")
	}
}

func assertHistoricalSubscription(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	subscription, err := fixture.root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope: fixture.ref,
	})
	if err != nil {
		t.Fatalf("SubscribeRecordingScope: %v", err)
	}
	for index, want := range fixture.events {
		outcome := subscription.Subscription(context.Background())
		if outcome.Kind != recordings.SubscriptionEvent || outcome.Event.ID != want.ID {
			t.Fatalf("historical subscription[%d] = %#v, want %s", index, outcome, want.ID)
		}
	}
	if outcome := subscription.Subscription(context.Background()); outcome.Kind != recordings.SubscriptionClosed {
		t.Fatalf("historical subscription terminal outcome = %#v, want closed", outcome)
	}

	cursor := fixture.events[0].Cursor
	continued, err := fixture.root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope:  fixture.ref,
		Cursor: &cursor,
	})
	if err != nil {
		t.Fatalf("historical subscription from cursor: %v", err)
	}
	if outcome := continued.Subscription(context.Background()); outcome.Kind != recordings.SubscriptionEvent ||
		outcome.Event.ID != fixture.events[1].ID {
		t.Fatalf("historical cursor continuation = %#v, want second event", outcome)
	}
}

func assertHistoricalProjections(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	through := fixture.events[0].Cursor
	projected, err := fixture.root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope:        fixture.ref,
		Through:      &through,
		SelectedTick: 4,
	})
	if err != nil {
		t.Fatalf("ReconstructRecordingScope: %v", err)
	}
	if projected.WorldState.Scope != fixture.eventScope ||
		projected.WorldState.Through != through || projected.WorldState.SelectedTick != 4 {
		t.Fatalf("scope projection = %#v, want coherent first-event prefix", projected.WorldState)
	}
	dashboard, err := fixture.root.QuerySimpleDashboardScope(context.Background(), recordings.QuerySimpleDashboardScopeRequest{
		Scope: fixture.ref, SelectedTick: 4,
	})
	if err != nil || dashboard.WorldState.Scope != fixture.eventScope {
		t.Fatalf("dashboard query = %#v, error %v", dashboard, err)
	}
	workstation, err := fixture.root.QueryWorkstationRequestsScope(context.Background(), recordings.QueryWorkstationRequestsScopeRequest{
		Scope: fixture.ref, SelectedTick: 4,
	})
	if err != nil || workstation.WorldState.Scope != fixture.eventScope {
		t.Fatalf("workstation query = %#v, error %v", workstation, err)
	}
}

func assertHistoricalReplayPlan(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	planned, err := fixture.root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{
		Scope:         fixture.ref,
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		SelectedTick:  4,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlanScope: %v", err)
	}
	var observed recordings.ObserveReplayScopeResult
	for range fixture.events {
		observed, err = fixture.root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{
			Scope: fixture.ref, Plan: planned.Plan.Handle,
		})
		if err != nil {
			t.Fatalf("ObserveReplayScope: %v", err)
		}
	}
	if observed.Observation.Kind != recordings.ReplayCompleted ||
		observed.Observation.WorldState.Scope != fixture.eventScope {
		t.Fatalf("replay observation = %#v, want completed scoped projection", observed.Observation)
	}
}

func assertHistoricalArtifacts(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	built, err := fixture.root.BuildPortableArtifactScope(context.Background(), recordings.BuildPortableArtifactScopeRequest{
		Scope: fixture.ref,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifactScope: %v", err)
	}
	if len(built.Artifact.Events) != len(fixture.events) || built.Artifact.Summary.Scope != fixture.eventScope {
		t.Fatalf("scoped artifact = %#v, want ordered artifact for event scope", built.Artifact)
	}
	exported, err := fixture.root.ExportPortableArtifactScope(context.Background(), recordings.ExportPortableArtifactScopeRequest{
		Scope: fixture.ref,
	})
	if err != nil {
		t.Fatalf("ExportPortableArtifactScope: %v", err)
	}
	fixture.exported = exported
	read, err := fixture.root.ReadPortableArtifactScope(context.Background(), recordings.ReadPortableArtifactScopeRequest{
		Scope: fixture.ref, Reference: exported.Reference,
	})
	if err != nil {
		t.Fatalf("ReadPortableArtifactScope: %v", err)
	}
	if read.Artifact.Integrity != exported.Artifact.Integrity || read.Artifact.Summary.Scope != fixture.eventScope {
		t.Fatalf("scoped artifact read = %#v, want exported detached artifact", read.Artifact)
	}
}

func TestRecordingScopeOpenRejectsInvalidSelections(t *testing.T) {
	t.Parallel()

	fixture := newFinalizedQueryFixture(t)
	activeRoot := newScopedQueryRoot(t)
	active, err := activeRoot.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "active-recording",
		Artifact:    "recording://active",
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "active-scope"},
	})
	if err != nil {
		t.Fatalf("BindRecording active: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	assertScopeOpenError(t, "canceled", fixture.root, canceled, recordings.OpenRecordingScopeRequest{}, context.Canceled)
	assertScopeOpenError(t, "whitespace scope", fixture.root, context.Background(), recordings.OpenRecordingScopeRequest{
		RecordingID: "missing", Scope: recordings.CanonicalEventScope{FactorySessionID: "   "},
	}, recordings.ErrInvalidRecordingScope)
	assertScopeOpenError(t, "unknown recording", fixture.root, context.Background(), recordings.OpenRecordingScopeRequest{
		RecordingID: "missing",
	}, recordings.ErrReplayRecordingNotFound)
	assertScopeOpenError(t, "active recording", activeRoot, context.Background(), recordings.OpenRecordingScopeRequest{
		RecordingID: active.Status.RecordingID,
	}, recordings.ErrReplayRecordingNotFinalized)
	assertScopeOpenError(t, "scope mismatch", fixture.root, context.Background(), recordings.OpenRecordingScopeRequest{
		RecordingID: fixture.recordingID,
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "other-scope"},
	}, recordings.ErrInvalidRecordingScope)
}

func assertScopeOpenError(
	t *testing.T,
	name string,
	root recordings.Service,
	ctx context.Context,
	request recordings.OpenRecordingScopeRequest,
	want error,
) {
	t.Helper()
	if _, err := root.OpenRecordingScope(ctx, request); !errors.Is(err, want) {
		t.Fatalf("%s error = %v, want %v", name, err, want)
	}
}

func TestRecordingScopeLiveSubscriptionAndCursorValidation(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	root := NewService(ledger, NewProjectionService())
	scope := recordings.CanonicalEventScope{FactorySessionID: "live-scope"}
	started, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true, Scope: scope, Target: recordings.RecordingTargetRequest{Artifact: "recording://live"},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope: %v", err)
	}
	first := appendScopedQueryEvent(t, root, started.Scope, "live-event-1", 0, scope)
	second := appendScopedQueryEvent(t, root, started.Scope, "live-event-2", 1, scope)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1", History: ledger.events,
	}
	all, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope: started.Scope,
	})
	if err != nil {
		t.Fatalf("SubscribeRecordingScope live: %v", err)
	}
	assertSubscriptionEvent(t, all.Subscription, first.Event.ID)
	assertSubscriptionEvent(t, all.Subscription, second.Event.ID)
	fromFirst, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope: started.Scope, Cursor: &first.Event.Cursor,
	})
	if err != nil {
		t.Fatalf("SubscribeRecordingScope from cursor: %v", err)
	}
	assertSubscriptionEvent(t, fromFirst.Subscription, second.Event.ID)
	assertLiveCursorErrors(t, root, started.Scope, first.Event.Cursor)
}

func TestRecordingScopeActiveBoundariesPreserveCancellationAndReadFailures(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	root := NewService(ledger, NewProjectionService())
	scope := recordings.CanonicalEventScope{FactorySessionID: "active-boundaries"}
	started, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   scope,
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://active-boundaries"},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope: %v", err)
	}
	assertActiveScopeReplayAndArtifactFailures(t, root, started.Scope)
	assertActiveScopeInvalidRequests(t, root)
	assertActiveScopeSubscriptionFailure(t, root, ledger, started.Scope)
	ledger.subscribeErr = nil
	assertActiveScopeCancellation(t, root, started.Scope)
	assertActiveScopeCloseAndQuery(t, root, started.Scope)
}

func assertActiveScopeReplayAndArtifactFailures(t *testing.T, root recordings.Service, scope recordings.RecordingScopeRef) {
	t.Helper()
	if _, err := root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{
		Scope: scope, SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing: recordings.ReplayTimingOrderOnly,
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFinalized) {
		t.Fatalf("CreateReplayPlanScope active = %v, want ErrReplayRecordingNotFinalized", err)
	}
	if _, err := root.BuildPortableArtifactScope(context.Background(), recordings.BuildPortableArtifactScopeRequest{
		Scope: scope,
	}); !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("BuildPortableArtifactScope without publication = %v, want ErrPortableArtifactUnavailable", err)
	}
	if _, err := root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{
		Scope: scope, Plan: "missing-plan",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("ObserveReplayScope missing plan = %v, want ErrReplayPlanNotFound", err)
	}
}

func assertActiveScopeInvalidRequests(t *testing.T, root recordings.Service) {
	t.Helper()
	if _, err := root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{
		Scope: recordings.RecordingScopeRef{}, Plan: "missing-plan",
	}); !errors.Is(err, recordings.ErrRecordingScopeInvalid) {
		t.Fatalf("ObserveReplayScope zero scope = %v, want ErrRecordingScopeInvalid", err)
	}
	if _, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{}); !errors.Is(err, recordings.ErrRecordingScopeInvalid) {
		t.Fatalf("SubscribeRecordingScope zero scope = %v, want ErrRecordingScopeInvalid", err)
	}
	if _, err := root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{}); !errors.Is(err, recordings.ErrRecordingScopeInvalid) {
		t.Fatalf("CreateReplayPlanScope zero scope = %v, want ErrRecordingScopeInvalid", err)
	}
	if _, err := root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{}); !errors.Is(err, recordings.ErrRecordingScopeInvalid) {
		t.Fatalf("ReconstructRecordingScope zero scope = %v, want ErrRecordingScopeInvalid", err)
	}
}

func assertActiveScopeSubscriptionFailure(t *testing.T, root recordings.Service, ledger *stubLedger, scope recordings.RecordingScopeRef) {
	t.Helper()
	validGeneration := recordings.CanonicalEventCursor{
		StreamGenerationID: ledger.StreamGenerationID(), Sequence: 0,
	}
	assertScopeSubscribeError(t, root, scope, validGeneration, recordings.ErrReconnectCursorExpired)
	foreignGeneration := validGeneration
	foreignGeneration.StreamGenerationID = "other-generation"
	assertScopeSubscribeError(t, root, scope, foreignGeneration, recordings.ErrReconnectCursorUnavailable)
	ledger.subscribeErr = errors.New("scope subscription unavailable")
	if _, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope: scope,
	}); !errors.Is(err, ledger.subscribeErr) {
		t.Fatalf("SubscribeRecordingScope ledger failure = %v, want %v", err, ledger.subscribeErr)
	}
}

func assertActiveScopeCancellation(t *testing.T, root recordings.Service, scope recordings.RecordingScopeRef) {
	t.Helper()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := root.ReconstructRecordingScope(canceled, recordings.ReconstructRecordingScopeRequest{
		Scope: scope,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReconstructRecordingScope canceled = %v, want context.Canceled", err)
	}
	if _, err := root.ObserveReplayScope(canceled, recordings.ObserveReplayScopeRequest{
		Scope: scope, Plan: "missing-plan",
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ObserveReplayScope canceled = %v, want context.Canceled", err)
	}
}

func assertActiveScopeCloseAndQuery(t *testing.T, root recordings.Service, scope recordings.RecordingScopeRef) {
	t.Helper()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	closed, err := root.CloseRecordingScope(canceled, recordings.CloseRecordingScopeRequest{
		Scope: scope, FinishedAt: time.Unix(1_700_000_400, 0).UTC(),
	})
	if !errors.Is(err, context.Canceled) || closed.Closed {
		t.Fatalf("CloseRecordingScope canceled = (%#v, %v), want unfinished canceled scope", closed, err)
	}
	closed, err = root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope: scope, FinishedAt: time.Unix(1_700_000_400, 0).UTC(),
	})
	if err != nil || !closed.Closed || closed.Status.State != recordings.RecordingFinalized {
		t.Fatalf("CloseRecordingScope retry = (%#v, %v), want finalized closed scope", closed, err)
	}
	if _, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{
		Scope: scope,
	}); !errors.Is(err, recordings.ErrRecordingScopeClosed) {
		t.Fatalf("QueryRecordingScope after close = %v, want ErrRecordingScopeClosed", err)
	}
}

func TestRecordingScopeDelegatesSnapshotAndReplayFailures(t *testing.T) {
	t.Parallel()

	root := newScopedQueryRoot(t).(*combinedService)
	active, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "snapshot-failures"},
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://snapshot-failures"},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope: %v", err)
	}
	root.Service = snapshotErrorLifecycle{
		Service: root.Service,
		Err:     recordings.ErrMissingRecordingTarget,
	}
	if _, err := root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope: active.Scope,
	}); !errors.Is(err, recordings.ErrRecordingScopeStale) {
		t.Fatalf("ReconstructRecordingScope stale = %v, want ErrRecordingScopeStale", err)
	}
	snapshotErr := errors.New("snapshot unavailable")
	root.Service = snapshotErrorLifecycle{Service: root.Service, Err: snapshotErr}
	if _, err := root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope: active.Scope,
	}); !errors.Is(err, snapshotErr) {
		t.Fatalf("ReconstructRecordingScope snapshot failure = %v, want delegated error", err)
	}
	if _, err := root.OpenRecordingScope(context.Background(), recordings.OpenRecordingScopeRequest{
		RecordingID: "snapshot-failure-open",
	}); !errors.Is(err, snapshotErr) || !strings.Contains(err.Error(), "snapshot unavailable") {
		t.Fatalf("OpenRecordingScope snapshot failure = %v, want delegated error", err)
	}

	fixture := newFinalizedQueryFixture(t)
	plan, err := fixture.root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{
		Scope: fixture.ref, SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing: recordings.ReplayTimingOrderOnly,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlanScope: %v", err)
	}
	finalizedRoot := fixture.root.(*combinedService)
	finalizedRoot.replayService = replayObservationErrorService{
		Service: finalizedRoot.replayService,
		Err:     errors.New("replay observation unavailable"),
	}
	if _, err := fixture.root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{
		Scope: fixture.ref, Plan: plan.Plan.Handle,
	}); err == nil || !strings.Contains(err.Error(), "replay observation unavailable") {
		t.Fatalf("ObserveReplayScope delegated failure = %v, want delegated error", err)
	}
}

func TestRecordingScopeRejectsForeignPortableArtifacts(t *testing.T) {
	t.Parallel()

	fixture := newFinalizedQueryFixture(t)
	root := fixture.root.(*combinedService)
	foreign := recordings.PortableArtifact{
		Summary: recordings.PortableArtifactSummary{
			Scope: recordings.CanonicalEventScope{FactorySessionID: "foreign-scope"},
		},
	}
	root.artifactsExport = foreignArtifactExportService{
		Service:  root.artifactsExport,
		Artifact: foreign,
	}
	if _, err := fixture.root.BuildPortableArtifactScope(context.Background(), recordings.BuildPortableArtifactScopeRequest{
		Scope: fixture.ref,
	}); !errors.Is(err, recordings.ErrForeignPortableArtifact) {
		t.Fatalf("BuildPortableArtifactScope foreign artifact = %v, want ErrForeignPortableArtifact", err)
	}
	if _, err := fixture.root.ExportPortableArtifactScope(context.Background(), recordings.ExportPortableArtifactScopeRequest{
		Scope: fixture.ref,
	}); !errors.Is(err, recordings.ErrForeignPortableArtifact) {
		t.Fatalf("ExportPortableArtifactScope foreign artifact = %v, want ErrForeignPortableArtifact", err)
	}
	if _, err := fixture.root.ReadPortableArtifactScope(context.Background(), recordings.ReadPortableArtifactScopeRequest{
		Scope: fixture.ref, Reference: "recording://foreign",
	}); !errors.Is(err, recordings.ErrForeignPortableArtifact) {
		t.Fatalf("ReadPortableArtifactScope foreign artifact = %v, want ErrForeignPortableArtifact", err)
	}
}

func TestObserveReplayScopePropagatesCancellationAfterObservation(t *testing.T) {
	t.Parallel()

	fixture := newFinalizedQueryFixture(t)
	plan, err := fixture.root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{
		Scope: fixture.ref, SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing: recordings.ReplayTimingOrderOnly,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlanScope: %v", err)
	}
	ctx := &cancelAfterFirstErrContext{}
	if _, err := fixture.root.ObserveReplayScope(ctx, recordings.ObserveReplayScopeRequest{
		Scope: fixture.ref, Plan: plan.Plan.Handle,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ObserveReplayScope cancellation after observation = %v, want context.Canceled", err)
	}
}

func TestBeginRecordingScopeCancellationWithoutClockCleansUp(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	planner := cancelingRecordingTargetPlanner{cancel: cancel}
	root := NewServiceWithLifecycleEffects(
		&stubLedger{}, NewProjectionService(), planner, nil, nil, nil,
	)
	if _, err := root.BeginRecordingScope(ctx, recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "cancel-without-clock"},
		Target:  recordings.RecordingTargetRequest{HomeDir: "home"},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("BeginRecordingScope cancellation without clock = %v, want context.Canceled", err)
	}

	runtimeRoot := NewRuntimeRootWithHistoricalQueryAndAppender(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		staticRecordingClock{at: time.Unix(1_700_000_500, 0).UTC()},
	)
	opening, ok := runtimeRoot.(recordings.RuntimeOpening)
	if !ok || opening == nil {
		t.Fatal("NewRuntimeRootWithHistoricalQueryAndAppender did not expose RuntimeOpening")
	}
	now := func() time.Time { return time.Unix(1_700_000_500, 0).UTC() }
	opened, err := opening.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{
		Topology:         runtimeOpeningTopology{},
		Now:              now,
		FactorySessionID: "constructor-behavior",
	})
	if err != nil {
		t.Fatalf("OpenRuntime from composed constructor: %v", err)
	}
	opened.Ledger.RecordRunRequest()
	if events := opened.Ledger.CanonicalEvents(); len(events) != 1 || events[0].Type != recordings.FactoryEventTypeRunRequest {
		t.Fatalf("OpenRuntime ledger events = %#v, want one run request", events)
	}
	if err := opened.Recorder.Finalize(now().Add(time.Second)); err != nil {
		t.Fatalf("Finalize composed runtime: %v", err)
	}
}

type snapshotErrorLifecycle struct {
	recordinglifecycle.Service
	Err error
}

func (service snapshotErrorLifecycle) Snapshot(recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	return recordinglifecycle.Snapshot{}, service.Err
}

type replayObservationErrorService struct {
	recordingsreplay.Service
	Err error
}

func (service replayObservationErrorService) ObserveReplay(recordings.ObserveReplayRequest) (recordings.ObserveReplayResult, error) {
	return recordings.ObserveReplayResult{}, service.Err
}

type foreignArtifactExportService struct {
	artifactsexport.Service
	Artifact recordings.PortableArtifact
}

func (service foreignArtifactExportService) BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
	return recordings.BuildPortableArtifactResult{Artifact: service.Artifact}, nil
}

func (service foreignArtifactExportService) ExportPortableArtifact(context.Context, recordings.ExportPortableArtifactRequest) (recordings.ExportPortableArtifactResult, error) {
	return recordings.ExportPortableArtifactResult{Reference: "recording://foreign", Artifact: service.Artifact}, nil
}

func (service foreignArtifactExportService) ReadPortableArtifact(context.Context, recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
	return recordings.ReadPortableArtifactResult{Artifact: service.Artifact}, nil
}

type cancelingRecordingTargetPlanner struct {
	cancel context.CancelFunc
}

func (planner cancelingRecordingTargetPlanner) PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
	planner.cancel()
	return recordings.LiveRecordingTarget{ServicePath: "recording-target", ReportedPath: "recording-target"}, nil
}

type cancelAfterFirstErrContext struct {
	calls int
}

func (ctx *cancelAfterFirstErrContext) Deadline() (time.Time, bool) { return time.Time{}, false }

func (ctx *cancelAfterFirstErrContext) Done() <-chan struct{} { return nil }

func (ctx *cancelAfterFirstErrContext) Err() error {
	ctx.calls++
	if ctx.calls > 1 {
		return context.Canceled
	}
	return nil
}

func (ctx *cancelAfterFirstErrContext) Value(any) any { return nil }

func appendScopedQueryEvent(
	t *testing.T,
	root recordings.Service,
	ref recordings.RecordingScopeRef,
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
) recordings.AppendRecordingScopeEventResult {
	t.Helper()
	result, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: ref, Event: scopedScopeEvent(id, sequence, scope),
	})
	if err != nil {
		t.Fatalf("AppendRecordingScopeEvent(%s): %v", id, err)
	}
	return result
}

func assertSubscriptionEvent(t *testing.T, subscription recordings.EventSubscription, id recordings.CanonicalEventID) {
	t.Helper()
	outcome := subscription(context.Background())
	if outcome.Kind != recordings.SubscriptionEvent || outcome.Event.ID != id {
		t.Fatalf("subscription outcome = %#v, want event %s", outcome, id)
	}
}

func assertLiveCursorErrors(t *testing.T, root recordings.Service, ref recordings.RecordingScopeRef, cursor recordings.CanonicalEventCursor) {
	t.Helper()
	malformed := cursor
	malformed.StreamGenerationID = ""
	assertScopeSubscribeError(t, root, ref, malformed, recordings.ErrInvalidReconnectCursor)
	foreign := cursor
	foreign.StreamGenerationID = "other-generation"
	assertScopeSubscribeError(t, root, ref, foreign, recordings.ErrReconnectCursorUnavailable)
	expired := cursor
	expired.Sequence = 99
	assertScopeSubscribeError(t, root, ref, expired, recordings.ErrReconnectCursorExpired)
}

func assertScopeSubscribeError(
	t *testing.T,
	root recordings.Service,
	ref recordings.RecordingScopeRef,
	cursor recordings.CanonicalEventCursor,
	want error,
) {
	t.Helper()
	if _, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope: ref, Cursor: &cursor,
	}); !errors.Is(err, want) {
		t.Fatalf("scope cursor %v error = %v, want %v", cursor, err, want)
	}
}

func malformedScope(ref recordings.RecordingScopeRef) recordings.RecordingScopeRef {
	unknown, _ := (recordings.RecordingScopeRef{}).Parse(ref.String() + "0")
	return unknown
}

func newScopedQueryRoot(t *testing.T) recordings.Service {
	return newScopedQueryRootWithLogger(t, logging.NoopLogger{})
}

func newScopedQueryRootWithLogger(t *testing.T, logger logging.Logger) recordings.Service {
	t.Helper()
	publication, err := NewPortableArtifactPublication(
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewPortableArtifactPublication: %v", err)
	}
	root := NewServiceWithLifecycleEffectsAndLogger(
		&stubLedger{}, NewProjectionService(), nil, nil, nil, publication,
		logger,
	)
	if root == nil {
		t.Fatal("NewServiceWithLifecycleEffects returned nil")
	}
	return root
}
