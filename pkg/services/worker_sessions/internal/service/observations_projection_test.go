package service

import (
	"context"
	"errors"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestListWorkerSessionObservationsKeepsBaseFactsWhenProviderProjectionIsUnavailable(t *testing.T) {
	registry := newObservationRegistry(nil, nil)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	registry.observations["worker-1"] = observationMetadata()

	result, err := registry.ListWorkerSessionObservations(
		context.Background(),
		workersessions.ListWorkerSessionObservationsRequest{},
	)
	if !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("top-level list error = %v, want optional projection error", err)
	}
	if len(result.Observations) != 1 || result.Observations[0].WorkerSessionID != "worker-1" {
		t.Fatalf("top-level list result = %#v, want preserved base observation", result)
	}
	if result.Observations[0].State != workersessions.StateRunning || result.Observations[0].WorkIDs[0] != "work-1" {
		t.Fatalf("preserved base observation = %#v, want lifecycle and Work facts", result.Observations[0])
	}
}
