package builtinreview

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/internal/authoredsource"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/packageassets"
)

var (
	factoryJSON  = authoredsource.MustFactoryJSON("review")
	promptAssets = authoredsource.MustFactoryFS("review")
)

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
