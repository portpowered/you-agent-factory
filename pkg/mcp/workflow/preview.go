// Package workflow exposes MCP tool contracts for workflow validation and preview.
package workflow

import (
	"encoding/json"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
)

// ToolError is a structured MCP tool failure for workflow validation or start surfaces.
type ToolError struct {
	Code       string                         `json:"code"`
	Message    string                         `json:"message"`
	Preview    factoryapi.WorkflowPreviewResult `json:"preview"`
	Capability string                         `json:"capability,omitempty"`
}

func (e ToolError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "workflow tool failed"
}

// ValidateTool runs the shared workflow validation/preview contract for MCP hosts.
func ValidateTool(input factoryapi.WorkflowPreviewRequest) (factoryapi.WorkflowPreviewResult, error) {
	previewInput, err := apisurface.WorkflowPreviewRequestFromAPI(input)
	if err != nil {
		return factoryapi.WorkflowPreviewResult{}, err
	}
	return apisurface.WorkflowPreviewResultFromPreview(apisurface.BuildWorkflowPreview(previewInput)), nil
}

// StartTool validates workflow source and policy before session start for MCP hosts.
func StartTool(input factoryapi.WorkflowPreviewRequest) (factoryapi.WorkflowPreviewResult, error) {
	return ValidateTool(input)
}

// StructuredErrorFromPreview maps one invalid preview into an MCP structured tool error.
func StructuredErrorFromPreview(preview factoryapi.WorkflowPreviewResult, capability string) ToolError {
	code := "workflow.preview.invalid"
	message := "workflow preview found blocking issues"
	if len(preview.SourceValidationIssues) > 0 {
		code = preview.SourceValidationIssues[0].Code
		message = preview.SourceValidationIssues[0].Message
	} else if preview.SourceResolution.ArtifactRoot != nil && preview.SourceResolution.ArtifactRoot.Diagnostic != nil {
		code = preview.SourceResolution.ArtifactRoot.Diagnostic.Code
		message = preview.SourceResolution.ArtifactRoot.Diagnostic.Message
	} else if len(preview.PolicyPreview.ValidationIssues) > 0 {
		code = preview.PolicyPreview.ValidationIssues[0].Code
		message = preview.PolicyPreview.ValidationIssues[0].Message
	} else if preview.SourceResolution.Diagnostics != nil && len(*preview.SourceResolution.Diagnostics) > 0 {
		code = (*preview.SourceResolution.Diagnostics)[0].Code
		message = (*preview.SourceResolution.Diagnostics)[0].Message
	}
	return ToolError{
		Code:       code,
		Message:    message,
		Preview:    preview,
		Capability: capability,
	}
}

// MarshalToolError encodes one structured MCP tool error for host agents.
func MarshalToolError(err ToolError) ([]byte, error) {
	return json.Marshal(err)
}

// PreviewInputFromRequest adapts one workflow preview request for MCP tool wiring.
func PreviewInputFromRequest(input preview.Request) factoryapi.WorkflowPreviewResult {
	return apisurface.WorkflowPreviewResultFromPreview(apisurface.BuildWorkflowPreview(input))
}
