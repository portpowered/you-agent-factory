// Package packages owns the catalog of factory definitions shipped with you.
package packages

import (
	"sort"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	builtindeepresearch "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/deepresearch"
	builtinfusion "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/fusion"
	builtingoal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/goal"
	builtinquorum "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/quorum"
	builtinsubagent "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/subagent"
	builtintts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/tts"
)

// Definition describes one factory shipped with the executable.
type Definition = factorydefinitions.PackagedDefinition

var catalog = map[string]Definition{
	"@you/deep-research": {
		Name:    "@you/deep-research",
		Project: "builtin-deep-research",
		JSON:    builtindeepresearch.BuiltInFactoryJSON,
	},
	"@you/fusion": {
		Name:    "@you/fusion",
		Project: "builtin-fusion",
		JSON:    builtinfusion.BuiltInFactoryJSON,
	},
	"@you/goal": {
		Name:    "@you/goal",
		Project: "builtin-goal",
		JSON:    builtingoal.BuiltInGoalFactoryJSON,
	},
	"@you/quorum": {
		Name:    "@you/quorum",
		Project: "builtin-quorum",
		JSON:    builtinquorum.BuiltInFactoryJSON,
	},
	"@you/subagent": {
		Name:    "@you/subagent",
		Project: "builtin-subagent",
		JSON:    builtinsubagent.BuiltInSubagentFactoryJSON,
	},
	"@you/tts": {
		Name:    "@you/tts",
		Project: "builtin-tts",
		JSON:    builtintts.BuiltInFactoryJSON,
	},
}

// Lookup returns an isolated copy of a packaged factory definition.
func Lookup(name string) (Definition, bool) {
	definition, ok := catalog[name]
	if !ok {
		return Definition{}, false
	}
	definition.JSON = append([]byte(nil), definition.JSON...)
	return definition, true
}

// Names returns the packaged factory identifiers in stable lexical order.
func Names() []string {
	names := make([]string, 0, len(catalog))
	for name := range catalog {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// All returns detached packaged Factory Definitions in stable lexical order.
func All() []factorydefinitions.PackagedDefinition {
	names := Names()
	definitions := make([]factorydefinitions.PackagedDefinition, 0, len(names))
	for _, name := range names {
		definition, ok := Lookup(name)
		if ok {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}
