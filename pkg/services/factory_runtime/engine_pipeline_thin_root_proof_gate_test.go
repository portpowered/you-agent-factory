package factory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPublishedServiceRejectsOperationsBeforeActivation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clock := clockwork.NewFakeClock()
	scheduler := platformclock.Real{}
	lifecycle, err := factoryruntimewire.NewLifecycle(clock, scheduler)
	if err != nil {
		t.Fatalf("NewLifecycle() error = %v", err)
	}
	host, err := factoryruntimewire.NewInstanceHost(clock, scheduler, lifecycle)
	if err != nil {
		t.Fatalf("NewInstanceHost() error = %v", err)
	}
	mapper, err := factoryruntimewire.NewDefinitionMapper(func() string { return "del-run-engine-pipeline-thin-root-proof-id" })
	if err != nil {
		t.Fatal(err)
	}
	orchestration := factoryruntimewire.NewOrchestration(mapper, nil, nil)
	dispatchPlan := factoryruntimewire.NewDispatchPlanning(
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
			return workers.WorkstationDispatchCancelResult{}, nil
		},
	)
	service, err := factoryruntimewire.NewService(orchestration, host, dispatchPlan)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var published factoryruntime.Service = service
	if published == nil {
		t.Fatal("NewService() returned nil published Service root")
	}

	_, err = published.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeStatus,
	})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("Observe(STATUS) error = %v, want ErrNotRunning", err)
	}

	_, err = published.PlanDispatch(ctx, factoryruntime.PlanDispatchRequest{
		DispatchID: "del-run-engine-pipeline-thin-root-proof-dispatch",
	})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("PlanDispatch() error = %v, want ErrNotRunning", err)
	}

	_, err = published.ControlPause(ctx, factoryruntime.PauseRequest{})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("ControlPause() error = %v, want ErrNotRunning", err)
	}
}
