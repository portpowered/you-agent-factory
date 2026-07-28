// Package packageassets is a transitional shim over the Distribution-owned
// package asset assembly implementation.
package packageassets

import (
	distributionpackageassets "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packageassets"
)

// Definition describes an authored packaged factory and the assets available
// beneath its package-owned asset root.
type Definition = distributionpackageassets.Definition

// Assemble resolves all supported package-owned assets and returns a new
// canonical JSON payload.
func Assemble(definition Definition) ([]byte, error) {
	return distributionpackageassets.Assemble(definition)
}
