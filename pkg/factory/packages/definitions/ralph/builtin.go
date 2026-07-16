package builtinralph

import (
	"embed"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/packages/packageassets"
)

//go:embed factory.json
var factoryJSON []byte

//go:embed prompts/*.md
var promptAssets embed.FS

// BuiltInRalphFactoryJSON is the canonical runnable @you/ralph package. It is
// assembled from its declarative definition and package-owned prompt assets.
var BuiltInRalphFactoryJSON = mustAssembleBuiltInRalphFactoryJSON()

// FactoryJSON returns the authored factory scaffold without assembled prompts.
func FactoryJSON() []byte {
	return append([]byte(nil), factoryJSON...)
}

func mustAssembleBuiltInRalphFactoryJSON() []byte {
	payload, err := packageassets.Assemble(packageassets.Definition{
		Package:     "@you/ralph",
		FactoryJSON: factoryJSON,
		Assets:      promptAssets,
	})
	if err != nil {
		panic(fmt.Sprintf("assemble built-in @you/ralph factory json: %v", err))
	}
	return payload
}
