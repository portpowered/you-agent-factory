package childcontract_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/childcontract"
)

func TestNormalize_AcceptsAndTrimsCanonicalFields(t *testing.T) {
	got, err := childcontract.Normalize(map[string]any{
		"prompt":          "  review this  ",
		"label":           "  reviewer  ",
		"preset":          "  careful  ",
		"modelProvider":   "  codex  ",
		"model":           "  gpt-test  ",
		"reasoningEffort": "  high  ",
	})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	want := childcontract.Spec{
		Prompt: "review this", Label: "reviewer", Preset: "careful",
		ModelProvider: "codex", Model: "gpt-test", ReasoningEffort: "high",
	}
	if got != want {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestNormalize_RejectsInvalidSupportedFieldValues(t *testing.T) {
	for _, field := range childcontract.SupportedFields() {
		t.Run(field, func(t *testing.T) {
			value := map[string]any{"prompt": "review", field: 42}
			_, err := childcontract.Normalize(value)
			if err == nil || !strings.Contains(err.Error(), `"`+field+`"`) {
				t.Fatalf("Normalize() error = %v, want field-specific string error", err)
			}
		})
	}
}

func TestNormalize_RequiresUsablePrompt(t *testing.T) {
	for _, value := range []map[string]any{{}, {"prompt": "   "}} {
		if _, err := childcontract.Normalize(value); err == nil {
			t.Fatalf("Normalize(%#v) error = nil, want unusable prompt error", value)
		}
	}
}
