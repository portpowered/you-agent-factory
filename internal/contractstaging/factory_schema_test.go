package contractstaging_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
	"gopkg.in/yaml.v3"
)

func TestFactorySchemaConverterIsBlockedByDocumentedB16Gaps(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)

	_, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(factory, components)
	if len(diagnostics) == 0 {
		t.Fatal("ConvertFailClosedSchema() succeeded, want documented B16-blocking diagnostics")
	}
	if diagnostics[0].Code != "openapi.convert.unsupported_keyword" || diagnostics[0].Path != "/example" {
		t.Fatalf("first diagnostic = %#v, want unsupported_keyword on /example", diagnostics[0])
	}

	artifacts, err := contractstaging.Artifacts(repositoryRoot)
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	if len(artifacts["packages/api/generated/schemas/factory.schema.json"]) == 0 {
		t.Fatal("Artifacts() returned empty factory schema while converter is blocked")
	}
}

func TestFactorySchemaB16GapRecordCoversCanonicalFactoryGraph(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)

	collected := contractstaging.CollectFactorySchemaConverterDiagnosticsForTest(factory, components)
	if len(collected) == 0 {
		t.Fatal("collectFactorySchemaConverterDiagnostics() = 0, want blocking diagnostics")
	}

	recordPath := filepath.Join(repositoryRoot, "docs", "internal", "contract", "factory-schema-b16-gaps.json")
	recordPayload, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read gap record: %v", err)
	}
	var record struct {
		BlockingCategories []struct {
			Code          string `json:"code"`
			InstanceCount int    `json:"instanceCount"`
		} `json:"blockingCategories"`
	}
	if err := json.Unmarshal(recordPayload, &record); err != nil {
		t.Fatalf("decode gap record: %v", err)
	}

	counts := map[string]int{}
	for _, diagnostic := range collected {
		matches, err := contractstaging.FactorySchemaDiagnosticMatchesGapRecordForTest(repositoryRoot, diagnostic)
		if err != nil {
			t.Fatalf("FactorySchemaDiagnosticMatchesGapRecordForTest() error = %v", err)
		}
		if !matches {
			t.Fatalf("diagnostic %#v is not covered by the B16 gap record", diagnostic)
		}
		counts[diagnostic.Code]++
	}

	for _, category := range record.BlockingCategories {
		if counts[category.Code] != category.InstanceCount {
			t.Fatalf("gap record %s instanceCount = %d, collected %d", category.Code, category.InstanceCount, counts[category.Code])
		}
	}

	factoryCopy := contractstaging.DeepCopyValueForTest(factory).(map[string]any)
	componentsCopy := contractstaging.DeepCopyValueForTest(components).(map[string]any)
	for _, diagnostic := range collected {
		if !contractstaging.StripFactorySchemaConverterGapForTest(factoryCopy, componentsCopy, diagnostic) {
			t.Fatalf("could not strip diagnostic %#v from copied Factory graph", diagnostic)
		}
	}
	_, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(factoryCopy, componentsCopy)
	if len(diagnostics) != 0 {
		t.Fatalf("ConvertFailClosedSchema() after stripping recorded gaps = %#v, want success", diagnostics)
	}
}

func TestProductionPackagesDoNotImportFactorySchemaConverter(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	packageRoots := []string{
		"pkg/transports/http",
		"pkg/transports/cli",
		"pkg/config",
		"pkg/service",
		"pkg/workers",
		"pkg/factory",
	}
	target := "github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	for _, packageRoot := range packageRoots {
		packageRoot := packageRoot
		t.Run(packageRoot, func(t *testing.T) {
			t.Parallel()
			if importFound(t, filepath.Join(repositoryRoot, packageRoot), target) {
				t.Fatalf("%s imports build-time converter %s", packageRoot, target)
			}
		})
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

func importFound(t *testing.T, root, target string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(payload), "\""+target+"\"") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && !found {
		t.Fatalf("walk %s: %v", root, err)
	}
	return found
}
