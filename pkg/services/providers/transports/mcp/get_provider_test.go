package providersmcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providersmcp "github.com/portpowered/infinite-you/pkg/services/providers/transports/mcp"
)

func TestBind_GetProviderSuccessReturnsDetachedDescriptorFromInjectedRoot(t *testing.T) {
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
		getProvider: func(
			_ context.Context,
			request providers.GetProviderRequest,
		) (providers.GetProviderResult, error) {
			if request.ID != providers.IDCodex {
				t.Fatalf("provider id = %q, want %q", request.ID, providers.IDCodex)
			}
			return providers.GetProviderResult{
				Provider: wantDescriptor.Clone(),
			}, nil
		},
	}
	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: fake})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolGetProvider,
		json.RawMessage(`{"id":"codex"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_provider) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake Providers root was not invoked")
	}
	var response providersmcp.ToolResponse[providers.GetProviderResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	got := response.Result.Provider
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

func TestDiscoverTools_GetProviderDiscoveryMatchesHandlerRegistration(t *testing.T) {
	t.Parallel()

	tool, ok := providersmcp.ToolByName(providersmcp.ToolGetProvider)
	if !ok {
		t.Fatal("ToolByName(get_provider) ok = false, want true")
	}
	if tool.Name != providersmcp.ToolGetProvider {
		t.Fatalf("discovered name = %q, want %q", tool.Name, providersmcp.ToolGetProvider)
	}
	if !providersmcp.IsCanonicalToolHandlerRegistered(providersmcp.ToolGetProvider) {
		t.Fatal("get_provider handler is not registered on canonical CallTool path")
	}
	if len(tool.SuccessStableFields) == 0 {
		t.Fatal("get_provider success stable fields are required")
	}
	for _, field := range []string{
		"result.Provider",
		"result.Provider.ID",
		"result.Provider.DisplayName",
		"result.Provider.Availability",
		"result.Provider.Readiness",
		"result.Provider.Capabilities",
	} {
		if !containsString(tool.SuccessStableFields, field) {
			t.Fatalf("success stable fields = %#v, want %q", tool.SuccessStableFields, field)
		}
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("get_provider input schema properties = %#v, want object map", tool.InputSchema["properties"])
	}
	if _, ok := properties["id"]; !ok {
		t.Fatal("get_provider input schema missing id property")
	}
	resultSchema, ok := tool.OutputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("get_provider output schema properties missing")
	}
	if _, ok := resultSchema["result"]; !ok {
		t.Fatal("get_provider output schema missing result envelope")
	}
}

func TestBind_GetProviderCatalogFailuresReturnTypedErrorEnvelopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		rootErr       error
		wantCode      string
		wantMessage   string
		wantRetryable bool
	}{
		{
			name:          "invalid provider id",
			rootErr:       providers.ErrInvalidID,
			wantCode:      "provider.identity.invalid",
			wantMessage:   "provider id is invalid",
			wantRetryable: false,
		},
		{
			name:          "unknown provider",
			rootErr:       providers.ErrUnknownProvider,
			wantCode:      "provider.catalog.unknown",
			wantMessage:   "provider is unknown",
			wantRetryable: false,
		},
		{
			name:          "unavailable provider",
			rootErr:       providers.ErrProviderUnavailable,
			wantCode:      "provider.catalog.unavailable",
			wantMessage:   "provider is unavailable",
			wantRetryable: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var invoked bool
			fake := fakeProvidersRoot{
				invoked: &invoked,
				getProvider: func(
					_ context.Context,
					request providers.GetProviderRequest,
				) (providers.GetProviderResult, error) {
					if request.ID != providers.IDCodex {
						t.Fatalf("provider id = %q, want %q", request.ID, providers.IDCodex)
					}
					return providers.GetProviderResult{}, tc.rootErr
				},
			}
			operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: fake})
			raw, err := operation(
				context.Background(),
				providersmcp.ToolGetProvider,
				json.RawMessage(`{"id":"codex"}`),
			)
			if err != nil {
				t.Fatalf("CallTool(get_provider) transport error = %v, want typed tool response", err)
			}
			if !invoked {
				t.Fatal("fake Providers root was not invoked")
			}
			envelope := assertTypedToolErrorEnvelope(t, raw, tc.wantCode, tc.wantRetryable)
			if envelope.Message != tc.wantMessage {
				t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, tc.wantMessage, envelope)
			}
		})
	}
}

func TestBind_GetProviderCatalogFailuresAreDistinct(t *testing.T) {
	t.Parallel()

	invalidRaw := mustCallGetProvider(t, fakeProvidersRoot{
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			return providers.GetProviderResult{}, providers.ErrInvalidID
		},
	})
	unknownRaw := mustCallGetProvider(t, fakeProvidersRoot{
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			return providers.GetProviderResult{}, providers.ErrUnknownProvider
		},
	})
	unavailableRaw := mustCallGetProvider(t, fakeProvidersRoot{
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			return providers.GetProviderResult{}, providers.ErrProviderUnavailable
		},
	})

	invalid := assertTypedToolErrorEnvelope(t, invalidRaw, "provider.identity.invalid", false)
	unknown := assertTypedToolErrorEnvelope(t, unknownRaw, "provider.catalog.unknown", false)
	unavailable := assertTypedToolErrorEnvelope(t, unavailableRaw, "provider.catalog.unavailable", true)

	if invalid.Code == unknown.Code || invalid.Code == unavailable.Code || unknown.Code == unavailable.Code {
		t.Fatalf(
			"catalog failure codes must be distinct: invalid=%q unknown=%q unavailable=%q",
			invalid.Code,
			unknown.Code,
			unavailable.Code,
		)
	}
}

func TestBind_GetProviderNilServiceReturnsUnavailableEnvelope(t *testing.T) {
	t.Parallel()

	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: nil})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolGetProvider,
		json.RawMessage(`{"id":"codex"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_provider) transport error = %v, want typed tool response", err)
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

func TestBind_GetProviderMalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := providersmcp.Bind(providersmcp.RootDependencies{
		Providers: fakeProvidersRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolGetProvider,
		json.RawMessage(`{`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_provider) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if !strings.Contains(envelope.Message, "decode get provider input") {
		t.Fatalf("error.message = %q, want decode get provider input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake Providers root was invoked for malformed JSON")
	}
}

func TestBind_GetProviderWrappedCatalogErrorsPreserveTypedCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		rootErr  error
		wantCode string
	}{
		{
			name:     "wrapped invalid id",
			rootErr:  fmt.Errorf("lookup provider: %w", providers.ErrInvalidID),
			wantCode: "provider.identity.invalid",
		},
		{
			name:     "wrapped unknown provider",
			rootErr:  fmt.Errorf("lookup provider: %w", providers.ErrUnknownProvider),
			wantCode: "provider.catalog.unknown",
		},
		{
			name:     "wrapped unavailable provider",
			rootErr:  fmt.Errorf("lookup provider: %w", providers.ErrProviderUnavailable),
			wantCode: "provider.catalog.unavailable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := mustCallGetProvider(t, fakeProvidersRoot{
				getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
					return providers.GetProviderResult{}, tc.rootErr
				},
			})
			assertTypedToolErrorEnvelope(t, raw, tc.wantCode, tc.wantCode == "provider.catalog.unavailable")
		})
	}
}

func mustCallGetProvider(t *testing.T, fake fakeProvidersRoot) json.RawMessage {
	t.Helper()

	var invoked bool
	fake.invoked = &invoked
	operation := providersmcp.Bind(providersmcp.RootDependencies{Providers: fake})
	raw, err := operation(
		context.Background(),
		providersmcp.ToolGetProvider,
		json.RawMessage(`{"id":"codex"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_provider) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake Providers root was not invoked")
	}
	return raw
}
