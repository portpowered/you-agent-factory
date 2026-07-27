// Package wire constructs the parent-private Operator Settings resolution
// subservice.
package wire

import (
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	resolutionservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/internal/service"
)

// NewService constructs an inert effective-resolution owner.
func NewService() (resolution.Service, error) {
	return resolutionservice.New()
}
