package builtingoal

import (
	"embed"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/packageassets"
)

//go:embed factory.json
var factoryJSON []byte

//go:embed prompts/*.md
var promptAssets embed.FS

// BuiltInGoalFactoryJSON is the canonical runnable @you/goal packaged factory payload
// assembled deterministically from the declarative promptFile references in
// factory.json and the embedded package-owned assets under prompts/.
var BuiltInGoalFactoryJSON = mustAssembleBuiltInGoalFactoryJSON()

// FactoryJSON returns the authored factory scaffold without assembled prompt bodies.
func FactoryJSON() []byte {
	return append([]byte(nil), factoryJSON...)
}

func mustAssembleBuiltInGoalFactoryJSON() []byte {
	payload, err := packageassets.Assemble(packageassets.Definition{
		Package:     "@you/goal",
		FactoryJSON: factoryJSON,
		Assets:      promptAssets,
	})
	if err != nil {
		panic(fmt.Sprintf("assemble built-in @you/goal factory json: %v", err))
	}
	return payload
}
