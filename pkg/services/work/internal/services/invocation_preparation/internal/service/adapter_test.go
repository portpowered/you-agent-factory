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
	input := work.InvocationInputPreparationRequest{Signature: &work.InvocationSignatureConfig{UnknownNamedArgumentPolicy: "REJECT", Parameters: []work.InvocationParameterConfig{{Name: "document", Aliases: []string{"file"}, Choices: []string{"text"}, DefaultValues: []string{"default"}, Bindings: []work.InvocationParameterBindingConfig{{Kind: "NAMED", Position: 1}}}}}, CompatibilityContent: []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"document":true}`), Metadata: map[string]any{"labels": []string{"first"}, "bytes": []byte("raw"), "nested": map[string]any{"rows": []any{"a"}}, "tags": map[string]string{"name": "input"}, "sets": map[string][]string{"names": []string{"first"}}}}}, Arguments: []string{"draft"}, DirectArgs: []work.NamedArgumentInput{{Key: "input", Values: []string{"original"}}}}
	adapter := InvocationInputPreparationAdapter{Inner: fakeInvocation(func(ctx context.Context, mapped invocationreturnpolicy.InvocationInputPreparationRequest) (invocationreturnpolicy.PreparedInvocationInput, error) {
		if ctx != t.Context() || mapped.Arguments[0] != "draft" || mapped.DirectArgs[0].Values[0] != "original" {
			t.Fatalf("mapped input = %#v", mapped)
		}
		if mapped.Signature.Parameters[0].Name != "document" || mapped.Signature.Parameters[0].Aliases[0] != "file" || len(mapped.CompatibilityContent) != 1 {
			t.Fatalf("signature/content mapping = %#v", mapped)
		}
		mapped.DirectArgs[0].Values[0] = "changed"
		return invocationreturnpolicy.PreparedInvocationInput{Source: invocationreturnpolicy.InputSourcePositionalText, ResolvedInput: &invocationreturnpolicy.ResolvedInput{Text: "prepared"}, NormalizedArguments: &invocationreturnpolicy.NormalizedArguments{Arguments: map[string]invocationreturnpolicy.NormalizedArgument{"document": {Values: []string{"prepared"}, Sensitive: true, Sources: []invocationreturnpolicy.ArgumentSource{{Kind: invocationreturnpolicy.ArgumentSourceKindNamed, Name: "file", Redact: true}}}}, UnknownNamedArgs: map[string][]string{"extra": []string{"value"}}, CompatibilityInput: &invocationreturnpolicy.ResolvedInput{Text: "prepared"}}}, nil
	})}
	got, err := adapter.PrepareInvocationInput(t.Context(), input)
	if got.NormalizedArguments == nil || got.NormalizedArguments.Arguments["document"].Sources[0].Name != "file" || got.NormalizedArguments.UnknownNamedArgs["extra"][0] != "value" {
		t.Fatalf("normalized result = %#v", got)
	}
	if err != nil || got.ResolvedInput == nil || got.ResolvedInput.Text != "prepared" || input.DirectArgs[0].Values[0] != "original" {
		t.Fatalf("result = %#v, error = %v", got, err)
	}
}

func TestInvocationMappingPreservesErrorIdentity(t *testing.T) {
	t.Parallel()
	input := work.InvocationInputPreparationRequest{}
	adapter := InvocationInputPreparationAdapter{}
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
	_, err := adapter.PrepareInvocationInput(t.Context(), input)
	var typed *work.InputError
	if !errors.As(err, &typed) || typed.Code != work.InputErrorCodeSourceConflict {
		t.Fatalf("typed error = %v", err)
	}
}

func TestInvocationMappingPreservesExactFileText(t *testing.T) {
	t.Parallel()
	path := "long prompt.txt"
	want := "  line one\r\nline two — 東京\r\n"
	adapter := InvocationInputPreparationAdapter{Inner: fakeInvocation(func(ctx context.Context, request invocationreturnpolicy.InvocationInputPreparationRequest) (invocationreturnpolicy.PreparedInvocationInput, error) {
		if ctx != t.Context() || request.FilePath == nil || *request.FilePath != path {
			t.Fatalf("mapped file request = %#v", request)
		}
		return invocationreturnpolicy.PreparedInvocationInput{Source: invocationreturnpolicy.InputSourceFileText, ResolvedInput: &invocationreturnpolicy.ResolvedInput{Text: want}}, nil
	})}
	got, err := adapter.PrepareInvocationInput(t.Context(), work.InvocationInputPreparationRequest{FilePath: &path})
	if err != nil || got.Source != work.InputSourceFileText || got.ResolvedInput == nil || got.ResolvedInput.Text != want {
		t.Fatalf("mapped output = %#v, error = %v, want exact file text", got, err)
	}
}
