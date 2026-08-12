// Package wire constructs the parent-private packaged Providers catalog.
package wire

import (
	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	builtins "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins"
	builtinsservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins/internal/service"
)

func NewService() (builtins.Service, error) {
	return NewServiceFromRuntimeCatalog(modelproviders.RuntimeACPJSON())
}

// NewServiceFromRuntimeCatalog loads a generated package-owned runtime
// projection. It is kept at this owner-wire boundary so callers can validate
// an alternate generated document without reaching through the service root.
func NewServiceFromRuntimeCatalog(document []byte) (builtins.Service, error) {
	return builtinsservice.New(document)
}
