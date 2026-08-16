package service

import "github.com/portpowered/infinite-you/pkg/services/workers"

// modelRuntimeInputFromRequest snapshots the explicit Models projection as it
// crosses from the public Execute request into the private runner request.
// The scope remains request-owned; this helper does not add another execution
// operation or context-based transport.
func modelRuntimeInputFromRequest(
	request workers.ExecuteRequest,
) *workers.ModelRuntimeInput {
	return request.Input.ModelRuntime.Clone()
}
