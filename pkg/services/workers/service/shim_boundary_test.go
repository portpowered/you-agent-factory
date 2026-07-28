package service_test

import (
	"context"
	"testing"

	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
	workerservice "github.com/portpowered/infinite-you/pkg/services/workers/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type shimBoundaryRuntimeAssembly struct{}

var _ runtimeassembly.Service = (*shimBoundaryRuntimeAssembly)(nil)

func (shimBoundaryRuntimeAssembly) Build(
	context.Context,
	workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	return workers.RuntimeBuildResult{}, nil
}

// TestServiceShimNewRootConstructsPublishedWorkersService proves the transitional
// owner-local shim still forwards root construction while peers use workers/wire.
func TestServiceShimNewRootConstructsPublishedWorkersService(t *testing.T) {
	t.Parallel()

	root, err := workerservice.NewRoot(&shimBoundaryRuntimeAssembly{}, workstationswire.NewService())
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewRoot() returned nil service")
	}
	var published workers.Service = root
	if published == nil {
		t.Fatal("constructed root is nil")
	}
}
