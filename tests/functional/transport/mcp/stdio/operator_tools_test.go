package stdio_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestMCPConfigurationToolsUseCanonicalHomeAndRefreshCurrentResource verifies
// that MCP configuration writes target the user's canonical settings file and
// are immediately visible through the live current-config resource.
func TestMCPConfigurationToolsUseCanonicalHomeAndRefreshCurrentResource(t *testing.T) {
	home := t.TempDir()
	server := startFixtureBackedMCPServerWithHome(t, home)
	defer server.cleanup()
	if result := server.client.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "operator-tools-test", "version": "test"},
	}); result.Error != nil {
		t.Fatalf("initialize error = %#v", result.Error)
	}
	readConfig := func() string {
		t.Helper()
		result := server.client.call("resources/read", map[string]any{"uri": "you://operator/config/current"})
		if result.Error != nil {
			t.Fatalf("read current config error = %#v", result.Error)
		}
		contents, ok := result.Result["contents"].([]any)
		if !ok || len(contents) != 1 {
			t.Fatalf("current config contents = %#v", result.Result["contents"])
		}
		item, ok := contents[0].(map[string]any)
		if !ok {
			t.Fatalf("current config item = %#v", contents[0])
		}
		text, _ := item["text"].(string)
		return text
	}
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("remove isolated bootstrap config: %v", err)
	}
	if got := readConfig(); got != "{}" {
		t.Fatalf("initial current config = %q, want empty object from isolated home", got)
	}
	callTool := func(name string, args map[string]any) mcpJSONRPCResponse {
		t.Helper()
		result := server.client.call("tools/call", map[string]any{"name": name, "arguments": args})
		if result.Error != nil {
			t.Fatalf("tools/call %s error = %#v", name, result.Error)
		}
		if result.Result["isError"] == true {
			t.Fatalf("tools/call %s returned error content: %#v", name, result.Result["content"])
		}
		return result
	}
	callTool("you.operator_settings.set_subagent_defaults", map[string]any{"provider": "codex", "model": "gpt-6-luna"})
	var configured map[string]any
	if err := json.Unmarshal([]byte(readConfig()), &configured); err != nil {
		t.Fatalf("decode current config: %v", err)
	}
	defaults, ok := configured["defaults"].(map[string]any)
	if !ok || defaults["workerModelProvider"] != "codex" || defaults["workerModel"] != "gpt-6-luna" {
		t.Fatalf("configured defaults = %#v", configured["defaults"])
	}
	isolatedConfig, err := os.ReadFile(configPath)
	if err != nil || string(isolatedConfig) != readConfig() {
		t.Fatalf("current resource did not reread isolated file %q: %v", configPath, err)
	}
	customProvider := "mcp-functional-" + uuid.NewString()
	added := callTool("you.operator_settings.add_acp_provider", map[string]any{
		"name": customProvider, "command": "opencode acp", "transport": "stdio",
	})
	content, _ := added.Result["content"].([]any)
	item, _ := content[0].(map[string]any)
	responseText, _ := item["text"].(string)
	var toolResponse map[string]any
	if err := json.Unmarshal([]byte(responseText), &toolResponse); err != nil {
		t.Fatalf("decode add ACP provider response %q: %v", responseText, err)
	}
	resultBody, _ := toolResponse["result"].(map[string]any)
	if resultBody["requiresRestart"] != false {
		t.Fatalf("add ACP provider result = %#v, want requiresRestart=false after live activation", resultBody)
	}
	if err := json.Unmarshal([]byte(readConfig()), &configured); err != nil {
		t.Fatalf("decode updated current config: %v", err)
	}
	workers, _ := configured["workers"].(map[string]any)
	acp, _ := workers["acp"].(map[string]any)
	integrations, _ := acp["integrations"].([]any)
	foundCustom := false
	for _, entry := range integrations {
		if value, ok := entry.(map[string]any); ok && value["name"] == customProvider {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Fatalf("configured ACP integrations = %#v", integrations)
	}
}

func TestMCPAddedACPProviderIsListedAfterDefaultRuntimeRestart(t *testing.T) {
	projectRoot := t.TempDir()
	home := t.TempDir()
	server := startRuntimeBackedMCPServerWithHome(t, projectRoot, home)
	initialize := func(current *stdioMCPServer, clientName string) {
		t.Helper()
		if result := current.client.call("initialize", map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": clientName, "version": "test"},
		}); result.Error != nil {
			t.Fatalf("initialize error = %#v", result.Error)
		}
	}
	initialize(server, "custom-provider-before-restart")
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	initialFile, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read isolated runtime config before mutation: %v", err)
	}
	initialResource := server.client.call("resources/read", map[string]any{"uri": "you://operator/config/current"})
	if initialResource.Error != nil {
		t.Fatalf("read isolated runtime config resource: %#v", initialResource.Error)
	}
	initialContents, _ := initialResource.Result["contents"].([]any)
	initialItem, _ := initialContents[0].(map[string]any)
	if initialItem["text"] != string(initialFile) {
		t.Fatalf("runtime config resource did not point at isolated home file")
	}

	customProvider := "mcp-restart-" + uuid.NewString()
	added := server.client.call("tools/call", map[string]any{
		"name":      "you.operator_settings.add_acp_provider",
		"arguments": map[string]any{"name": customProvider, "command": "opencode acp", "transport": "stdio"},
	})
	if added.Error != nil || added.Result["isError"] == true {
		t.Fatalf("add custom ACP provider = %#v error=%#v", added.Result, added.Error)
	}
	updatedFile, err := os.ReadFile(configPath)
	if err != nil || !strings.Contains(string(updatedFile), customProvider) {
		t.Fatalf("custom provider was not persisted in isolated config: %v", err)
	}
	assertListed := func(current *stdioMCPServer, label string) {
		t.Helper()
		listing := current.client.call("tools/call", map[string]any{
			"name": "you.provider.list_providers", "arguments": map[string]any{},
		})
		if listing.Error != nil || listing.Result["isError"] == true {
			t.Fatalf("provider catalog %s = %#v error=%#v", label, listing.Result, listing.Error)
		}
		catalogJSON, _ := json.Marshal(listing.Result["content"])
		if !strings.Contains(string(catalogJSON), customProvider) {
			t.Fatalf("provider catalog %s does not include %q", label, customProvider)
		}
	}
	assertListed(server, "in current server")
	defaultResult := server.client.call("tools/call", map[string]any{
		"name": "you.operator_settings.set_subagent_defaults", "arguments": map[string]any{"provider": customProvider},
	})
	if defaultResult.Error != nil || defaultResult.Result["isError"] == true {
		t.Fatalf("set custom provider default = %#v error=%#v", defaultResult.Result, defaultResult.Error)
	}
	server.cleanup()
	server = startRuntimeBackedMCPServerWithHome(t, projectRoot, home)
	initialize(server, "custom-provider-after-restart")
	assertListed(server, "after restart")
	afterRestart := server.client.call("resources/read", map[string]any{"uri": "you://operator/config/current"})
	if afterRestart.Error != nil {
		t.Fatalf("read custom default after restart: %#v", afterRestart.Error)
	}
	contents, _ := afterRestart.Result["contents"].([]any)
	item, _ := contents[0].(map[string]any)
	var persisted map[string]any
	if err := json.Unmarshal([]byte(item["text"].(string)), &persisted); err != nil {
		t.Fatalf("decode custom default after restart: %v", err)
	}
	defaults, _ := persisted["defaults"].(map[string]any)
	if defaults["workerModelProvider"] != customProvider {
		t.Fatalf("custom default after restart = %#v, want %q", defaults, customProvider)
	}
}
