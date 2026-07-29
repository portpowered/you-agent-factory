package wire

import (
	"reflect"
	"testing"
)

func TestPackagedACPCatalogIsExactAndDetached(t *testing.T) {
	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	first := service.ACPIntegrations()
	want := []string{
		"pi-acp", "openclaw-acp", "gemini-acp",
		"cursor-acp", "copilot-acp", "droid-acp", "fast-agent-acp",
		"grok-build-acp", "iflow-acp", "kilocode-acp", "kimi-acp", "kiro-acp",
		"mux-acp", "opencode-acp", "pool-acp", "qoder-acp", "qwen-acp",
		"reasonix-acp", "trae-acp", "zeroclaw-acp",
	}
	got := make([]string, len(first))
	for index, integration := range first {
		got[index] = integration.Name.String()
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packaged ACP identities = %#v, want %#v", got, want)
	}
	if len(first[5].Aliases) != 2 || first[5].Aliases[0] != "factory-droid" || first[5].Aliases[1] != "factorydroid" {
		t.Fatalf("droid aliases = %#v", first[5].Aliases)
	}
	first[5].Aliases[0] = "mutated"
	second := service.ACPIntegrations()
	if second[5].Aliases[0] != "factory-droid" {
		t.Fatalf("catalog retained caller mutation: %#v", second[5].Aliases)
	}
}
