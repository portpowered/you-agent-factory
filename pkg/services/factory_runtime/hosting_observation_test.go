package factory_test

import (
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
