// Package wire constructs the private Workers Runners subservice.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/service"
)

// NewService validates registrations into one immutable private registry.
func NewService(registrations []runners.Registration) (runners.Service, error) {
	return internalservice.New(registrations)
}
