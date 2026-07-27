package factory

import (
	"testing"
)

func TestFactoryStatusFromObservation_MapsNeutralFields(t *testing.T) {
	t.Parallel()
	observation := Observation{
		Status: ObservationStatusActive,
		Progress: ObservationProgress{
			TotalWorkCount: 5,
			WorkCategories: ObservationWorkCategories{
				Failed: 1, Initial: 1, Processing: 1, Terminal: 2,
			},
		},
		Resources: []ObservationResourceView{{
			ResourceID: "agent-slot", InUseCount: 1, AvailableCount: 2,
		}},
		Health: ObservationHealth{
			FactoryState:           "RUNNING",
			LifecycleControlStatus: "RUNNING",
		},
	}

	got := FactoryStatusFromObservation(observation)
	if got.RuntimeStatus != "ACTIVE" || got.FactoryState != "RUNNING" || got.TotalTokens != 5 {
		t.Fatalf("status = %#v, want ACTIVE/RUNNING with five tokens", got)
	}
	if got.Categories != (FactoryStatusCategories{Failed: 1, Initial: 1, Processing: 1, Terminal: 2}) {
		t.Fatalf("categories = %#v, want neutral category counts", got.Categories)
	}
	if len(got.Resources) != 1 || got.Resources[0] != (FactoryResourceUsage{Name: "agent-slot", Available: 2, Total: 3}) {
		t.Fatalf("resources = %#v, want agent-slot 2/3", got.Resources)
	}
}

func TestProjectFactoryStatusFromObservation_DelegatesToNeutralMapper(t *testing.T) {
	t.Parallel()
	observation := Observation{
		Status: ObservationStatusFinished,
		Progress: ObservationProgress{
			TotalWorkCount: 1,
			WorkCategories: ObservationWorkCategories{Terminal: 1},
		},
		Health: ObservationHealth{FactoryState: "FINISHED"},
	}
	got := NewFactoryStatusProjector().ProjectFactoryStatusFromObservation(observation)
	want := FactoryStatusFromObservation(observation)
	if got.RuntimeStatus != want.RuntimeStatus || got.FactoryState != want.FactoryState ||
		got.TotalTokens != want.TotalTokens || got.Categories != want.Categories ||
		len(got.Resources) != len(want.Resources) {
		t.Fatalf("projector observation = %#v, want %#v", got, want)
	}
	for i := range got.Resources {
		if got.Resources[i] != want.Resources[i] {
			t.Fatalf("projector resources[%d] = %#v, want %#v", i, got.Resources[i], want.Resources[i])
		}
	}
}
