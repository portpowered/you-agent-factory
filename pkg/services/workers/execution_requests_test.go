package workers

import (
	"reflect"
	"testing"
)

func TestCloneProviderInferenceRequestDeeplyDetachesInputTokens(t *testing.T) {
	t.Parallel()

	source := ProviderInferenceRequest{
		InputTokens: []any{
			[]any{
				map[string]any{
					"strings": []string{"alpha"},
					"bytes":   []byte("beta"),
					"labels":  map[string]string{"kind": "original"},
					"groups":  map[string][]string{"items": {"first"}},
					"scalar":  "unchanged",
				},
			},
		},
	}

	cloned := CloneProviderInferenceRequest(source)
	clonedMap := cloned.InputTokens[0].([]any)[0].(map[string]any)
	clonedMap["strings"].([]string)[0] = "changed"
	clonedMap["bytes"].([]byte)[0] = 'X'
	clonedMap["labels"].(map[string]string)["kind"] = "changed"
	clonedMap["groups"].(map[string][]string)["items"][0] = "changed"
	clonedMap["scalar"] = "changed"

	sourceMap := source.InputTokens[0].([]any)[0].(map[string]any)
	if got := sourceMap["strings"].([]string)[0]; got != "alpha" {
		t.Fatalf("source strings = %q, want detached original", got)
	}
	if got := string(sourceMap["bytes"].([]byte)); got != "beta" {
		t.Fatalf("source bytes = %q, want detached original", got)
	}
	if got := sourceMap["labels"].(map[string]string)["kind"]; got != "original" {
		t.Fatalf("source label = %q, want detached original", got)
	}
	if got := sourceMap["groups"].(map[string][]string)["items"][0]; got != "first" {
		t.Fatalf("source group = %q, want detached original", got)
	}
	if got := sourceMap["scalar"]; got != "unchanged" {
		t.Fatalf("source scalar = %#v, want original", got)
	}
}

func TestRequestClonesNormalizeEmptyInputTokensToNil(t *testing.T) {
	t.Parallel()

	if got := CloneWorkstationExecutionRequest(WorkstationExecutionRequest{
		InputTokens: []any{},
	}).InputTokens; got != nil {
		t.Fatalf("workstation input tokens = %#v, want nil", got)
	}
	if got := CloneProviderInferenceRequest(ProviderInferenceRequest{
		InputTokens: []any{},
	}).InputTokens; got != nil {
		t.Fatalf("provider input tokens = %#v, want nil", got)
	}
	if got := CloneSubprocessExecutionRequest(SubprocessExecutionRequest{
		InputTokens: []any{},
	}).InputTokens; got != nil {
		t.Fatalf("subprocess input tokens = %#v, want nil", got)
	}
}

func TestCloneProviderInferenceRequestPreservesScalarInputTokenValues(t *testing.T) {
	t.Parallel()

	source := ProviderInferenceRequest{
		InputTokens: []any{"text", float64(3), true, nil},
	}
	cloned := CloneProviderInferenceRequest(source)

	if !reflect.DeepEqual(cloned.InputTokens, source.InputTokens) {
		t.Fatalf("cloned input tokens = %#v, want %#v", cloned.InputTokens, source.InputTokens)
	}
}
