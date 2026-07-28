package invocationreturnpolicy

import (
	"context"
	"errors"
	"testing"
)

func TestPolicyServicePrepareInvocationInputAcceptsPositionalText(t *testing.T) {
	t.Parallel()

	prepared, err := NewPolicyService().PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Arguments: []string{"hello"},
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "hello" {
		t.Fatalf("prepared = %#v, want positional hello text", prepared)
	}
}

func TestPolicyServiceResolvePrimaryResultAllowsNilContext(t *testing.T) {
	t.Parallel()

	_, err := NewPolicyService().ResolvePrimaryResult(nil, PrimaryResultSelectionInput{
		RequestID: "request-1",
		InvocationReturn: &InvocationReturnConfig{Policy: "NOT_A_POLICY"},
		WorldState: InvocationWorldState{
			WorkRequestsByID: map[string]InvocationWorkRequest{
				"request-1": {WorkItems: []WorkItem{{ID: "work-1"}}},
			},
		},
	})
	if !errors.Is(err, ErrUnsupportedReturnPolicy) {
		t.Fatalf("error = %v, want ErrUnsupportedReturnPolicy", err)
	}
}

func TestPolicyServicePrepareInvocationInputPropagatesCanceledContext(t *testing.T) {
	t.Parallel()

	service := NewPolicyService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.PrepareInvocationInput(ctx, InvocationInputPreparationRequest{
		Arguments: []string{"hello"},
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestPolicyServicePrepareInvocationInputWrapsTypedFailures(t *testing.T) {
	t.Parallel()

	service := NewPolicyService()
	_, err := service.PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Arguments: []string{"   "},
	})
	if err == nil || !errors.Is(err, ErrInvalidInvocationInput) {
		t.Fatalf("error = %v, want ErrInvalidInvocationInput", err)
	}
}

func TestPolicyServiceResolvePrimaryResultHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	service := NewPolicyService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.ResolvePrimaryResult(ctx, PrimaryResultSelectionInput{})
	if err == nil {
		t.Fatal("ResolvePrimaryResult() = nil, want context error")
	}
}
