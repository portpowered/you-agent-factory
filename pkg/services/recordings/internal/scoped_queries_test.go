package internal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestRecordingScopeReplayProjectionInspectionAndArtifactQueries(t *testing.T) {
	t.Parallel()

	root := newScopedQueryRoot(t)
	eventScope := recordings.CanonicalEventScope{FactorySessionID: "history-scope"}
	const recordingID recordings.RecordingID = "recording-history-scope"
	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: recordingID,
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

	loaded, err := root.LoadReplayRecordingScope(context.Background(), recordings.LoadReplayRecordingScopeRequest{
		Scope: opened.Scope,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecordingScope: %v", err)
	}
	if len(loaded.Recording.Events) != len(events) ||
		loaded.Recording.Events[0].ID != events[0].ID ||
		loaded.Recording.Events[1].Cursor != events[1].Cursor {
		t.Fatalf("loaded replay = %#v, want detached ordered history", loaded.Recording)
	}
	loaded.Recording.Events[0].Payload = `{"mutated":true}`
	loadedAgain, err := root.LoadReplayRecordingScope(context.Background(), recordings.LoadReplayRecordingScopeRequest{
		Scope: opened.Scope,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecordingScope after detached mutation: %v", err)
	}
	if loadedAgain.Recording.Events[0].Payload == `{"mutated":true}` {
		t.Fatal("scope replay returned mutable lifecycle-owned event data")
	}

	subscription, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope: opened.Scope,
	})
	if err != nil {
		t.Fatalf("SubscribeRecordingScope: %v", err)
	}
	for index, want := range events {
		outcome := subscription.Subscription(context.Background())
		if outcome.Kind != recordings.SubscriptionEvent || outcome.Event.ID != want.ID {
			t.Fatalf("historical subscription[%d] = %#v, want %s", index, outcome, want.ID)
		}
	}
	if outcome := subscription.Subscription(context.Background()); outcome.Kind != recordings.SubscriptionClosed {
		t.Fatalf("historical subscription terminal outcome = %#v, want closed", outcome)
	}
	invalidCursor := recordings.CanonicalEventCursor{Sequence: events[0].Sequence}
	if _, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope:  opened.Scope,
		Cursor: &invalidCursor,
	}); !errors.Is(err, recordings.ErrInvalidReconnectCursor) {
		t.Fatalf("malformed historical cursor error = %v, want ErrInvalidReconnectCursor", err)
	}
	unavailableCursor := events[0].Cursor
	unavailableCursor.StreamGenerationID = "replaced-generation"
	if _, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope:  opened.Scope,
		Cursor: &unavailableCursor,
	}); !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("unavailable historical cursor error = %v, want ErrReconnectCursorUnavailable", err)
	}
	expiredCursor := events[0].Cursor
	expiredCursor.Sequence = 99
	if _, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope:  opened.Scope,
		Cursor: &expiredCursor,
	}); !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("expired historical cursor error = %v, want ErrReconnectCursorExpired", err)
	}

	through := events[0].Cursor
	projected, err := root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope:        opened.Scope,
		Through:      &through,
		SelectedTick: 4,
	})
	if err != nil {
		t.Fatalf("ReconstructRecordingScope: %v", err)
	}
	if projected.WorldState.Scope != eventScope ||
		projected.WorldState.Through != through ||
		projected.WorldState.SelectedTick != 4 {
		t.Fatalf("scope projection = %#v, want coherent first-event prefix", projected.WorldState)
	}
	dashboard, err := root.QuerySimpleDashboardScope(context.Background(), recordings.QuerySimpleDashboardScopeRequest{
		Scope:        opened.Scope,
		SelectedTick: 4,
	})
	if err != nil {
		t.Fatalf("QuerySimpleDashboardScope: %v", err)
	}
	if dashboard.WorldState.Scope != eventScope {
		t.Fatalf("dashboard world state scope = %#v, want %q", dashboard.WorldState.Scope, eventScope.FactorySessionID)
	}
	workstation, err := root.QueryWorkstationRequestsScope(context.Background(), recordings.QueryWorkstationRequestsScopeRequest{
		Scope:        opened.Scope,
		SelectedTick: 4,
	})
	if err != nil {
		t.Fatalf("QueryWorkstationRequestsScope: %v", err)
	}
	if workstation.WorldState.Scope != eventScope {
		t.Fatalf("workstation world state scope = %#v, want %q", workstation.WorldState.Scope, eventScope.FactorySessionID)
	}

	planned, err := root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{
		Scope:         opened.Scope,
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		SelectedTick:  4,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlanScope: %v", err)
	}
	var observed recordings.ObserveReplayScopeResult
	for range events {
		observed, err = root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{
			Scope: opened.Scope,
			Plan:  planned.Plan.Handle,
		})
		if err != nil {
			t.Fatalf("ObserveReplayScope: %v", err)
		}
	}
	if observed.Observation.Kind != recordings.ReplayCompleted ||
		observed.Observation.WorldState.Scope != eventScope {
		t.Fatalf("replay observation = %#v, want completed scoped projection", observed.Observation)
	}

	built, err := root.BuildPortableArtifactScope(context.Background(), recordings.BuildPortableArtifactScopeRequest{
		Scope: opened.Scope,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifactScope: %v", err)
	}
	if len(built.Artifact.Events) != len(events) || built.Artifact.Summary.Scope != eventScope {
		t.Fatalf("scoped artifact = %#v, want ordered artifact for event scope", built.Artifact)
	}
	exported, err := root.ExportPortableArtifactScope(context.Background(), recordings.ExportPortableArtifactScopeRequest{
		Scope: opened.Scope,
	})
	if err != nil {
		t.Fatalf("ExportPortableArtifactScope: %v", err)
	}
	read, err := root.ReadPortableArtifactScope(context.Background(), recordings.ReadPortableArtifactScopeRequest{
		Scope:     opened.Scope,
		Reference: exported.Reference,
	})
	if err != nil {
		t.Fatalf("ReadPortableArtifactScope: %v", err)
	}
	if read.Artifact.Integrity != exported.Artifact.Integrity ||
		read.Artifact.Summary.Scope != eventScope {
		t.Fatalf("scoped artifact read = %#v, want exported detached artifact", read.Artifact)
	}
}

func TestRecordingScopeQueriesRemainIsolatedAcrossConcurrentScopes(t *testing.T) {
	t.Parallel()

	root := NewService(&stubLedger{}, NewProjectionService())
	type openedScope struct {
		ref   recordings.RecordingScopeRef
		scope recordings.CanonicalEventScope
	}
	opened := make([]openedScope, 2)
	for index, sessionID := range []string{"query-scope-a", "query-scope-b"} {
		eventScope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
		started, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
			Enabled: true,
			Scope:   eventScope,
			Target: recordings.RecordingTargetRequest{
				Artifact: recordings.RecordingArtifactReference("recording://" + sessionID),
			},
		})
		if err != nil {
			t.Fatalf("BeginRecordingScope(%s): %v", sessionID, err)
		}
		if _, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
			Scope: started.Scope,
			Event: scopedScopeEvent(sessionID+"-event", 0, eventScope),
		}); err != nil {
			t.Fatalf("AppendRecordingScopeEvent(%s): %v", sessionID, err)
		}
		opened[index] = openedScope{ref: started.Scope, scope: eventScope}
	}

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
				if result.WorldState.Scope != selected.scope ||
					result.Status.EventScope != selected.scope ||
					result.Status.AcceptedEvents != 1 {
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

	unknown, _ := (recordings.RecordingScopeRef{}).Parse(opened[0].ref.String() + "0")
	if _, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{Scope: unknown}); !errors.Is(err, recordings.ErrRecordingScopeInvalid) {
		t.Fatalf("malformed scope query = %v, want invalid scope", err)
	}
	if _, err := root.QuerySimpleDashboardScope(context.Background(), recordings.QuerySimpleDashboardScopeRequest{Scope: opened[0].ref, SelectedTick: -1}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("invalid scoped projection = %v, want ErrInvalidProjectionInput", err)
	}
}

func newScopedQueryRoot(t *testing.T) recordings.Service {
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
	root := NewServiceWithLifecycleEffects(
		&stubLedger{},
		NewProjectionService(),
		nil,
		nil,
		nil,
		publication,
	)
	if root == nil {
		t.Fatal("NewServiceWithLifecycleEffects returned nil")
	}
	return root
}
