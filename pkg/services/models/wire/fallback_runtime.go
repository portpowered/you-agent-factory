package wire

import (
	"context"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

// failClosedInvocationRuntime is the production default when no operation
// adapter was composed. It intentionally emits no content, preserving the
// distinction between an unavailable backend and generated model output.
type failClosedInvocationRuntime struct{}

func (failClosedInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	operation := strings.TrimSpace(request.Operation.Name)
	if operation == "" {
		operation = strings.TrimSpace(request.Request.Operation)
	}
	return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
		Class:     models.InvocationFailureClassConfiguration,
		Operation: operation,
		Message:   "model operation backend is unavailable",
		Cause:     models.ErrUnavailable,
	}
}
