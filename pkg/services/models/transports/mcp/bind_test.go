package modelmcp_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelmcp "github.com/portpowered/infinite-you/pkg/services/models/transports/mcp"
)

const testRuntimeScopeRef = "models-mcp-bind-scope-001"

func TestRootBinding_IsUnaryModelsRootBinding(t *testing.T) {
	t.Parallel()

	typeOfBinding := reflect.TypeOf(modelmcp.RootBinding{})
	if typeOfBinding.NumField() != 1 {
		t.Fatalf("RootBinding fields = %d, want one Models root field", typeOfBinding.NumField())
	}
	field := typeOfBinding.Field(0)
	wantType := reflect.TypeOf((*models.Service)(nil)).Elem()
	if field.Name != "Models" || field.Type != wantType {
		t.Fatalf("RootBinding field = %s %s, want Models %s", field.Name, field.Type, wantType)
	}
}

func TestBind_FakeRootInvokedThroughListCatalogTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeModelsRoot{
		invoked: &invoked,
		listCatalog: func(_ context.Context, request models.ListModelsRequest) (models.ListModelsResult, error) {
			if request.Scope.String() != testRuntimeScopeRef {
				t.Fatalf("scope = %q, want %q", request.Scope.String(), testRuntimeScopeRef)
			}
			return models.ListModelsResult{
				Models: []models.Summary{{
					Name:   "stub-model",
					Status: models.StatusReady,
				}},
			}, nil
		},
	}
	operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolListCatalog,
		json.RawMessage(`{"runtimeScopeRef":"`+testRuntimeScopeRef+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_catalog) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake models root was not invoked")
	}
	var response modelmcp.ToolResponse[models.ListModelsResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("CallTool(list_catalog) = %s, want success", raw)
	}
	if len(response.Result.Models) != 1 || response.Result.Models[0].Name != "stub-model" {
		t.Fatalf("models = %#v, want one stub-model summary", response.Result.Models)
	}
}

func TestNewFromRoot_FakeRootInvokedThroughListCatalogTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeModelsRoot{
		invoked: &invoked,
		listCatalog: func(context.Context, models.ListModelsRequest) (models.ListModelsResult, error) {
			return models.ListModelsResult{}, nil
		},
	}
	operation := modelmcp.NewFromRoot(modelmcp.RootBinding{Models: fake})
	_, err := operation(
		context.Background(),
		modelmcp.ToolListCatalog,
		json.RawMessage(`{"runtimeScopeRef":"`+testRuntimeScopeRef+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_catalog) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake models root was not invoked through NewFromRoot binding")
	}
}

func TestCallTool_UnknownToolReturnsStableError(t *testing.T) {
	t.Parallel()

	_, err := modelmcp.CallTool(
		context.Background(),
		fakeModelsRoot{},
		"you.model.unknown_tool",
		json.RawMessage(`{}`),
	)
	if err == nil {
		t.Fatal("CallTool(unknown tool) error = nil, want unsupported-tool error")
	}
	if got := err.Error(); got != `unsupported tool "you.model.unknown_tool"` {
		t.Fatalf("CallTool(unknown tool) error = %q, want %q", got, `unsupported tool "you.model.unknown_tool"`)
	}
}

func TestBind_ToolOperationRejectsMissingContext(t *testing.T) {
	t.Parallel()

	operation := modelmcp.BindToolOperation(fakeModelsRoot{})
	_, err := operation(nil, modelmcp.ToolListCatalog, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("ToolOperation(nil context) error = %v, want required-context error", err)
	}
}

type fakeModelsRoot struct {
	models.Service
	invoked              *bool
	listCatalog          func(context.Context, models.ListModelsRequest) (models.ListModelsResult, error)
	prepareModelAssets   func(context.Context, models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error)
	acquireModelLease    func(context.Context, models.AcquireModelLeaseRequest) (models.AcquireModelLeaseResult, error)
	invokeModelWithLease func(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error)
}

func (fake fakeModelsRoot) markInvoked() {
	if fake.invoked != nil {
		*fake.invoked = true
	}
}

func (fake fakeModelsRoot) ListCatalog(
	ctx context.Context,
	request models.ListModelsRequest,
) (models.ListModelsResult, error) {
	fake.markInvoked()
	if fake.listCatalog == nil {
		panic("unexpected ListCatalog on fake models root")
	}
	return fake.listCatalog(ctx, request)
}

func (fake fakeModelsRoot) PrepareModelAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	fake.markInvoked()
	if fake.prepareModelAssets == nil {
		panic("unexpected PrepareModelAssets on fake models root")
	}
	return fake.prepareModelAssets(ctx, request)
}

func (fake fakeModelsRoot) AcquireModelLease(
	ctx context.Context,
	request models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	fake.markInvoked()
	if fake.acquireModelLease == nil {
		panic("unexpected AcquireModelLease on fake models root")
	}
	return fake.acquireModelLease(ctx, request)
}

func (fake fakeModelsRoot) InvokeModelWithLease(
	ctx context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	fake.markInvoked()
	if fake.invokeModelWithLease == nil {
		panic("unexpected InvokeModelWithLease on fake models root")
	}
	return fake.invokeModelWithLease(ctx, request)
}
