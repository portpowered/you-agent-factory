// Package wire constructs the owner-private Workers workstation service.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
)

// NewService constructs an inert workstation capability. Logger is optional
// and passed directly at construction; omitting it selects a no-op logger.
func NewService(logger ...logging.Logger) workstations.Service {
	return internalservice.New(logger...)
}

var NewFactoryDocsLoader = prompting.NewFactoryDocsLoader
