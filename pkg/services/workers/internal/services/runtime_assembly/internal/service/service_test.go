package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
)

func TestBuildReturnsDetachedBindingsInRequestOrder(t *testing.T) {
	request := detachmentRequest()

	var retained []workers.RuntimeBuildOpeningOptions
	assembler := New(
		recognizedRunner(workers.RunnerIDCodex),
		func(
			_ context.Context,
			role workers.RuntimeBuildRoleRequest,
			opening workers.RuntimeBuildOpeningOptions,
			selection workers.ResolvedRunnerSelection,
		) (workers.AssembledRuntimeBinding, error) {
			retained = append(retained, opening)
			return bindingFor(role, selection), nil
		},
	)

	first, err := assembler.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	assertOrderedBindings(t, first)

	request.Roles[0].Name = "mutated-request"
	*request.Opening.InvocationSkipPermissionsOverride = false
	request.Opening.MockWorkers.MockWorkers[0].WorkInputs[0].WorkID = "mutated-work"
	*request.Opening.MockWorkers.MockWorkers[0].RejectConfig.ExitCode = 99
	assertRetainedOpening(t, retained[0])
	first.Bindings[0].RoleName = "mutated-result"
	*retained[0].InvocationSkipPermissionsOverride = false
	retained[0].MockWorkers.MockWorkers[0].WorkInputs[0].WorkID = "mutated-retained"

	secondRequest := detachmentRequest()
	second, err := assembler.Build(context.Background(), secondRequest)
	if err != nil {
		t.Fatalf("second Build() error = %v", err)
	}
	assertDetachedSecondBuild(t, second, retained)
}

func assertOrderedBindings(t *testing.T, result workers.RuntimeBuildResult) {
	t.Helper()
	if len(result.Bindings) != 2 ||
		result.Bindings[0].RoleName != "writer" ||
		result.Bindings[0].RoleKind != workers.RuntimeBuildRoleKindWorker ||
		result.Bindings[1].RoleName != "review" ||
		result.Bindings[1].RoleKind != workers.RuntimeBuildRoleKindWorkstation {
		t.Fatalf("Build() bindings = %#v, want writer then review", result.Bindings)
	}
	if result.RunnerSelection.RunnerID != workers.RunnerIDCodex ||
		result.RunnerSelection.Source != workers.RunnerSelectionSourceFactory {
		t.Fatalf("Build() runner selection = %#v", result.RunnerSelection)
	}
}

func assertRetainedOpening(t *testing.T, opening workers.RuntimeBuildOpeningOptions) {
	t.Helper()
	if !*opening.InvocationSkipPermissionsOverride ||
		opening.MockWorkers.MockWorkers[0].WorkInputs[0].WorkID != "work-1" ||
		*opening.MockWorkers.MockWorkers[0].RejectConfig.ExitCode != 17 {
		t.Fatalf("retained opening changed with request mutation: %#v", opening)
	}
}

func assertDetachedSecondBuild(
	t *testing.T,
	second workers.RuntimeBuildResult,
	retained []workers.RuntimeBuildOpeningOptions,
) {
	t.Helper()
	if second.Bindings[0].RoleName != "writer" ||
		second.Bindings[1].RoleName != "review" {
		t.Fatalf("second Build() bindings = %#v, want detached originals", second.Bindings)
	}
	if retained[3].InvocationSkipPermissionsOverride == retained[2].InvocationSkipPermissionsOverride ||
		retained[3].MockWorkers == retained[2].MockWorkers {
		t.Fatal("per-role opening options alias each other")
	}
	if retained[3].MockWorkers.MockWorkers[0].WorkInputs[0].WorkID != "work-1" {
		t.Fatalf(
			"second role opening WorkID = %q, want work-1",
			retained[3].MockWorkers.MockWorkers[0].WorkInputs[0].WorkID,
		)
	}
}

func TestBuildRejectsInvalidRequestsBeforeCallingCollaborators(t *testing.T) {
	for _, test := range invalidRequestCases() {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			assembler := New(
				func(context.Context, string) (workers.ResolvedRunnerSelection, bool, error) {
					calls++
					return workers.ResolvedRunnerSelection{}, false, nil
				},
				func(
					context.Context,
					workers.RuntimeBuildRoleRequest,
					workers.RuntimeBuildOpeningOptions,
					workers.ResolvedRunnerSelection,
				) (workers.AssembledRuntimeBinding, error) {
					calls++
					return workers.AssembledRuntimeBinding{}, nil
				},
			)
			result, err := assembler.Build(context.Background(), test.request)
			if !errors.Is(err, test.want) {
				t.Fatalf("Build() error = %v, want %v", err, test.want)
			}
			if len(result.Bindings) != 0 {
				t.Fatalf("Build() bindings = %#v, want none", result.Bindings)
			}
			if calls != 0 {
				t.Fatalf("collaborator calls = %d, want 0", calls)
			}
		})
	}
}

func TestBuildReturnsTypedResolutionAndAssemblyFailuresAtomically(t *testing.T) {
	rejection := errors.New("policy denied")
	cases := append(resolutionFailureCases(rejection), bindingFailureCases(rejection)...)
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assembler := New(test.resolver, test.assembler)
			result, err := assembler.Build(context.Background(), detachmentRequest())
			if !errors.Is(err, test.want) {
				t.Fatalf("Build() error = %v, want %v", err, test.want)
			}
			if len(result.Bindings) != 0 {
				t.Fatalf("Build() bindings = %#v, want atomic empty result", result.Bindings)
			}
		})
	}
}

type invalidRequestCase struct {
	name    string
	request workers.RuntimeBuildRequest
	want    error
}

func invalidRequestCases() []invalidRequestCase {
	invalid := workers.ErrInvalidRuntimeBuildRequest
	return []invalidRequestCase{
		{
			name:    "missing runner",
			request: workers.RuntimeBuildRequest{Roles: validRoles()},
			want:    workers.ErrMissingRunnerSelection,
		},
		{
			name:    "whitespace runner",
			request: runtimeRequest(" codex ", validRoles()),
			want:    invalid,
		},
		{
			name:    "no roles",
			request: runtimeRequest(workers.RunnerIDCodex, nil),
			want:    invalid,
		},
		{
			name: "empty role name",
			request: runtimeRequest(workers.RunnerIDCodex, []workers.RuntimeBuildRoleRequest{{
				Kind: workers.RuntimeBuildRoleKindWorker,
			}}),
			want: invalid,
		},
		{
			name: "malformed role name",
			request: runtimeRequest(workers.RunnerIDCodex, []workers.RuntimeBuildRoleRequest{{
				Name: " writer ",
				Kind: workers.RuntimeBuildRoleKindWorker,
			}}),
			want: invalid,
		},
		{
			name: "unknown role kind",
			request: runtimeRequest(workers.RunnerIDCodex, []workers.RuntimeBuildRoleRequest{{
				Name: "writer",
				Kind: "agent",
			}}),
			want: invalid,
		},
		{
			name: "duplicate role",
			request: runtimeRequest(workers.RunnerIDCodex, []workers.RuntimeBuildRoleRequest{
				{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
				{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
			}),
			want: invalid,
		},
		{
			name: "conflicting role",
			request: runtimeRequest(workers.RunnerIDCodex, []workers.RuntimeBuildRoleRequest{
				{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
				{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorkstation},
			}),
			want: invalid,
		},
		{
			name:    "invalid opening options",
			request: invalidOpeningRequest(),
			want:    invalid,
		},
	}
}

func invalidOpeningRequest() workers.RuntimeBuildRequest {
	request := runtimeRequest(workers.RunnerIDCodex, validRoles())
	request.Opening.MockWorkers = &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			RunType: workers.MockWorkerRunTypeScript,
		}},
	}
	return request
}

type assemblyFailureCase struct {
	name      string
	resolver  runtimeassembly.RunnerResolver
	assembler runtimeassembly.BindingAssembler
	want      error
}

func resolutionFailureCases(rejection error) []assemblyFailureCase {
	return []assemblyFailureCase{
		{
			name: "missing collaborators",
			want: workers.ErrIncompleteRuntimeAssembly,
		},
		{
			name: "unknown runner",
			resolver: func(
				context.Context,
				string,
			) (workers.ResolvedRunnerSelection, bool, error) {
				return workers.ResolvedRunnerSelection{}, false, nil
			},
			assembler: successfulAssembler,
			want:      workers.ErrUnknownRunnerSelection,
		},
		{
			name: "resolver rejection",
			resolver: func(
				context.Context,
				string,
			) (workers.ResolvedRunnerSelection, bool, error) {
				return workers.ResolvedRunnerSelection{}, false, rejection
			},
			assembler: successfulAssembler,
			want:      workers.ErrRuntimeAssemblyRejected,
		},
		{
			name: "incomplete runner",
			resolver: func(
				context.Context,
				string,
			) (workers.ResolvedRunnerSelection, bool, error) {
				return workers.ResolvedRunnerSelection{RunnerID: workers.RunnerIDCodex}, true, nil
			},
			assembler: successfulAssembler,
			want:      workers.ErrIncompleteRuntimeAssembly,
		},
	}
}

func bindingFailureCases(rejection error) []assemblyFailureCase {
	rejectSecond := func(
		_ context.Context,
		role workers.RuntimeBuildRoleRequest,
		_ workers.RuntimeBuildOpeningOptions,
		selection workers.ResolvedRunnerSelection,
	) (workers.AssembledRuntimeBinding, error) {
		if role.Name == "review" {
			return workers.AssembledRuntimeBinding{}, rejection
		}
		return bindingFor(role, selection), nil
	}
	incompleteSecond := func(
		_ context.Context,
		role workers.RuntimeBuildRoleRequest,
		_ workers.RuntimeBuildOpeningOptions,
		selection workers.ResolvedRunnerSelection,
	) (workers.AssembledRuntimeBinding, error) {
		if role.Name == "review" {
			return workers.AssembledRuntimeBinding{}, nil
		}
		return bindingFor(role, selection), nil
	}
	conflicting := func(
		_ context.Context,
		role workers.RuntimeBuildRoleRequest,
		_ workers.RuntimeBuildOpeningOptions,
		selection workers.ResolvedRunnerSelection,
	) (workers.AssembledRuntimeBinding, error) {
		binding := bindingFor(role, selection)
		binding.RoleName = "different"
		return binding, nil
	}
	return []assemblyFailureCase{
		{
			name:      "second binding rejected",
			resolver:  recognizedRunner(workers.RunnerIDCodex),
			assembler: rejectSecond,
			want:      workers.ErrRuntimeAssemblyRejected,
		},
		{
			name:      "incomplete second binding",
			resolver:  recognizedRunner(workers.RunnerIDCodex),
			assembler: incompleteSecond,
			want:      workers.ErrIncompleteRuntimeAssembly,
		},
		{
			name:      "conflicting binding",
			resolver:  recognizedRunner(workers.RunnerIDCodex),
			assembler: conflicting,
			want:      workers.ErrRuntimeAssemblyRejected,
		},
	}
}

func detachmentRequest() workers.RuntimeBuildRequest {
	skipPermissions := true
	exitCode := 17
	request := runtimeRequest(
		workers.RunnerIDCodex,
		[]workers.RuntimeBuildRoleRequest{
			{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
			{Name: "review", Kind: workers.RuntimeBuildRoleKindWorkstation},
		},
	)
	request.Opening = workers.RuntimeBuildOpeningOptions{
		MockWorkers: &workers.MockWorkersConfig{
			MockWorkers: []workers.MockWorkerConfig{{
				ID:      "mock-1",
				RunType: workers.MockWorkerRunTypeReject,
				WorkInputs: []workers.MockWorkInputSelector{{
					WorkID: "work-1",
				}},
				RejectConfig: &workers.MockWorkerRejectConfig{ExitCode: &exitCode},
			}},
		},
		InvocationSkipPermissionsOverride: &skipPermissions,
	}
	return request
}

func runtimeRequest(
	runnerID string,
	roles []workers.RuntimeBuildRoleRequest,
) workers.RuntimeBuildRequest {
	return workers.RuntimeBuildRequest{RunnerID: runnerID, Roles: roles}
}

func recognizedRunner(identity string) runtimeassembly.RunnerResolver {
	return func(
		_ context.Context,
		requested string,
	) (workers.ResolvedRunnerSelection, bool, error) {
		if requested != identity {
			return workers.ResolvedRunnerSelection{}, false, nil
		}
		return workers.ResolvedRunnerSelection{
			RunnerID: identity,
			Source:   workers.RunnerSelectionSourceFactory,
		}, true, nil
	}
}

func successfulAssembler(
	_ context.Context,
	role workers.RuntimeBuildRoleRequest,
	_ workers.RuntimeBuildOpeningOptions,
	selection workers.ResolvedRunnerSelection,
) (workers.AssembledRuntimeBinding, error) {
	return bindingFor(role, selection), nil
}

func bindingFor(
	role workers.RuntimeBuildRoleRequest,
	selection workers.ResolvedRunnerSelection,
) workers.AssembledRuntimeBinding {
	return workers.AssembledRuntimeBinding{
		RoleName:        role.Name,
		RoleKind:        role.Kind,
		RunnerSelection: selection,
	}
}

func validRoles() []workers.RuntimeBuildRoleRequest {
	return []workers.RuntimeBuildRoleRequest{{
		Name: "writer",
		Kind: workers.RuntimeBuildRoleKindWorker,
	}}
}

func TestBuildPreservesCollaboratorFailureContext(t *testing.T) {
	rejection := errors.New("denied")
	assembler := New(
		recognizedRunner(workers.RunnerIDCodex),
		func(
			context.Context,
			workers.RuntimeBuildRoleRequest,
			workers.RuntimeBuildOpeningOptions,
			workers.ResolvedRunnerSelection,
		) (workers.AssembledRuntimeBinding, error) {
			return workers.AssembledRuntimeBinding{}, rejection
		},
	)
	_, err := assembler.Build(context.Background(), workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		Roles:    validRoles(),
	})
	if !errors.Is(err, workers.ErrRuntimeAssemblyRejected) ||
		!errors.Is(err, rejection) ||
		err == nil ||
		!strings.Contains(err.Error(), rejection.Error()) {
		t.Fatalf("Build() error = %v, want typed rejection with context", err)
	}
}
