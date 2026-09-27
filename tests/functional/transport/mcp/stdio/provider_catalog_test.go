package stdio_test

import (
	"context"
	"encoding/json"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providersmcp "github.com/portpowered/infinite-you/pkg/services/providers/transports/mcp"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

// TestMCPProviderCatalogAdapterReadsTheRuntimeCatalog exercises the Providers
// MCP adapter against the real, inert Providers root and catalog composition.
func TestMCPProviderCatalogAdapterReadsTheRuntimeCatalog(t *testing.T) {
	names := providersmcp.ToolNames()
	if len(names) == 0 || names[0] != providersmcp.ToolListProviders {
		t.Fatalf("provider MCP tool catalog = %v, want list providers first", names)
	}
	service, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("construct Providers service: %v", err)
	}

	call := func(name string, input string) json.RawMessage {
		t.Helper()
		raw, callErr := providersmcp.CallTool(context.Background(), service, name, json.RawMessage(input))
		if callErr != nil {
			t.Fatalf("CallTool(%s): %v", name, callErr)
		}
		return raw
	}

	var listed providersmcp.ToolResponse[providers.ListProvidersResult]
	if err := json.Unmarshal(call(providersmcp.ToolListProviders, `{}`), &listed); err != nil {
		t.Fatalf("decode list providers response: %v", err)
	}
	if listed.Error != nil || listed.Result == nil {
		t.Fatalf("list providers response = %#v, want successful catalog", listed)
	}
	if len(listed.Result.Providers) == 0 {
		t.Fatal("real provider catalog is empty")
	}

	providerID := listed.Result.Providers[0].ID
	var got providersmcp.ToolResponse[providers.GetProviderResult]
	if err := json.Unmarshal(call(providersmcp.ToolGetProvider, `{"id":"`+string(providerID)+`"}`), &got); err != nil {
		t.Fatalf("decode get provider response: %v", err)
	}
	if got.Error != nil || got.Result == nil {
		t.Fatalf("get provider response = %#v, want successful descriptor", got)
	}
	if got.Result.Provider.ID != providerID || got.Result.Provider.DisplayName == "" {
		t.Fatalf("get provider descriptor = %#v, want listed provider %q with a display name", got.Result.Provider, providerID)
	}
}
