package workflowvalidation_test

import (
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
)

func TestLoad_JavaScriptWorkflowPreservesSourceHash(t *testing.T) {
	source := validWorkflowSource
	loaded, issues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: "review.js",
		Content:   source,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}
	if loaded.Format != workflowvalidation.FormatJavaScript {
		t.Fatalf("format = %q, want %q", loaded.Format, workflowvalidation.FormatJavaScript)
	}
	if loaded.SourceRef != "review.js" {
		t.Fatalf("source ref = %q, want review.js", loaded.SourceRef)
	}
	wantHash := workflowvalidation.SourceHash([]byte(source))
	if loaded.SourceHash != wantHash {
		t.Fatalf("source hash = %q, want %q", loaded.SourceHash, wantHash)
	}
	if loaded.ExecutableSource != source {
		t.Fatalf("executable source changed for .js workflow")
	}
}

func TestLoad_TypeScriptWorkflowStripsSupportedSyntax(t *testing.T) {
	source := `interface ReviewArgs {
  prompt: string;
}

type ReviewResult = { ok: boolean };

meta({ name: "review", version: 1 });
phase("setup");
const result: ReviewResult = { ok: true };
workflow.final(result);
`
	loaded, issues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: "review.ts",
		Content:   source,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none for supported TypeScript syntax", issues)
	}
	if loaded.Format != workflowvalidation.FormatTypeScript {
		t.Fatalf("format = %q, want %q", loaded.Format, workflowvalidation.FormatTypeScript)
	}

	result := workflowvalidation.ValidateLoaded(loaded, workflowvalidation.Request{
		SourceRef: "review.ts",
	})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want none after TypeScript stripping", result.Issues)
	}
}

func TestLoad_TypeScriptWorkflowRejectsUnsupportedImport(t *testing.T) {
	source := `import fs from "node:fs";
phase("setup");
`
	_, issues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: "unsafe.ts",
		Content:   source,
	})
	if len(issues) == 0 {
		t.Fatal("expected unsupported import loader issue")
	}
	if issues[0].Code != workflowvalidation.CodeUnsupportedLoader {
		t.Fatalf("issue code = %q, want %q", issues[0].Code, workflowvalidation.CodeUnsupportedLoader)
	}
	if issues[0].Line != 1 {
		t.Fatalf("issue line = %d, want authored line 1", issues[0].Line)
	}
}

func TestLoad_SourceHashStableForUnchangedSource(t *testing.T) {
	source := validWorkflowSource
	first, _ := workflowvalidation.Load(workflowvalidation.LoadRequest{SourceRef: "review.js", Content: source})
	second, _ := workflowvalidation.Load(workflowvalidation.LoadRequest{SourceRef: "review.js", Content: source})
	if first.SourceHash != second.SourceHash {
		t.Fatalf("source hash changed for unchanged source: %q vs %q", first.SourceHash, second.SourceHash)
	}
}

func TestLoad_TypeScriptDiagnosticsRemapToAuthoredLines(t *testing.T) {
	source := `interface Ignored {
  prompt: string;
}
phase("setup";
`
	loaded, issues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: "broken.ts",
		Content:   source,
	})
	if len(issues) > 0 {
		t.Fatalf("load issues = %#v, want none", issues)
	}

	result := workflowvalidation.ValidateLoaded(loaded, workflowvalidation.Request{SourceRef: "broken.ts"})
	if !result.HasIssues() {
		t.Fatal("expected syntax validation issue")
	}
	if result.Issues[0].Line != 4 {
		t.Fatalf("issue line = %d, want authored line 4", result.Issues[0].Line)
	}
}

func TestWorkflowSourceTargets_LoadsTypeScriptWorkflowFile(t *testing.T) {
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

	reader := workflowvalidation.FileSourceReader(dir)
	targets := factoryvalidation.WorkflowSourceTargets(&interfaces.FactoryOrchestratorJavaScriptConfig{
		SourceRef: "review.ts",
	}, reader)
	if len(targets) > 0 {
		t.Fatalf("workflow source targets = %#v, want none for supported .ts workflow", targets)
	}
}

func TestWorkflowSourceTargets_RejectsTypeScriptImportBeforeRuntime(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "unsafe.ts")
	if err := os.WriteFile(sourcePath, []byte(`import fs from "node:fs";`), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	reader := workflowvalidation.FileSourceReader(dir)
	targets := factoryvalidation.WorkflowSourceTargets(&interfaces.FactoryOrchestratorJavaScriptConfig{
		SourceRef: "unsafe.ts",
	}, reader)
	if len(targets) == 0 {
		t.Fatal("expected unsupported import validation target")
	}
	if targets[0].Code != workflowvalidation.CodeUnsupportedLoader {
		t.Fatalf("target code = %q, want %q", targets[0].Code, workflowvalidation.CodeUnsupportedLoader)
	}
}

func TestWorkflowSourceTargets_RejectsSourceHashMismatch(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "review.js")
	if err := os.WriteFile(sourcePath, []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	reader := workflowvalidation.FileSourceReader(dir)
	targets := factoryvalidation.WorkflowSourceTargets(&interfaces.FactoryOrchestratorJavaScriptConfig{
		SourceRef:  "review.js",
		SourceHash: "sha256:deadbeef",
	}, reader)
	if len(targets) == 0 {
		t.Fatal("expected source hash mismatch validation target")
	}
	if targets[0].Code != workflowvalidation.CodeSourceHashMismatch {
		t.Fatalf("target code = %q, want %q", targets[0].Code, workflowvalidation.CodeSourceHashMismatch)
	}
}
