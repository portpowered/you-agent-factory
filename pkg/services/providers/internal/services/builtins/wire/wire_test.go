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
	want := []struct {
		name      string
		aliases   []string
		transport string
		command   string
	}{
		{name: "copilot-acp", transport: "stdio", command: "copilot --acp --stdio"},
		{name: "cursor-acp", transport: "stdio", command: "cursor-agent acp"},
		{name: "droid-acp", aliases: []string{"factory-droid", "factorydroid"}, transport: "stdio", command: "droid exec --output-format acp"},
		{name: "fast-agent-acp", transport: "stdio", command: "uvx fast-agent-mcp acp"},
		{name: "gemini-acp", transport: "stdio", command: "gemini --acp"},
		{name: "grok-build-acp", transport: "stdio", command: "grok agent stdio"},
		{name: "iflow-acp", transport: "stdio", command: "iflow --experimental-acp"},
		{name: "kilocode-acp", transport: "stdio", command: "npx -y @kilocode/cli acp"},
		{name: "kimi-acp", transport: "stdio", command: "kimi acp"},
		{name: "kiro-acp", transport: "stdio", command: "kiro-cli-chat acp"},
		{name: "mux-acp", transport: "stdio", command: "mux acp"},
		{name: "openclaw-acp", transport: "stdio", command: "openclaw acp"},
		{name: "opencode-acp", transport: "stdio", command: "npx -y opencode-ai acp"},
		{name: "pi-acp", transport: "stdio", command: "npx pi-acp"},
		{name: "pool-acp", transport: "stdio", command: "pool acp"},
		{name: "qoder-acp", transport: "stdio", command: "qodercli --acp"},
		{name: "qwen-acp", transport: "stdio", command: "qwen --acp"},
		{name: "reasonix-acp", transport: "stdio", command: "reasonix acp"},
		{name: "trae-acp", transport: "stdio", command: "traecli acp serve"},
		{name: "zeroclaw-acp", transport: "stdio", command: "zeroclaw acp"},
	}
	if len(first) != len(want) {
		t.Fatalf("packaged ACP count = %d, want %d", len(first), len(want))
	}
	for index, integration := range first {
		if integration.Name.String() != want[index].name || integration.ID != want[index].name || integration.Transport != want[index].transport || integration.Command != want[index].command || !reflect.DeepEqual(integration.Aliases, want[index].aliases) {
			t.Fatalf("packaged ACP integration[%d] = %#v, want name=%q aliases=%v transport=%q command=%q", index, integration, want[index].name, want[index].aliases, want[index].transport, want[index].command)
		}
	}
	droidIndex := 2
	first[droidIndex].Aliases[0] = "mutated"
	second := service.ACPIntegrations()
	if second[droidIndex].Aliases[0] != "factory-droid" {
		t.Fatalf("catalog retained caller mutation: %#v", second[droidIndex].Aliases)
	}
}
