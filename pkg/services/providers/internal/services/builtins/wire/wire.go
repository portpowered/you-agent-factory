// Package wire constructs the parent-private packaged Providers catalog.
package wire

import (
	_ "embed"

	builtins "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins"
	builtinsservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins/internal/service"
)

//go:embed catalog.json
var packagedCatalog []byte

func NewService() (builtins.Service, error) {
	return builtinsservice.New(packagedCatalog)
}
