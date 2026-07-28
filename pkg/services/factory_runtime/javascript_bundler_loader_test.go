package factory_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

func TestLoad_BundlesDefaultAndNamedRelativeImportsExecutes(t *testing.T) {
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
	reader := factory.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})

	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef:    "workflow.js",
		Content:      entry,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}
	if !strings.Contains(loaded.ExecutableSource, "__factoryRequire(\"lib/helpers.js\").default") {
		t.Fatalf("executable source = %q, want default import bound from module.default", loaded.ExecutableSource)
	}
	if !strings.Contains(loaded.ExecutableSource, "aliasValue: importedAlias") {
		t.Fatalf("executable source = %q, want named alias binding preserved alongside default import", loaded.ExecutableSource)
	}

	outcome, err := validationLoaderWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source:    loaded.ExecutableSource,
		SourceRef: "workflow.js",
		SessionID: "session-default-named-import",
		Args:      json.RawMessage(`{}`),
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	if string(outcome.Value.JSON) != `"helper:alias"` {
		t.Fatalf("primary result = %s, want \"helper:alias\"", outcome.Value.JSON)
	}
}

func TestLoad_BundlesNamespaceRelativeImportExecutes(t *testing.T) {
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
	reader := factory.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})

	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef:    "workflow.js",
		Content:      entry,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}

	outcome, err := validationLoaderWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source:    loaded.ExecutableSource,
		SourceRef: "workflow.js",
		SessionID: "session-namespace-import",
		Args:      json.RawMessage(`{}`),
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	if string(outcome.Value.JSON) != `"namespace-ok"` {
		t.Fatalf("primary result = %s, want \"namespace-ok\"", outcome.Value.JSON)
	}
}

func TestLoad_BundlesNestedRelativeImportChainExecutes(t *testing.T) {
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
	reader := factory.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})

	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
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

	outcome, err := validationLoaderWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source:    loaded.ExecutableSource,
		SourceRef: "workflow.js",
		SessionID: "session-nested-import-chain",
		Args:      json.RawMessage(`{}`),
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	if string(outcome.Value.JSON) != `"LEAF-MID"` {
		t.Fatalf("primary result = %s, want \"LEAF-MID\"", outcome.Value.JSON)
	}
}

func TestLoad_BundlesSameModuleExportReferencesExecute(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "helpers.js"),
		[]byte(`export const leaf = "LEAF";
export const doubled = leaf + "-X";
export function suffix(value) { return value + "-FN"; }`),
		0o600,
	); err != nil {
		t.Fatalf("write helpers module: %v", err)
	}
	entry := `import { doubled, suffix } from "./lib/helpers.js";
workflow.final(suffix(doubled));`
	reader := factory.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})

	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef:    "workflow.js",
		Content:      entry,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}

	outcome, err := validationLoaderWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source:    loaded.ExecutableSource,
		SourceRef: "workflow.js",
		SessionID: "session-same-module-export-refs",
		Args:      json.RawMessage(`{}`),
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	if string(outcome.Value.JSON) != `"LEAF-X-FN"` {
		t.Fatalf("primary result = %s, want \"LEAF-X-FN\"", outcome.Value.JSON)
	}
}

func TestLoad_BundlesDestructuredExportBindingsExecute(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "pairs.js"),
		[]byte(`export const { a, b } = { a: "A", b: "B" };`),
		0o600,
	); err != nil {
		t.Fatalf("write pairs module: %v", err)
	}
	entry := `import { a, b } from "./lib/pairs.js";
workflow.final(a + b);`
	reader := factory.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})

	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef:    "workflow.js",
		Content:      entry,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}

	outcome, err := validationLoaderWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source:    loaded.ExecutableSource,
		SourceRef: "workflow.js",
		SessionID: "session-destructured-export",
		Args:      json.RawMessage(`{}`),
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	if string(outcome.Value.JSON) != `"AB"` {
		t.Fatalf("primary result = %s, want \"AB\"", outcome.Value.JSON)
	}
}

func TestLoad_BundlesMutableLetExportSnapshotsFinalValue(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "mutable.js"),
		[]byte(`export let value = "initial";
value = "updated";`),
		0o600,
	); err != nil {
		t.Fatalf("write mutable module: %v", err)
	}
	entry := `import { value } from "./lib/mutable.js";
workflow.final(value);`
	reader := factory.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})

	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef:    "workflow.js",
		Content:      entry,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}

	outcome, err := validationLoaderWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source:    loaded.ExecutableSource,
		SourceRef: "workflow.js",
		SessionID: "session-mutable-let-export",
		Args:      json.RawMessage(`{}`),
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	if string(outcome.Value.JSON) != `"updated"` {
		t.Fatalf("primary result = %s, want \"updated\"", outcome.Value.JSON)
	}
}

func TestLoad_BundlesDefaultExportRelativeImportExecutes(t *testing.T) {
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
	reader := factory.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})

	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef:    "workflow.js",
		Content:      entry,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}

	outcome, err := validationLoaderWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source:    loaded.ExecutableSource,
		SourceRef: "workflow.js",
		SessionID: "session-default-import",
		Args:      json.RawMessage(`{}`),
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	if string(outcome.Value.JSON) != `"default-export"` {
		t.Fatalf("primary result = %s, want \"default-export\"", outcome.Value.JSON)
	}
}

func TestLoad_BundlesNamedDefaultFunctionLocalBindingExecutes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "default-fn.js"),
		[]byte(`export default function helper() { return "OK"; }
export const tag = typeof helper;`),
		0o600,
	); err != nil {
		t.Fatalf("write default-fn module: %v", err)
	}
	entry := `import helper, { tag } from "./lib/default-fn.js";
workflow.final(helper() + ":" + tag);`
	reader := factory.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})

	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef:    "workflow.js",
		Content:      entry,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}
	if strings.Contains(loaded.ExecutableSource, "exports.default = function helper") {
		t.Fatalf("executable source = %q, want local function declaration before exports.default assignment", loaded.ExecutableSource)
	}

	outcome, err := validationLoaderWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source:    loaded.ExecutableSource,
		SourceRef: "workflow.js",
		SessionID: "session-named-default-function",
		Args:      json.RawMessage(`{}`),
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	if string(outcome.Value.JSON) != `"OK:function"` {
		t.Fatalf("primary result = %s, want \"OK:function\"", outcome.Value.JSON)
	}
}

func TestLoad_BundlesCircularRelativeImportReturnsUnsupportedLoader(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o700); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "a.js"),
		[]byte(`import { b } from "./b.js";
export const a = b;`),
		0o600,
	); err != nil {
		t.Fatalf("write a module: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "lib", "b.js"),
		[]byte(`import { a } from "./a.js";
export const b = a;`),
		0o600,
	); err != nil {
		t.Fatalf("write b module: %v", err)
	}
	entry := `import { a } from "./lib/a.js";
workflow.final(a);`
	reader := factory.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})

	_, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef:    "workflow.js",
		Content:      entry,
		FactoryRoot:  dir,
		BundleReader: reader,
	})
	if len(issues) == 0 {
		t.Fatal("load issues = nil, want circular import failure")
	}
	if issues[0].Code != "workflow.source.unsupportedLoader" {
		t.Fatalf("issue code = %q, want workflow.source.unsupportedLoader", issues[0].Code)
	}
	if !strings.Contains(issues[0].Message, "circular factory-relative import") {
		t.Fatalf("issue message = %q, want circular import diagnostic", issues[0].Message)
	}
}
