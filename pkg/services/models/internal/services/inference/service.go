// Package inference defines the parent-private Models inference service.
package inference

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// InvocationRuntime executes one validated scoped model operation under accepted
// lease capacity. Implementations must return detached content facts without
// exposing runtime handles, endpoints, processes, or filesystem paths.
type InvocationRuntime interface {
	Invoke(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, error)
}

// Service owns scoped invoke and cancellation behind the singular Models root.
// Peers reach lease-backed invocation only through the process-scoped Models
// service; this interface stays parent-private.
type Service interface {
	InvokeModelWithLease(
		context.Context,
		models.InvokeModelRequest,
	) (models.InvokeModelResult, error)
	CancelInvocation(
		context.Context,
		models.CancelInvocationRequest,
	) (models.CancelInvocationResult, error)
}
