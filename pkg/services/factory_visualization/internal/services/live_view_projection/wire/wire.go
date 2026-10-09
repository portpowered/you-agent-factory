// Package wire constructs the Factory Visualization live_view_projection subservice.
package wire

import (
	projectionservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Owner is the private behavior owner retained by Visualization composition.
type Owner = projectionservice.Owner

// NewOwner constructs the fixed behavior once, before any runtime opening.
func NewOwner(peer recordings.Service) *Owner {
	return projectionservice.NewOwner(peer)
}
