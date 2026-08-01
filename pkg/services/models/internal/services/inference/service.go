// Package inference defines the parent-private Models inference service.
package inference

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// TargetError and the direct-invocation values remain private to the Models
// Inference owner while old local-runtime adapters finish converging on this
// canonical nested service package.
var ErrUnsupportedResponseMode = models.ErrUnsupportedResponseMode

type ResponseMode = models.ResponseMode
type Options = models.Options
type Request = models.Request
type Result = models.Result
type ResolvedModelOperationBinding = models.ResolvedModelOperationBinding
type TargetError = models.TargetError

const ResponseModeAudioStream = models.ResponseModeAudioStream

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
