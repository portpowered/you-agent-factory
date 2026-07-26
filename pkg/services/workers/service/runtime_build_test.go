package service

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
)

type recordingRuntimeAssembly struct {
	request workers.RuntimeBuildRequest
	result  workers.RuntimeBuildResult
	err     error
}

var _ runtimeassembly.Service = (*recordingRuntimeAssembly)(nil)

type inertConstructionSpy struct {
	currentRuntimeCalls int
	commandCalls        int
}

func (spy *inertConstructionSpy) CurrentRuntime() *factorysessions.LiveRuntime {
	spy.currentRuntimeCalls++
	return nil
}

func (spy *inertConstructionSpy) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	spy.commandCalls++
	return workers.CommandResult{}, errors.New("unexpected runner execution")
}

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
		workers.ErrUnsupportedRunnerCapability,
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

type runtimeRunnerSpy struct {
	calls int
}

func (runner *runtimeRunnerSpy) Execute(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	runner.calls++
	return workers.RunnerExecutionResult{}, errors.New("unexpected runner execution")
}

func TestRuntimeBuildUsesPrivateRegistryWithoutRunnerExecution(t *testing.T) {
	t.Parallel()

	runner := &runtimeRunnerSpy{}
	metadata, ok := workers.BuiltInRunnerMetadata(workers.RunnerIDCodex)
	if !ok {
		t.Fatal("Codex metadata is unavailable")
	}
	assembly, err := newRuntimeAssemblyFromRegistrations([]runners.Registration{{
		Identity: workers.RunnerIDCodex,
		Metadata: metadata,
		Runner:   runner,
	}})
	if err != nil {
		t.Fatalf("newRuntimeAssemblyFromRegistrations() error = %v", err)
	}
	var root workers.Service = &Service{runtimeAssembly: assembly}
	valid := workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		RequiredRunnerCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityWorktree,
		},
		Roles: []workers.RuntimeBuildRoleRequest{{
			Name: "writer",
			Kind: workers.RuntimeBuildRoleKindWorker,
		}},
	}

	result, err := root.BuildRuntime(t.Context(), valid)
	if err != nil {
		t.Fatalf("BuildRuntime(valid) error = %v", err)
	}
	if result.RunnerSelection.RunnerID != workers.RunnerIDCodex ||
		len(result.Bindings) != 1 ||
		result.Bindings[0].RunnerSelection != result.RunnerSelection {
		t.Fatalf("BuildRuntime(valid) = %#v, want registry-backed binding", result)
	}

	cases := []struct {
		name    string
		request workers.RuntimeBuildRequest
		want    error
	}{
		{
			name: "missing",
			request: workers.RuntimeBuildRequest{
				Roles: valid.Roles,
			},
			want: workers.ErrMissingRunnerSelection,
		},
		{
			name: "unknown",
			request: workers.RuntimeBuildRequest{
				RunnerID: "unknown",
				Roles:    valid.Roles,
			},
			want: workers.ErrUnknownRunnerSelection,
		},
		{
			name: "unsupported",
			request: workers.RuntimeBuildRequest{
				RunnerID: workers.RunnerIDCodex,
				RequiredRunnerCapabilities: []workers.RunnerOptionalCapability{
					workers.RunnerOptionalCapability("unsupported"),
				},
				Roles: valid.Roles,
			},
			want: workers.ErrUnsupportedRunnerCapability,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			failed, buildErr := root.BuildRuntime(t.Context(), test.request)
			if !errors.Is(buildErr, test.want) {
				t.Fatalf("BuildRuntime() error = %v, want %v", buildErr, test.want)
			}
			if len(failed.Bindings) != 0 {
				t.Fatalf("BuildRuntime() result = %#v, want unusable result", failed)
			}
		})
	}
	if runner.calls != 0 {
		t.Fatalf("runner execution calls = %d, want zero", runner.calls)
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

func TestRuntimeAssemblyConstructionAndBuildAreInert(t *testing.T) {
	t.Parallel()

	spy := &inertConstructionSpy{}
	assembly, err := newRuntimeAssembly(nil)
	if err != nil {
		t.Fatalf("newRuntimeAssembly() error = %v", err)
	}
	var root workers.Service = &Service{
		sessions:              spy,
		providerCommandRunner: spy,
		scriptCommandRunner:   spy,
		runtimeAssembly:       assembly,
	}
	if spy.currentRuntimeCalls != 0 || spy.commandCalls != 0 {
		t.Fatalf(
			"construction side effects = current runtime %d, commands %d; want zero",
			spy.currentRuntimeCalls,
			spy.commandCalls,
		)
	}

	result, err := root.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		Roles: []workers.RuntimeBuildRoleRequest{
			{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
			{Name: "review", Kind: workers.RuntimeBuildRoleKindWorkstation},
		},
	})
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if len(result.Bindings) != 2 {
		t.Fatalf("BuildRuntime() bindings = %#v, want two", result.Bindings)
	}
	if spy.currentRuntimeCalls != 0 || spy.commandCalls != 0 {
		t.Fatalf(
			"assembly side effects = current runtime %d, commands %d; want zero",
			spy.currentRuntimeCalls,
			spy.commandCalls,
		)
	}
}
