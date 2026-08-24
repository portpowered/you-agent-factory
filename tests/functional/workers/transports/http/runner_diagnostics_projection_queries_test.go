package http_test

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRecordingsProjectionQueriesThroughRootProcess exercises the additional
// stateless Recordings projections through the canonical process boundary. The
// test keeps the topology and reconnect facts at the public contract edge so
// the dashboard, throttle-pause, and reconnect paths are observed without
// importing projection implementation packages.
func TestRecordingsProjectionQueriesThroughRootProcess(t *testing.T) {
	dir := support.ScaffoldFactory(t, runnerDiagnosticsFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"request":"inspect projection queries"}`))
	runner := support.NewRecordingCommandRunner("safe agent result COMPLETE")

	_, _, generatedEvents, projection := support.RunFactoryToCompletionWithEdgesAndObservationsAndProjection(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		60*time.Second,
	)
	queries, ok := projection.(root.RecordingsProjectionQueries)
	if !ok {
		t.Fatalf("root Recordings projection = %T, want the public projection-query capability", projection)
	}

	events := canonicalEventsFromGenerated(t, generatedEvents)
	if len(events) == 0 {
		t.Fatal("canonical events = empty, want the completed Factory history")
	}
	selectedTick := 0
	for _, event := range events {
		if event.Context.Tick > selectedTick {
			selectedTick = event.Context.Tick
		}
	}
	world, err := queries.ReconstructFactoryWorldState(events, selectedTick)
	if err != nil {
		t.Fatalf("reconstruct Factory World through root projection queries: %v", err)
	}

	dashboard := queries.SimpleDashboardRenderData(world)
	if !dashboard.Session.HasData || dashboard.Session.CompletedCount < 1 {
		t.Fatalf("dashboard projection = %+v, want completed customer Work data", dashboard.Session)
	}

	pausedUntil := time.Date(2026, 8, 24, 12, 5, 0, 0, time.UTC)
	pauses := queries.ProjectActiveThrottlePauses(
		recordings.InitialStructurePayload{
			Workers: []recordings.FactoryWorker{
				{ID: "worker-provider-fallback", Provider: "openai", Model: "gpt-5"},
			},
			Workstations: []recordings.FactoryWorkstation{
				{ID: "review", Name: "Review", WorkerID: "worker-provider-fallback", InputPlaceIDs: []string{"task:init"}},
			},
			Places: []recordings.FactoryPlace{
				{ID: "task:init", TypeID: "task"},
				{ID: factorydefinitions.SystemTimePendingPlaceID, TypeID: factorydefinitions.SystemTimeWorkTypeID},
			},
		},
		[]recordings.ActiveThrottlePause{{
			LaneID:      "openai/gpt-5",
			Provider:    "OPENAI",
			Model:       "gpt-5",
			PausedAt:    pausedUntil.Add(-time.Minute),
			PausedUntil: pausedUntil,
		}},
	)
	if len(pauses) != 1 || len(pauses[0].AffectedTransitionIDs) != 1 || pauses[0].AffectedTransitionIDs[0] != "review" {
		t.Fatalf("throttle pause projection = %#v, want the matching workstation", pauses)
	}

	validationEvents := append([]recordings.FactoryEvent(nil), events...)
	validationSessionID := "functional-projection-session"
	for index := range validationEvents {
		validationEvents[index].Context.SessionID = &validationSessionID
	}
	lastEvent := validationEvents[len(validationEvents)-1]
	scope := recordings.FactoryEventReconnectScope{SessionID: validationSessionID}
	if err := queries.ValidateReconnectReplay(
		validationEvents,
		recordings.FactoryEventReconnectCursor{AfterEventID: lastEvent.Id},
		scope,
	); err != nil {
		t.Fatalf("valid reconnect replay cursor: %v", err)
	}
	if err := queries.ValidateReconnectReplay(
		validationEvents,
		recordings.FactoryEventReconnectCursor{AfterEventID: "missing-event"},
		scope,
	); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("missing reconnect replay cursor error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func canonicalEventsFromGenerated(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) []recordings.FactoryEvent {
	t.Helper()
	canonical := make([]recordings.FactoryEvent, 0, len(events))
	for _, event := range events {
		converted, err := recordingshttp.CanonicalFactoryEvent(event)
		if err != nil {
			t.Fatalf("convert generated Factory Event %q: %v", event.Id, err)
		}
		canonical = append(canonical, converted)
	}
	return canonical
}
