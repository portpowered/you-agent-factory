package factory_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/orchestratorcontract"
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
