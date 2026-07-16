package apicontract_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractguard"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNoHandwrittenLegacyReplayModelsOrGeneratedAliases(t *testing.T) {
	moduleRoot := filepath.Clean(filepath.Join("..", "..", "..", ".."))
	generatedImportPaths := map[string]struct{}{
		"github.com/portpowered/infinite-you/pkg/transports/http/generated": {},
		"pkg/transports/http/generated":                                     {},
	}
	deletedTypeNames := map[string]struct{}{
		"FactoryEventEnvelope": {},
		"RecordedWorkRequest":  {},
		"RecordedSubmission":   {},
		"RecordedDispatch":     {},
		"RecordedCompletion":   {},
		"SubmissionDiagnostic": {},
		"DispatchDiagnostic":   {},
	}
	canonicalFactoryEventTypeOwner := filepath.Join(moduleRoot, "pkg", "factory", "contracts", "factory_events.go")

	fset := token.NewFileSet()
	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if contractguard.ShouldSkipDir(
				moduleRoot,
				path,
				"pkg/transports/http/generated",
				"ui/dist",
				"ui/node_modules",
				"ui/storybook-static",
			) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		generatedImportNames := generatedAPIAliases(file, generatedImportPaths)
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				assertAllowedFactoryContractType(t, path, canonicalFactoryEventTypeOwner, typeSpec, deletedTypeNames, generatedImportNames)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan handwritten API models: %v", err)
	}
}

func assertAllowedFactoryContractType(
	t *testing.T,
	path string,
	canonicalFactoryEventTypeOwner string,
	typeSpec *ast.TypeSpec,
	deletedTypeNames map[string]struct{},
	generatedImportNames map[string]struct{},
) {
	t.Helper()
	if (typeSpec.Name.Name == "FactoryEventType" || typeSpec.Name.Name == "FactoryEventContext") && path != canonicalFactoryEventTypeOwner {
		t.Fatalf("%s declares %s outside canonical Factory owner %s", path, typeSpec.Name.Name, canonicalFactoryEventTypeOwner)
	}
	if _, deleted := deletedTypeNames[typeSpec.Name.Name]; deleted {
		t.Fatalf("%s declares deleted legacy replay/event type %s", path, typeSpec.Name.Name)
	}
	if typeSpec.Assign.IsValid() && aliasesGeneratedAPI(typeSpec.Type, generatedImportNames) {
		t.Fatalf("%s aliases generated API type %s; use generated types directly", path, typeSpec.Name.Name)
	}
}

func generatedAPIAliases(file *ast.File, generatedImportPaths map[string]struct{}) map[string]struct{} {
	aliases := map[string]struct{}{}
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if _, ok := generatedImportPaths[importPath]; !ok {
			continue
		}
		name := "generated"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		aliases[name] = struct{}{}
	}
	return aliases
}

func aliasesGeneratedAPI(expr ast.Expr, generatedImportNames map[string]struct{}) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = generatedImportNames[ident.Name]
	return ok
}
func TestOpenAPIContract_WorkerModelProviderEnumMatchesSupportedBackendProviders(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)
	schema := schemaObject(t, schemas, "WorkerModelProvider")

	wantPublic := make([]string, 0, len(modelprovider.Supported()))
	for _, internal := range modelprovider.Supported() {
		public, ok := interfaces.PublicWorkerModelProviderFromInternal(internal)
		if !ok {
			t.Fatalf("PublicWorkerModelProviderFromInternal(%q) = false", internal)
		}
		wantPublic = append(wantPublic, string(public))
	}
	sort.Strings(wantPublic)
	assertEnumValues(t, schema, "WorkerModelProvider", wantPublic)

	for _, internal := range modelprovider.Supported() {
		public, ok := interfaces.PublicWorkerModelProviderFromInternal(internal)
		if !ok {
			t.Fatalf("PublicWorkerModelProviderFromInternal(%q) = false", internal)
		}
		mapped, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(string(public))
		if !ok || mapped != internal {
			t.Fatalf("InternalModelProviderFromPublicWorkerModelProvider(%q) = (%q, %v), want (%q, true)", public, mapped, ok, internal)
		}
	}
}

func TestOpenAPIContract_GeneratedWorkerModelProviderConstantsMatchOpenAPIEnum(t *testing.T) {
	want := []factoryapi.WorkerModelProvider{
		factoryapi.WorkerModelProviderClaude,
		factoryapi.WorkerModelProviderCodex,
		factoryapi.WorkerModelProviderCursor,
		factoryapi.WorkerModelProviderGemini,
		factoryapi.WorkerModelProviderKiro,
		factoryapi.WorkerModelProviderOpenCode,
		factoryapi.WorkerModelProviderPi,
		factoryapi.WorkerModelProviderAgy,
	}
	if len(want) != len(modelprovider.Supported()) {
		t.Fatalf("generated WorkerModelProvider constants = %d, supported internal providers = %d", len(want), len(modelprovider.Supported()))
	}
	for _, public := range want {
		if _, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(string(public)); !ok {
			t.Fatalf("InternalModelProviderFromPublicWorkerModelProvider(%q) = false", public)
		}
	}
}
