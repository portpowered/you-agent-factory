package interfaces

import (
	"slices"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestSupportedModelProviders_IncludesAllCanonicalCommands(t *testing.T) {
	got := SupportedModelProviders()
	want := []ModelProvider{
		ModelProviderClaude,
		ModelProviderCodex,
		ModelProviderGemini,
		ModelProviderKiro,
		ModelProviderCursor,
		ModelProviderOpenCode,
	}
	if len(got) != len(want) {
		t.Fatalf("supported provider count = %d, want %d", len(got), len(want))
	}
	for _, provider := range want {
		if !slices.Contains(got, provider) {
			t.Fatalf("supported providers missing %q", provider)
		}
	}
}

func TestModelProviderPublicInternalMapping_RoundTripsAllSupportedProviders(t *testing.T) {
	cases := []struct {
		public factoryapi.WorkerModelProvider
		want   ModelProvider
	}{
		{factoryapi.WorkerModelProviderClaude, ModelProviderClaude},
		{factoryapi.WorkerModelProviderCodex, ModelProviderCodex},
		{factoryapi.WorkerModelProviderCursor, ModelProviderCursor},
		{factoryapi.WorkerModelProviderGemini, ModelProviderGemini},
		{factoryapi.WorkerModelProviderKiro, ModelProviderKiro},
		{factoryapi.WorkerModelProviderOpenCode, ModelProviderOpenCode},
	}

	for _, tt := range cases {
		t.Run(string(tt.public), func(t *testing.T) {
			internal, ok := InternalModelProviderFromPublicWorkerModelProvider(tt.public)
			if !ok || internal != tt.want {
				t.Fatalf("InternalModelProviderFromPublicWorkerModelProvider(%q) = (%q, %v), want (%q, true)", tt.public, internal, ok, tt.want)
			}
			public, ok := PublicWorkerModelProviderFromInternal(internal)
			if !ok || public != tt.public {
				t.Fatalf("PublicWorkerModelProviderFromInternal(%q) = (%q, %v), want (%q, true)", internal, public, ok, tt.public)
			}
		})
	}
}

func TestGeneratedPublicFactoryWorkerModelProvider_CanonicalizesProviderAliases(t *testing.T) {
	cases := []struct {
		input string
		want  factoryapi.WorkerModelProvider
	}{
		{"gemini", factoryapi.WorkerModelProviderGemini},
		{"kiro-cli", factoryapi.WorkerModelProviderKiro},
		{"opencode", factoryapi.WorkerModelProviderOpenCode},
		{"GEMINI", factoryapi.WorkerModelProviderGemini},
		{"KIRO", factoryapi.WorkerModelProviderKiro},
		{"OPENCODE", factoryapi.WorkerModelProviderOpenCode},
	}

	for _, tt := range cases {
		t.Run(tt.input, func(t *testing.T) {
			if got := GeneratedPublicFactoryWorkerModelProvider(tt.input); got != tt.want {
				t.Fatalf("GeneratedPublicFactoryWorkerModelProvider(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStrictPublicFactoryWorkerModelProvider_AcceptsAllCanonicalPublicValues(t *testing.T) {
	for _, provider := range []string{
		"CLAUDE", "CODEX", "CURSOR", "GEMINI", "KIRO", "OPENCODE",
	} {
		if got := StrictPublicFactoryWorkerModelProvider(provider); got != provider {
			t.Fatalf("StrictPublicFactoryWorkerModelProvider(%q) = %q, want %q", provider, got, provider)
		}
	}
}
