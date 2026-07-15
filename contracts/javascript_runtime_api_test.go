package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/callbehavior"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/symbolidentity"
)

func javascriptManifestFixtureRegistry(fixture string) contractvalidator.Registry {
	const (
		documentationID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
		deprecationsID  = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
	)
	return contractvalidator.NewRegistry(contractvalidator.Entry{
		Family:        "javascript",
		FormatVersion: "1.0.0",
		Schemas: []contractvalidator.Schema{
			{ID: documentationID, Path: "contracts/common/documentation.schema.json"},
			{ID: deprecationsID, Path: "contracts/common/deprecations.schema.json"},
			{ID: runtimeManifestSchemaID, Path: "contracts/javascript/runtime-manifest.schema.json"},
		},
		Documents: []contractvalidator.Document{{
			Path:     fixture,
			SchemaID: runtimeManifestSchemaID,
		}},
	})
}

func TestJavaScriptRuntimeAPICatalogRootValuesArgsAndMeta(t *testing.T) {
	t.Parallel()

	catalog := loadAuthoredJavaScriptRuntimeCatalog(t)
	symbolsByPath := catalogSymbolsByPath(t, catalog)

	for _, path := range []string{"args", "meta"} {
		symbol, ok := symbolsByPath[path]
		if !ok {
			t.Fatalf("catalog missing root value symbol at path %q", path)
		}
		if got := countCatalogPaths(symbolsByPath, path); got != 1 {
			t.Fatalf("catalog path %q appears %d times, want exactly once", path, got)
		}

		identity := symbolidentity.ProjectInstalledBindings()
		identityByPath := symbolRecordsByPath(identity)
		wantIdentity, ok := identityByPath[path]
		if !ok {
			t.Fatalf("identity baseline missing path %q", path)
		}
		assertCatalogValueMatchesIdentityBaseline(t, path, symbol, wantIdentity)

		callInventory := callbehavior.ProjectInstalledCallBehavior()
		callByPath := callRecordsByPath(callInventory)
		wantCallBehavior, ok := callByPath[path]
		if !ok {
			t.Fatalf("call-behavior baseline missing path %q", path)
		}
		assertCatalogValueMatchesCallBehaviorBaseline(t, path, symbol, wantCallBehavior)
	}
}

func TestJavaScriptRuntimeAPIAuthoredCatalogBoundary(t *testing.T) {
	t.Parallel()

	catalogPath := filepath.Join("javascript", "runtime-api.json")
	if _, err := os.Stat(catalogPath); err != nil {
		t.Fatalf("contracts/javascript/runtime-api.json must exist as the authored catalog: %v", err)
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	diagnostics := contractvalidator.Validate(root, contractvalidator.JavaScriptRegistry(), "javascript", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("authored catalog validation diagnostics = %+v", diagnostics)
	}
}

func TestJavaScriptRuntimeAPIBrokenSharedSchemaReference(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	const (
		fixture  = "contracts/testdata/javascript/invalid-broken-shared-schema-ref.json"
		wantPath = "/symbols/javascript.workflow.checkpoint/parameters/0/serializableValue/$ref"
	)
	diagnostics := contractvalidator.Validate(root, javascriptManifestFixtureRegistry(fixture), "javascript", "1.0.0")
	if len(diagnostics) == 0 {
		t.Fatal("expected broken shared-schema reference diagnostics, got none")
	}
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "reference.fragment" && diagnostic.Path == wantPath && diagnostic.Document == fixture {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want code=reference.fragment path=%q document=%q", diagnostics, wantPath, fixture)
	}
}

func TestJavaScriptRuntimeAPIOpenSharedSchema(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	const (
		fixture  = "contracts/testdata/javascript/invalid-open-shared-schema.json"
		wantPath = "/sharedSchemas/javascript.schema.open_object/schema"
	)
	diagnostics := contractvalidator.Validate(root, javascriptManifestFixtureRegistry(fixture), "javascript", "1.0.0")
	if len(diagnostics) == 0 {
		t.Fatal("expected open shared-schema diagnostics, got none")
	}
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "javascript.serializable_value.open" && diagnostic.Path == wantPath && diagnostic.Document == fixture {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want code=javascript.serializable_value.open path=%q document=%q", diagnostics, wantPath, fixture)
	}
}

func loadAuthoredJavaScriptRuntimeCatalog(t *testing.T) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("javascript", "runtime-api.json"))
	if err != nil {
		t.Fatalf("read authored catalog: %v", err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("unmarshal authored catalog: %v", err)
	}
	return catalog
}

func catalogSymbolsByPath(t *testing.T, catalog map[string]any) map[string]map[string]any {
	t.Helper()

	symbolsValue, ok := catalog["symbols"].(map[string]any)
	if !ok {
		t.Fatalf("catalog symbols = %#v, want object", catalog["symbols"])
	}
	byPath := make(map[string]map[string]any, len(symbolsValue))
	for _, symbolValue := range symbolsValue {
		symbol, ok := symbolValue.(map[string]any)
		if !ok {
			t.Fatalf("catalog symbol = %#v, want object", symbolValue)
		}
		path, _ := symbol["path"].(string)
		if path == "" {
			t.Fatal("catalog symbol missing path")
		}
		byPath[path] = symbol
	}
	return byPath
}

func countCatalogPaths(byPath map[string]map[string]any, path string) int {
	count := 0
	for candidate := range byPath {
		if candidate == path {
			count++
		}
	}
	return count
}

func symbolRecordsByPath(inventory symbolidentity.Inventory) map[string]symbolidentity.SymbolRecord {
	byPath := make(map[string]symbolidentity.SymbolRecord, len(inventory.Symbols))
	for _, record := range inventory.Symbols {
		byPath[record.Path] = record
	}
	return byPath
}

func callRecordsByPath(inventory callbehavior.Inventory) map[string]callbehavior.CallBehaviorRecord {
	byPath := make(map[string]callbehavior.CallBehaviorRecord, len(inventory.Records))
	for _, record := range inventory.Records {
		byPath[record.Path] = record
	}
	return byPath
}

func assertCatalogValueMatchesIdentityBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want symbolidentity.SymbolRecord,
) {
	t.Helper()

	if got, _ := symbol["id"].(string); got != "javascript."+path {
		t.Fatalf("catalog %q id = %q, want %q", path, got, "javascript."+path)
	}
	if got, _ := symbol["name"].(string); got != want.Name {
		t.Fatalf("catalog %q name = %q, want %q", path, got, want.Name)
	}
	if got, _ := symbol["path"].(string); got != want.Path {
		t.Fatalf("catalog %q path = %q, want %q", path, got, want.Path)
	}
	if got, _ := symbol["kind"].(string); got != want.Kind {
		t.Fatalf("catalog %q kind = %q, want %q", path, got, want.Kind)
	}
	if parent, ok := symbol["parent"].(string); ok && parent != "" {
		t.Fatalf("catalog %q parent = %q, want no parent", path, parent)
	}
	if members, ok := symbol["members"].([]any); ok && len(members) > 0 {
		t.Fatalf("catalog %q members = %#v, want no members", path, members)
	}
	assertCatalogValueHasDocumentationExample(t, path, symbol)
}

func assertCatalogValueMatchesCallBehaviorBaseline(
	t *testing.T,
	path string,
	symbol map[string]any,
	want callbehavior.CallBehaviorRecord,
) {
	t.Helper()

	if got, _ := symbol["mutability"].(string); got != want.Mutability {
		t.Fatalf("catalog %q mutability = %q, want %q", path, got, want.Mutability)
	}
	if got, _ := symbol["nullability"].(string); got != want.Nullability {
		t.Fatalf("catalog %q nullability = %q, want %q", path, got, want.Nullability)
	}
	if got, _ := symbol["bindingLifecycle"].(string); got != want.Lifecycle {
		t.Fatalf("catalog %q bindingLifecycle = %q, want %q", path, got, want.Lifecycle)
	}
}

func assertCatalogValueHasDocumentationExample(t *testing.T, path string, symbol map[string]any) {
	t.Helper()

	documentation, ok := symbol["documentation"].(map[string]any)
	if !ok {
		t.Fatalf("catalog %q documentation = %#v, want object", path, symbol["documentation"])
	}
	examples, ok := documentation["examples"].([]any)
	if !ok || len(examples) == 0 {
		t.Fatalf("catalog %q documentation.examples = %#v, want at least one example", path, documentation["examples"])
	}
	first, ok := examples[0].(string)
	if !ok || first == "" {
		t.Fatalf("catalog %q first documentation example = %#v, want non-empty string", path, examples[0])
	}
}
