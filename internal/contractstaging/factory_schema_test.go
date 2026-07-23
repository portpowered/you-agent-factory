package contractstaging_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
	"gopkg.in/yaml.v3"
)

func TestFactorySchemaGenerationUsesConverterPath(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)

	converted, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(factory, components)
	if len(diagnostics) != 0 {
		t.Fatalf("ConvertFailClosedSchema() diagnostics = %#v, want none", diagnostics)
	}
	if converted == nil {
		t.Fatal("ConvertFailClosedSchema() = nil, want converted schema")
	}

	artifacts := testArtifactsForRepository(t, repositoryRoot)
	payload := artifacts[contractstaging.FactorySchemaAuthoredPath]
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode authored factory schema: %v", err)
	}
	for _, key := range []string{"$schema", "$id", "title"} {
		if document[key] == nil || document[key] == "" {
			t.Fatalf("authored factory schema missing %s metadata", key)
		}
	}
	if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %#v, want Draft 2020-12", document["$schema"])
	}
	if document["$id"] != "https://schemas.portpowered.com/you/config/factory.schema.json" {
		t.Fatalf("$id = %#v, want factory config schema id", document["$id"])
	}
	if document["title"] != "You Factory configuration" {
		t.Fatalf("title = %#v, want You Factory configuration", document["title"])
	}
	if containsNullableKeyword(document) {
		t.Fatal("authored factory schema still contains legacy nullable keyword; want converter type unions")
	}
	if !containsTypeNullUnion(document) {
		t.Fatal("authored factory schema missing converter-backed nullable type unions")
	}
	if !bytes.Equal(payload, artifacts["packages/api/generated/schemas/factory.schema.json"]) {
		t.Fatal("authored and staged factory schema bytes differ")
	}
}

func containsNullableKeyword(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if _, ok := typed["nullable"]; ok {
			return true
		}
		for _, child := range typed {
			if containsNullableKeyword(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsNullableKeyword(child) {
				return true
			}
		}
	}
	return false
}

func containsTypeNullUnion(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if typeValue, ok := typed["type"]; ok {
			switch array := typeValue.(type) {
			case []any:
				for _, item := range array {
					if item == "null" {
						return true
					}
				}
			}
		}
		for _, child := range typed {
			if containsTypeNullUnion(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsTypeNullUnion(child) {
				return true
			}
		}
	}
	return false
}

func TestFactorySchemaConverterHasNoUnsupportedReferenceDiagnostics(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)

	collected := contractstaging.CollectFactorySchemaConverterDiagnosticsForTest(factory, components)
	for _, diagnostic := range collected {
		if diagnostic.Code == "openapi.convert.unsupported_reference" {
			t.Fatalf("unsupported_reference diagnostic remains: %#v", diagnostic)
		}
	}
}

func TestFactorySchemaB16GapRecordCoversCanonicalFactoryGraph(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)

	collected := contractstaging.CollectFactorySchemaConverterDiagnosticsForTest(factory, components)

	recordPath := filepath.Join(repositoryRoot, "docs", "internal", "contract", "factory-schema-b16-gaps.json")
	recordPayload, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read gap record: %v", err)
	}
	var record struct {
		Status             string `json:"status"`
		BlockingCategories []struct {
			Code          string `json:"code"`
			InstanceCount int    `json:"instanceCount"`
		} `json:"blockingCategories"`
	}
	if err := json.Unmarshal(recordPayload, &record); err != nil {
		t.Fatalf("decode gap record: %v", err)
	}
	if record.Status != "converter_endorsed" {
		t.Fatalf("gap record status = %q, want converter_endorsed", record.Status)
	}
	if len(record.BlockingCategories) != 0 {
		t.Fatalf("gap record blockingCategories = %#v, want empty for converter endorsement", record.BlockingCategories)
	}
	if len(collected) != 0 {
		t.Fatalf("canonical Factory graph still emits converter diagnostics = %#v, want none under converter_endorsed", collected)
	}
}

func TestFactorySchemaDigestsStableAcrossRepeatedArtifactsCalls(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	first := testArtifactsForRepository(t, repositoryRoot)
	second := testArtifactsForRepository(t, repositoryRoot)
	for _, path := range []string{
		contractstaging.FactorySchemaAuthoredPath,
		"packages/api/generated/schemas/factory.schema.json",
		"packages/packaged-factories/schemas/factory.schema.json",
		"packages/packaged-factories/schemas/factory.schema.yaml",
	} {
		if !bytes.Equal(first[path], second[path]) {
			t.Fatalf("repeated Artifacts() changed factory schema bytes at %s", path)
		}
	}
}

func TestPackagedFactorySchemasAreEquivalentCanonicalProjections(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	artifacts := testArtifactsForRepository(t, repositoryRoot)
	apiJSON := artifacts["packages/api/generated/schemas/factory.schema.json"]
	packagedJSON := artifacts["packages/packaged-factories/schemas/factory.schema.json"]
	packagedYAML := artifacts["packages/packaged-factories/schemas/factory.schema.yaml"]

	if !bytes.Equal(packagedJSON, apiJSON) {
		t.Fatal("packaged JSON Factory schema differs from the generated API Factory schema")
	}

	var jsonSchema any
	if err := json.Unmarshal(packagedJSON, &jsonSchema); err != nil {
		t.Fatalf("decode packaged JSON Factory schema: %v", err)
	}
	var yamlSchema any
	if err := yaml.Unmarshal(packagedYAML, &yamlSchema); err != nil {
		t.Fatalf("decode packaged YAML Factory schema: %v", err)
	}
	normalizedYAML, err := json.Marshal(yamlSchema)
	if err != nil {
		t.Fatalf("normalize packaged YAML Factory schema: %v", err)
	}
	var normalizedYAMLSchema any
	if err := json.Unmarshal(normalizedYAML, &normalizedYAMLSchema); err != nil {
		t.Fatalf("decode normalized packaged YAML Factory schema: %v", err)
	}
	if !reflect.DeepEqual(jsonSchema, normalizedYAMLSchema) {
		t.Fatal("packaged JSON and YAML Factory schemas parse to different values")
	}

	document := jsonSchema.(map[string]any)
	if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %#v, want Draft 2020-12", document["$schema"])
	}
	if document["$id"] != "https://schemas.portpowered.com/you/config/factory.schema.json" {
		t.Fatalf("$id = %#v, want stable Factory schema identifier", document["$id"])
	}
	properties := document["properties"].(map[string]any)
	for _, property := range []string{"metadata", "examples"} {
		if _, ok := properties[property]; !ok {
			t.Fatalf("Factory schema does not accept top-level %s", property)
		}
	}
	definitions := document["$defs"].(map[string]any)
	if _, ok := definitions["FactoryInvocationExample"]; !ok {
		t.Fatal("Factory schema does not include the reachable invocation-example contract")
	}
}

func TestFactorySchemaGenerationLeavesAuthoredAndStagedDigestsStableOnSecondRun(t *testing.T) {
	t.Parallel()
	defer contractstaging.LockRepositoryStagingForTest()()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factoryPaths := []string{
		contractstaging.FactorySchemaAuthoredPath,
		"packages/api/generated/schemas/factory.schema.json",
		"packages/packaged-factories/schemas/factory.schema.json",
		"packages/packaged-factories/schemas/factory.schema.yaml",
	}
	before := factorySchemaDigests(t, repositoryRoot, factoryPaths)

	drift, err := contractstaging.Check(repositoryRoot)
	if err != nil {
		t.Fatalf("precondition Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("precondition Check() drift = %#v, want clean staging before regeneration test", drift)
	}

	if err := contractstaging.Generate(repositoryRoot); err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	afterFirst := factorySchemaDigests(t, repositoryRoot, factoryPaths)
	if drift, err := contractstaging.Check(repositoryRoot); err != nil {
		t.Fatalf("Check() after first Generate() error = %v", err)
	} else if !drift.Empty() {
		t.Fatalf("Check() after first Generate() drift = %#v, want none", drift)
	}

	if err := contractstaging.Generate(repositoryRoot); err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	afterSecond := factorySchemaDigests(t, repositoryRoot, factoryPaths)
	if drift, err := contractstaging.Check(repositoryRoot); err != nil {
		t.Fatalf("Check() after second Generate() error = %v", err)
	} else if !drift.Empty() {
		t.Fatalf("Check() after second Generate() drift = %#v, want none", drift)
	}

	if !reflect.DeepEqual(before, afterFirst) {
		t.Fatalf("first Generate() changed factory schema digests:\nbefore=%x\nafter=%x", before, afterFirst)
	}
	if !reflect.DeepEqual(afterFirst, afterSecond) {
		t.Fatalf("second Generate() changed factory schema digests:\nfirst=%x\nsecond=%x", afterFirst, afterSecond)
	}
}

func factorySchemaDigests(t *testing.T, root string, paths []string) map[string][sha256.Size]byte {
	t.Helper()
	digests := make(map[string][sha256.Size]byte, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		digests[path] = sha256.Sum256(content)
	}
	return digests
}

func TestFactorySchemaGenerationFailsClosedOnUndocumentedDiagnostics(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)
	factoryCopy := contractstaging.DeepCopyValueForTest(factory).(map[string]any)
	componentsCopy := contractstaging.DeepCopyValueForTest(components).(map[string]any)
	factoryCopy["deprecated"] = true

	_, err := contractstaging.GenerateFactorySchemaFromGraphForTest(repositoryRoot, factoryCopy, componentsCopy)
	if err == nil {
		t.Fatal("GenerateFactorySchemaFromGraphForTest() = nil, want fail-closed error")
	}
	if !strings.Contains(err.Error(), "undocumented diagnostics") {
		t.Fatalf("error = %v, want undocumented diagnostics fail-closed message", err)
	}
	if !strings.Contains(err.Error(), "openapi.convert.unsupported_keyword") {
		t.Fatalf("error = %v, want unsupported_keyword diagnostic payload", err)
	}
	if !strings.Contains(err.Error(), "/deprecated") {
		t.Fatalf("error = %v, want reachable unsupported keyword path", err)
	}
}

func TestFactorySchemaGenerationIgnoresUnsupportedUnreachableComponents(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)
	factoryCopy := contractstaging.DeepCopyValueForTest(factory).(map[string]any)
	componentsCopy := contractstaging.DeepCopyValueForTest(components).(map[string]any)
	componentsCopy["UnsupportedButUnreachable"] = map[string]any{
		"type":       "string",
		"deprecated": true,
	}

	if _, err := contractstaging.GenerateFactorySchemaFromGraphForTest(
		repositoryRoot,
		factoryCopy,
		componentsCopy,
	); err != nil {
		t.Fatalf("unreachable unsupported component altered Factory generation: %v", err)
	}
}

func loadFactoryGraph(t *testing.T, repositoryRoot string) (map[string]any, map[string]any) {
	t.Helper()
	path := filepath.Join(repositoryRoot, "api", "openapi.yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}
	componentsValue, ok := document["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components")
	}
	components, ok := componentsValue["schemas"].(map[string]any)
	if !ok {
		t.Fatal("missing components.schemas")
	}
	factory, ok := components["Factory"].(map[string]any)
	if !ok {
		t.Fatal("missing Factory schema")
	}
	return factory, components
}
