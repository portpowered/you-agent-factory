package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"
)

type fakeInvocation func(context.Context, invocationreturnpolicy.InvocationInputPreparationRequest) (invocationreturnpolicy.PreparedInvocationInput, error)

func (f fakeInvocation) PrepareInvocationInput(ctx context.Context, input invocationreturnpolicy.InvocationInputPreparationRequest) (invocationreturnpolicy.PreparedInvocationInput, error) {
	return f(ctx, input)
}
func TestInvocationMappingPreservesValuesAndErrorIdentity(t *testing.T) {
	t.Parallel()
	input := work.InvocationInputPreparationRequest{Arguments: []string{"draft"}, DirectArgs: []work.NamedArgumentInput{{Key: "input", Values: []string{"original"}}}}
	adapter := InvocationInputPreparationAdapter{Inner: fakeInvocation(func(ctx context.Context, mapped invocationreturnpolicy.InvocationInputPreparationRequest) (invocationreturnpolicy.PreparedInvocationInput, error) {
		if ctx != t.Context() || mapped.Arguments[0] != "draft" || mapped.DirectArgs[0].Values[0] != "original" {
			t.Fatalf("mapped input = %#v", mapped)
		}
		mapped.DirectArgs[0].Values[0] = "changed"
		return invocationreturnpolicy.PreparedInvocationInput{Source: invocationreturnpolicy.InputSourcePositionalText, ResolvedInput: &invocationreturnpolicy.ResolvedInput{Text: "prepared"}}, nil
	})}
	got, err := adapter.PrepareInvocationInput(t.Context(), input)
	if err != nil || got.ResolvedInput == nil || got.ResolvedInput.Text != "prepared" || input.DirectArgs[0].Values[0] != "original" {
		t.Fatalf("result = %#v, error = %v", got, err)
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("reader failed")} {
		adapter.Inner = fakeInvocation(func(context.Context, invocationreturnpolicy.InvocationInputPreparationRequest) (invocationreturnpolicy.PreparedInvocationInput, error) {
			return invocationreturnpolicy.PreparedInvocationInput{}, cause
		})
		_, err := adapter.PrepareInvocationInput(t.Context(), input)
		if !errors.Is(err, cause) {
			t.Fatalf("error = %v, want %v", err, cause)
		}
	}
	adapter.Inner = fakeInvocation(func(context.Context, invocationreturnpolicy.InvocationInputPreparationRequest) (invocationreturnpolicy.PreparedInvocationInput, error) {
		return invocationreturnpolicy.PreparedInvocationInput{}, &invocationreturnpolicy.InputError{Code: invocationreturnpolicy.InputErrorCodeSourceConflict, Message: "conflicting input"}
	})
	_, err = adapter.PrepareInvocationInput(t.Context(), input)
	var typed *work.InputError
	if !errors.As(err, &typed) || typed.Code != work.InputErrorCodeSourceConflict {
		t.Fatalf("typed error = %v", err)
	}
}
