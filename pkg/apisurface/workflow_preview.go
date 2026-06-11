package apisurface

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/workflowpreview"
)

// BuildWorkflowPreview is a compatibility alias for BuildFactoryPreview.
//
// Deprecated: use BuildFactoryPreview for canonical Factory preview semantics.
func BuildWorkflowPreview(input workflowpreview.Request) workflowpreview.Preview {
	return BuildFactoryPreview(input)
}

// WorkflowPreviewRequestFromAPI maps one obsolete workflow-preview request into the shared preview contract.
//
// Deprecated: use FactoryPreviewRequestFromAPI for canonical Factory preview semantics.
func WorkflowPreviewRequestFromAPI(req factoryapi.WorkflowPreviewRequest) (workflowpreview.Request, error) {
	return FactoryPreviewRequestFromAPI(req)
}

// WorkflowPreviewResultFromPreview maps the shared preview contract to the obsolete workflow-preview API shape.
//
// Deprecated: use FactoryPreviewResultFromPreview for canonical Factory preview semantics.
func WorkflowPreviewResultFromPreview(preview workflowpreview.Preview) factoryapi.WorkflowPreviewResult {
	return FactoryPreviewResultFromPreview(preview)
}
