package apisurface

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// FactoryWorkflowValidationResult is the shared workflow source validation contract
// for CLI loopback and future HTTP parity surfaces.
type FactoryWorkflowValidationResult struct {
	Valid               bool                                `json:"valid"`
	SourceResolution    factoryapi.WorkflowSourceResolution `json:"sourceResolution"`
	BlockingDiagnostics []factoryapi.WorkflowDiagnostic     `json:"blockingDiagnostics"`
}

// BuildFactoryWorkflowValidation resolves and validates workflow source without execution.
func BuildFactoryWorkflowValidation(input workflowpreview.Request) workflowpreview.Preview {
	return BuildFactoryPreview(input)
}

// FactoryWorkflowValidationResultFromPreview maps preview output to the validation contract.
func FactoryWorkflowValidationResultFromPreview(preview workflowpreview.Preview) FactoryWorkflowValidationResult {
	previewResult := FactoryPreviewResultFromPreview(preview)
	return FactoryWorkflowValidationResult{
		Valid:               preview.Valid,
		SourceResolution:    previewResult.SourceResolution,
		BlockingDiagnostics: blockingDiagnosticsFromPreviewResult(previewResult),
	}
}

func blockingDiagnosticsFromPreviewResult(result factoryapi.FactoryPreviewResult) []factoryapi.WorkflowDiagnostic {
	out := make([]factoryapi.WorkflowDiagnostic, 0)
	if result.SourceResolution.Diagnostics != nil {
		if !result.SourceResolution.Found {
			out = append(out, *result.SourceResolution.Diagnostics...)
		} else {
			for _, diagnostic := range *result.SourceResolution.Diagnostics {
				if diagnostic.Code == workflowsource.CodeSourceConflict {
					out = append(out, diagnostic)
				}
			}
		}
	}
	if result.SourceResolution.ArtifactRoot != nil &&
		result.SourceResolution.ArtifactRoot.Diagnostic != nil &&
		!result.SourceResolution.ArtifactRoot.Allowed &&
		strings.TrimSpace(result.SourceResolution.ArtifactRoot.Requested) != "" {
		out = append(out, *result.SourceResolution.ArtifactRoot.Diagnostic)
	}
	out = append(out, result.SourceValidationIssues...)
	out = append(out, result.PolicyPreview.ValidationIssues...)
	return out
}
