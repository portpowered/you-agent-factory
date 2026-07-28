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

type successfulACPProvidersService struct{}

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

func (successfulACPProvidersService) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (successfulACPProvidersService) GetProvider(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (successfulACPProvidersService) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{
		Content:    "completed over ACP",
		SessionRef: &providers.SessionRef{Provider: "cursor-acp", Kind: "acp", ID: "session-1"},
		Diagnostics: &providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{
			{Phase: "started", Metadata: map[string]string{"kind": "reasoning", "item_id": "reasoning-1", "native_type": "session/update"}},
			{Phase: "updated", Detail: "thinking", Metadata: map[string]string{"kind": "reasoning", "item_id": "reasoning-1", "native_type": "session/update"}},
			{Phase: "completed", Detail: "done", Metadata: map[string]string{"kind": "reasoning", "item_id": "reasoning-1", "native_type": "session/update"}},
			{Phase: "updated", Detail: "tool", Metadata: map[string]string{"kind": "tool", "item_id": "tool-1", "status": "completed", "raw_input": `{}`, "raw_output": `{}`, "native_type": "session/update"}},
			{Phase: "updated", Metadata: map[string]string{"kind": "usage", "used_tokens": "7", "native_type": "session/update"}},
		}},
	}, nil
}

func TestACPProgressProjectionCoversProtocolEventKinds(t *testing.T) {
	t.Parallel()

	cases := []providers.ExecuteProgress{
		{Phase: "started", Metadata: map[string]string{"kind": "message", "item_id": "m1", "native_type": "session/update"}},
		{Phase: "updated", Detail: "delta", Metadata: map[string]string{"kind": "message", "item_id": "m1", "native_type": "session/update"}},
		{Phase: "completed", Detail: "done", Metadata: map[string]string{"kind": "message", "item_id": "m1", "native_type": "session/update"}},
		{Phase: "started", Metadata: map[string]string{"kind": "reasoning", "item_id": "r1", "native_type": "session/update"}},
		{Phase: "completed", Detail: "reason", Metadata: map[string]string{"kind": "reasoning", "item_id": "r1", "native_type": "session/update"}},
		{Phase: "started", Detail: "tool", Metadata: map[string]string{"kind": "tool", "item_id": "t1", "status": "running", "native_type": "session/update"}},
		{Phase: "updated", Detail: "tool", Metadata: map[string]string{"kind": "tool", "item_id": "t1", "status": "completed", "raw_input": `{}`, "raw_output": `{}`, "native_type": "session/update"}},
		{Phase: "updated", Metadata: map[string]string{"kind": "plan", "entries": `[{"content":"ship","status":"completed"}]`, "native_type": "session/update"}},
		{Phase: "updated", Metadata: map[string]string{"kind": "usage", "used_tokens": "12", "native_type": "session/update"}},
		{Phase: "started", Metadata: map[string]string{"kind": "session", "native_type": "session/update"}},
		{Phase: "updated", Metadata: map[string]string{"kind": "file_change", "path": "main.go", "operation": "modify", "native_type": "session/update"}},
	}
	for index, progress := range cases {
		if _, ok := acpProgressDraft("run", "cursor-acp", "session", progress); !ok {
			t.Fatalf("acpProgressDraft(%d) ok = false", index)
		}
	}
	if _, ok := acpProgressDraft("run", "cursor-acp", "session", providers.ExecuteProgress{Metadata: map[string]string{"kind": "unknown"}}); ok {
		t.Fatal("acpProgressDraft(unknown) ok = true")
	}
	if acpInferenceSession(nil) != nil || acpInferenceSession(&providers.SessionRef{Provider: "cursor-acp", Kind: "acp", ID: "s1"}) == nil {
		t.Fatal("acpInferenceSession projection mismatch")
	}
	for _, kind := range []workers.Kind{workers.KindReasoning, workers.KindTool, workers.KindSession, workers.KindMessage} {
		_ = acpLifecycleCompletionOrder(kind)
	}
	if _, ok := acpCompletedMessageDraft("run", "cursor-acp", "session", "done"); !ok {
		t.Fatal("acpCompletedMessageDraft() ok = false")
	}
	if _, ok := acpPartialMessageDraft("run", "cursor-acp", "session", "partial"); !ok {
		t.Fatal("acpPartialMessageDraft() ok = false")
	}
	if got := acpRegistryRegistrations(successfulACPProvidersService{}, " cursor-acp "); len(got) != 1 {
		t.Fatalf("acpRegistryRegistrations() = %#v", got)
	}
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
			Diagnostics: &providers.ExecuteDiagnostics{
				Progress: []providers.ExecuteProgress{
					{Phase: "started", Metadata: map[string]string{"kind": "reasoning", "item_id": "r1", "native_type": "session/update"}},
					{Phase: "started", Detail: "tool", Metadata: map[string]string{"kind": "tool", "item_id": "t1", "status": "running", "native_type": "session/update"}},
					{Phase: "started", Metadata: map[string]string{"kind": "message", "item_id": "m1", "native_type": "session/update"}},
				},
				Metadata: map[string]string{"partial_content": "partial response"},
			},
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

func TestACPProviderIntegrationProjectsSuccessfulProgressAndSession(t *testing.T) {
	t.Parallel()

	integration := &acpProviderIntegration{id: "cursor-acp", providers: successfulACPProvidersService{}}
	if discovery, err := integration.Discover(t.Context()); err != nil || discovery.Readiness() != inference.ReadinessReady {
		t.Fatalf("Discover() = (%#v, %v)", discovery, err)
	}
	if capabilities, err := integration.Capabilities(t.Context(), inference.InvocationRequest{}); err != nil || len(capabilities.Values()) == 0 {
		t.Fatalf("Capabilities() = (%#v, %v)", capabilities, err)
	}
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "dispatch-success",
		UserMessage:  "run ACP",
		Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
		Execution:    workers.ProviderInferenceRequest{Dispatch: work.WorkDispatch{DispatchID: "dispatch-success"}},
	})
	destination := &acpIntegrationDestination{}
	if err := inference.ExecuteInvocation(t.Context(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation() = %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil || destination.completion.Failure() != nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
}

func TestACPProviderIntegrationMapsDeclaredFailureKinds(t *testing.T) {
	t.Parallel()

	for _, kind := range []providers.ExecuteFailureKind{
		providers.ExecuteFailureKindAuthentication,
		providers.ExecuteFailureKindInvalidRequest,
		providers.ExecuteFailureKindMisconfigured,
		providers.ExecuteFailureKindCanceled,
		providers.ExecuteFailureKindTimeout,
		providers.ExecuteFailureKindUnknown,
	} {
		kind := kind
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			integration := &acpProviderIntegration{id: "cursor-acp", providers: failingACPProvidersService{failure: providers.ExecuteFailure{Kind: kind, Message: "declared failure"}}}
			request := inference.NewInvocationRequest(inference.InvocationInput{
				InvocationID: "dispatch-" + string(kind),
				UserMessage:  "run ACP",
				Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
				Execution:    workers.ProviderInferenceRequest{Dispatch: work.WorkDispatch{DispatchID: "dispatch-" + string(kind)}},
			})
			destination := &acpIntegrationDestination{}
			if err := inference.ExecuteInvocation(t.Context(), integration, request, destination); err != nil {
				t.Fatalf("ExecuteInvocation() = %v", err)
			}
			if destination.completion == nil || destination.completion.Failure() == nil {
				t.Fatalf("completion = %#v, want failure", destination.completion)
			}
		})
	}
}
