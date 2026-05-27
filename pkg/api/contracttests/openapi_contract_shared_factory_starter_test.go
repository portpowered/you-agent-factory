package apicontract_test

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIContract_DocumentsSharedFactoryStarterWorkCopySemantics(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	bundledFileSchema := requireOpenAPI3ComponentSchema(t, doc, "BundledFile")
	bundledFileTypeSchema := assertOpenAPI3PropertyDescription(t, bundledFileSchema, "BundledFile", "type")

	data, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	schemas := componentSchemas(t, raw)
	factoryRaw, ok := schemas["Factory"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.Factory must be an object schema")
	}
	factoryProperties, ok := factoryRaw["properties"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.Factory.properties is missing")
	}
	supportingFilesRaw, ok := factoryProperties["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.Factory.properties.supportingFiles is missing")
	}
	supportingFilesDescription, ok := supportingFilesRaw["description"].(string)
	if !ok {
		t.Fatalf("components.schemas.Factory.properties.supportingFiles.description is missing")
	}
	resourceManifestRaw, ok := schemas["ResourceManifest"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.ResourceManifest must be an object schema")
	}
	resourceManifestProperties, ok := resourceManifestRaw["properties"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.ResourceManifest.properties is missing")
	}
	bundledFilesRaw, ok := resourceManifestProperties["bundledFiles"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.ResourceManifest.properties.bundledFiles is missing")
	}
	bundledFilesDescription, ok := bundledFilesRaw["description"].(string)
	if !ok {
		t.Fatalf("components.schemas.ResourceManifest.properties.bundledFiles.description is missing")
	}

	for _, expectation := range []struct {
		path        string
		description string
		substrings  []string
	}{
		{
			path:        "Factory.supportingFiles",
			description: supportingFilesDescription,
			substrings:  []string{"share-time snapshot", "detached starter-work copies"},
		},
		{
			path:        "ResourceManifest.bundledFiles",
			description: bundledFilesDescription,
			substrings:  []string{"current starter work at share time", "independent recipient copies"},
		},
		{
			path:        "BundledFile",
			description: bundledFileSchema.Description,
			substrings:  []string{"share-time snapshot", "detached seeded work"},
		},
		{
			path:        "BundledFile.type",
			description: bundledFileTypeSchema.Description,
			substrings:  []string{"snapshot current source inputs at share time", "live link"},
		},
	} {
		for _, substring := range expectation.substrings {
			if !strings.Contains(expectation.description, substring) {
				t.Fatalf("%s description = %q, want substring %q", expectation.path, expectation.description, substring)
			}
		}
	}
}
