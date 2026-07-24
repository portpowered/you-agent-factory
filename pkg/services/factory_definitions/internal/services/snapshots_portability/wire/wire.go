// Package wire constructs the Factory Definitions snapshots_portability
// subservice from exact injected effect ports.
package wire

import (
	"fmt"

	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/service"
)

// NewService constructs the private snapshots_portability implementation.
func NewService() (snapshotsportability.Service, error) {
	service := internalservice.New()
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions snapshots_portability: implementation rejected construction")
	}
	return service, nil
}
