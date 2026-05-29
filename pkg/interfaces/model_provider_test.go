package interfaces

import (
	"slices"
	"testing"
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
