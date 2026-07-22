// Apisurface preview tests exercise orchestrator-owned JavaScript preview and
// source packages directly; active behavior must not depend on root pkg/workflow*
// compatibility shims.
package apisurface_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

var mappingWorkflows = scriptedMappingWorkflows()

func derefTestString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func TestFactoryPreviewResultFromPreview_MapsServiceRootPreview(t *testing.T) {
	projectRoot := t.TempDir()

	ctx, err := mappingWorkflows.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:  factory.WorkflowSourceKindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}

	preview := mappingWorkflows.BuildPreview(req)
	result := apisurface.FactoryPreviewResultFromPreview(preview)
	if result.Valid != preview.Valid ||
		derefTestString(result.SourceResolution.SourceHash) != preview.SourceResolution.SourceHash ||
		result.PolicyPreview.PolicyHash != preview.PolicyPreview.PolicyHash {
		t.Fatalf("result = %#v, preview = %#v, want mapped service-root preview", result, preview)
	}
}

func TestFactoryPreviewResultFromPreview_PreservesPathAwareDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()

	ctx, err := mappingWorkflows.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	preview := mappingWorkflows.BuildPreview(factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:  factory.WorkflowSourceKindWorkflowName,
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
	wantPath := factory.WorkflowSourceProjectClaudeWorkflowsDir + "/unsafe.js"
	if result.SourceValidationIssues[0].Path == nil || strings.TrimSpace(*result.SourceValidationIssues[0].Path) != wantPath {
		t.Fatalf("issue path = %v, want %q", result.SourceValidationIssues[0].Path, wantPath)
	}
}

func TestFactoryPreviewInputFromAPI_ForwardsCanonicalEdgeFields(t *testing.T) {
	projectRoot := t.TempDir()

	projectRootPtr := projectRoot
	sourceValue := "review"
	mapped, err := apisurface.FactoryPreviewInputFromAPI(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	})
	if err != nil {
		t.Fatalf("FactoryPreviewInputFromAPI: %v", err)
	}
	if mapped.Source.Kind != factory.WorkflowSourceKindWorkflowName || mapped.Source.Value != "review" {
		t.Fatalf("mapped source = %#v, want workflow name review", mapped.Source)
	}
	if strings.TrimSpace(mapped.ProjectRoot) != projectRoot {
		t.Fatalf("project root = %q, want %q", mapped.ProjectRoot, projectRoot)
	}
}

func TestFactoryWorkflowValidationResultFromPreview_MapsServiceRootPreview(t *testing.T) {
	projectRoot := t.TempDir()

	ctx, err := mappingWorkflows.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:  factory.WorkflowSourceKindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}

	preview := mappingWorkflows.BuildPreview(req)
	result := apisurface.FactoryWorkflowValidationResultFromPreview(preview)
	if result.Valid != preview.Valid ||
		derefTestString(result.SourceResolution.SourceHash) != preview.SourceResolution.SourceHash {
		t.Fatalf("result = %#v, preview = %#v, want mapped service-root validation preview", result, preview)
	}
}

func TestFactoryWorkflowValidationResultFromPreview_ValidWorkflowHasEmptyBlockingDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()

	ctx, err := mappingWorkflows.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	preview := mappingWorkflows.BuildPreview(factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:  factory.WorkflowSourceKindWorkflowName,
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
	wantRef := factory.WorkflowSourceProjectClaudeWorkflowsDir + "/review.js"
	if result.SourceResolution.SourceRef == nil || strings.TrimSpace(*result.SourceResolution.SourceRef) != wantRef {
		t.Fatalf("source ref = %v, want %q", result.SourceResolution.SourceRef, wantRef)
	}
}

func TestFactoryWorkflowValidationResultFromPreview_BlockingDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()

	ctx, err := mappingWorkflows.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	cases := []struct {
		name     string
		request  factory.WorkflowPreviewRequest
		wantCode string
		wantLine bool
	}{
		{
			name: "syntax error",
			request: factory.WorkflowPreviewRequest{
				Source: factory.WorkflowSourceRequest{
					Kind:  factory.WorkflowSourceKindWorkflowName,
					Value: "broken",
				},
				Context: ctx,
			},
			wantCode: factory.WorkflowValidationCodeSyntaxError,
			wantLine: true,
		},
		{
			name: "unsupported global",
			request: factory.WorkflowPreviewRequest{
				Source: factory.WorkflowSourceRequest{
					Kind:  factory.WorkflowSourceKindWorkflowName,
					Value: "unsafe-global",
				},
				Context: ctx,
			},
			wantCode: factory.WorkflowValidationCodeUnsupportedGlobal,
		},
		{
			name: "forbidden host access",
			request: factory.WorkflowPreviewRequest{
				Source: factory.WorkflowSourceRequest{
					Kind:  factory.WorkflowSourceKindWorkflowName,
					Value: "unsafe-host",
				},
				Context: ctx,
			},
			wantCode: factory.WorkflowValidationCodeForbiddenHostAccess,
		},
		{
			name: "invalid args schema",
			request: factory.WorkflowPreviewRequest{
				Source: factory.WorkflowSourceRequest{
					Kind:  factory.WorkflowSourceKindWorkflowName,
					Value: "review",
				},
				Context:    ctx,
				ArgsSchema: []byte(`{"type":"array"}`),
			},
			wantCode: factory.WorkflowValidationCodeInvalidArgsSchema,
		},
		{
			name: "policy denied host access",
			request: factory.WorkflowPreviewRequest{
				Source: factory.WorkflowSourceRequest{
					Kind:  factory.WorkflowSourceKindWorkflowName,
					Value: "review",
				},
				Context:         ctx,
				RequestedPolicy: map[string]any{"allowNetwork": true},
			},
			wantCode: factory.JavaScriptPolicyCodeDeniedCapability,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			preview := mappingWorkflows.BuildPreview(tc.request)
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

func scriptedMappingWorkflows() factory.JavaScriptWorkflowDefinitions {
	return testutil.ScriptedJavaScriptWorkflowDefinitions{
		DefaultSourceContextFunc: func(root string) (factory.WorkflowSourceContext, error) {
			return factory.WorkflowSourceContext{ProjectRoot: root}, nil
		},
		BuildPreviewFunc: scriptedMappingPreview,
		ResolveSourceFunc: func(
			request factory.WorkflowSourceRequest,
			_ factory.WorkflowSourceContext,
		) factory.WorkflowSourceResolution {
			if request.Value == "missing" {
				return factory.WorkflowSourceResolution{
					RequestKind:  request.Kind,
					RequestValue: request.Value,
					Diagnostics: []factory.WorkflowSourceDiagnostic{{
						Code:    factory.WorkflowSourceCodeNotFound,
						Message: "scripted source was not found",
					}},
				}
			}
			return scriptedMappingPreview(factory.WorkflowPreviewRequest{Source: request}).SourceResolution
		},
	}
}

func scriptedMappingPreview(request factory.WorkflowPreviewRequest) factory.WorkflowPreview {
	sourceRef := factory.WorkflowSourceProjectClaudeWorkflowsDir + "/" + request.Source.Value + ".js"
	preview := factory.WorkflowPreview{
		Valid: true,
		SourceResolution: factory.WorkflowSourceResolution{
			RequestKind:  request.Source.Kind,
			RequestValue: request.Source.Value,
			ResolvedKind: request.Source.Kind,
			SourceRef:    sourceRef,
			SourceHash:   "sha256:test-source",
			Found:        true,
			ArtifactRoot: factory.WorkflowSourceArtifactRootDecision{Allowed: true},
		},
		PolicyPreview: factory.JavaScriptPolicyPreview{PolicyHash: "sha256:test-policy"},
	}

	code := ""
	line := 0
	switch request.Source.Value {
	case "unsafe":
		code = factory.WorkflowValidationCodeForbiddenHostAccess
	case "broken":
		code = factory.WorkflowValidationCodeSyntaxError
		line = 1
	case "unsafe-global":
		code = factory.WorkflowValidationCodeUnsupportedGlobal
	case "unsafe-host":
		code = factory.WorkflowValidationCodeForbiddenHostAccess
	}
	if len(request.ArgsSchema) > 0 {
		code = factory.WorkflowValidationCodeInvalidArgsSchema
	}
	if request.RequestedPolicy != nil {
		preview.Valid = false
		preview.PolicyPreview.ValidationIssues = []factory.JavaScriptPolicyIssue{{
			Code:    factory.JavaScriptPolicyCodeDeniedCapability,
			Message: "capability denied by scripted Factory Runtime result",
		}}
		return preview
	}
	if code != "" {
		preview.Valid = false
		preview.SourceValidationIssues = []factory.WorkflowPreviewSourceValidationIssue{{
			Code:    code,
			Message: "scripted Factory Runtime validation issue",
			Path:    sourceRef,
			Line:    line,
		}}
	}
	return preview
}

func findValidationDiagnostic(diagnostics []factoryapi.WorkflowDiagnostic, code string) *factoryapi.WorkflowDiagnostic {
	for i := range diagnostics {
		if diagnostics[i].Code == code {
			return &diagnostics[i]
		}
	}
	return nil
}
