package apisurface

import "github.com/portpowered/infinite-you/pkg/workflowpolicy"

// BuildWorkflowPolicyPreview is the shared API, CLI, MCP, and website entry
// point for workflow policy preview and session-start projection.
func BuildWorkflowPolicyPreview(input workflowpolicy.PreviewInput) workflowpolicy.Preview {
	return workflowpolicy.BuildPreview(input)
}

// ResolveWorkflowPolicy is the shared entry point for effective policy resolution.
func ResolveWorkflowPolicy(request workflowpolicy.Request) workflowpolicy.Resolution {
	return workflowpolicy.Resolve(request)
}
