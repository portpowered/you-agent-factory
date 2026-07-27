package workflowvalidation

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dop251/goja"
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
	finalValue, err := executeBundledWorkflowFinalForTest(t, bundled)
	if err != nil {
		t.Fatalf("execute bundled source: %v", err)
	}
	if finalValue != "ok" {
		t.Fatalf("workflow.final value = %q, want ok", finalValue)
	}
}

func wrapWorkflowSourceForBundleTest(source string) string {
	return "(function(){\n" + source + "\n})()"
}

func executeBundledWorkflowFinalForTest(t *testing.T, bundled string) (string, error) {
	t.Helper()

	vm := goja.New()
	var captured string
	workflow := vm.NewObject()
	if err := workflow.Set("final", func(call goja.FunctionCall) goja.Value {
		captured = call.Argument(0).String()
		return goja.Undefined()
	}); err != nil {
		t.Fatalf("set workflow.final: %v", err)
	}
	if err := vm.Set("workflow", workflow); err != nil {
		t.Fatalf("set workflow: %v", err)
	}
	_, err := vm.RunString(wrapWorkflowSourceForBundleTest(bundled))
	return captured, err
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

func TestBundleFactoryRelativeImportsNamespaceImportExecutes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "constants.js"),
		[]byte(`export const successResult = "namespace-ok";`),
		0o600,
	); err != nil {
		t.Fatalf("write constants module: %v", err)
	}
	entry := `import * as constants from "./lib/constants.js";
workflow.final(constants.successResult);`
	reader := FileSourceReader(dir, bundleTestFileSystem{})

	bundled, issues := BundleFactoryRelativeImports("workflow.js", entry, reader)
	if len(issues) > 0 {
		t.Fatalf("bundle issues = %#v, want none", issues)
	}
	finalValue, err := executeBundledWorkflowFinalForTest(t, bundled)
	if err != nil {
		t.Fatalf("execute bundled source: %v", err)
	}
	if finalValue != "namespace-ok" {
		t.Fatalf("workflow.final value = %q, want namespace-ok", finalValue)
	}
}

func TestBundleFactoryRelativeImportsSideEffectImportExecutes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "constants.js"),
		[]byte(`export const successResult = "side-effect-ok";`),
		0o600,
	); err != nil {
		t.Fatalf("write constants module: %v", err)
	}
	entry := `import "./lib/constants.js";
import { successResult } from "./lib/constants.js";
workflow.final(successResult);`
	reader := FileSourceReader(dir, bundleTestFileSystem{})

	bundled, issues := BundleFactoryRelativeImports("workflow.js", entry, reader)
	if len(issues) > 0 {
		t.Fatalf("bundle issues = %#v, want none", issues)
	}
	if !strings.Contains(bundled, "__factoryRequire(\"lib/constants.js\");") {
		t.Fatalf("bundled source = %q, want side-effect import invocation", bundled)
	}
	finalValue, err := executeBundledWorkflowFinalForTest(t, bundled)
	if err != nil {
		t.Fatalf("execute bundled source: %v", err)
	}
	if finalValue != "side-effect-ok" {
		t.Fatalf("workflow.final value = %q, want side-effect-ok", finalValue)
	}
}

func TestBundleFactoryRelativeImportsNamedAliasBindingExecutes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "helpers.js"),
		[]byte(`export const aliasValue = "alias-only";`),
		0o600,
	); err != nil {
		t.Fatalf("write helpers module: %v", err)
	}
	entry := `import { aliasValue as importedAlias } from "./lib/helpers.js";
workflow.final(importedAlias);`
	reader := FileSourceReader(dir, bundleTestFileSystem{})

	bundled, issues := BundleFactoryRelativeImports("workflow.js", entry, reader)
	if len(issues) > 0 {
		t.Fatalf("bundle issues = %#v, want none", issues)
	}
	if !strings.Contains(bundled, "aliasValue: importedAlias") {
		t.Fatalf("bundled source = %q, want named alias binding", bundled)
	}
	finalValue, err := executeBundledWorkflowFinalForTest(t, bundled)
	if err != nil {
		t.Fatalf("execute bundled source: %v", err)
	}
	if finalValue != "alias-only" {
		t.Fatalf("workflow.final value = %q, want alias-only", finalValue)
	}
}

func TestBundleFactoryRelativeImportsDefaultAndNamedBindingsExecute(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "helpers.js"),
		[]byte(`export default function helper() { return "helper"; }
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
	if !strings.Contains(bundled, "__factoryRequire(\"lib/helpers.js\").default") {
		t.Fatalf("bundled source = %q, want default import bound from module.default", bundled)
	}
	if !strings.Contains(bundled, "aliasValue: importedAlias") {
		t.Fatalf("bundled source = %q, want named alias binding preserved alongside default import", bundled)
	}
	finalValue, err := executeBundledWorkflowFinalForTest(t, bundled)
	if err != nil {
		t.Fatalf("execute bundled source: %v", err)
	}
	if finalValue != "helper:alias" {
		t.Fatalf("workflow.final value = %q, want helper:alias", finalValue)
	}
}

func TestBundleFactoryRelativeImportsDefaultExportOnlyExecutes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "default-only.js"),
		[]byte(`export default "default-export";`),
		0o600,
	); err != nil {
		t.Fatalf("write default-only module: %v", err)
	}
	entry := `import importedDefault from "./lib/default-only.js";
workflow.final(importedDefault);`
	reader := FileSourceReader(dir, bundleTestFileSystem{})

	bundled, issues := BundleFactoryRelativeImports("workflow.js", entry, reader)
	if len(issues) > 0 {
		t.Fatalf("bundle issues = %#v, want none", issues)
	}
	finalValue, err := executeBundledWorkflowFinalForTest(t, bundled)
	if err != nil {
		t.Fatalf("execute bundled source: %v", err)
	}
	if finalValue != "default-export" {
		t.Fatalf("workflow.final value = %q, want default-export", finalValue)
	}
}

func TestLoadBundlesNestedRelativeImportChainExecutes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "leaf.js"),
		[]byte(`export const leaf = "LEAF";`),
		0o600,
	); err != nil {
		t.Fatalf("write leaf module: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "mid.js"),
		[]byte(`import { leaf } from "./leaf.js";
export const mid = leaf + "-MID";`),
		0o600,
	); err != nil {
		t.Fatalf("write mid module: %v", err)
	}
	entry := `import { mid } from "./lib/mid.js";
workflow.final(mid);`
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
	if !strings.Contains(loaded.ExecutableSource, "const {leaf} = __factoryRequire(\"lib/leaf.js\");") {
		t.Fatalf("executable source = %q, want nested import bindings inside mid module wrapper", loaded.ExecutableSource)
	}
	finalValue, err := executeBundledWorkflowFinalForTest(t, loaded.ExecutableSource)
	if err != nil {
		t.Fatalf("execute bundled source: %v", err)
	}
	if finalValue != "LEAF-MID" {
		t.Fatalf("workflow.final value = %q, want LEAF-MID", finalValue)
	}
}

func TestBundleFactoryRelativeImportsNestedImportChainExecutes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "leaf.js"),
		[]byte(`export const leaf = "LEAF";`),
		0o600,
	); err != nil {
		t.Fatalf("write leaf module: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "mid.js"),
		[]byte(`import { leaf } from "./leaf.js";
export const mid = leaf + "-MID";`),
		0o600,
	); err != nil {
		t.Fatalf("write mid module: %v", err)
	}
	entry := `import { mid } from "./lib/mid.js";
workflow.final(mid);`
	reader := FileSourceReader(dir, bundleTestFileSystem{})

	bundled, issues := BundleFactoryRelativeImports("workflow.js", entry, reader)
	if len(issues) > 0 {
		t.Fatalf("bundle issues = %#v, want none", issues)
	}
	if !strings.Contains(bundled, "const {leaf} = __factoryRequire(\"lib/leaf.js\");") {
		t.Fatalf("bundled source = %q, want nested import bindings inside mid module wrapper", bundled)
	}
	finalValue, err := executeBundledWorkflowFinalForTest(t, bundled)
	if err != nil {
		t.Fatalf("execute bundled source: %v", err)
	}
	if finalValue != "LEAF-MID" {
		t.Fatalf("workflow.final value = %q, want LEAF-MID", finalValue)
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
