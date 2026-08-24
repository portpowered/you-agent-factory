package http_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
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

// TestWorkSnapshotReaderProjectsCompletedHistoryThroughComposedService keeps
// the Work read path at the public process boundary while exercising the
// Recordings-owned snapshot adapter against the same completed event history.
// The adapter is selected from the composed projection rather than rebuilt
// with a test fake, so the assertion observes the customer Work item and its
// admission lineage after a real Factory run.
func TestWorkSnapshotReaderProjectsCompletedHistoryThroughComposedService(t *testing.T) {
	dir := support.ScaffoldFactory(t, runnerDiagnosticsFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"request":"read completed snapshot"}`))
	runner := support.NewRecordingCommandRunner("safe agent result COMPLETE")

	var service recordings.Service
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges:      serviceedges.Edges{ProviderCommandRunner: runner},
		BeforeStart: func(_ testing.TB, process support.Process, _ root.Input) {
			service = root.RecordingsServiceFromProcess(process)
		},
	})
	if service == nil {
		t.Fatal("root process does not expose the composed Recordings service")
	}
	support.WaitForTerminalStatus(t, server.URL(), 60*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	publicEvents := server.GetFactoryEvents(t)
	if len(publicEvents) == 0 {
		t.Fatal("public Factory Event history is empty, want the completed run")
	}

	// The live canonical subscription is intentionally open-ended. Bound only
	// its test read at the last event already observed through the public HTTP
	// history; all event and world-state operations still delegate to the
	// composed Recordings service.
	reader := recordingswire.NewWorkSnapshotReader(boundedRecordingsReadRoot{
		service:     service,
		stopAfterID: recordings.CanonicalEventID(publicEvents[len(publicEvents)-1].Id),
	})
	snapshot, err := reader.ReadWorkSnapshot(context.Background(), factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatalf("ReadWorkSnapshot(completed session): %v", err)
	}
	if len(snapshot.Items) != len(listed.Results) {
		t.Fatalf("snapshot items = %d, public Work results = %d; snapshot=%#v",
			len(snapshot.Items), len(listed.Results), snapshot)
	}
	if len(snapshot.Admissions) == 0 {
		t.Fatalf("snapshot admissions = %#v, want Work Request lineage from canonical events", snapshot.Admissions)
	}
	if snapshot.Items[0].WorkID == "" || snapshot.Items[0].Name == "" || snapshot.Items[0].State == nil {
		t.Fatalf("snapshot item = %#v, want detached public Work identity, name, and state", snapshot.Items[0])
	}
}

// TestFactoryEventContractParityThroughRecordingsWire exercises the public
// Recordings wire seam against the bundled OpenAPI event contract. The parity
// result is the same inventory authority used during runtime composition.
func TestFactoryEventContractParityThroughRecordingsWire(t *testing.T) {
	openAPIPath := filepath.Join(testutil.MustRepoRoot(t), "api", "openapi.yaml")
	data, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read bundled OpenAPI contract %s: %v", openAPIPath, err)
	}
	if err := recordingswire.ValidateFactoryEventContract(data); err != nil {
		t.Fatalf("ValidateFactoryEventContract(): %v", err)
	}
}

type boundedRecordingsReadRoot struct {
	service     recordings.Service
	stopAfterID recordings.CanonicalEventID
}

func (root boundedRecordingsReadRoot) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	result, err := root.service.SubscribeFrom(ctx, request)
	if err != nil || result.Subscription == nil {
		return result, err
	}
	closed := false
	original := result.Subscription
	result.Subscription = func(nextContext context.Context) recordings.SubscriptionOutcome {
		if closed {
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
		}
		outcome := original(nextContext)
		if outcome.Kind == recordings.SubscriptionEvent && outcome.Event.ID == root.stopAfterID {
			closed = true
		}
		return outcome
	}
	return result, nil
}

func (root boundedRecordingsReadRoot) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	return root.service.ReconstructWorldState(request)
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
