package stdio_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestMCPConfigurationResourcesRereadTheOperatorFile verifies that the MCP
// surface exposes configuration as resources and reads the canonical file on
// each request, so a customer's direct file edits take effect without a tool
// that mutates their configuration.
func TestMCPConfigurationResourcesRereadTheOperatorFile(t *testing.T) {
	home := t.TempDir()
	server := startRuntimeBackedMCPServerWithHome(t, t.TempDir(), home)
	defer server.cleanup()
	if result := server.client.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "operator-resources-test", "version": "test"},
	}); result.Error != nil {
		t.Fatalf("initialize error = %#v", result.Error)
	}

	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("remove isolated bootstrap config: %v", err)
	}
	readResource := func(uri string) string {
		t.Helper()
		result := server.client.call("resources/read", map[string]any{"uri": uri})
		if result.Error != nil {
			t.Fatalf("read resource %q error = %#v", uri, result.Error)
		}
		contents, ok := result.Result["contents"].([]any)
		if !ok || len(contents) != 1 {
			t.Fatalf("resource %q contents = %#v", uri, result.Result["contents"])
		}
		item, ok := contents[0].(map[string]any)
		if !ok {
			t.Fatalf("resource %q content item = %#v", uri, contents[0])
		}
		text, ok := item["text"].(string)
		if !ok || text == "" {
			t.Fatalf("resource %q text = %#v", uri, item["text"])
		}
		return text
	}
	if got := readResource("you://operator/config/current"); got != "{}" {
		t.Fatalf("initial current config = %q, want empty object from isolated home", got)
	}
	if schema := readResource("you://operator/config/schema"); !strings.Contains(schema, "GlobalConfigACPIntegration") {
		t.Fatalf("configuration schema does not describe ACP integrations: %s", schema)
	}

	customProvider := "mcp-functional-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	config := map[string]any{
		"defaults": map[string]any{"workerModelProvider": "codex", "workerModel": "gpt-6-luna"},
		"workers": map[string]any{"acp": map[string]any{"integrations": []any{
			map[string]any{"id": "mcp-functional-custom", "name": customProvider, "command": "opencode acp", "transport": "stdio"},
		}}},
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("encode custom operator config: %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0o600); err != nil {
		t.Fatalf("write custom operator config: %v", err)
	}
	if got := readResource("you://operator/config/current"); got != string(encoded) {
		t.Fatalf("current config resource = %q, want latest file contents %q", got, string(encoded))
	}

	catalog := readResource("you://providers/catalog")
	if !strings.Contains(catalog, customProvider) {
		t.Fatalf("provider catalog resource does not include newly configured provider %q: %s", customProvider, catalog)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(catalog), &decoded); err != nil {
		t.Fatalf("decode provider catalog resource: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatalf("provider catalog resource is empty: %s", catalog)
	}
}
