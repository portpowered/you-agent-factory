// Package inference defines the parent-private Models inference service.
package inference

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

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
