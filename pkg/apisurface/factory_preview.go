package apisurface

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
)

// BuildFactoryPreview is the shared API, CLI, MCP, and website entry point for
// JavaScript-orchestrated Factory preview preparation.
func BuildFactoryPreview(input preview.Request) preview.Preview {
	return preview.BuildFactoryPreview(input)
}

// FactoryPreviewRequestFromAPI maps one public Factory preview request into the
// shared preview contract.
func FactoryPreviewRequestFromAPI(req factoryapi.FactoryPreviewRequest) (preview.Request, error) {
	legacy := workflowPreviewRequestFromFactoryPreview(req)
	return WorkflowPreviewRequestFromAPI(legacy)
}

// FactoryPreviewResultFromPreview maps the shared preview contract to the public
// Factory preview response shape.
func FactoryPreviewResultFromPreview(previewResult preview.Preview) factoryapi.FactoryPreviewResult {
	legacy := WorkflowPreviewResultFromPreview(previewResult)
	return factoryPreviewResultFromWorkflowPreview(legacy)
}

func workflowPreviewRequestFromFactoryPreview(req factoryapi.FactoryPreviewRequest) factoryapi.WorkflowPreviewRequest {
	out := factoryapi.WorkflowPreviewRequest{
		SourceKind:         factoryapi.WorkflowPreviewRequestSourceKind(req.SourceKind),
		SourceValue:        req.SourceValue,
		InlineSource:       req.InlineSource,
		ArtifactRoot:       req.ArtifactRoot,
		AllowFactoryLookup: req.AllowFactoryLookup,
		ProjectRoot:        req.ProjectRoot,
		Metadata:           req.Metadata,
		ArgsSchema:         req.ArgsSchema,
		DefaultPolicy:      req.DefaultPolicy,
		RequestedPolicy:    req.RequestedPolicy,
		RequestedRunner:    req.RequestedRunner,
		RequestedModel:     req.RequestedModel,
		RequestedProfile:   req.RequestedProfile,
		TimeoutMillis:      req.TimeoutMillis,
	}
	return out
}

func factoryPreviewResultFromWorkflowPreview(result factoryapi.WorkflowPreviewResult) factoryapi.FactoryPreviewResult {
	return factoryapi.FactoryPreviewResult{
		Valid:                  result.Valid,
		SourceResolution:       result.SourceResolution,
		SourceValidationIssues: result.SourceValidationIssues,
		PolicyPreview:          result.PolicyPreview,
		ResultConstraints:      result.ResultConstraints,
	}
}
