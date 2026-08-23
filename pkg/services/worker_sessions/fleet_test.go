package workersessions

import (
	"context"
	"encoding/base64"
	"testing"
	"time"
)

func TestFleetObservationServiceMergesSourcesBeforeApplyingCursor(t *testing.T) {
	started := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	first := &fleetObservationListSource{pages: map[string]ListWorkerSessionObservationsResult{
		"": {
			Observations: []Observation{{
				WorkerSessionID: "factory-2", Direct: false, State: StateCompleted, StartedAt: &started,
			}},
			NextToken: base64.StdEncoding.EncodeToString([]byte("factory-2")),
		},
		base64.StdEncoding.EncodeToString([]byte("factory-2")): {
			Observations: []Observation{{
				WorkerSessionID: "direct-4", Direct: true, State: StateRunning, StartedAt: &started,
			}},
		},
	}}
	second := &fleetObservationListSource{pages: map[string]ListWorkerSessionObservationsResult{
		"": {
			Observations: []Observation{{
				WorkerSessionID: "direct-1", Direct: true, State: StateRunning, StartedAt: &started,
			}},
			NextToken: base64.StdEncoding.EncodeToString([]byte("direct-1")),
		},
		base64.StdEncoding.EncodeToString([]byte("direct-1")): {
			Observations: []Observation{{
				WorkerSessionID: "factory-3", Direct: false, State: StateFailed, StartedAt: &started,
			}},
		},
	}}
	service := NewFleetObservationService(func(context.Context) ([]ObservationService, error) {
		return []ObservationService{first, second, first}, nil
	})

	page, err := service.ListWorkerSessionObservations(context.Background(), ListWorkerSessionObservationsRequest{MaxResults: 2})
	if err != nil {
		t.Fatalf("first fleet page: %v", err)
	}
	assertFleetObservationIDs(t, page.Observations, []string{"direct-1", "direct-4"})
	if page.MaxResults != 2 || page.NextToken == "" {
		t.Fatalf("first fleet page metadata = %#v, want maxResults 2 and next token", page)
	}

	secondPage, err := service.ListWorkerSessionObservations(context.Background(), ListWorkerSessionObservationsRequest{
		MaxResults: 2,
		NextToken:  page.NextToken,
	})
	if err != nil {
		t.Fatalf("second fleet page: %v", err)
	}
	assertFleetObservationIDs(t, secondPage.Observations, []string{"factory-2", "factory-3"})
	if secondPage.NextToken != "" {
		t.Fatalf("second fleet page next token = %q, want exhaustion", secondPage.NextToken)
	}

	direct, err := service.ListWorkerSessionObservations(context.Background(), ListWorkerSessionObservationsRequest{
		Scope:  ObservationScopeDirect,
		States: []State{StateRunning},
	})
	if err != nil {
		t.Fatalf("filtered fleet page: %v", err)
	}
	assertFleetObservationIDs(t, direct.Observations, []string{"direct-1", "direct-4"})
}

func TestFleetObservationServiceReturnsNonNilEmptyCollection(t *testing.T) {
	service := NewFleetObservationService(func(context.Context) ([]ObservationService, error) {
		return nil, nil
	})
	result, err := service.ListWorkerSessionObservations(context.Background(), ListWorkerSessionObservationsRequest{})
	if err != nil {
		t.Fatalf("empty fleet list: %v", err)
	}
	if result.Observations == nil || len(result.Observations) != 0 {
		t.Fatalf("empty fleet observations = %#v, want non-nil empty collection", result.Observations)
	}
	if result.MaxResults != DefaultWorkerSessionObservationListMaxResults {
		t.Fatalf("empty fleet max results = %d, want default %d", result.MaxResults, DefaultWorkerSessionObservationListMaxResults)
	}
}

func assertFleetObservationIDs(t *testing.T, observations []Observation, want []string) {
	t.Helper()
	if len(observations) != len(want) {
		t.Fatalf("fleet observation count = %d, want %d: %#v", len(observations), len(want), observations)
	}
	for index, observation := range observations {
		if observation.WorkerSessionID != want[index] {
			t.Fatalf("fleet observation IDs = %#v, want %#v", observationIDs(observations), want)
		}
	}
}

func observationIDs(observations []Observation) []string {
	ids := make([]string, 0, len(observations))
	for _, observation := range observations {
		ids = append(ids, observation.WorkerSessionID)
	}
	return ids
}

type fleetObservationListSource struct {
	Service
	pages map[string]ListWorkerSessionObservationsResult
}

func (source *fleetObservationListSource) ListWorkerSessionObservations(
	_ context.Context,
	request ListWorkerSessionObservationsRequest,
) (ListWorkerSessionObservationsResult, error) {
	return source.pages[request.NextToken], nil
}
