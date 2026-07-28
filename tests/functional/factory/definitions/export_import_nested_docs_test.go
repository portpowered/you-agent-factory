package definitions

import (
	"os"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	exportImportNestedDocPath = "factory/docs/standards/review.md"
	exportImportNestedDocBody = "# Review standards\n"
)

type exportImportNestedDocsResult struct {
	ExportedFactory  factoryapi.Factory
	ImportedFactory  factoryapi.Factory
	CurrentFactory   factoryapi.Factory
	SourceFactoryDir string
	ImportedDir      string
}

// TestExportImportSmokePreservesNestedFactoryDocsThroughExportImport proves nested
// factory docs seeded on disk survive export and import through public flatten and
// factory create customer paths without external provider command execution.
func TestExportImportSmokePreservesNestedFactoryDocsThroughExportImport(t *testing.T) {
	fixture := newServiceSimpleExportImportFixture(t)
	result := runExportImportNestedDocsSmokeViaCLI(t, fixture)

	assertExportImportNestedDocInBundledFiles(
		t,
		result.ExportedFactory,
		exportImportNestedDocPath,
		exportImportNestedDocBody,
		"exported factory",
	)
	assertExportImportNestedDocTargetPresent(
		t,
		result.ImportedFactory,
		exportImportNestedDocPath,
		"imported factory",
	)
	assertExportImportNestedDocTargetPresent(
		t,
		result.CurrentFactory,
		exportImportNestedDocPath,
		"current factory after import",
	)
	assertExportImportNestedDocOnDisk(
		t,
		filepath.Join(result.ImportedDir, "docs", "standards", "review.md"),
		exportImportNestedDocBody,
	)

	nestedDocPath := filepath.Join(result.SourceFactoryDir, "docs", "standards", "review.md")
	assertExportImportNestedDocOnDisk(t, nestedDocPath, exportImportNestedDocBody)
}

func runExportImportNestedDocsSmokeViaCLI(
	t *testing.T,
	fixture serviceSimpleExportImportFixture,
) exportImportNestedDocsResult {
	t.Helper()

	runner := support.NewRecordingCommandRunner("export/import smoke must not execute providers")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	homeDir := t.TempDir()
	workingDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	namedFactoriesRoot := initializeImportExportCustomerHome(t, env, workingDir)

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "factory.json")
	if err := os.WriteFile(sourcePath, fixture.CanonicalFactoryJSON, 0o600); err != nil {
		t.Fatalf("write export source factory.json: %v", err)
	}
	sourceFactoryDir := createImportExportActivatedNamedFactory(
		t,
		env,
		workingDir,
		namedFactoriesRoot,
		"exported-service-simple",
		sourcePath,
	)
	seedExportImportNestedDocOnDisk(t, sourceFactoryDir)

	exportedPayload, err := flattenFactoryConfigWithEdges(
		t,
		edges,
		filepath.Join(sourceFactoryDir, "factory.json"),
	)
	if err != nil {
		t.Fatalf("flattenFactoryConfigWithEdges(exported source): %v", err)
	}
	exportedFactory, err := decodeFactoryDefinitionForTest(exportedPayload)
	if err != nil {
		t.Fatalf("decode exported factory: %v", err)
	}

	exportPath := filepath.Join(t.TempDir(), "reexported-service-simple.json")
	if err := os.WriteFile(exportPath, exportedPayload, 0o644); err != nil {
		t.Fatalf("write reexported factory payload: %v", err)
	}

	importedDir := createImportExportActivatedNamedFactory(
		t,
		env,
		workingDir,
		namedFactoriesRoot,
		"reimported-service-simple",
		exportPath,
	)
	importedFactory, err := decodeFactoryDefinitionForTest(
		mustFlattenFactoryConfigWithEdges(t, edges, filepath.Join(importedDir, "factory.json")),
	)
	if err != nil {
		t.Fatalf("decode imported factory: %v", err)
	}
	currentFactory, err := decodeFactoryDefinitionForTest(
		mustFlattenFactoryConfigWithEdges(t, edges, filepath.Join(importedDir, "factory.json")),
	)
	_ = edges
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during export/import smoke", runner.CallCount())
	}

	return exportImportNestedDocsResult{
		ExportedFactory:  exportedFactory,
		ImportedFactory:  importedFactory,
		CurrentFactory:   currentFactory,
		SourceFactoryDir: sourceFactoryDir,
		ImportedDir:      importedDir,
	}
}

func seedExportImportNestedDocOnDisk(t *testing.T, factoryDir string) {
	t.Helper()

	nestedDocPath := filepath.Join(factoryDir, "docs", "standards", "review.md")
	if err := os.MkdirAll(filepath.Dir(nestedDocPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(nestedDocPath), err)
	}
	if err := os.WriteFile(nestedDocPath, []byte(exportImportNestedDocBody), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", nestedDocPath, err)
	}
}

func assertExportImportNestedDocInBundledFiles(
	t *testing.T,
	factory factoryapi.Factory,
	targetPath string,
	wantInline string,
	contextLabel string,
) {
	t.Helper()

	bundledFile, ok := findImportExportBundledFileByTargetPath(factory, targetPath)
	if !ok {
		t.Fatalf("%s missing bundled doc target %q", contextLabel, targetPath)
	}
	if bundledFile.Type != factoryapi.BundledFileTypeDOC {
		t.Fatalf(
			"%s bundled file %q type = %q, want DOC",
			contextLabel,
			targetPath,
			bundledFile.Type,
		)
	}
	if bundledFile.Content.Inline != wantInline {
		t.Fatalf(
			"%s bundled doc %q inline = %q, want %q",
			contextLabel,
			targetPath,
			bundledFile.Content.Inline,
			wantInline,
		)
	}
}

func assertExportImportNestedDocTargetPresent(
	t *testing.T,
	factory factoryapi.Factory,
	targetPath string,
	contextLabel string,
) {
	t.Helper()

	if _, ok := findImportExportBundledFileByTargetPath(factory, targetPath); !ok {
		t.Fatalf("%s missing bundled doc target %q", contextLabel, targetPath)
	}
}

func assertExportImportNestedDocOnDisk(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}
