// Package promptassets is a transitional shim over the Distribution-owned
// prompt asset assembly implementation.
package promptassets

import (
	distributionpromptassets "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/promptassets"
)

// Definition describes an authored packaged factory and the assets available
// beneath its package-owned asset root.
type Definition = distributionpromptassets.Definition

// Assemble resolves worker and workstation promptFile declarations and returns
// a new canonical JSON payload.
func Assemble(definition Definition) ([]byte, error) {
	return distributionpromptassets.Assemble(definition)
}
