package providersmcp_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providersmcp "github.com/portpowered/infinite-you/pkg/services/providers/transports/mcp"
)

func TestBind_FakeRootInvokedThroughListProvidersTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeProvidersRoot{
		invoked: &invoked,
		listProviders: func(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
			return providers.ListProvidersResult{
				Providers: []providers.Descriptor{{
					ID:          providers.IDCodex,
					DisplayName: "Codex",
				}},
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
		t.Fatalf("CallTool(list_providers) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake Providers root was not invoked")
	}
	var response providersmcp.ToolResponse[providers.ListProvidersResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("CallTool(list_providers) = %s, want success", raw)
	}
	if len(response.Result.Providers) != 1 || response.Result.Providers[0].ID != providers.IDCodex {
		t.Fatalf("providers = %#v, want one codex descriptor", response.Result.Providers)
	}
}

func TestNewFromRoot_FakeRootInvokedThroughListProvidersTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeProvidersRoot{
		invoked: &invoked,
		listProviders: func(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
			return providers.ListProvidersResult{}, nil
		},
	}
	operation := providersmcp.NewFromRoot(providersmcp.RootDependencies{Providers: fake})
	_, err := operation(
		context.Background(),
		providersmcp.ToolListProviders,
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_providers) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake Providers root was not invoked through NewFromRoot binding")
	}
}

func TestBind_UnsupportedToolReturnsStableErrorWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := providersmcp.Bind(providersmcp.RootDependencies{
		Providers: fakeProvidersRoot{invoked: &invoked},
	})
	_, err := operation(context.Background(), "you.provider.unknown", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("CallTool(unknown) error = nil, want unsupported tool error")
	}
	if !strings.Contains(err.Error(), "unsupported tool") {
		t.Fatalf("CallTool(unknown) error = %v, want unsupported tool error", err)
	}
	if invoked {
		t.Fatal("fake Providers root was invoked for unknown tool")
	}
}

func TestBind_ToolOperationRejectsMissingContext(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := providersmcp.BindToolOperation(fakeProvidersRoot{invoked: &invoked})
	_, err := operation(nil, providersmcp.ToolListProviders, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "MCP request context is required") {
		t.Fatalf("ToolOperation(nil context) error = %v, want required-context error", err)
	}
	if invoked {
		t.Fatal("fake Providers root was invoked for nil context")
	}
}

func TestRootDependencies_BindsExactlyOneProvidersRoot(t *testing.T) {
	t.Parallel()

	typeOfDependencies := reflect.TypeOf(providersmcp.RootDependencies{})
	if typeOfDependencies.NumField() != 1 {
		t.Fatalf("RootDependencies field count = %d, want one unary owner root", typeOfDependencies.NumField())
	}
	field := typeOfDependencies.Field(0)
	if field.Name != "Providers" {
		t.Fatalf("RootDependencies field = %q, want Providers", field.Name)
	}
	wantType := reflect.TypeOf((*providers.Service)(nil)).Elem()
	if field.Type != wantType {
		t.Fatalf("RootDependencies.Providers type = %v, want Providers Service root %v", field.Type, wantType)
	}
}

type fakeProvidersRoot struct {
	providers.Service
	invoked       *bool
	listProviders func(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error)
	getProvider   func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error)
	execute       func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)
}

func (fake fakeProvidersRoot) markInvoked() {
	if fake.invoked != nil {
		*fake.invoked = true
	}
}

func (fake fakeProvidersRoot) ListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	fake.markInvoked()
	if fake.listProviders == nil {
		panic("unexpected ListProviders on fake Providers root")
	}
	return fake.listProviders(ctx, request)
}

func (fake fakeProvidersRoot) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	fake.markInvoked()
	if fake.getProvider == nil {
		panic("unexpected GetProvider on fake Providers root")
	}
	return fake.getProvider(ctx, request)
}

func (fake fakeProvidersRoot) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.markInvoked()
	if fake.execute == nil {
		panic("unexpected Execute on fake Providers root")
	}
	return fake.execute(ctx, request)
}
