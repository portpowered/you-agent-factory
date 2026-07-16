package builtinloop

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/packages/packageassets"
)

func mustAssembleBuiltInLoopFactoryJSON() []byte {
	payload, err := packageassets.Assemble(packageassets.Definition{
		Package:     "@you/loop",
		FactoryJSON: factoryJSON,
		Assets:      promptAssets,
	})
	if err != nil {
		panic(fmt.Sprintf("assemble built-in @you/loop factory json: %v", err))
	}
	return payload
}
