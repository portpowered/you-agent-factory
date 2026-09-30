package stdio_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMCPProviderCatalogIsPublishedAsAReadableResource exercises provider
// discovery through the resource surface, where the agent can inspect the
// available providers, models, and reasoning efforts before invoking a
// subagent.
func TestMCPProviderCatalogIsPublishedAsAReadableResource(t *testing.T) {
	server := startRuntimeBackedMCPServerWithHome(t, t.TempDir(), t.TempDir())
	defer server.cleanup()
	if result := server.client.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "provider-catalog-resource-test", "version": "test"},
	}); result.Error != nil {
		t.Fatalf("initialize error = %#v", result.Error)
	}

	resources := server.client.call("resources/list", map[string]any{})
	if resources.Error != nil {
		t.Fatalf("resources/list error = %#v", resources.Error)
	}
	listed, ok := resources.Result["resources"].([]any)
	if !ok {
		t.Fatalf("resources/list resources = %#v, want array", resources.Result["resources"])
	}
	found := false
	for _, value := range listed {
		resource, ok := value.(map[string]any)
		if ok && resource["uri"] == "you://providers/catalog" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("resources/list omitted provider catalog resource: %#v", listed)
	}

	read := server.client.call("resources/read", map[string]any{"uri": "you://providers/catalog"})
	if read.Error != nil {
		t.Fatalf("resources/read provider catalog error = %#v", read.Error)
	}
	contents, ok := read.Result["contents"].([]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("provider catalog contents = %#v, want one JSON resource", read.Result["contents"])
	}
	item, ok := contents[0].(map[string]any)
	if !ok || item["mimeType"] != "application/json" {
		t.Fatalf("provider catalog content item = %#v, want JSON MIME type", contents[0])
	}
	text, _ := item["text"].(string)
	var catalog map[string]any
	if err := json.Unmarshal([]byte(text), &catalog); err != nil {
		t.Fatalf("decode provider catalog %q: %v", text, err)
	}
	providers, ok := catalog["providers"].([]any)
	if !ok || len(providers) == 0 {
		t.Fatalf("provider catalog providers = %#v, want nonempty list", catalog["providers"])
	}
	first, ok := providers[0].(map[string]any)
	if !ok || first["id"] == nil || first["models"] == nil {
		t.Fatalf("provider catalog entry = %#v, want id and models", providers[0])
	}
	if !strings.Contains(text, "codex") && !strings.Contains(text, "opencode") {
		t.Fatalf("provider catalog does not include expected default providers: %s", text)
	}
	if len(catalog) == 0 {
		t.Fatalf("provider catalog is empty: %s", text)
	}
}
