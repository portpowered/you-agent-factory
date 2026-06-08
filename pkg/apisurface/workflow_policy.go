package apisurface

import "github.com/portpowered/infinite-you/pkg/workflowpolicy"

// ResolveWorkflowPolicy is the shared entry point for effective policy resolution.
func ResolveWorkflowPolicy(request workflowpolicy.Request) workflowpolicy.Resolution {
	return workflowpolicy.Resolve(request)
}
