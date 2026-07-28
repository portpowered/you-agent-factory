package factory_test

import (
	"context"
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestObservationHasActiveWork(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		obs  factoryruntime.Observation
		want bool
	}{
		{
			name: "in-flight dispatch count",
			obs: factoryruntime.Observation{
				Progress: factoryruntime.ObservationProgress{InFlightDispatchCount: 1},
			},
			want: true,
		},
		{
			name: "processing work category",
			obs: factoryruntime.Observation{
				Progress: factoryruntime.ObservationProgress{
					WorkCategories: factoryruntime.ObservationWorkCategories{Processing: 1},
				},
			},
			want: true,
		},
		{
			name: "terminal-only work",
			obs: factoryruntime.Observation{
				Progress: factoryruntime.ObservationProgress{
					WorkCategories: factoryruntime.ObservationWorkCategories{Terminal: 2},
				},
			},
			want: false,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := factoryruntime.ObservationHasActiveWork(test.obs); got != test.want {
				t.Fatalf("ObservationHasActiveWork() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRequireIdleRuntimeFromObservation(t *testing.T) {
	t.Parallel()

	idle := factoryruntime.Observation{Status: factoryruntime.ObservationStatusIdle}
	if err := factoryruntime.RequireIdleRuntimeFromObservation(idle); err != nil {
		t.Fatalf("RequireIdleRuntimeFromObservation(idle) = %v, want nil", err)
	}

	if err := factoryruntime.RequireIdleRuntimeFromObservation(factoryruntime.Observation{}); err == nil ||
		!errors.Is(err, interfaces.ErrFactoryActivationRequiresIdle) {
		t.Fatalf("RequireIdleRuntimeFromObservation(empty) = %v, want ErrFactoryActivationRequiresIdle", err)
	}

	active := factoryruntime.Observation{Status: factoryruntime.ObservationStatusActive}
	if err := factoryruntime.RequireIdleRuntimeFromObservation(active); err == nil ||
		!errors.Is(err, interfaces.ErrFactoryActivationRequiresIdle) {
		t.Fatalf("RequireIdleRuntimeFromObservation(active) = %v, want ErrFactoryActivationRequiresIdle", err)
	}

	busy := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusIdle,
		Progress: factoryruntime.ObservationProgress{
			WorkCategories: factoryruntime.ObservationWorkCategories{Processing: 1},
		},
	}
	if err := factoryruntime.RequireIdleRuntimeFromObservation(busy); err == nil ||
		!errors.Is(err, interfaces.ErrFactoryActivationRequiresIdle) {
		t.Fatalf("RequireIdleRuntimeFromObservation(busy) = %v, want ErrFactoryActivationRequiresIdle", err)
	}
}

// peerShapedRuntimeService is a minimal Service fake that exercises observation
// without implementing LegacySnapshotProvider or APIFactory snapshot methods.
type peerShapedRuntimeService struct {
	factoryruntime.Service
	observation factoryruntime.Observation
}

func (f *peerShapedRuntimeService) Observe(
	_ context.Context,
	req factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	if req.Scope == "" {
		return factoryruntime.ObserveResult{}, factoryruntime.ErrInvalidObservationScope
	}
	return factoryruntime.ObserveResult{Observation: f.observation}, nil
}

func TestPeerShapedServiceFakeObservesWithoutLegacySnapshot(t *testing.T) {
	t.Parallel()

	want := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Health: factoryruntime.ObservationHealth{FactoryState: "RUNNING"},
	}
	runtime := &peerShapedRuntimeService{observation: want}

	var _ factoryruntime.Service = runtime
	if _, ok := any(runtime).(factoryruntime.LegacySnapshotProvider); ok {
		t.Fatal("peer-shaped Service fake must not implement LegacySnapshotProvider")
	}

	result, err := runtime.Observe(context.Background(), factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeHealth,
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if result.Observation.Status != want.Status || result.Observation.Health.FactoryState != want.Health.FactoryState {
		t.Fatalf("observation = %#v, want %#v", result.Observation, want)
	}
}

func TestPeerShapedServiceFakeObserveScopedViewsWithoutPetriMarkings(t *testing.T) {
	t.Parallel()

	observation := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			TickCount:             12,
			InFlightDispatchCount: 2,
			WorkCategories: factoryruntime.ObservationWorkCategories{
				Processing: 2,
			},
		},
		Health: factoryruntime.ObservationHealth{
			FactoryState:           "RUNNING",
			LifecycleControlStatus: "RUNNING",
			ActiveThrottlePauses: []interfaces.ActiveThrottlePause{{
				LaneID: "openai/gpt-5",
			}},
		},
	}
	runtime := &peerShapedRuntimeService{observation: observation}

	progress, err := runtime.Observe(context.Background(), factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeProgress,
	})
	if err != nil {
		t.Fatalf("Observe(PROGRESS): %v", err)
	}
	if progress.Observation.Progress.TickCount != 12 || progress.Observation.Progress.InFlightDispatchCount != 2 {
		t.Fatalf("progress observation = %#v, want tick 12 and two in-flight dispatches", progress.Observation.Progress)
	}

	health, err := runtime.Observe(context.Background(), factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeHealth,
	})
	if err != nil {
		t.Fatalf("Observe(HEALTH): %v", err)
	}
	if health.Observation.Health.FactoryState != "RUNNING" ||
		len(health.Observation.Health.ActiveThrottlePauses) != 1 ||
		health.Observation.Health.ActiveThrottlePauses[0].LaneID != "openai/gpt-5" {
		t.Fatalf("health observation = %#v, want RUNNING with one throttle pause", health.Observation.Health)
	}
}
