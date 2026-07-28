package wire

import (
	"context"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

type failingACPProvidersService struct{ failure providers.ExecuteFailure }

func (s failingACPProvidersService) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func TestACPProviderIntegrationPreservesDependencyFailureThroughConductor(t *testing.T) {
	t.Parallel()

	service := failingACPProvidersService{failure: providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindMisconfigured,
		Message: `ACP provider "cursor-acp" negotiated unsupported protocol version 999`,
	}}
	registry, err := buildProviderRegistry(serviceedges.Edges{}, service)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "dispatch-1",
		UserMessage:  "run ACP",
		Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
		Execution: workers.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
		},
	})
	destination := &acpIntegrationDestination{}
	if err := conductor.New(registry).Invoke(t.Context(), "cursor-acp", request, destination); err != nil {
		t.Fatalf("Conductor.Invoke() error = %v", err)
	}
	if destination.completion == nil || destination.completion.Failure() == nil || destination.completion.Failure().Kind() != inference.FailureDependency {
		t.Fatalf("completion = %#v, want dependency failure", destination.completion)
	}
	if got := destination.completion.Failure().Diagnostics()["work-failure-type"]; got != string(workers.WorkFailureTypeMisconfigured) {
		t.Fatalf("work-failure-type = %q, want %q", got, workers.WorkFailureTypeMisconfigured)
	}
}

func (s failingACPProvidersService) GetProvider(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (s failingACPProvidersService) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, s.failure
}

type acpIntegrationDestination struct{ completion *inference.Completion }

func (*acpIntegrationDestination) WriteEvent(context.Context, inference.EventDraft) error { return nil }

func (d *acpIntegrationDestination) Close(_ context.Context, completion inference.Completion) error {
	d.completion = &completion
	return nil
}

func TestACPProviderIntegrationPreservesDependencyFailureCompletion(t *testing.T) {
	t.Parallel()

	integration := &acpProviderIntegration{
		id: "cursor-acp",
		providers: failingACPProvidersService{failure: providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "ACP executable is unavailable",
		}},
	}
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "dispatch-1",
		UserMessage:  "run ACP",
		Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
		Execution: workers.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
		},
	})
	destination := &acpIntegrationDestination{}
	if err := inference.ExecuteInvocation(t.Context(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation() error = %v", err)
	}
	if destination.completion == nil || destination.completion.Failure() == nil {
		t.Fatalf("completion = %#v, want dependency failure", destination.completion)
	}
	failure := destination.completion.Failure()
	if failure.Kind() != inference.FailureDependency || failure.Diagnostics()["work-failure-type"] != string(workers.WorkFailureTypeMissingExecutable) {
		t.Fatalf("failure = %#v diagnostics=%#v", failure, failure.Diagnostics())
	}
}
