// Package wire constructs the owner-private Workers workstation service.
package wire

import (
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
)

// NewService constructs an inert workstation capability.
func NewService() workstations.Service {
	return internalservice.New()
}

var NewFactoryDocsLoader = prompting.NewFactoryDocsLoader
