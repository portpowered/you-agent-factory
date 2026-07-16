package builtinreview

import (
	"embed"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/packages/packageassets"
)

//go:embed factory.json
var factoryJSON []byte

//go:embed prompts/*.md
var promptAssets embed.FS

// BuiltInReviewFactoryJSON is the canonical runnable @you/review packaged factory payload.
var BuiltInReviewFactoryJSON = mustAssembleBuiltInReviewFactoryJSON()

// FactoryJSON returns the authored factory scaffold without assembled prompt bodies.
func FactoryJSON() []byte {
	return append([]byte(nil), factoryJSON...)
}

func mustAssembleBuiltInReviewFactoryJSON() []byte {
	payload, err := packageassets.Assemble(packageassets.Definition{
		Package:     "@you/review",
		FactoryJSON: factoryJSON,
		Assets:      promptAssets,
	})
	if err != nil {
		panic(fmt.Sprintf("assemble built-in @you/review factory json: %v", err))
	}
	return payload
}
