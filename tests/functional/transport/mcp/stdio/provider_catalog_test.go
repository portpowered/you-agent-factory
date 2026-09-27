package stdio_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMCPProviderCatalogReadsTheRuntimeCatalog exercises provider discovery
// through the canonical runtime composition and the public stdio MCP surface.
func TestMCPProviderCatalogReadsTheRuntimeCatalog(t *testing.T) {
	server := startRuntimeBackedMCPServerWithHome(t, t.TempDir(), t.TempDir())
	defer server.cleanup()
	if result := server.client.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "provider-catalog-test", "version": "test"},
	}); result.Error != nil {
		t.Fatalf("initialize error = %#v", result.Error)
	}

	listing := server.client.call("tools/call", map[string]any{
		"name": "you.provider.list_providers", "arguments": map[string]any{},
	})
	if listing.Error != nil || listing.Result["isError"] == true {
		t.Fatalf("provider catalog = %#v error=%#v", listing.Result, listing.Error)
	}
	content, ok := listing.Result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("provider catalog content = %#v, want provider catalog", listing.Result["content"])
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("provider catalog content item = %#v", content[0])
	}
	text, _ := item["text"].(string)
	var response struct {
		Result struct {
			Providers []struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
			} `json:"providers"`
		} `json:"result"`
		Error *json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("decode provider catalog response %q: %v", text, err)
	}
	if response.Error != nil || len(response.Result.Providers) == 0 {
		t.Fatalf("provider catalog response = %#v, want successful nonempty catalog", response)
	}
	for _, provider := range response.Result.Providers {
		if strings.TrimSpace(provider.ID) == "" || strings.TrimSpace(provider.DisplayName) == "" {
			t.Fatalf("provider descriptor = %#v, want ID and display name", provider)
		}
	}

	providerID := response.Result.Providers[0].ID
	details := server.client.call("tools/call", map[string]any{
		"name": "you.provider.get_provider", "arguments": map[string]any{"id": providerID},
	})
	if details.Error != nil || details.Result["isError"] == true {
		t.Fatalf("provider details for %q = %#v error=%#v", providerID, details.Result, details.Error)
	}
	detailContent, ok := details.Result["content"].([]any)
	if !ok || len(detailContent) == 0 {
		t.Fatalf("provider detail content = %#v", details.Result["content"])
	}
	detailItem, ok := detailContent[0].(map[string]any)
	if !ok {
		t.Fatalf("provider detail content item = %#v", detailContent[0])
	}
	detailText, _ := detailItem["text"].(string)
	var detailResponse struct {
		Result struct {
			Provider struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
			} `json:"provider"`
		} `json:"result"`
		Error *json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(detailText), &detailResponse); err != nil {
		t.Fatalf("decode provider detail response %q: %v", detailText, err)
	}
	if detailResponse.Error != nil || detailResponse.Result.Provider.ID != providerID || detailResponse.Result.Provider.DisplayName == "" {
		t.Fatalf("provider detail response = %#v, want descriptor for %q", detailResponse, providerID)
	}
	assertMissingProviderCatalogLookup(t, server)
}

func assertMissingProviderCatalogLookup(t *testing.T, server *stdioMCPServer) {
	t.Helper()
	missing := server.client.call("tools/call", map[string]any{
		"name": "you.provider.get_provider", "arguments": map[string]any{"id": "mcp-functional-missing-provider"},
	})
	if missing.Error != nil || missing.Result["isError"] == true {
		t.Fatalf("missing provider lookup = %#v error=%#v, want stable tool error envelope", missing.Result, missing.Error)
	}
	missingContent, ok := missing.Result["content"].([]any)
	if !ok || len(missingContent) == 0 {
		t.Fatalf("missing provider content = %#v", missing.Result["content"])
	}
	missingItem, ok := missingContent[0].(map[string]any)
	if !ok {
		t.Fatalf("missing provider content item = %#v", missingContent[0])
	}
	missingText, _ := missingItem["text"].(string)
	var missingResponse struct {
		Error *json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(missingText), &missingResponse); err != nil {
		t.Fatalf("decode missing provider response %q: %v", missingText, err)
	}
	if missingResponse.Error == nil {
		t.Fatalf("missing provider response = %s, want stable error envelope", missingText)
	}
}
