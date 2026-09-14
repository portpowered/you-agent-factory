//go:build managed_process_integration

package wire

import (
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

// NewModelsServiceForManagedProcessIntegration exposes the canonical Models
// construction path only to the dedicated compiled-artifact integration lane.
// The build tag keeps this test seam out of the ordinary package and product
// API while preserving the production pkg/wire launcher and defaults.
func NewModelsServiceForManagedProcessIntegration(edges serviceedges.Edges) (models.Service, error) {
	return provideModelsService(edges)
}
