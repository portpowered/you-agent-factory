package workers_test

import (
	"context"
	"errors"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// fakeWorkersPeer is a peer-owned stand-in that depends only on the Workers
// root package (plus approved peer root contracts such as Models and Work). It
// proves cross-service consumers can satisfy the singular root Service without
// importing Workers implementation packages or provider/*.
type fakeWorkersPeer struct {
	lastModelName string
	lastRequest   modelinference.Request
	result        modelinference.Result
	err           error

	lastRuntimeBuildRequest workers.RuntimeBuildRequest
	runtimeBuildResult      workers.RuntimeBuildResult
	runtimeBuildErr         error

	lastWorkstationDispatchRequest workers.WorkstationDispatchRequest
	workstationDispatchResult      workers.WorkstationDispatchResult
	workstationDispatchErr         error

	lastRunnerExecuteRequest workers.RunnerExecuteRequest
	runnerExecuteResult      workers.RunnerExecuteResult
	runnerExecuteErr         error
}

func (f *fakeWorkersPeer) InvokeModel(
	_ context.Context,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	f.lastModelName = modelName
	f.lastRequest = request
	return f.result, f.err
}

func (f *fakeWorkersPeer) BuildRuntime(
	_ context.Context,
	request workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	f.lastRuntimeBuildRequest = request
	return f.runtimeBuildResult, f.runtimeBuildErr
}

func (f *fakeWorkersPeer) DispatchWorkstation(
	_ context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	f.lastWorkstationDispatchRequest = request
	return f.workstationDispatchResult, f.workstationDispatchErr
}

func (f *fakeWorkersPeer) ExecuteRunner(
	_ context.Context,
	request workers.RunnerExecuteRequest,
) (workers.RunnerExecuteResult, error) {
	f.lastRunnerExecuteRequest = request
	return f.runnerExecuteResult, f.runnerExecuteErr
}

var _ workers.Service = (*fakeWorkersPeer)(nil)

func TestServiceRootContract_FakeImplementsAndExercisesSingularSeam(t *testing.T) {
	fake := &fakeWorkersPeer{
		result: modelinference.Result{
			ModelName: "MODEL-A",
			Operation: "summarize",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "ok",
			}},
		},
	}
	// Peers consume only the singular root Service seam. RuntimeService
	// composition helpers are not required for the published root authority.
	var service workers.Service = fake
	ctx := context.Background()

	result, err := service.InvokeModel(ctx, "MODEL-A", modelinference.Request{
		Operation: "summarize",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "hello",
		}},
	})
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if result.ModelName != "MODEL-A" || len(result.Content) != 1 || result.Content[0].Text != "ok" {
		t.Fatalf("result = %#v, want MODEL-A content ok", result)
	}
	if fake.lastModelName != "MODEL-A" || fake.lastRequest.Operation != "summarize" {
		t.Fatalf(
			"routed = (%q, %#v), want MODEL-A summarize",
			fake.lastModelName,
			fake.lastRequest,
		)
	}
}

func TestServiceRootContract_FakeTypedFailureThroughSingularSeam(t *testing.T) {
	wantErr := modelinference.ErrNotFound
	fake := &fakeWorkersPeer{err: wantErr}
	var service workers.Service = fake

	_, err := service.InvokeModel(context.Background(), "missing", modelinference.Request{
		Operation: "chat",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("InvokeModel error = %v, want %v", err, wantErr)
	}
}

func TestServiceRootContract_RuntimeServiceIsNotRequiredPeerAuthority(t *testing.T) {
	// Compile-time proof: a peer that only implements Service satisfies the
	// published Workers root without also implementing RuntimeService
	// composition methods (WithCommandRunners / WithProgressPublisher /
	// ProviderCommandInjected). Those remain Factory Session opening /
	// construction helpers, not the peer source of truth for runtime-build,
	// workstation-dispatch, or Runner-neutral slices.
	var service workers.Service = &fakeWorkersPeer{}
	if service == nil {
		t.Fatal("expected non-nil Service")
	}
	_ = workers.RuntimeService(nil)
}

func TestServiceRootContract_RuntimeBuildSuccessThroughSingularSeam(t *testing.T) {
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
	fake := &fakeWorkersPeer{runtimeBuildResult: want}
	var service workers.Service = fake

	request := workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		Opening: workers.RuntimeBuildOpeningOptions{
			MockWorkers: workers.NewEmptyMockWorkersConfig(),
		},
		Roles: []workers.RuntimeBuildRoleRequest{{
			Name: "writer",
			Kind: workers.RuntimeBuildRoleKindWorker,
		}},
	}
	result, err := service.BuildRuntime(context.Background(), request)
	if err != nil {
		t.Fatalf("BuildRuntime: %v", err)
	}
	if result.RunnerSelection.RunnerID != workers.RunnerIDCodex {
		t.Fatalf("runner = %#v, want codex factory selection", result.RunnerSelection)
	}
	if len(result.Bindings) != 1 || result.Bindings[0].RoleName != "writer" {
		t.Fatalf("bindings = %#v, want detached writer binding", result.Bindings)
	}
	if fake.lastRuntimeBuildRequest.RunnerID != workers.RunnerIDCodex {
		t.Fatalf("routed request = %#v, want codex", fake.lastRuntimeBuildRequest)
	}
}

func TestServiceRootContract_RuntimeBuildTypedFailuresThroughSingularSeam(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "invalid", err: workers.ErrInvalidRuntimeBuildRequest},
		{name: "missing_runner", err: workers.ErrMissingRunnerSelection},
		{name: "unknown_runner", err: workers.ErrUnknownRunnerSelection},
		{name: "rejected", err: workers.ErrRuntimeAssemblyRejected},
		{name: "incomplete", err: workers.ErrIncompleteRuntimeAssembly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeWorkersPeer{runtimeBuildErr: tc.err}
			var service workers.Service = fake
			_, err := service.BuildRuntime(context.Background(), workers.RuntimeBuildRequest{})
			if !errors.Is(err, tc.err) {
				t.Fatalf("BuildRuntime error = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestServiceRootContract_WorkstationDispatchSuccessThroughSingularSeam(t *testing.T) {
	want := workers.WorkstationDispatchResult{
		DispatchID:      "dispatch-1",
		TransitionID:    "transition-1",
		WorkstationName: "writer",
		RunnerSelection: workers.ResolvedRunnerSelection{
			RunnerID: workers.RunnerIDCodex,
			Source:   workers.RunnerSelectionSourceWorkstation,
		},
		Outcome: workers.OutcomeAccepted,
		Output:  "done",
	}
	fake := &fakeWorkersPeer{workstationDispatchResult: want}
	var service workers.Service = fake

	request := workers.WorkstationDispatchRequest{
		DispatchID:      "dispatch-1",
		TransitionID:    "transition-1",
		WorkstationName: "writer",
		WorkerType:      "codex-worker",
		RunnerSelection: workers.ResolvedRunnerSelection{
			RunnerID: workers.RunnerIDCodex,
			Source:   workers.RunnerSelectionSourceWorkstation,
		},
	}
	result, err := service.DispatchWorkstation(context.Background(), request)
	if err != nil {
		t.Fatalf("DispatchWorkstation: %v", err)
	}
	if result.DispatchID != "dispatch-1" || result.TransitionID != "transition-1" {
		t.Fatalf("identity = %#v, want dispatch-1/transition-1", result)
	}
	if result.WorkstationName != "writer" || result.Outcome != workers.OutcomeAccepted {
		t.Fatalf("result = %#v, want writer ACCEPTED", result)
	}
	if result.RunnerSelection.RunnerID != workers.RunnerIDCodex {
		t.Fatalf("runner = %#v, want codex workstation selection", result.RunnerSelection)
	}
	if fake.lastWorkstationDispatchRequest.WorkstationName != "writer" {
		t.Fatalf("routed request = %#v, want writer", fake.lastWorkstationDispatchRequest)
	}
}

func TestServiceRootContract_WorkstationDispatchTypedFailuresThroughSingularSeam(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "invalid", err: workers.ErrInvalidWorkstationDispatchRequest},
		{name: "routing_rejected", err: workers.ErrWorkstationDispatchRoutingRejected},
		{name: "cancelled", err: workers.ErrWorkstationDispatchCancelled},
		{name: "saturated", err: workers.ErrWorkstationDispatchSaturated},
		{name: "incomplete", err: workers.ErrIncompleteWorkstationDispatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeWorkersPeer{workstationDispatchErr: tc.err}
			var service workers.Service = fake
			_, err := service.DispatchWorkstation(context.Background(), workers.WorkstationDispatchRequest{})
			if !errors.Is(err, tc.err) {
				t.Fatalf("DispatchWorkstation error = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestServiceRootContract_RunnerExecuteSuccessThroughSingularSeam(t *testing.T) {
	wantCapabilities := workers.NewCapabilities(
		workers.RunnerOptionalCapabilitySupport{
			Capability: workers.RunnerOptionalCapabilityWorkingDirectory,
			Status:     workers.RunnerOptionalCapabilityStatusSupported,
		},
	)
	want := workers.RunnerExecuteResult{
		RunnerID:     workers.RunnerIDCodex,
		Kind:         workers.RunnerKindBuiltIn,
		Capabilities: wantCapabilities,
		Outcome:      workers.OutcomeAccepted,
		Output:       "runner-ok",
	}
	fake := &fakeWorkersPeer{runnerExecuteResult: want}
	var service workers.Service = fake

	request := workers.RunnerExecuteRequest{
		RunnerID: workers.RunnerIDCodex,
		Kind:     workers.RunnerKindBuiltIn,
		Validation: workers.RunnerValidationRequest{
			RunnerID: workers.RunnerIDCodex,
			Kind:     workers.RunnerKindBuiltIn,
			RequiredCapabilities: []workers.RunnerOptionalCapability{
				workers.RunnerOptionalCapabilityWorkingDirectory,
			},
		},
		Input:            "summarize the change",
		WorkingDirectory: "/tmp/work",
	}
	result, err := service.ExecuteRunner(context.Background(), request)
	if err != nil {
		t.Fatalf("ExecuteRunner: %v", err)
	}
	if result.RunnerID != workers.RunnerIDCodex || result.Kind != workers.RunnerKindBuiltIn {
		t.Fatalf("identity = %#v, want codex built_in", result)
	}
	if result.Outcome != workers.OutcomeAccepted || result.Output != "runner-ok" {
		t.Fatalf("result = %#v, want ACCEPTED runner-ok", result)
	}
	if len(result.Capabilities.Optional) != 1 ||
		result.Capabilities.Optional[0].Capability != workers.RunnerOptionalCapabilityWorkingDirectory {
		t.Fatalf("capabilities = %#v, want working_directory support", result.Capabilities)
	}
	if fake.lastRunnerExecuteRequest.RunnerID != workers.RunnerIDCodex {
		t.Fatalf("routed request = %#v, want codex", fake.lastRunnerExecuteRequest)
	}
	if fake.lastRunnerExecuteRequest.Validation.Kind != workers.RunnerKindBuiltIn {
		t.Fatalf("validation = %#v, want built_in kind", fake.lastRunnerExecuteRequest.Validation)
	}
}

func TestServiceRootContract_RunnerExecuteTypedFailuresThroughSingularSeam(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "invalid", err: workers.ErrInvalidRunnerRequest},
		{name: "unsupported_capability", err: workers.ErrUnsupportedRunnerCapability},
		{name: "execution_failed", err: workers.ErrRunnerExecutionFailed},
		{name: "incomplete", err: workers.ErrIncompleteRunnerExecution},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeWorkersPeer{runnerExecuteErr: tc.err}
			var service workers.Service = fake
			_, err := service.ExecuteRunner(context.Background(), workers.RunnerExecuteRequest{})
			if !errors.Is(err, tc.err) {
				t.Fatalf("ExecuteRunner error = %v, want %v", err, tc.err)
			}
		})
	}
}
