package composition

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type factoryStatusRuntimeRole struct {
	factoryruntime.Service
	observation  factoryruntime.Observation
	observations int
}

func (role *factoryStatusRuntimeRole) Observe(_ context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	role.observations++
	return factoryruntime.ObserveResult{Observation: role.observation}, nil
}

func TestFactoryStatusAPIUsesBoundRuntimeObservationForCurrentAndSessionRoutes(t *testing.T) {
	scopedObservation := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			TotalWorkCount: 2,
			WorkCategories: factoryruntime.ObservationWorkCategories{Processing: 1, Terminal: 1},
		},
		Health: factoryruntime.ObservationHealth{FactoryState: "SCOPED"},
	}
	runtime := &factoryStatusRuntimeRole{observation: scopedObservation}
	api := newFactoryStatusAPI(runtime, factoryruntime.NewFactoryStatusProjector())

	got, err := api.ProjectFactoryStatus(context.Background(), "")
	if err != nil || got.FactoryState != "SCOPED" || got.RuntimeStatus != "ACTIVE" ||
		got.TotalTokens != 2 || got.Categories.Processing != 1 || runtime.observations != 1 {
		t.Fatalf("default status = (%#v, %v), observations = %d", got, err, runtime.observations)
	}
	got, err = api.ProjectFactoryStatus(context.Background(), "session-beta")
	if err != nil || got.FactoryState != "SCOPED" || got.TotalTokens != 2 || runtime.observations != 2 {
		t.Fatalf("scoped status = (%#v, %v), observations = %d", got, err, runtime.observations)
	}
}
