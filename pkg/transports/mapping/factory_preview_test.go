// Apisurface preview tests exercise orchestrator-owned JavaScript preview and
// source packages directly; active behavior must not depend on root pkg/workflow*
// compatibility shims.
package apisurface_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestBuildFactoryPreview_MatchesOrchestratorPreviewSeam(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}

	viaSurface := apisurface.BuildFactoryPreview(req)
	direct := workflowpreview.BuildPreview(req)
	if viaSurface.Valid != direct.Valid ||
		viaSurface.SourceResolution.SourceHash != direct.SourceResolution.SourceHash ||
		viaSurface.PolicyPreview.PolicyHash != direct.PolicyPreview.PolicyHash {
		t.Fatalf("surface = %#v, orchestrator = %#v, want equivalent preview", viaSurface, direct)
	}
}

func TestFactoryPreviewResultFromPreview_PreservesPathAwareDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "unsafe.js", "require('fs');")

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	preview := apisurface.BuildFactoryPreview(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "unsafe",
		},
		Context: ctx,
	})
	if preview.Valid {
		t.Fatal("expected invalid preview")
	}

	result := apisurface.FactoryPreviewResultFromPreview(preview)
	if len(result.SourceValidationIssues) == 0 {
		t.Fatalf("issues = %#v, want source validation diagnostics", result.SourceValidationIssues)
	}
	wantPath := workflowsource.ProjectClaudeWorkflowsDir + "/unsafe.js"
	if result.SourceValidationIssues[0].Path == nil || strings.TrimSpace(*result.SourceValidationIssues[0].Path) != wantPath {
		t.Fatalf("issue path = %v, want %q", result.SourceValidationIssues[0].Path, wantPath)
	}
}

func TestFactoryPreviewRequestFromAPI_MapsCanonicalPreviewInput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	projectRootPtr := projectRoot
	sourceValue := "review"
	mapped, err := apisurface.FactoryPreviewRequestFromAPI(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	})
	if err != nil {
		t.Fatalf("FactoryPreviewRequestFromAPI: %v", err)
	}
	if mapped.Source.Kind != workflowsource.KindWorkflowName || mapped.Source.Value != "review" {
		t.Fatalf("mapped source = %#v, want workflow name review", mapped.Source)
	}
	if strings.TrimSpace(mapped.Context.ProjectRoot) != projectRoot {
		t.Fatalf("project root = %q, want %q", mapped.Context.ProjectRoot, projectRoot)
	}
}

func writeWorkflow(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}

func TestBuildFactoryWorkflowValidation_MatchesOrchestratorPreviewSeam(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}

	viaSurface := apisurface.BuildFactoryWorkflowValidation(req)
	direct := workflowpreview.BuildPreview(req)
	if viaSurface.Valid != direct.Valid ||
		viaSurface.SourceResolution.SourceHash != direct.SourceResolution.SourceHash {
		t.Fatalf("surface = %#v, orchestrator = %#v, want equivalent validation preview", viaSurface, direct)
	}
}

func TestFactoryWorkflowValidationResultFromPreview_ValidWorkflowHasEmptyBlockingDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	preview := apisurface.BuildFactoryWorkflowValidation(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	})
	if !preview.Valid {
		t.Fatal("expected valid workflow validation preview")
	}

	result := apisurface.FactoryWorkflowValidationResultFromPreview(preview)
	if !result.Valid {
		t.Fatalf("valid = false, want true")
	}
	if len(result.BlockingDiagnostics) != 0 {
		t.Fatalf("blocking diagnostics = %#v, want empty", result.BlockingDiagnostics)
	}
	if result.SourceResolution.SourceHash == nil || strings.TrimSpace(*result.SourceResolution.SourceHash) == "" {
		t.Fatalf("source hash = %v, want non-empty hash", result.SourceResolution.SourceHash)
	}
	wantRef := workflowsource.ProjectClaudeWorkflowsDir + "/review.js"
	if result.SourceResolution.SourceRef == nil || strings.TrimSpace(*result.SourceResolution.SourceRef) != wantRef {
		t.Fatalf("source ref = %v, want %q", result.SourceResolution.SourceRef, wantRef)
	}
}

func TestFactoryWorkflowValidationResultFromPreview_BlockingDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	cases := []struct {
		name     string
		request  workflowpreview.Request
		wantCode string
		wantLine bool
	}{
		{
			name: "syntax error",
			request: workflowpreview.Request{
				Source: workflowsource.Request{
					Kind:  workflowsource.KindWorkflowName,
					Value: "broken",
				},
				Context: ctx,
			},
			wantCode: workflowvalidation.CodeSyntaxError,
			wantLine: true,
		},
		{
			name: "unsupported global",
			request: workflowpreview.Request{
				Source: workflowsource.Request{
					Kind:  workflowsource.KindWorkflowName,
					Value: "unsafe-global",
				},
				Context: ctx,
			},
			wantCode: workflowvalidation.CodeUnsupportedGlobal,
		},
		{
			name: "forbidden host access",
			request: workflowpreview.Request{
				Source: workflowsource.Request{
					Kind:  workflowsource.KindWorkflowName,
					Value: "unsafe-host",
				},
				Context: ctx,
			},
			wantCode: workflowvalidation.CodeForbiddenHostAccess,
		},
		{
			name: "invalid args schema",
			request: workflowpreview.Request{
				Source: workflowsource.Request{
					Kind:  workflowsource.KindWorkflowName,
					Value: "review",
				},
				Context:    ctx,
				ArgsSchema: []byte(`{"type":"array"}`),
			},
			wantCode: workflowvalidation.CodeInvalidArgsSchema,
		},
		{
			name: "policy denied host access",
			request: workflowpreview.Request{
				Source: workflowsource.Request{
					Kind:  workflowsource.KindWorkflowName,
					Value: "review",
				},
				Context:         ctx,
				RequestedPolicy: map[string]any{"allowNetwork": true},
			},
			wantCode: workflowpolicy.CodeDeniedCapability,
		},
	}

	writeWorkflow(t, projectRoot, "broken.js", `meta({ name: "broken" );`)
	writeWorkflow(t, projectRoot, "unsafe-global.js", `console.log("unsupported global");`)
	writeWorkflow(t, projectRoot, "unsafe-host.js", `require('fs');`)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			preview := apisurface.BuildFactoryWorkflowValidation(tc.request)
			result := apisurface.FactoryWorkflowValidationResultFromPreview(preview)
			if result.Valid {
				t.Fatal("expected invalid validation result")
			}
			diagnostic := findValidationDiagnostic(result.BlockingDiagnostics, tc.wantCode)
			if diagnostic == nil {
				t.Fatalf("blocking diagnostics = %#v, want code %q", result.BlockingDiagnostics, tc.wantCode)
			}
			if strings.TrimSpace(diagnostic.Message) == "" {
				t.Fatalf("diagnostic = %#v, want message", diagnostic)
			}
			if tc.wantLine && (diagnostic.Line == nil || *diagnostic.Line <= 0) {
				t.Fatalf("diagnostic = %#v, want source line", diagnostic)
			}
		})
	}
}

func findValidationDiagnostic(diagnostics []factoryapi.WorkflowDiagnostic, code string) *factoryapi.WorkflowDiagnostic {
	for i := range diagnostics {
		if diagnostics[i].Code == code {
			return &diagnostics[i]
		}
	}
	return nil
}
