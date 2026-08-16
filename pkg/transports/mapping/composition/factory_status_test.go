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

type factoryStatusProjectorRole struct {
	status      factoryruntime.FactoryStatus
	projections int
}

func (role *factoryStatusRuntimeRole) Observe(_ context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	role.observations++
	return factoryruntime.ObserveResult{Observation: role.observation}, nil
}

func (role *factoryStatusProjectorRole) ProjectFactoryStatusFromObservation(factoryruntime.Observation) factoryruntime.FactoryStatus {
	role.projections++
	return role.status
}

func TestFactoryStatusAPIUsesBoundRuntimeObservationForCurrentAndSessionRoutes(t *testing.T) {
	scopedObservation := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			TotalWorkCount: 99,
			WorkCategories: factoryruntime.ObservationWorkCategories{Processing: 7, Terminal: 92},
		},
		Health: factoryruntime.ObservationHealth{FactoryState: "OBSERVATION_SOURCE"},
	}
	runtime := &factoryStatusRuntimeRole{observation: scopedObservation}
	projector := &factoryStatusProjectorRole{status: factoryruntime.FactoryStatus{
		FactoryState:  "SCOPED",
		RuntimeStatus: "ACTIVE",
		TotalTokens:   2,
		Categories:    factoryruntime.FactoryStatusCategories{Processing: 1},
	}}
	api := newFactoryStatusAPI(runtime, projector)

	got, err := api.ProjectFactoryStatus(context.Background(), "")
	if err != nil || got.FactoryState != "SCOPED" || got.RuntimeStatus != "ACTIVE" ||
		got.TotalTokens != 2 || got.Categories.Processing != 1 || runtime.observations != 1 || projector.projections != 1 {
		t.Fatalf("default status = (%#v, %v), observations = %d, projections = %d", got, err, runtime.observations, projector.projections)
	}
	got, err = api.ProjectFactoryStatus(context.Background(), "session-beta")
	if err != nil || got.FactoryState != "SCOPED" || got.TotalTokens != 2 || runtime.observations != 2 || projector.projections != 2 {
		t.Fatalf("scoped status = (%#v, %v), observations = %d, projections = %d", got, err, runtime.observations, projector.projections)
	}
}
