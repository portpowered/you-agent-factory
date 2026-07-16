package apicontract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIContract_DocumentsExplicitRootHelperBundlingPolicy(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	bundledFileSchema := requireOpenAPI3ComponentSchema(t, doc, "BundledFile")
	bundledFileTypeSchema := assertOpenAPI3PropertyDescription(t, bundledFileSchema, "BundledFile", "type")
	bundledFileContentSchema := requireOpenAPI3ComponentSchema(t, doc, "BundledFileContent")
	bundledFileInlineSchema := assertOpenAPI3PropertyDescription(t, bundledFileContentSchema, "BundledFileContent", "inline")

	data, err := os.ReadFile("../../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	schemas := componentSchemas(t, raw)
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
			path:        "ResourceManifest.bundledFiles",
			description: bundledFilesDescription,
			substrings: []string{
				"explicit only",
				"ROOT_HELPER entries such as Makefile are opt-in manifest entries",
			},
		},
		{
			path:        "BundledFile",
			description: bundledFileSchema.Description,
			substrings: []string{
				"only when declared explicitly in bundledFiles",
				"do not auto-discover project-root helpers",
			},
		},
		{
			path:        "BundledFile.type",
			description: bundledFileTypeSchema.Description,
			substrings: []string{
				"only when explicitly declared in bundledFiles",
			},
		},
		{
			path:        "BundledFileContent.inline",
			description: bundledFileInlineSchema.Description,
			substrings: []string{
				"explicit ROOT_HELPER entries in bundledFiles",
			},
		},
	} {
		for _, substring := range expectation.substrings {
			if !strings.Contains(expectation.description, substring) {
				t.Fatalf("%s description = %q, want substring %q", expectation.path, expectation.description, substring)
			}
		}
	}
}

func TestOpenAPIContract_ExplicitRootHelperBundlingRoundTripsThroughFlattenBoundary(t *testing.T) {
	factorySchema := loadFactorySchemaForSmoke(t)

	implicitDir := writeExplicitRootHelperContractFactoryDir(t, explicitRootHelperContractFactoryJSON(false))
	implicitFlattened := flattenFactoryConfigForSmoke(t, implicitDir)
	assertFactorySchemaAcceptsJSON(t, factorySchema, implicitFlattened)
	assertFlattenedBundledFilesOmitMakefile(t, implicitFlattened)

	explicitDir := writeExplicitRootHelperContractFactoryDir(t, explicitRootHelperContractFactoryJSON(true))
	explicitFlattened := flattenFactoryConfigForSmoke(t, explicitDir)
	assertFactorySchemaAcceptsJSON(t, factorySchema, explicitFlattened)
	assertFlattenedBundledFilesIncludeExplicitMakefile(t, explicitFlattened)
}

func writeExplicitRootHelperContractFactoryDir(t *testing.T, factoryJSON string) string {
	t.Helper()

	projectDir := t.TempDir()
	factoryDir := filepath.Join(projectDir, "factory")
	if err := os.MkdirAll(filepath.Join(factoryDir, interfaces.WorkersDir, "executor"), 0o755); err != nil {
		t.Fatalf("mkdir worker dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(factoryDir, interfaces.WorkstationsDir, "execute-story"), 0o755); err != nil {
		t.Fatalf("mkdir workstation dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(factoryDir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(factoryDir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), []byte(factoryJSON), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName), []byte("---\ntype: SCRIPT_WORKER\n---\nExecute the story.\n"), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.WorkstationsDir, "execute-story", interfaces.FactoryAgentsFileName), []byte("---\ntype: MODEL_WORKSTATION\n---\nExecute {{ (index .Inputs 0).WorkID }}.\n"), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "scripts", "setup.py"), []byte("print('portable')\n"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "docs", "usage.md"), []byte("# Usage\n"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "Makefile"), []byte("test:\n\tgo test ./...\n"), 0o644); err != nil {
		t.Fatalf("write Makefile: %v", err)
	}
	return factoryDir
}

func explicitRootHelperContractFactoryJSON(includeExplicitMakefile bool) string {
	supportingFiles := ""
	if includeExplicitMakefile {
		supportingFiles = `,
  "supportingFiles": {
    "bundledFiles": [
      {"type":"ROOT_HELPER","targetPath":"Makefile","content":{}}
    ]
  }`
	}
	return `{
  "name":"explicit-root-helper-contract",
  "workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "resources":[],
  "workers":[{"name":"executor"}],
  "workstations":[{
    "name":"execute-story",
    "worker":"executor",
    "inputs":[{"workType":"task","state":"init"}],
    "outputs":[{"workType":"task","state":"complete"}],
    "onFailure":[{"workType":"task","state":"failed"}]
  }]` + supportingFiles + `
}`
}

func assertFlattenedBundledFilesOmitMakefile(t *testing.T, flattened []byte) {
	t.Helper()

	cfg, err := config.FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.ResourceManifest == nil {
		t.Fatal("expected flattened config to include resourceManifest")
	}
	for _, bundledFile := range cfg.ResourceManifest.BundledFiles {
		if bundledFile.Type == interfaces.BundledFileTypeRootHelper && bundledFile.TargetPath == "Makefile" {
			t.Fatalf("expected implicit flatten to omit Makefile, got %#v", cfg.ResourceManifest.BundledFiles)
		}
	}
}

func assertFlattenedBundledFilesIncludeExplicitMakefile(t *testing.T, flattened []byte) {
	t.Helper()

	cfg, err := config.FactoryConfigFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.ResourceManifest == nil {
		t.Fatal("expected flattened config to include resourceManifest")
	}
	var makefileEntry *interfaces.BundledFileConfig
	for i := range cfg.ResourceManifest.BundledFiles {
		bundledFile := cfg.ResourceManifest.BundledFiles[i]
		if bundledFile.Type == interfaces.BundledFileTypeRootHelper && bundledFile.TargetPath == "Makefile" {
			makefileEntry = &cfg.ResourceManifest.BundledFiles[i]
			break
		}
	}
	if makefileEntry == nil {
		t.Fatalf("expected explicit ROOT_HELPER Makefile, got %#v", cfg.ResourceManifest.BundledFiles)
	}
	if makefileEntry.Content.Inline != "test:\n\tgo test ./...\n" {
		t.Fatalf("bundled Makefile content = %#v, want disk-backed inline body", makefileEntry.Content)
	}
}

func TestOpenAPIContract_DocumentsSharedFactoryStarterWorkCopySemantics(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	bundledFileSchema := requireOpenAPI3ComponentSchema(t, doc, "BundledFile")
	bundledFileTypeSchema := assertOpenAPI3PropertyDescription(t, bundledFileSchema, "BundledFile", "type")

	data, err := os.ReadFile("../../../../api/openapi.yaml")
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
