package apisurface

import workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"

// ResolveWorkflowPolicy is the shared entry point for effective policy resolution.
func ResolveWorkflowPolicy(request workflowpolicy.Request) workflowpolicy.Resolution {
	return workflowpolicy.Resolve(request)
}
