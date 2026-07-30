package wire_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonboulle/clockwork"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const deletedServiceImportPrefix = "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service"

// DEL-RUN-SERVICE proof tests verify the transitional factory_runtime/service
// public tree is gone, wire still constructs the published Service root, and
// engine/pipeline public packages are folded under orchestration/instance_host.

func TestServiceDeletionProof_WireConstructsPublishedControlObservationDispatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, err := factoryruntimewire.NewService(
		func() string { return "del-run-service-proof-id" },
		nil,
		nil,
		clockwork.NewFakeClock(),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(
			context.Context,
			workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			return workers.WorkstationDispatchCancelResult{}, nil
		},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var root factoryruntime.Service = service
	if root == nil {
		t.Fatal("NewService() returned nil published Service root")
	}

	_, err = root.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeStatus,
	})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("Observe(STATUS) error = %v, want ErrNotRunning", err)
	}

	_, err = root.PlanDispatch(ctx, factoryruntime.PlanDispatchRequest{DispatchID: "del-run-proof-dispatch"})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("PlanDispatch() error = %v, want ErrNotRunning", err)
	}

	_, err = root.ControlPause(ctx, factoryruntime.PauseRequest{})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("ControlPause() error = %v, want ErrNotRunning", err)
	}
}
