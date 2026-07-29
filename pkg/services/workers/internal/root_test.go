package internal_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
)

type recordingRuntimeAssembly struct {
	request workers.RuntimeBuildRequest
	result  workers.RuntimeBuildResult
	err     error
}

var _ runtimeassembly.Service = (*recordingRuntimeAssembly)(nil)

func (assembly *recordingRuntimeAssembly) Build(
	_ context.Context,
	request workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	assembly.request = request
	return assembly.result, assembly.err
}

func TestNewRootConstructsPublishedWorkersService(t *testing.T) {
	t.Parallel()

	root, err := workersinternal.NewRoot(&recordingRuntimeAssembly{}, workstationswire.NewService())
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

func TestNewRootRejectsMissingOwners(t *testing.T) {
	t.Parallel()

	if _, err := workersinternal.NewRoot(nil, workstationswire.NewService()); err == nil {
		t.Fatal("NewRoot(nil assembly) error = nil, want missing runtime assembly")
	}
	if _, err := workersinternal.NewRoot(&recordingRuntimeAssembly{}, nil); err == nil {
		t.Fatal("NewRoot(nil workstations) error = nil, want missing workstations owner")
	}
}

func TestNewRootBuildRuntimeDelegatesWithoutLifecycle(t *testing.T) {
	t.Parallel()

	want := workers.RuntimeBuildResult{
		RunnerSelection: workers.ResolvedRunnerSelection{
			RunnerID: workers.RunnerIDAntigravity,
			Source:   workers.RunnerSelectionSourceFactory,
		},
	}
	assembly := &recordingRuntimeAssembly{result: want}
	root, err := workersinternal.NewRoot(assembly, workstationswire.NewService())
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}

	got, err := root.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDAntigravity,
		Roles: []workers.RuntimeBuildRoleRequest{{
			Name: "writer",
			Kind: workers.RuntimeBuildRoleKindWorker,
		}},
	})
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if got.RunnerSelection != want.RunnerSelection {
		t.Fatalf("BuildRuntime() = %#v, want runner selection %#v", got, want.RunnerSelection)
	}
}
