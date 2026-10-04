package wire

import (
	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	"reflect"
	"strings"
	"testing"
)

func TestPackagedACPCatalogIsExactAndDetached(t *testing.T) {
	t.Parallel()
	first, err := DecodeACPIntegrations(modelproviders.RuntimeACPJSON())
	if err != nil {
		t.Fatalf("DecodeACPIntegrations() error = %v", err)
	}

	want := []struct {
		name      string
		aliases   []string
		transport string
		command   string
		profile   string
	}{
		{name: "copilot-acp", transport: "stdio", command: "copilot --acp --stdio"},
		{name: "cursor", transport: "stdio", command: "cursor-agent acp", profile: "cursor-acp"},
		{name: "droid-acp", aliases: []string{"factory-droid", "factorydroid"}, transport: "stdio", command: "droid exec --output-format acp"},
		{name: "fast-agent-acp", transport: "stdio", command: "uvx fast-agent-mcp acp"},
		{name: "gemini", transport: "stdio", command: "gemini --acp", profile: "gemini-acp"},
		{name: "grok-build-acp", transport: "stdio", command: "grok agent stdio"},
		{name: "iflow-acp", transport: "stdio", command: "iflow --experimental-acp"},
		{name: "kilocode-acp", transport: "stdio", command: "npx -y @kilocode/cli acp"},
		{name: "kimi-acp", transport: "stdio", command: "kimi acp"},
		{name: "kiro", transport: "stdio", command: "kiro-cli-chat acp", profile: "kiro-acp"},
		{name: "mux-acp", transport: "stdio", command: "mux acp"},
		{name: "openclaw-acp", transport: "stdio", command: "openclaw acp"},
		{name: "opencode", transport: "stdio", command: "opencode acp", profile: "opencode-acp"},
		{name: "pi", transport: "stdio", command: "you pi-acp", profile: "pi-acp"},
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
		if got := integration.Arguments; !reflect.DeepEqual(got, strings.Fields(want[index].command)[1:]) {
			t.Fatalf("packaged ACP integration[%d] arguments = %#v, want command arguments %#v", index, got, strings.Fields(want[index].command)[1:])
		}
		wantProfile := want[index].name
		if want[index].profile != "" {
			wantProfile = want[index].profile
		}
		if integration.ImplementationProfile != wantProfile {
			t.Fatalf("packaged ACP integration[%d] profile = %q, want %q", index, integration.ImplementationProfile, wantProfile)
		}
		if integration.RuntimePosture != wantPosture(integration.Name.String()) {
			t.Fatalf("packaged ACP integration[%d] posture = %q, want %q", index, integration.RuntimePosture, wantPosture(integration.Name.String()))
		}
	}
	droidIndex := 2
	first[droidIndex].Aliases[0] = "mutated"
	second, err := DecodeACPIntegrations(modelproviders.RuntimeACPJSON())
	if err != nil {
		t.Fatalf("DecodeACPIntegrations() error = %v", err)
	}
	if second[droidIndex].Aliases[0] != "factory-droid" {
		t.Fatalf("catalog retained caller mutation: %#v", second[droidIndex].Aliases)
	}
}

func wantPosture(name string) string {
	switch name {
	case "fast-agent-acp", "kilocode-acp", "pi":
		return "package_runner"
	default:
		return "installed_executable"
	}
}
