// Package acp declares the parent-private Agent Client Protocol service.
package acp

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service owns configured ACP peers and their protocol lifecycle. Execution
// delegates ACP attempts here instead of constructing protocol adapters.
type Service interface {
	Execute(context.Context, providers.ID, providers.ExecuteRequest) (providers.ExecuteResult, error)
	Close(context.Context) error
}
