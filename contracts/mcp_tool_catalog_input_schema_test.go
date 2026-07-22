package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestMCPToolCatalogInputSchemaParity_AuthoredCatalogMatchesDiscoverTools(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	resolved, diagnostics := contractvalidator.LoadAndResolve(root, "contracts/mcp/tools.json", []string{"contracts/mcp/tools.json"})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve authored catalog diagnostics = %+v", diagnostics)
	}
	schemas, err := mcpfactorycatalog.CatalogInputSchemasFromCatalogDocument(resolved)
	if err != nil {
		t.Fatalf("CatalogInputSchemasFromCatalogDocument() error = %v", err)
	}
	if err := mcpfactorycatalog.VerifyCatalogInputSchemaParity(schemas, mcpfactorysession.DiscoverTools()); err != nil {
		t.Fatalf("VerifyCatalogInputSchemaParity() error = %v", err)
	}
}

func TestMCPToolCatalogInputSchemaParity_ContractValidatorPassesForAuthoredCatalog(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	diagnostics := contractvalidator.Validate(root, contractvalidator.MCPRegistry(), "mcp", "1.0.0")
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "catalog.input_schema.parity" {
			t.Fatalf("unexpected input-schema parity diagnostic: %+v", diagnostic)
		}
	}
}

func TestMCPToolCatalogNestedInputValidation_ValidStartSyncArgumentPasses(t *testing.T) {
	schema := resolvedAuthoredCatalogInputSchema(t, mcpfactorysession.ToolStartSync)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("catalog-input", schema); err != nil {
		t.Fatalf("register catalog input schema: %v", err)
	}
	compiled, err := compiler.Compile("catalog-input")
	if err != nil {
		t.Fatalf("compile catalog input schema: %v", err)
	}
	argument := map[string]any{
		"requestId": "req-start-001",
		"source": map[string]any{
			"kind":         "WORKFLOW_NAME",
			"workflowName": "example.workflow",
		},
		"wait": map[string]any{
			"timeoutMillis":   60000,
			"cancelOnTimeout": true,
		},
	}
	if err := compiled.Validate(argument); err != nil {
		t.Fatalf("validate nested start_sync argument: %v", err)
	}
}

func TestMCPToolCatalogNestedInputValidation_InvalidClosedSourcePropertyFailsDeterministically(t *testing.T) {
	schema := resolvedAuthoredCatalogInputSchema(t, mcpfactorysession.ToolStartSync)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("catalog-input", schema); err != nil {
		t.Fatalf("register catalog input schema: %v", err)
	}
	compiled, err := compiler.Compile("catalog-input")
	if err != nil {
		t.Fatalf("compile catalog input schema: %v", err)
	}
	argument := map[string]any{
		"requestId": "req-start-001",
		"source": map[string]any{
			"kind":         "WORKFLOW_NAME",
			"workflowName": "example.workflow",
			"unexpected":   "closed-source-reject",
		},
	}
	err = compiled.Validate(argument)
	if err == nil {
		t.Fatal("expected invalid nested source property to fail validation")
	}
	paths := validationPaths(t, err)
	wantPaths := []string{"/source/unexpected", "/source"}
	if !containsAnyString(paths, wantPaths) {
		t.Fatalf("validation paths = %v, want one of %v", paths, wantPaths)
	}
}

func TestMCPToolCatalogNestedInputValidation_ValidValidateSourceArgumentPasses(t *testing.T) {
	schema := resolvedAuthoredCatalogInputSchema(t, mcpfactorysession.ToolValidateSource)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("catalog-input", schema); err != nil {
		t.Fatalf("register catalog input schema: %v", err)
	}
	compiled, err := compiler.Compile("catalog-input")
	if err != nil {
		t.Fatalf("compile catalog input schema: %v", err)
	}
	argument := map[string]any{
		"sourceKind":  "WORKFLOW_NAME",
		"sourceValue": "example.workflow",
	}
	if err := compiled.Validate(argument); err != nil {
		t.Fatalf("validate nested validate_source argument: %v", err)
	}
}

func resolvedAuthoredCatalogInputSchema(t *testing.T, toolName string) map[string]any {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	resolved, diagnostics := contractvalidator.LoadAndResolve(root, "contracts/mcp/tools.json", []string{"contracts/mcp/tools.json"})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve authored catalog diagnostics = %+v", diagnostics)
	}
	document, ok := resolved.(map[string]any)
	if !ok {
		t.Fatal("resolved catalog is not an object")
	}
	tools, ok := document["tools"].(map[string]any)
	if !ok {
		t.Fatal("resolved catalog tools missing")
	}
	record, ok := tools[mcpfactorycatalog.CatalogToolIDForName(toolName)].(map[string]any)
	if !ok {
		t.Fatalf("resolved catalog missing tool %q", toolName)
	}
	input, ok := record["input"].(map[string]any)
	if !ok {
		t.Fatalf("tool %q input missing", toolName)
	}
	schema, ok := input["schema"].(map[string]any)
	if !ok {
		t.Fatalf("tool %q input.schema missing", toolName)
	}
	return schema
}

func containsAnyString(values []string, wants []string) bool {
	for _, want := range wants {
		if containsString(values, want) {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
