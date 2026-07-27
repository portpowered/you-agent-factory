package workflowvalidation

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestContainsFactoryRelativeImports(t *testing.T) {
	t.Parallel()

	if ContainsFactoryRelativeImports(`return (async function () { workflow.final("ok"); })();`) {
		t.Fatal("ContainsFactoryRelativeImports = true, want false for workflow without imports")
	}
	if !ContainsFactoryRelativeImports(`import { ok } from "./lib/constants.js"; workflow.final(ok);`) {
		t.Fatal("ContainsFactoryRelativeImports = false, want true for factory-relative import")
	}
}

func TestLoadSkipsBundlingWithoutFactoryRelativeImports(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reader := FileSourceReader(dir, bundleTestFileSystem{})
	source := `return (async function () { workflow.final("ok"); })();`
	loaded, issues := Load(LoadRequest{
		SourceRef:    "workflow.js",
		Content:      source,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}
	if loaded.ExecutableSource != source {
		t.Fatalf("executable source = %q, want authored source without bundler preamble", loaded.ExecutableSource)
	}
}

func TestLoadBundlesFactoryRelativeImports(t *testing.T) {
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

	loaded, issues := Load(LoadRequest{
		SourceRef:    "workflow.js",
		Content:      entry,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}
	if loaded.ExecutableSource == entry {
		t.Fatal("executable source = authored entry, want bundled executable")
	}
	if !strings.Contains(loaded.ExecutableSource, "__factoryRequire") {
		t.Fatalf("executable source = %q, want bundled factory-relative import helper", loaded.ExecutableSource)
	}
}

func TestContainsFactoryRelativeImportsIgnoresNonRelativeImports(t *testing.T) {
	t.Parallel()

	if ContainsFactoryRelativeImports(`import fs from "node:fs"; workflow.final("ok");`) {
		t.Fatal("ContainsFactoryRelativeImports = true, want false for non-factory-relative import")
	}
	if ContainsFactoryRelativeImports(`import { broken`) {
		t.Fatal("ContainsFactoryRelativeImports = true, want false for unparseable source")
	}
}

func TestBundleFactoryRelativeImportsNamedExportFunctionAndAliasBindings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "helpers.js"),
		[]byte(`export function helper() { return "helper"; }
export const aliasValue = "alias";`),
		0o600,
	); err != nil {
		t.Fatalf("write helpers module: %v", err)
	}
	entry := `import helper, { aliasValue as importedAlias } from "./lib/helpers.js";
workflow.final(helper() + ":" + importedAlias);`
	reader := FileSourceReader(dir, bundleTestFileSystem{})

	bundled, issues := BundleFactoryRelativeImports("workflow.js", entry, reader)
	if len(issues) > 0 {
		t.Fatalf("bundle issues = %#v, want none", issues)
	}
	result := Validate(Request{
		Source:    wrapWorkflowSourceForBundleTest(bundled),
		SourceRef: "workflow.js",
	})
	if result.HasIssues() {
		t.Fatalf("validate bundled source issues = %#v", result.Issues)
	}
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
