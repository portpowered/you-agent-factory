package internal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

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

func TestRecordingScopeQueryAndReplayFailuresRemainTyped(t *testing.T) {
	t.Parallel()

	fixture := newFinalizedQueryFixture(t)
	assertProjectionCursorErrors(t, fixture)
	assertReplayPlanErrors(t, fixture)
	assertHistoricalSubscriptionHelperErrors(t, fixture)
}

func assertProjectionCursorErrors(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	malformed := fixture.events[0].Cursor
	malformed.StreamGenerationID = ""
	assertProjectionError(t, fixture, malformed, recordings.ErrInvalidReconnectCursor)
	foreign := fixture.events[0].Cursor
	foreign.StreamGenerationID = "other-generation"
	assertProjectionError(t, fixture, foreign, recordings.ErrReconnectCursorUnavailable)
	expired := fixture.events[0].Cursor
	expired.Sequence = 99
	assertProjectionError(t, fixture, expired, recordings.ErrReconnectCursorNotFound)
}

func assertProjectionError(t *testing.T, fixture *scopedQueryFixture, cursor recordings.CanonicalEventCursor, want error) {
	t.Helper()
	if _, err := fixture.root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope: fixture.ref, Through: &cursor,
	}); !errors.Is(err, want) {
		t.Fatalf("projection cursor %v error = %v, want %v", cursor, err, want)
	}
}

func assertReplayPlanErrors(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	if _, err := fixture.root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{
		Scope: fixture.ref, SchemaVersion: "unsupported", Timing: recordings.ReplayTimingOrderOnly,
	}); !errors.Is(err, recordings.ErrUnsupportedReplayPlan) {
		t.Fatalf("unsupported replay schema error = %v, want ErrUnsupportedReplayPlan", err)
	}
	if _, err := fixture.root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{
		Scope: fixture.ref, Plan: "missing-plan",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("missing replay plan error = %v, want ErrReplayPlanNotFound", err)
	}
}

func assertHistoricalSubscriptionHelperErrors(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	expired := fixture.events[0].Cursor
	expired.Sequence = 99
	if _, err := newHistoricalScopeSubscription(context.Background(), fixture.events, &expired); !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("historical helper expired cursor error = %v, want ErrReconnectCursorExpired", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	subscription, err := newHistoricalScopeSubscription(canceled, fixture.events, nil)
	if err != nil {
		t.Fatalf("newHistoricalScopeSubscription canceled setup: %v", err)
	}
	if outcome := subscription(nil); outcome.Kind != recordings.SubscriptionClosed {
		t.Fatalf("historical helper canceled outcome = %#v, want closed", outcome)
	}
}

func TestClosedRecordingScopeRejectsEveryReadOperation(t *testing.T) {
	t.Parallel()

	fixture := newFinalizedQueryFixture(t)
	closed, err := fixture.root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope: fixture.ref, FinishedAt: time.Unix(1_700_000_200, 0).UTC(),
	})
	if err != nil || !closed.Closed {
		t.Fatalf("CloseRecordingScope = %#v, error %v", closed, err)
	}
	repeated, err := fixture.root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope: fixture.ref, FinishedAt: time.Unix(1_700_000_201, 0).UTC(),
	})
	if err != nil || !repeated.Closed || !reflect.DeepEqual(repeated.Status, closed.Status) {
		t.Fatalf("repeated CloseRecordingScope = %#v, error %v", repeated, err)
	}
	calls := closedScopeCalls(fixture)
	for name, call := range calls {
		if err := call(); !errors.Is(err, recordings.ErrRecordingScopeClosed) {
			t.Errorf("closed scope %s error = %v, want ErrRecordingScopeClosed", name, err)
		}
	}
}

func TestRecordingScopeOperationsEmitStructuredLifecycleLogs(t *testing.T) {
	t.Parallel()

	logger := &recordingOperationLogger{}
	root := newScopedQueryRootWithLogger(t, logger)
	started, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "logged-scope"},
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://logged"},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope: %v", err)
	}
	if _, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{Scope: started.Scope}); err != nil {
		t.Fatalf("QueryRecordingScope: %v", err)
	}
	if _, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope: started.Scope, FinishedAt: time.Unix(1_700_000_300, 0).UTC(),
	}); err != nil {
		t.Fatalf("CloseRecordingScope: %v", err)
	}

	if len(logger.infos) < 6 {
		t.Fatalf("recording operation logs = %#v, want start and finish for three operations", logger.infos)
	}
	for _, entry := range logger.infos {
		if entry.message != "recordings operation started" && entry.message != "recordings operation finished" {
			t.Fatalf("unexpected operation log = %#v", entry)
		}
		if entry.fields["operation"] == "" {
			t.Fatalf("operation log missing operation name: %#v", entry)
		}
		if entry.fields["scope_ref"] == started.Scope.String() && entry.fields["factory_session_id"] != nil {
			t.Fatalf("scope log duplicated session identity unexpectedly: %#v", entry)
		}
	}
}

func closedScopeCalls(fixture *scopedQueryFixture) map[string]func() error {
	root, ref := fixture.root, fixture.ref
	return map[string]func() error{
		"append": func() error {
			_, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{Scope: ref, Event: fixture.events[0]})
			return err
		},
		"subscribe": func() error {
			_, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{Scope: ref})
			return err
		},
		"flush": func() error {
			_, err := root.FlushRecordingScope(context.Background(), recordings.FlushRecordingScopeRequest{Scope: ref})
			return err
		},
		"finalize": func() error {
			_, err := root.FinalizeRecordingScope(context.Background(), recordings.FinalizeRecordingScopeRequest{Scope: ref})
			return err
		},
		"query": func() error {
			_, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{Scope: ref})
			return err
		},
		"replay": func() error {
			_, err := root.LoadReplayRecordingScope(context.Background(), recordings.LoadReplayRecordingScopeRequest{Scope: ref})
			return err
		},
		"plan": func() error {
			_, err := root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{Scope: ref})
			return err
		},
		"observe": func() error {
			_, err := root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{Scope: ref})
			return err
		},
		"projection": func() error {
			_, err := root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{Scope: ref})
			return err
		},
		"dashboard": func() error {
			_, err := root.QuerySimpleDashboardScope(context.Background(), recordings.QuerySimpleDashboardScopeRequest{Scope: ref})
			return err
		},
		"workstation": func() error {
			_, err := root.QueryWorkstationRequestsScope(context.Background(), recordings.QueryWorkstationRequestsScopeRequest{Scope: ref})
			return err
		},
		"artifact": func() error {
			_, err := root.BuildPortableArtifactScope(context.Background(), recordings.BuildPortableArtifactScopeRequest{Scope: ref})
			return err
		},
		"export": func() error {
			_, err := root.ExportPortableArtifactScope(context.Background(), recordings.ExportPortableArtifactScopeRequest{Scope: ref})
			return err
		},
		"read": func() error {
			_, err := root.ReadPortableArtifactScope(context.Background(), recordings.ReadPortableArtifactScopeRequest{Scope: ref, Reference: fixture.exported.Reference})
			return err
		},
	}
}

func TestRecordingScopeQueriesRemainIsolatedAcrossConcurrentScopes(t *testing.T) {
	t.Parallel()

	root := NewService(&stubLedger{}, NewProjectionService())
	opened := make([]openedScopedQuery, 2)
	for index, sessionID := range []string{"query-scope-a", "query-scope-b"} {
		eventScope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
		started, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
			Enabled: true, Scope: eventScope,
			Target: recordings.RecordingTargetRequest{Artifact: recordings.RecordingArtifactReference("recording://" + sessionID)},
		})
		if err != nil {
			t.Fatalf("BeginRecordingScope(%s): %v", sessionID, err)
		}
		if _, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
			Scope: started.Scope, Event: scopedScopeEvent(sessionID+"-event", 0, eventScope),
		}); err != nil {
			t.Fatalf("AppendRecordingScopeEvent(%s): %v", sessionID, err)
		}
		opened[index] = openedScopedQuery{ref: started.Scope, scope: eventScope}
	}
	assertConcurrentScopeProjections(t, root, opened)
	if _, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{Scope: malformedScope(opened[0].ref)}); !errors.Is(err, recordings.ErrRecordingScopeInvalid) {
		t.Fatalf("malformed scope query = %v, want invalid scope", err)
	}
	if _, err := root.QuerySimpleDashboardScope(context.Background(), recordings.QuerySimpleDashboardScopeRequest{Scope: opened[0].ref, SelectedTick: -1}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("invalid scoped projection = %v, want ErrInvalidProjectionInput", err)
	}
}

func assertConcurrentScopeProjections(t *testing.T, root recordings.Service, opened []openedScopedQuery) {
	t.Helper()
	var wait sync.WaitGroup
	errs := make(chan error, 20)
	for _, selected := range opened {
		selected := selected
		for range 10 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				result, err := root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
					Scope: selected.ref, SelectedTick: 1,
				})
				if err != nil {
					errs <- err
					return
				}
				if result.WorldState.Scope != selected.scope || result.Status.EventScope != selected.scope || result.Status.AcceptedEvents != 1 {
					errs <- errors.New("concurrent scoped projection crossed recording ownership")
				}
			}()
		}
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
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

type recordingOperationLogEntry struct {
	message string
	fields  map[string]any
}

type recordingOperationLogger struct {
	infos []recordingOperationLogEntry
}

func (logger *recordingOperationLogger) Debug(string, ...any)   {}
func (logger *recordingOperationLogger) Warn(string, ...any)    {}
func (logger *recordingOperationLogger) Error(string, ...any)   {}
func (logger *recordingOperationLogger) Verbose(string, ...any) {}

func (logger *recordingOperationLogger) Info(message string, fields ...any) {
	values := make(map[string]any, len(fields)/2)
	for index := 0; index+1 < len(fields); index += 2 {
		key, ok := fields[index].(string)
		if ok {
			values[key] = fields[index+1]
		}
	}
	logger.infos = append(logger.infos, recordingOperationLogEntry{message: message, fields: values})
}
