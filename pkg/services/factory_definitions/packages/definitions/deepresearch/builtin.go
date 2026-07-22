// Package deepresearch owns the authored @you/deep-research factory definition.
package deepresearch

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/internal/authoredsource"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/packageassets"
)

var (
	factoryJSON = authoredsource.MustFactoryJSON("deep-research")
	assets      = authoredsource.MustFactoryFS("deep-research")
)

// BuiltInFactoryJSON is the canonical runnable @you/deep-research payload,
// including its authored JavaScript workflow asset.
var BuiltInFactoryJSON = mustAssembleBuiltInFactoryJSON()

// FactoryJSON returns the authored scaffold before package assets are assembled.
func FactoryJSON() []byte {
	return append([]byte(nil), factoryJSON...)
}

func mustAssembleBuiltInFactoryJSON() []byte {
	payload, err := packageassets.Assemble(packageassets.Definition{
		Package:     "@you/deep-research",
		FactoryJSON: factoryJSON,
		Assets:      assets,
	})
	if err != nil {
		panic(fmt.Sprintf("assemble built-in @you/deep-research factory json: %v", err))
	}
	return payload
}
