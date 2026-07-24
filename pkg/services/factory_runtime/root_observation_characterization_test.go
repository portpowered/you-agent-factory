package factory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// peerObservationService extends the singular root Service fake with
// observation-slice outcomes. It depends only on published root types plus
// approved peer contracts and never imports factory_runtime/internal.
type peerObservationService struct {
	peerRootService

	observeErr error
	observation factoryruntime.Observation
}

var _ factoryruntime.Service = (*peerObservationService)(nil)

func (s *peerObservationService) Observe(_ context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	if s.observeErr != nil {
		return factoryruntime.ObserveResult{}, s.observeErr
	}
	return factoryruntime.ObserveResult{Observation: s.observation}, nil
}

func TestRootObservation_FakePeerRunningSuccessShape(t *testing.T) {
	t.Parallel()

	want := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			InFlightDispatchCount: 1,
			TickCount:             7,
		},
		InFlightDispatches: []factoryruntime.ObservationDispatchSummary{{
			DispatchID:      "dispatch-1",
			WorkIDs:         []string{"work-1"},
			WorkstationName: "plan",
			Status:          "IN_FLIGHT",
		}},
		Results: []factoryruntime.ObservationResultView{{
			DispatchID: "dispatch-0",
			WorkID:     "work-0",
			Outcome:    "SUCCESS",
		}},
		Resources: []factoryruntime.ObservationResourceView{{
			ResourceID:     "resource-slot",
			ResourceName:   "executor-slot",
			ResourceType:   "INVOCATION_SLOT",
			InUseCount:     1,
			AvailableCount: 0,
		}},
		Health: factoryruntime.ObservationHealth{
			FactoryState:           "RUNNING",
			LifecycleControlStatus: "RUNNING",
			StreamGenerationID:     "gen-1",
			Uptime:                 2 * time.Second,
		},
	}
	var runtime factoryruntime.Service = &peerObservationService{observation: want}

	got, err := factoryruntime.ApplyObserve(context.Background(), runtime, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		t.Fatalf("ApplyObserve error = %v, want nil", err)
	}
	if got.Observation.Status != want.Status {
		t.Fatalf("Observation.Status = %q, want %q", got.Observation.Status, want.Status)
	}
	if got.Observation.Progress != want.Progress {
		t.Fatalf("Observation.Progress = %#v, want %#v", got.Observation.Progress, want.Progress)
	}
	if len(got.Observation.InFlightDispatches) != 1 || got.Observation.InFlightDispatches[0].DispatchID != "dispatch-1" {
		t.Fatalf("InFlightDispatches = %#v, want plain dispatch summary", got.Observation.InFlightDispatches)
	}
	if len(got.Observation.Results) != 1 || got.Observation.Results[0].Outcome != "SUCCESS" {
		t.Fatalf("Results = %#v, want plain result view", got.Observation.Results)
	}
	if len(got.Observation.Resources) != 1 || got.Observation.Resources[0].ResourceID != "resource-slot" {
		t.Fatalf("Resources = %#v, want plain resource view", got.Observation.Resources)
	}
	if got.Observation.Health.StreamGenerationID != "gen-1" || got.Observation.Health.Uptime != 2*time.Second {
		t.Fatalf("Health = %#v, want retained live health", got.Observation.Health)
	}
}

func TestRootObservation_FakePeerTypedFailures(t *testing.T) {
	t.Parallel()

	t.Run("missing instance", func(t *testing.T) {
		t.Parallel()
		_, err := factoryruntime.ApplyObserve(context.Background(), nil, factoryruntime.ObserveRequest{})
		if !errors.Is(err, factoryruntime.ErrNotFound) {
			t.Fatalf("ApplyObserve(nil) error = %v, want ErrNotFound", err)
		}
	})

	t.Run("not running", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerObservationService{observeErr: factoryruntime.ErrNotRunning}
		_, err := factoryruntime.ApplyObserve(context.Background(), runtime, factoryruntime.ObserveRequest{
			Scope: factoryruntime.ObservationScopeFull,
		})
		if !errors.Is(err, factoryruntime.ErrNotRunning) {
			t.Fatalf("ApplyObserve error = %v, want ErrNotRunning", err)
		}
	})

	t.Run("invalid observation scope", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerObservationService{}
		_, err := factoryruntime.ApplyObserve(context.Background(), runtime, factoryruntime.ObserveRequest{
			Scope: factoryruntime.ObservationScope("petri-marking"),
		})
		if !errors.Is(err, factoryruntime.ErrInvalidObservationScope) {
			t.Fatalf("ApplyObserve error = %v, want ErrInvalidObservationScope", err)
		}
	})
}
