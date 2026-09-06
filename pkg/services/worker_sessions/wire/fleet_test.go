package wire

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestFleetObservationServiceMergesAndPagesSources(t *testing.T) {
	first := &fleetObservationSource{result: workersessions.ListWorkerSessionObservationsResult{
		Observations: []workersessions.Observation{
			{WorkerSessionID: "factory-2", Direct: false, State: workersessions.StateCompleted},
			{WorkerSessionID: "direct-4", Direct: true, State: workersessions.StateRunning},
		}}}
	second := &fleetObservationSource{result: workersessions.ListWorkerSessionObservationsResult{
		Observations: []workersessions.Observation{
			{WorkerSessionID: "direct-1", Direct: true, State: workersessions.StateRunning},
			{WorkerSessionID: "factory-3", Direct: false, State: workersessions.StateFailed},
		}}}
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{first, second, first}, nil
	})

	page, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{MaxResults: 2})
	if err != nil {
		t.Fatalf("first fleet page: %v", err)
	}
	assertFleetObservationIDs(t, page.Observations, []string{"direct-1", "direct-4"})
	if page.NextToken == "" {
		t.Fatal("first fleet page next token is empty")
	}

	secondPage, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{
		MaxResults: 2,
		NextToken:  page.NextToken,
	})
	if err != nil {
		t.Fatalf("second fleet page: %v", err)
	}
	assertFleetObservationIDs(t, secondPage.Observations, []string{"factory-2", "factory-3"})
	if secondPage.NextToken != "" {
		t.Fatalf("second fleet page next token = %q, want exhausted", secondPage.NextToken)
	}

	direct, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{
		Scope: workersessions.ObservationScopeDirect,
		States: []workersessions.State{
			workersessions.StateRunning,
		},
	})
	if err != nil {
		t.Fatalf("filtered fleet page: %v", err)
	}
	assertFleetObservationIDs(t, direct.Observations, []string{"direct-1", "direct-4"})
}

func TestFleetObservationServicePreservesOptionalProjectionFacts(t *testing.T) {
	source := &fleetObservationSource{
		result: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{{
			WorkerSessionID: "worker-base", State: workersessions.StateFailed,
		}}},
		err: workersessions.ErrObservationProjectionUnavailable,
	}
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{source}, nil
	})

	result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{})
	if err != nil {
		t.Fatalf("fleet list with unavailable optional projection: %v", err)
	}
	assertFleetObservationIDs(t, result.Observations, []string{"worker-base"})
}

func TestFleetObservationServiceMergesDuplicateProjectionFacts(t *testing.T) {
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	duration := 2 * time.Second
	weak := &fleetObservationSource{result: workersessions.ListWorkerSessionObservationsResult{
		Observations: []workersessions.Observation{{
			WorkerSessionID:  "worker-duplicate",
			FactorySessionID: "factory-session-1",
			State:            workersessions.StateCompleted,
			DurationBasis:    workersessions.DurationBasisUnavailable,
		}}}}
	rich := &fleetObservationSource{result: workersessions.ListWorkerSessionObservationsResult{
		Observations: []workersessions.Observation{{
			WorkerSessionID:          "worker-duplicate",
			ProviderSession:          providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-1"},
			ProviderSessionAvailable: true,
			WorkIDs:                  []string{"work-1"},
			StartedAt:                &started,
			Duration:                 &duration,
			DurationBasis:            workersessions.DurationBasisRecordedTimestamps,
			Transcript:               workersessions.TranscriptAvailabilityAvailable,
			ConfirmationState:        workersessions.ConfirmationStateConfirmed,
		}}}}
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{weak, rich}, nil
	})

	result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{})
	if err != nil {
		t.Fatalf("fleet duplicate merge: %v", err)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("fleet duplicate count = %d, want 1", len(result.Observations))
	}
	observation := result.Observations[0]
	if !observation.ProviderSessionAvailable || observation.ProviderSession.ID != "provider-1" {
		t.Fatalf("fleet duplicate provider = %#v, want provider-1", observation.ProviderSession)
	}
	if len(observation.WorkIDs) != 1 || observation.WorkIDs[0] != "work-1" || observation.StartedAt == nil {
		t.Fatalf("fleet duplicate attribution/timing = %#v", observation)
	}
	if observation.Duration == nil || *observation.Duration != duration || observation.DurationBasis != workersessions.DurationBasisRecordedTimestamps {
		t.Fatalf("fleet duplicate duration = %v/%q, want %s/RECORDED_TIMESTAMPS", observation.Duration, observation.DurationBasis, duration)
	}
	if observation.Transcript != workersessions.TranscriptAvailabilityAvailable || observation.ConfirmationState != workersessions.ConfirmationStateConfirmed {
		t.Fatalf("fleet duplicate optional facts = %#v", observation)
	}
}

func TestFleetObservationServiceReturnsNonNilEmptyAndRejectsInvalidInput(t *testing.T) {
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return nil, nil
	})
	result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{})
	if err != nil || result.Observations == nil || len(result.Observations) != 0 {
		t.Fatalf("empty fleet list = %#v, %v, want non-nil empty result", result, err)
	}
	if result.MaxResults != workersessions.DefaultWorkerSessionObservationListMaxResults {
		t.Fatalf("empty fleet max results = %d, want default %d", result.MaxResults, workersessions.DefaultWorkerSessionObservationListMaxResults)
	}

	for name, request := range map[string]workersessions.ListWorkerSessionObservationsRequest{
		"invalid scope":  {Scope: workersessions.ObservationScope("unknown")},
		"negative limit": {MaxResults: -1},
		"invalid cursor": {NextToken: "%%%"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.ListWorkerSessionObservations(context.Background(), request); err == nil {
				t.Fatal("invalid request unexpectedly succeeded")
			}
		})
	}

	if NewFleetObservationService(nil) != nil {
		t.Fatal("NewFleetObservationService(nil) returned a service")
	}
	var unavailable *FleetObservationService
	if _, err := unavailable.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("nil fleet service error = %v, want projection unavailable", err)
	}
	got, err := decodeFleetObservationCursor(base64.StdEncoding.EncodeToString([]byte("worker-1")))
	if err != nil || got != "worker-1" {
		t.Fatalf("decoded cursor = %q, %v, want worker-1", got, err)
	}
}

func assertFleetObservationIDs(t *testing.T, observations []workersessions.Observation, want []string) {
	t.Helper()
	if len(observations) != len(want) {
		t.Fatalf("fleet observation count = %d, want %d: %#v", len(observations), len(want), observations)
	}
	for index, observation := range observations {
		if observation.WorkerSessionID != want[index] {
			t.Fatalf("fleet observation IDs = %#v, want %#v", fleetObservationIDsForTest(observations), want)
		}
	}
}

func fleetObservationIDsForTest(observations []workersessions.Observation) []string {
	ids := make([]string, 0, len(observations))
	for _, observation := range observations {
		ids = append(ids, observation.WorkerSessionID)
	}
	return ids
}

type fleetObservationSource struct {
	workersessions.Service
	result workersessions.ListWorkerSessionObservationsResult
	err    error
}

func (source *fleetObservationSource) ListWorkerSessionObservations(
	context.Context,
	workersessions.ListWorkerSessionObservationsRequest,
) (workersessions.ListWorkerSessionObservationsResult, error) {
	return source.result, source.err
}
