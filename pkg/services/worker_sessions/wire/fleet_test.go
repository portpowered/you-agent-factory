package wire

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestFleetObservationServiceMergesSourcesBeforeApplyingCursor(t *testing.T) {
	started := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	first := &fleetObservationListSource{pages: map[string]workersessions.ListWorkerSessionObservationsResult{
		"": {
			Observations: []workersessions.Observation{{
				WorkerSessionID: "factory-2", Direct: false, State: workersessions.StateCompleted, StartedAt: &started,
			}},
			NextToken: base64.StdEncoding.EncodeToString([]byte("factory-2")),
		},
		base64.StdEncoding.EncodeToString([]byte("factory-2")): {
			Observations: []workersessions.Observation{{
				WorkerSessionID: "direct-4", Direct: true, State: workersessions.StateRunning, StartedAt: &started,
			}},
		},
	}}
	second := &fleetObservationListSource{pages: map[string]workersessions.ListWorkerSessionObservationsResult{
		"": {
			Observations: []workersessions.Observation{{
				WorkerSessionID: "direct-1", Direct: true, State: workersessions.StateRunning, StartedAt: &started,
			}},
			NextToken: base64.StdEncoding.EncodeToString([]byte("direct-1")),
		},
		base64.StdEncoding.EncodeToString([]byte("direct-1")): {
			Observations: []workersessions.Observation{{
				WorkerSessionID: "factory-3", Direct: false, State: workersessions.StateFailed, StartedAt: &started,
			}},
		},
	}}
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{first, second, first}, nil
	})

	page, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{MaxResults: 2})
	if err != nil {
		t.Fatalf("first fleet page: %v", err)
	}
	assertFleetObservationIDs(t, page.Observations, []string{"direct-1", "direct-4"})
	if page.MaxResults != 2 || page.NextToken == "" {
		t.Fatalf("first fleet page metadata = %#v, want maxResults 2 and next token", page)
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
		t.Fatalf("second fleet page next token = %q, want exhaustion", secondPage.NextToken)
	}

	direct, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{
		Scope:  workersessions.ObservationScopeDirect,
		States: []workersessions.State{workersessions.StateRunning},
	})
	if err != nil {
		t.Fatalf("filtered fleet page: %v", err)
	}
	assertFleetObservationIDs(t, direct.Observations, []string{"direct-1", "direct-4"})
}

func TestFleetObservationServiceKeepsBasePageWhenProjectionIsUnavailable(t *testing.T) {
	source := &fleetObservationListSource{
		pages: map[string]workersessions.ListWorkerSessionObservationsResult{
			"": {
				Observations: []workersessions.Observation{{
					WorkerSessionID: "worker-base",
					State:           workersessions.StateFailed,
					Direct:          false,
				}},
			},
		},
		errs: map[string]error{"": workersessions.ErrObservationProjectionUnavailable},
	}
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{source}, nil
	})

	result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{})
	if err != nil {
		t.Fatalf("fleet list with unavailable optional projection: %v", err)
	}
	assertFleetObservationIDs(t, result.Observations, []string{"worker-base"})
	if result.Observations[0].State != workersessions.StateFailed {
		t.Fatalf("base observation state = %q, want FAILED", result.Observations[0].State)
	}
}

func TestFleetObservationServiceReturnsNonNilEmptyCollection(t *testing.T) {
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return nil, nil
	})
	result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{})
	if err != nil {
		t.Fatalf("empty fleet list: %v", err)
	}
	if result.Observations == nil || len(result.Observations) != 0 {
		t.Fatalf("empty fleet observations = %#v, want non-nil empty collection", result.Observations)
	}
	if result.MaxResults != workersessions.DefaultWorkerSessionObservationListMaxResults {
		t.Fatalf("empty fleet max results = %d, want default %d", result.MaxResults, workersessions.DefaultWorkerSessionObservationListMaxResults)
	}
}

func assertFleetObservationIDs(t *testing.T, observations []workersessions.Observation, want []string) {
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

func observationIDs(observations []workersessions.Observation) []string {
	ids := make([]string, 0, len(observations))
	for _, observation := range observations {
		ids = append(ids, observation.WorkerSessionID)
	}
	return ids
}

type fleetObservationListSource struct {
	workersessions.Service
	pages map[string]workersessions.ListWorkerSessionObservationsResult
	errs  map[string]error
}

func (source *fleetObservationListSource) ListWorkerSessionObservations(
	_ context.Context,
	request workersessions.ListWorkerSessionObservationsRequest,
) (workersessions.ListWorkerSessionObservationsResult, error) {
	return source.pages[request.NextToken], source.errs[request.NextToken]
}
