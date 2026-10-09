// Package wire constructs the Factory Visualization activation-lifecycle subservice.
package wire

import (
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Owner is the private behavior owner retained by Visualization composition.
type Owner = lifecycleservice.Owner

// NewOwner constructs the fixed behavior once, before any runtime opening.
func NewOwner(peer recordings.Service) *Owner {
	return lifecycleservice.NewOwner(peer)
}
