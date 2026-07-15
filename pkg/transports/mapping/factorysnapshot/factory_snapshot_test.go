package factorysnapshot

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestToAPIPreservesPublicFactoryShape(t *testing.T) {
	t.Parallel()

	workers := []api.Worker{{Name: "reviewer"}}
	workstations := []api.Workstation{{Name: "review", Worker: "reviewer"}}
	want := api.Factory{
		Name:         "factory-a",
		Workers:      &workers,
		Workstations: &workstations,
	}
	snapshot, err := interfaces.NewFactorySnapshot(want)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	got, err := ToAPI(snapshot)
	if err != nil {
		t.Fatalf("ToAPI: %v", err)
	}
	if got.Name != want.Name || got.Workers == nil || len(*got.Workers) != 1 || (*got.Workers)[0].Name != "reviewer" {
		t.Fatalf("ToAPI = %#v, want public factory shape preserved", got)
	}
	if got.Workstations == nil || len(*got.Workstations) != 1 || (*got.Workstations)[0].Name != "review" {
		t.Fatalf("ToAPI workstations = %#v, want review", got.Workstations)
	}
}
