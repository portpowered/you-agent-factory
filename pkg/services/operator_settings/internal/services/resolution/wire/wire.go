// Package wire constructs the parent-private Operator Settings resolution
// subservice.
package wire

import (
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	resolutionservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/internal/service"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// NewService constructs an inert effective-resolution owner backed by the
// accepted Providers root for semantic validation and canonicalization.
func NewService(providersRoot providers.Service) (resolution.Service, error) {
	return resolutionservice.New(providersRoot)
}
