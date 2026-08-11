// Package wire constructs the parent-private packaged Providers catalog.
package wire

import (
	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	builtins "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins"
	builtinsservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins/internal/service"
)

func NewService() (builtins.Service, error) {
	return builtinsservice.New(modelproviders.RuntimeACPJSON())
}
