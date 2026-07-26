package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
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

func TestBuildRuntimeDelegatesThroughWorkersRoot(t *testing.T) {
	t.Parallel()

	want := workers.RuntimeBuildResult{
		RunnerSelection: workers.ResolvedRunnerSelection{
			RunnerID: workers.RunnerIDCodex,
			Source:   workers.RunnerSelectionSourceFactory,
		},
		Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: "writer",
			RoleKind: workers.RuntimeBuildRoleKindWorker,
			RunnerSelection: workers.ResolvedRunnerSelection{
				RunnerID: workers.RunnerIDCodex,
				Source:   workers.RunnerSelectionSourceFactory,
			},
		}},
	}
	assembly := &recordingRuntimeAssembly{result: want}
	var root workers.Service = &Service{runtimeAssembly: assembly}
	request := workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		Roles: []workers.RuntimeBuildRoleRequest{{
			Name: "writer",
			Kind: workers.RuntimeBuildRoleKindWorker,
		}},
	}

	got, err := root.BuildRuntime(t.Context(), request)
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if assembly.request.RunnerID != request.RunnerID ||
		len(assembly.request.Roles) != 1 ||
		assembly.request.Roles[0] != request.Roles[0] {
		t.Fatalf("delegated request = %#v, want %#v", assembly.request, request)
	}
	if got.RunnerSelection != want.RunnerSelection ||
		len(got.Bindings) != 1 ||
		got.Bindings[0] != want.Bindings[0] {
		t.Fatalf("BuildRuntime() = %#v, want %#v", got, want)
	}
}

func TestBuildRuntimePreservesAssemblyErrorIdentity(t *testing.T) {
	t.Parallel()

	for _, wantErr := range []error{
		workers.ErrInvalidRuntimeBuildRequest,
		workers.ErrMissingRunnerSelection,
		workers.ErrUnknownRunnerSelection,
		workers.ErrRuntimeAssemblyRejected,
		workers.ErrIncompleteRuntimeAssembly,
	} {
		wantErr := wantErr
		t.Run(wantErr.Error(), func(t *testing.T) {
			t.Parallel()

			assembly := &recordingRuntimeAssembly{err: wantErr}
			var root workers.Service = &Service{runtimeAssembly: assembly}

			result, err := root.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{})
			if !errors.Is(err, wantErr) {
				t.Fatalf("BuildRuntime() error = %v, want errors.Is(_, %v)", err, wantErr)
			}
			if result.RunnerSelection != (workers.ResolvedRunnerSelection{}) ||
				len(result.Bindings) != 0 {
				t.Fatalf("BuildRuntime() result = %#v, want no usable bindings", result)
			}
		})
	}
}

func TestBuildRuntimeRequiresPrivateAssemblyCapability(t *testing.T) {
	t.Parallel()

	for _, root := range []workers.Service{
		(*Service)(nil),
		&Service{},
	} {
		result, err := root.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{})
		if !errors.Is(err, workers.ErrIncompleteRuntimeAssembly) {
			t.Fatalf(
				"BuildRuntime() error = %v, want errors.Is(_, ErrIncompleteRuntimeAssembly)",
				err,
			)
		}
		if result.RunnerSelection != (workers.ResolvedRunnerSelection{}) ||
			len(result.Bindings) != 0 {
			t.Fatalf("BuildRuntime() result = %#v, want no usable bindings", result)
		}
	}
}
