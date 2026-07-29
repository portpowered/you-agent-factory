package apicontract_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractguard"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"gopkg.in/yaml.v3"
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
	canonicalFactoryEventTypeOwners := map[string]struct{}{
		filepath.Join(moduleRoot, "pkg", "services", "factory_definitions", "contracts", "factory_events.go"): {},
		filepath.Join(moduleRoot, "pkg", "services", "factory_definitions", "contracts_root.go"):              {},
	}

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
				assertAllowedFactoryContractType(t, path, canonicalFactoryEventTypeOwners, typeSpec, deletedTypeNames, generatedImportNames)
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
	canonicalFactoryEventTypeOwners map[string]struct{},
	typeSpec *ast.TypeSpec,
	deletedTypeNames map[string]struct{},
	generatedImportNames map[string]struct{},
) {
	t.Helper()
	if typeSpec.Name.Name == "FactoryEventType" || typeSpec.Name.Name == "FactoryEventContext" {
		if _, allowed := canonicalFactoryEventTypeOwners[path]; !allowed {
			t.Fatalf("%s declares %s outside canonical Factory Definition contract owners", path, typeSpec.Name.Name)
		}
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
func TestOpenAPIContract_WorkerModelProviderConvenienceConstantsMatchBuiltIns(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)
	schema := schemaObject(t, schemas, "WorkerModelProvider")

	assertEnumValues(t, schema, "WorkerModelProvider", publicWorkerModelProviderValues())
}

func TestOpenAPIContract_ProviderIdentityIsOpenAndSyntaxConstrained(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)
	schema := schemaObject(t, schemas, "ProviderIdentity")

	if _, closed := schema["enum"]; closed {
		t.Fatal("ProviderIdentity must not define a closed enum")
	}
	if schema["minLength"] != 1 || schema["maxLength"] != 128 {
		t.Fatalf("ProviderIdentity length bounds = %v/%v, want 1/128", schema["minLength"], schema["maxLength"])
	}
	pattern, _ := schema["pattern"].(string)
	if pattern == "" || !strings.Contains(pattern, "[a-z][a-z0-9]*") {
		t.Fatalf("ProviderIdentity.pattern = %q, want standardized lowercase identity syntax", pattern)
	}
}

func TestOpenAPIContract_GeneratedWorkerModelProviderConstantsMatchOpenAPIEnum(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)
	schema := schemaObject(t, schemas, "WorkerModelProvider")

	generated := []factoryapi.WorkerModelProvider{
		factoryapi.WorkerModelProviderClaude,
		factoryapi.WorkerModelProviderCodex,
		factoryapi.WorkerModelProviderCursor,
		factoryapi.WorkerModelProviderAntigravity,
	}
	values := make([]string, 0, len(generated))
	for _, provider := range generated {
		values = append(values, string(provider))
	}
	assertEnumValues(t, schema, "WorkerModelProvider", values)
}

func publicWorkerModelProviderValues() []string {
	return []string{
		"CLAUDE",
		"CODEX",
		"CURSOR",
		"ANTIGRAVITY",
	}
}

type providerPublicationPosture struct {
	support      factoryapi.ProviderTechnicalSupportLevel
	availability factoryapi.ProviderImplementationAvailability
}

func TestProviderCatalogContract_RepresentativeFixtureValidatesAndRoundTrips(t *testing.T) {
	t.Parallel()

	fixture := loadProviderCatalogFixture(t)
	doc := loadValidatedOpenAPIContract(t)
	if err := doc.Components.Schemas["ProviderCatalog"].Value.VisitJSON(fixture); err != nil {
		t.Fatalf("validate representative ProviderCatalog fixture: %v", err)
	}

	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal representative fixture: %v", err)
	}
	var catalog factoryapi.ProviderCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("unmarshal generated ProviderCatalog: %v", err)
	}
	if catalog.FormatVersion != factoryapi.ProviderCatalogFormatVersionV1 {
		t.Fatalf("format version = %q, want 1.0.0", catalog.FormatVersion)
	}
	if len(catalog.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(catalog.Providers))
	}
	alpha := catalog.Providers[0]
	if alpha.Id != "example-alpha" ||
		alpha.TechnicalSupportLevel != factoryapi.ProviderTechnicalSupportLevelExperimental ||
		alpha.ImplementationAvailability != factoryapi.ProviderImplementationAvailabilityExternallySupplied {
		t.Fatalf("generated provider metadata changed meaning: %#v", alpha)
	}
	if alpha.Deprecation == nil || alpha.Deprecation.ReplacementProviderId == nil ||
		*alpha.Deprecation.ReplacementProviderId != "example-next" {
		t.Fatalf("generated deprecation metadata changed meaning: %#v", alpha.Deprecation)
	}

	roundTrip, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("marshal generated ProviderCatalog: %v", err)
	}
	var roundTripValue any
	if err := json.Unmarshal(roundTrip, &roundTripValue); err != nil {
		t.Fatalf("decode generated ProviderCatalog JSON: %v", err)
	}
	if !reflect.DeepEqual(roundTripValue, fixture) {
		t.Fatalf("generated Go round trip changed the public fixture\n got: %#v\nwant: %#v", roundTripValue, fixture)
	}
}

func TestProviderManifestContract_RejectsOperationalDiscoveryData(t *testing.T) {
	t.Parallel()

	doc := loadValidatedOpenAPIContract(t)
	schema := doc.Components.Schemas["ProviderManifest"].Value
	for _, field := range []string{"readiness", "credentialValue", "environmentValue", "endpointAddress", "machinePath", "pricing"} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			manifest := providerManifestFromFixture(t)
			manifest["discovery"].(map[string]any)[field] = "not-public-contract-data"
			if err := schema.VisitJSON(manifest); err == nil {
				t.Fatalf("ProviderManifest accepted forbidden discovery field %q", field)
			}
		})
	}
}

func TestProviderManifestContract_RequiresCoherentDeprecationShape(t *testing.T) {
	t.Parallel()

	doc := loadValidatedOpenAPIContract(t)
	schema := doc.Components.Schemas["ProviderManifest"].Value
	manifest := providerManifestFromFixture(t)
	manifest["deprecation"] = map[string]any{
		"replacementProviderId": "example-next",
	}
	if err := schema.VisitJSON(manifest); err == nil {
		t.Fatal("ProviderManifest accepted deprecation metadata without deprecatedSince and reason")
	}
}

func TestProviderCatalogContract_UsesClosedPublicationVocabulary(t *testing.T) {
	t.Parallel()

	doc := loadValidatedOpenAPIContract(t)
	assertSchemaEnum := func(schemaName string, want []any) {
		t.Helper()
		got := doc.Components.Schemas[schemaName].Value.Enum
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s enum = %#v, want %#v", schemaName, got, want)
		}
	}
	assertSchemaEnum("ProviderTechnicalSupportLevel", []any{"production", "experimental", "not-supported"})
	assertSchemaEnum("ProviderImplementationAvailability", []any{"bundled", "externally-supplied", "catalog-only"})
}

func TestProviderManifestContract_FirstPartyCatalogIsEvidenceConservative(t *testing.T) {
	t.Parallel()

	wantPosture := map[string]providerPublicationPosture{
		"antigravity": {factoryapi.ProviderTechnicalSupportLevelExperimental, factoryapi.ProviderImplementationAvailabilityBundled},
		"claude":      {factoryapi.ProviderTechnicalSupportLevelExperimental, factoryapi.ProviderImplementationAvailabilityBundled},
		"codex":       {factoryapi.ProviderTechnicalSupportLevelProduction, factoryapi.ProviderImplementationAvailabilityBundled},
		"cursor":      {factoryapi.ProviderTechnicalSupportLevelExperimental, factoryapi.ProviderImplementationAvailabilityBundled},
	}
	doc := loadValidatedOpenAPIContract(t)
	schema := doc.Components.Schemas["ProviderManifest"].Value
	manifestPaths, err := filepath.Glob("../../../../packages/model-providers/providers/*/provider.yaml")
	if err != nil {
		t.Fatalf("enumerate first-party provider manifests: %v", err)
	}

	var ids []string
	aliases := make(map[string]string)
	for _, manifestPath := range manifestPaths {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("read %s: %v", manifestPath, err)
		}
		var manifestValue map[string]any
		if err := yaml.Unmarshal(data, &manifestValue); err != nil {
			t.Fatalf("decode %s: %v", manifestPath, err)
		}
		if err := schema.VisitJSON(manifestValue); err != nil {
			t.Fatalf("validate %s against ProviderManifest: %v", manifestPath, err)
		}

		jsonData, err := json.Marshal(manifestValue)
		if err != nil {
			t.Fatalf("marshal %s: %v", manifestPath, err)
		}
		var manifest factoryapi.ProviderManifest
		if err := json.Unmarshal(jsonData, &manifest); err != nil {
			t.Fatalf("decode %s as generated ProviderManifest: %v", manifestPath, err)
		}
		if filepath.Base(filepath.Dir(manifestPath)) != manifest.Id {
			t.Fatalf("%s contains id %q", manifestPath, manifest.Id)
		}
		ids = append(ids, manifest.Id)
		recordProviderAliases(t, aliases, manifest)
		assertProviderPublicationPosture(t, manifest, wantPosture)
	}

	sort.Strings(ids)
	wantIDs := []string{"antigravity", "claude", "codex", "cursor"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("first-party provider ids = %v, want %v", ids, wantIDs)
	}
	for _, id := range ids {
		if owner, shadowed := aliases[id]; shadowed {
			t.Fatalf("provider id %q is shadowed by an alias from %q", id, owner)
		}
	}
}

func recordProviderAliases(t *testing.T, aliases map[string]string, manifest factoryapi.ProviderManifest) {
	t.Helper()

	for _, alias := range manifest.Aliases {
		if owner, exists := aliases[alias]; exists {
			t.Fatalf("alias %q is shared by %q and %q", alias, owner, manifest.Id)
		}
		aliases[alias] = manifest.Id
	}
}

func assertProviderPublicationPosture(
	t *testing.T,
	manifest factoryapi.ProviderManifest,
	wantPosture map[string]providerPublicationPosture,
) {
	t.Helper()

	posture, ok := wantPosture[manifest.Id]
	if !ok {
		t.Fatalf("unexpected first-party provider %q", manifest.Id)
	}
	if manifest.TechnicalSupportLevel != posture.support ||
		manifest.ImplementationAvailability != posture.availability {
		t.Fatalf(
			"%s posture = (%s, %s), want (%s, %s)",
			manifest.Id,
			manifest.TechnicalSupportLevel,
			manifest.ImplementationAvailability,
			posture.support,
			posture.availability,
		)
	}
}

func loadProviderCatalogFixture(t *testing.T) map[string]any {
	t.Helper()

	data, err := os.ReadFile("../../../../api/testdata/provider-catalog-contract.json")
	if err != nil {
		t.Fatalf("read representative ProviderCatalog fixture: %v", err)
	}
	var fixture map[string]any
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode representative ProviderCatalog fixture: %v", err)
	}
	return fixture
}

func providerManifestFromFixture(t *testing.T) map[string]any {
	t.Helper()

	fixture := loadProviderCatalogFixture(t)
	providers, ok := fixture["providers"].([]any)
	if !ok || len(providers) == 0 {
		t.Fatal("representative ProviderCatalog fixture has no providers")
	}
	manifest, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatal("representative ProviderManifest fixture is not an object")
	}
	return manifest
}
