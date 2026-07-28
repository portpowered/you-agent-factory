package providersmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providersmcp "github.com/portpowered/infinite-you/pkg/services/providers/transports/mcp"
)

func TestBind_ListProvidersSuccessReturnsDetachedCatalogFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	wantDescriptor := providers.Descriptor{
		ID:           providers.IDCodex,
		Aliases:      []string{"openai-codex"},
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{
			providers.CapabilityPromptSubmission,
			providers.CapabilityNativeStreaming,
		},
	}
	fake := fakeProvidersRoot{
		invoked: &invoked,
		listProviders: func(
			_ context.Context,
			request providers.ListProvidersRequest,
		) (providers.ListProvidersResult, error) {
			if request != (providers.ListProvidersRequest{}) {
				t.Fatalf("list request = %#v, want empty request", request)
			}
			return providers.ListProvidersResult{
				Providers: []providers.Descriptor{wantDescriptor.Clone()},
			}, nil
		},
	}
	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: fake})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolListProviders,
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_providers) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake Providers root was not invoked")
	}
	var response providersmcp.ToolResponse[providers.ListProvidersResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if len(response.Result.Providers) != 1 {
		t.Fatalf("providers = %#v, want one descriptor", response.Result.Providers)
	}
	got := response.Result.Providers[0]
	if got.ID != wantDescriptor.ID ||
		got.DisplayName != wantDescriptor.DisplayName ||
		got.Availability != wantDescriptor.Availability ||
		got.Readiness != wantDescriptor.Readiness {
		t.Fatalf("descriptor = %#v, want %#v", got, wantDescriptor)
	}
	if len(got.Capabilities) != len(wantDescriptor.Capabilities) {
		t.Fatalf("capabilities = %#v, want %#v", got.Capabilities, wantDescriptor.Capabilities)
	}
	for i, capability := range got.Capabilities {
		if capability != wantDescriptor.Capabilities[i] {
			t.Fatalf("capabilities[%d] = %q, want %q", i, capability, wantDescriptor.Capabilities[i])
		}
	}
}

func TestDiscoverTools_ListProvidersDiscoveryMatchesHandlerRegistration(t *testing.T) {
	t.Parallel()

	tool, ok := providersmcp.ToolByName(providersmcp.ToolListProviders)
	if !ok {
		t.Fatal("ToolByName(list_providers) ok = false, want true")
	}
	if tool.Name != providersmcp.ToolListProviders {
		t.Fatalf("discovered name = %q, want %q", tool.Name, providersmcp.ToolListProviders)
	}
	if !providersmcp.IsCanonicalToolHandlerRegistered(providersmcp.ToolListProviders) {
		t.Fatal("list_providers handler is not registered on canonical CallTool path")
	}
	if len(tool.SuccessStableFields) == 0 {
		t.Fatal("list_providers success stable fields are required")
	}
	for _, field := range []string{
		"result.Providers",
		"result.Providers[].ID",
		"result.Providers[].DisplayName",
		"result.Providers[].Availability",
		"result.Providers[].Readiness",
		"result.Providers[].Capabilities",
	} {
		if !containsString(tool.SuccessStableFields, field) {
			t.Fatalf("success stable fields = %#v, want %q", tool.SuccessStableFields, field)
		}
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("list_providers input schema properties = %#v, want object map", tool.InputSchema["properties"])
	}
	if len(properties) != 0 {
		t.Fatalf("list_providers input properties = %#v, want empty object input", properties)
	}
	resultSchema, ok := tool.OutputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("list_providers output schema properties missing")
	}
	if _, ok := resultSchema["result"]; !ok {
		t.Fatal("list_providers output schema missing result envelope")
	}
}

func TestBind_ListProvidersNilServiceReturnsUnavailableEnvelope(t *testing.T) {
	t.Parallel()

	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: nil})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolListProviders,
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_providers) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"provider.service.unavailable",
		false,
	)
	if envelope.Message != "providers service is unavailable" {
		t.Fatalf("error.message = %q, want unavailable message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ListProvidersMalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := providersmcp.Bind(providersmcp.RootDependencies{
		Providers: fakeProvidersRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolListProviders,
		json.RawMessage(`{`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_providers) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if !strings.Contains(envelope.Message, "decode list providers input") {
		t.Fatalf("error.message = %q, want decode list providers input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake Providers root was invoked for malformed JSON")
	}
}

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
) *providersmcp.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage              `json:"result"`
		Error  *providersmcp.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("tool response result = %s, want error envelope only", raw)
	}
	if response.Error == nil {
		t.Fatalf("tool response = %s, want typed error envelope", raw)
	}
	if response.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q; envelope = %#v", response.Error.Code, wantCode, response.Error)
	}
	if response.Error.Retryable != wantRetryable {
		t.Fatalf("error.retryable = %v, want %v; envelope = %#v", response.Error.Retryable, wantRetryable, response.Error)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	return response.Error
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
