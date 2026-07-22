package builtinsubagent

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/internal/authoredsource"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/packageassets"
)

var (
	factoryJSON  = authoredsource.MustFactoryJSON("subagent")
	promptAssets = authoredsource.MustFactoryFS("subagent")
)

// BuiltInSubagentFactoryJSON is the canonical runnable @you/subagent packaged
// factory payload assembled from authored factory.json and prompt files.
var BuiltInSubagentFactoryJSON = mustAssembleBuiltInSubagentFactoryJSON()

// FactoryJSON returns the authored factory scaffold without assembled prompt bodies.
func FactoryJSON() []byte {
	return append([]byte(nil), factoryJSON...)
}

func mustAssembleBuiltInSubagentFactoryJSON() []byte {
	payload, err := packageassets.Assemble(packageassets.Definition{
		Package:     "@you/subagent",
		FactoryJSON: factoryJSON,
		Assets:      promptAssets,
	})
	if err != nil {
		panic(fmt.Sprintf("assemble built-in @you/subagent factory json: %v", err))
	}
	return payload
}
