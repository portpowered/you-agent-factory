package bootstrap_portability

import (
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	exportImportPortableNestedDocPath = "factory/docs/standards/review.md"
	exportImportPortableNestedDocBody = "# Review standards\n"
)

func TestExportImportSmoke_PreservesNestedFactoryDocsThroughExportImport(t *testing.T) {
	t.Parallel()
	fixture := newExportImportFixture(t)
	harness := newExportImportSmokeHarness(fixture, withExportImportBeforeExport(seedNestedFactoryDocOnDisk))

	result := harness.Run(t)

	assertNestedFactoryDocInBundledFiles(
		t,
		result.ExportedFactory,
		exportImportPortableNestedDocPath,
		exportImportPortableNestedDocBody,
		"exported factory",
	)
	assertNestedFactoryDocTargetPresent(
		t,
		result.ImportedFactory,
		exportImportPortableNestedDocPath,
		"imported factory",
	)
	assertNestedFactoryDocTargetPresent(
		t,
		result.CurrentFactory,
		exportImportPortableNestedDocPath,
		"current factory after import",
	)
	assertImportedPortableFile(
		t,
		filepath.Join(result.ImportedDir, "docs", "standards", "review.md"),
		exportImportPortableNestedDocBody,
	)

	nestedDocPath := filepath.Join(result.SourceFactoryDir, "docs", "standards", "review.md")
	assertImportedPortableFile(t, nestedDocPath, exportImportPortableNestedDocBody)
}

func seedNestedFactoryDocOnDisk(t *testing.T, factoryDir string) {
	t.Helper()

	nestedDocPath := filepath.Join(factoryDir, "docs", "standards", "review.md")
	if err := os.MkdirAll(filepath.Dir(nestedDocPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(nestedDocPath), err)
	}
	if err := os.WriteFile(nestedDocPath, []byte(exportImportPortableNestedDocBody), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", nestedDocPath, err)
	}
}

func assertNestedFactoryDocInBundledFiles(
	t *testing.T,
	factory factoryapi.Factory,
	targetPath string,
	wantInline string,
	contextLabel string,
) {
	t.Helper()

	bundledFile, ok := findBundledFileByTargetPath(factory, targetPath)
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

func assertNestedFactoryDocTargetPresent(
	t *testing.T,
	factory factoryapi.Factory,
	targetPath string,
	contextLabel string,
) {
	t.Helper()

	if _, ok := findBundledFileByTargetPath(factory, targetPath); !ok {
		t.Fatalf("%s missing bundled doc target %q", contextLabel, targetPath)
	}
}

func findBundledFileByTargetPath(
	factory factoryapi.Factory,
	targetPath string,
) (factoryapi.BundledFile, bool) {
	if factory.SupportingFiles == nil || factory.SupportingFiles.BundledFiles == nil {
		return factoryapi.BundledFile{}, false
	}
	for _, bundledFile := range *factory.SupportingFiles.BundledFiles {
		if bundledFile.TargetPath == targetPath {
			return bundledFile, true
		}
	}
	return factoryapi.BundledFile{}, false
}
