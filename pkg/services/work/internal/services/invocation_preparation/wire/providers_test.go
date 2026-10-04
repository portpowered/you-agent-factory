package wire

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"
)

type selectedInvocationPolicy struct{ failure error }

func (p selectedInvocationPolicy) PrepareInvocationInput(ctx context.Context, input invocationreturnpolicy.InvocationInputPreparationRequest) (invocationreturnpolicy.PreparedInvocationInput, error) {
	return invocationreturnpolicy.PreparedInvocationInput{ResolvedInput: &invocationreturnpolicy.ResolvedInput{Text: input.Arguments[0]}}, p.failure
}
func TestInvocationAdapterUsesCompletedPolicy(t *testing.T) {
	t.Parallel()
	adapter := NewInvocationInputAdapter(selectedInvocationPolicy{})
	got, err := adapter.PrepareInvocationInput(t.Context(), work.InvocationInputPreparationRequest{Arguments: []string{"selected"}})
	if err != nil || got.ResolvedInput == nil || got.ResolvedInput.Text != "selected" {
		t.Fatalf("prepared=%#v,error=%v", got, err)
	}
	adapter = NewInvocationInputAdapter(selectedInvocationPolicy{failure: context.Canceled})
	if _, err := adapter.PrepareInvocationInput(t.Context(), work.InvocationInputPreparationRequest{Arguments: []string{"selected"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("policy error=%v", err)
	}
}
