package workers

import (
	"context"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

// ModelInvoker executes one model operation through the configured Worker
// path.
type ModelInvoker interface {
	InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error)
}

// Service is the aggregate customer-facing Worker execution boundary.
// Provider factories, command runners, and workstation builders remain
// implementation details or explicit Worker subservices.
type Service interface {
	ModelInvoker
}
