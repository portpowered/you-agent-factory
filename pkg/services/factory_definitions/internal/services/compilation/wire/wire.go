// Package wire constructs the Factory Definitions compilation nested subservice
// for Definitions-local composition. Root pkg/wire does not own this package.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/service"
)

// NewService constructs the inert compilation capability for Definitions-owned
// composition. It returns only the compilation.Service contract.
func NewService() compilation.Service {
	return compilationservice.New()
}
