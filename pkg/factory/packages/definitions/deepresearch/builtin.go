// Package deepresearch owns the authored @you/deep-research factory definition.
package deepresearch

import (
	"embed"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/packages/packageassets"
)

//go:embed factory.json
var factoryJSON []byte

//go:embed scripts/*.js
var assets embed.FS

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
