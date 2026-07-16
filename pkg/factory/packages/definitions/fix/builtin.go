package builtinfix

import (
	"embed"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/packages/packageassets"
)

//go:embed factory.json
var factoryJSON []byte

//go:embed prompts/*.md
var promptAssets embed.FS

// BuiltInFixFactoryJSON is the canonical runnable @you/fix packaged factory
// payload assembled from its declarative prompt assets.
var BuiltInFixFactoryJSON = mustAssembleBuiltInFixFactoryJSON()

// FactoryJSON returns the authored factory scaffold without assembled prompt bodies.
func FactoryJSON() []byte {
	return append([]byte(nil), factoryJSON...)
}

func mustAssembleBuiltInFixFactoryJSON() []byte {
	payload, err := packageassets.Assemble(packageassets.Definition{
		Package:     "@you/fix",
		FactoryJSON: factoryJSON,
		Assets:      promptAssets,
	})
	if err != nil {
		panic(fmt.Sprintf("assemble built-in @you/fix factory json: %v", err))
	}
	return payload
}
