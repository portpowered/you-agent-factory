// Package wire constructs the parent-private Providers catalog subservice.
package wire

import (
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/internal/service"
)

// Option configures catalog construction.
type Option = catalogservice.Option

// WithProbeQuery configures request-time readiness probing for catalog list/get.
var WithProbeQuery = catalogservice.WithProbeQuery

// WithDescriptors contributes customer-configured provider descriptors.
var WithDescriptors = catalogservice.WithDescriptors

// NewService constructs an inert catalog over the accepted standardized
// provider catalog publication.
func NewService(options ...Option) (catalog.Service, error) {
	return catalogservice.New(options...)
}
