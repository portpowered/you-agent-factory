package factory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestNormalize_AcceptsAndTrimsCanonicalFields(t *testing.T) {
	t.Parallel()
	got, err := factory.NormalizeJavaScriptChild(map[string]any{
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
	want := factory.JavaScriptChildSpec{
		Prompt: "review this", Label: "reviewer", Preset: "careful",
		ModelProvider: "codex", Model: "gpt-test", ReasoningEffort: "high",
	}
	if got != want {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestNormalize_RejectsInvalidSupportedFieldValues(t *testing.T) {
	t.Parallel()
	for _, field := range factory.JavaScriptChildSupportedFields() {
		t.Run(field, func(t *testing.T) {
			value := map[string]any{"prompt": "review", field: 42}
			_, err := factory.NormalizeJavaScriptChild(value)
			if err == nil || !strings.Contains(err.Error(), `"`+field+`"`) {
				t.Fatalf("Normalize() error = %v, want field-specific string error", err)
			}
		})
	}
}

func TestNormalize_RequiresUsablePrompt(t *testing.T) {
	t.Parallel()
	for _, value := range []map[string]any{{}, {"prompt": "   "}} {
		if _, err := factory.NormalizeJavaScriptChild(value); err == nil {
			t.Fatalf("Normalize(%#v) error = nil, want unusable prompt error", value)
		}
	}
}

func TestNormalize_RejectsUnsupportedFieldsWithoutExposingValues(t *testing.T) {
	t.Parallel()
	unsupported := []string{
		"writableRoots", "allowNetwork", "network", "allowDangerFullAccess", "dangerFullAccess",
		"schema", "outputSchema", "concurrency", "maxAgents", "duration", "timeout", "timeoutMs",
	}
	const secret = "secret-value-that-must-not-appear"
	for _, field := range unsupported {
		t.Run(field, func(t *testing.T) {
			_, err := factory.NormalizeJavaScriptChild(map[string]any{
				"prompt": "prompt-that-must-not-appear",
				field:    secret,
			})
			if err == nil {
				t.Fatal("Normalize() error = nil, want unsupported-field error")
			}
			want := `agent.run() does not support field "` + field + `"`
			if err.Error() != want {
				t.Fatalf("Normalize() error = %q, want %q", err, want)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "prompt-that-must-not-appear") {
				t.Fatalf("Normalize() error = %q, want redacted diagnostic", err)
			}
		})
	}
}
