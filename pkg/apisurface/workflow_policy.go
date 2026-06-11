package apisurface

import "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"

// ResolveWorkflowPolicy is the shared entry point for effective policy resolution.
func ResolveWorkflowPolicy(request policy.Request) policy.Resolution {
	return policy.Resolve(request)
}
