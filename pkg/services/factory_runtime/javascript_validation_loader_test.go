package factory_test

import (
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	factoryruntimetestkit "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/testkit"
)

var validationLoaderWorkflows = factoryruntimetestkit.JavaScriptWorkflows()

func TestLoad_JavaScriptWorkflowPreservesSourceHash(t *testing.T) {
	t.Parallel()
	source := validWorkflowSource
	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef: "review.js",
		Content:   source,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}
	if loaded.Format != factory.WorkflowValidationFormatJavaScript {
		t.Fatalf("format = %q, want %q", loaded.Format, factory.WorkflowValidationFormatJavaScript)
	}
	if loaded.SourceRef != "review.js" {
		t.Fatalf("source ref = %q, want review.js", loaded.SourceRef)
	}
	wantHash := factoryruntimetestkit.HashWorkflowSource([]byte(source))
	if loaded.SourceHash != wantHash {
		t.Fatalf("source hash = %q, want %q", loaded.SourceHash, wantHash)
	}
	if loaded.ExecutableSource != source {
		t.Fatalf("executable source changed for .js workflow")
	}
}

func TestLoad_TypeScriptWorkflowStripsSupportedSyntax(t *testing.T) {
	t.Parallel()
	source := `interface ReviewArgs {
  prompt: string;
}

type ReviewResult = { ok: boolean };

meta({ name: "review", version: 1 });
phase("setup");
const result: ReviewResult = { ok: true };
workflow.final(result);
`
	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef: "review.ts",
		Content:   source,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none for supported TypeScript syntax", issues)
	}
	if loaded.Format != factory.WorkflowValidationFormatTypeScript {
		t.Fatalf("format = %q, want %q", loaded.Format, factory.WorkflowValidationFormatTypeScript)
	}

	result := validationLoaderWorkflows.ValidateLoaded(loaded, factory.WorkflowValidationRequest{
		SourceRef: "review.ts",
	})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want none after TypeScript stripping", result.Issues)
	}
}

func TestLoad_TypeScriptWorkflowRejectsUnsupportedImport(t *testing.T) {
	t.Parallel()
	source := `import fs from "node:fs";
phase("setup");
`
	_, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef: "unsafe.ts",
		Content:   source,
	})
	if len(issues) == 0 {
		t.Fatal("expected unsupported import loader issue")
	}
	if issues[0].Code != factory.WorkflowValidationCodeUnsupportedLoader {
		t.Fatalf("issue code = %q, want %q", issues[0].Code, factory.WorkflowValidationCodeUnsupportedLoader)
	}
	if issues[0].Line != 1 {
		t.Fatalf("issue line = %d, want authored line 1", issues[0].Line)
	}
}

func TestLoad_SourceHashStableForUnchangedSource(t *testing.T) {
	t.Parallel()
	source := validWorkflowSource
	first, _ := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{SourceRef: "review.js", Content: source})
	second, _ := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{SourceRef: "review.js", Content: source})
	if first.SourceHash != second.SourceHash {
		t.Fatalf("source hash changed for unchanged source: %q vs %q", first.SourceHash, second.SourceHash)
	}
}

func TestLoad_TypeScriptDiagnosticsRemapToAuthoredLines(t *testing.T) {
	t.Parallel()
	source := `interface Ignored {
  prompt: string;
}
phase("setup";
`
	loaded, issues := validationLoaderWorkflows.LoadSource(factory.WorkflowValidationLoadRequest{
		SourceRef: "broken.ts",
		Content:   source,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}

	result := validationLoaderWorkflows.ValidateLoaded(loaded, factory.WorkflowValidationRequest{SourceRef: "broken.ts"})
	if !result.HasIssues() {
		t.Fatal("expected syntax validation issue")
	}
	if result.Issues[0].Line != 4 {
		t.Fatalf("issue line = %d, want authored line 4", result.Issues[0].Line)
	}
}

func TestWorkflowSourceTargets_LoadsTypeScriptWorkflowFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "review.ts")
	source := `interface ReviewArgs { prompt: string; }
meta({ name: "review", version: 1 });
phase("setup");
workflow.final({ ok: true });
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	reader := factoryruntimetestkit.NewFileWorkflowSourceReader(dir, localWorkflowSourceFiles{})
	targets := factoryruntimewire.NewOrchestratorDefinitionValidator(testJavaScriptWorkflows()).ValidateJavaScriptFactoryDefinition(
		t.Context(),
		&interfaces.FactoryOrchestratorJavaScriptConfig{SourceRef: "review.ts"},
		reader,
	)
	if len(targets) > 0 {
		t.Fatalf("workflow source targets = %#v, want none for supported .ts workflow", targets)
	}
}

func TestWorkflowSourceTargets_RejectsTypeScriptImportBeforeRuntime(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "unsafe.ts")
	if err := os.WriteFile(sourcePath, []byte(`import fs from "node:fs";`), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	reader := factoryruntimetestkit.NewFileWorkflowSourceReader(dir, localWorkflowSourceFiles{})
	targets := factoryruntimewire.NewOrchestratorDefinitionValidator(testJavaScriptWorkflows()).ValidateJavaScriptFactoryDefinition(
		t.Context(),
		&interfaces.FactoryOrchestratorJavaScriptConfig{SourceRef: "unsafe.ts"},
		reader,
	)
	if len(targets) == 0 {
		t.Fatal("expected unsupported import validation target")
	}
	if targets[0].Code != factory.WorkflowValidationCodeUnsupportedLoader {
		t.Fatalf("target code = %q, want %q", targets[0].Code, factory.WorkflowValidationCodeUnsupportedLoader)
	}
}

func TestWorkflowSourceTargets_RejectsSourceHashMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "review.js")
	if err := os.WriteFile(sourcePath, []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	reader := factoryruntimetestkit.NewFileWorkflowSourceReader(dir, localWorkflowSourceFiles{})
	targets := factoryruntimewire.NewOrchestratorDefinitionValidator(testJavaScriptWorkflows()).ValidateJavaScriptFactoryDefinition(
		t.Context(),
		&interfaces.FactoryOrchestratorJavaScriptConfig{
			SourceRef:  "review.js",
			SourceHash: "sha256:deadbeef",
		},
		reader,
	)
	if len(targets) == 0 {
		t.Fatal("expected source hash mismatch validation target")
	}
	if targets[0].Code != factory.WorkflowValidationCodeSourceHashMismatch {
		t.Fatalf("target code = %q, want %q", targets[0].Code, factory.WorkflowValidationCodeSourceHashMismatch)
	}
}
