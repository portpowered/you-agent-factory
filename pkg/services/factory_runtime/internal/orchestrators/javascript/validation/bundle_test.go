package workflowvalidation

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type bundleTestFileSystem struct{}

func (bundleTestFileSystem) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (bundleTestFileSystem) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (bundleTestFileSystem) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }

func TestBundleFactoryRelativeImportsResolvesDependencyExports(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "constants.js"),
		[]byte(`export const successResult = "ok";`),
		0o600,
	); err != nil {
		t.Fatalf("write constants module: %v", err)
	}
	entry := `import { successResult } from "./lib/constants.js";
workflow.final(successResult);`
	reader := FileSourceReader(dir, bundleTestFileSystem{})

	bundled, issues := BundleFactoryRelativeImports("workflow.js", entry, reader)
	if len(issues) > 0 {
		t.Fatalf("bundle issues = %#v, want none", issues)
	}
	if bundled == "" {
		t.Fatal("bundled source = empty, want executable JavaScript")
	}
	result := Validate(Request{
		Source:    wrapWorkflowSourceForBundleTest(bundled),
		SourceRef: "workflow.js",
	})
	if result.HasIssues() {
		t.Fatalf("validate bundled source issues = %#v", result.Issues)
	}
}

func wrapWorkflowSourceForBundleTest(source string) string {
	return "(function(){\n" + source + "\n})()"
}

func TestBundleFactoryRelativeImportsMissingModuleReturnsNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reader := FileSourceReader(dir, bundleTestFileSystem{})
	_, issues := BundleFactoryRelativeImports(
		"workflow.js",
		`import { missing } from "./lib/missing.js"; workflow.final(missing);`,
		reader,
	)
	if len(issues) == 0 {
		t.Fatal("bundle issues = nil, want missing import failure")
	}
	if issues[0].Code != CodeImportNotFound {
		t.Fatalf("issue code = %q, want %q", issues[0].Code, CodeImportNotFound)
	}
}
