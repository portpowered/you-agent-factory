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
	t.Parallel()
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
	assertMCPResourceFailuresAndRecovery(t, server, configPath, encoded, catalog, readResource)
}

// W5 resource failures are ordered because these reads observe successive edits
// to one operator-owned document. The same live protocol invocation must recover
// without replacing its process or returning content for a missing resource.
func assertMCPResourceFailuresAndRecovery(t *testing.T, server *stdioMCPServer, configPath string, encoded []byte, catalog string, readResource func(string) string) {
	t.Helper()
	const privatePayload = "private-provider-settings-payload"
	if err := os.WriteFile(configPath, []byte(`{"workers":`+privatePayload), 0o600); err != nil {
		t.Fatal(err)
	}
	assertMCPResourceFailure(t, server, "you://providers/catalog", privatePayload)
	if err := os.Remove(configPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(configPath, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, uri := range []string{"you://providers/catalog", "you://operator/config/current"} {
		assertMCPResourceFailure(t, server, uri, privatePayload)
	}
	if err := os.Remove(configPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, uri := range []string{"skill://subagent-configuration/missing.md", "you://operator/config/missing"} {
		assertMCPResourceFailure(t, server, uri, privatePayload)
	}
	if got := readResource("you://operator/config/current"); got != string(encoded) {
		t.Fatalf("recovered operator resource = %q, want original document %q", got, encoded)
	}
	if got := readResource("you://providers/catalog"); got != catalog {
		t.Fatalf("recovered provider catalog = %s, want original catalog %s", got, catalog)
	}
	if got := readResource("skill://subagent-configuration/SKILL.md"); !strings.Contains(got, "subagent") {
		t.Fatalf("published skill unavailable after resource failures: %q", got)
	}
	retained, err := os.ReadFile(configPath)
	if err != nil || string(retained) != string(encoded) {
		t.Fatalf("resource reads mutated operator document: %q: %v", retained, err)
	}
}

func assertMCPResourceFailure(t *testing.T, server *stdioMCPServer, uri, privatePayload string) {
	t.Helper()
	result := server.client.call("resources/read", map[string]any{"uri": uri})
	if result.Error == nil || result.Result != nil {
		t.Fatalf("resource %q must fail without successful content: %#v", uri, result)
	}
	if result.Error.Message == "" || strings.Contains(result.Error.Message, privatePayload) {
		t.Fatalf("resource %q lacks a safe diagnostic: %#v", uri, result.Error)
	}
}
