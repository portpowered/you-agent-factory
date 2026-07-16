// Package packages owns the catalog of factory definitions shipped with you.
package packages

import (
	"sort"

	builtinfusion "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/fusion"
	builtingoal "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/goal"
	builtinloop "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/loop"
	builtinsubagent "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/subagent"
	builtintts "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/tts"
)

// Definition describes one factory shipped with the executable.
type Definition struct {
	Name    string
	Project string
	JSON    []byte
}

var catalog = map[string]Definition{
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
	"@you/loop": {
		Name:    "@you/loop",
		Project: "builtin-loop",
		JSON:    builtinloop.BuiltInLoopFactoryJSON,
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
