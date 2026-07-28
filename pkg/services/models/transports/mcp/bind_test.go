package modelmcp_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelmcp "github.com/portpowered/infinite-you/pkg/services/models/transports/mcp"
)

const testRuntimeScopeRef = "models-mcp-bind-scope-001"

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

func TestPackageBoundary_DoesNotImportModelsInternal(t *testing.T) {
	t.Parallel()

	forbidden := "github.com/portpowered/infinite-you/pkg/services/models/internal"
	packagePath := "github.com/portpowered/infinite-you/pkg/services/models/transports/mcp"
	assertPackageDirectImportsForbidden(t, packagePath, []string{forbidden})
}

type fakeModelsRoot struct {
	models.Service
	invoked            *bool
	listCatalog        func(context.Context, models.ListModelsRequest) (models.ListModelsResult, error)
	prepareModelAssets func(context.Context, models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error)
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

func assertPackageDirectImportsForbidden(t *testing.T, packagePath string, forbiddenRoots []string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, forbidden := range forbiddenRoots {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf("%s must not import forbidden ownership %s; found direct import %s", packagePath, forbidden, importPath)
			}
		}
	}
}
