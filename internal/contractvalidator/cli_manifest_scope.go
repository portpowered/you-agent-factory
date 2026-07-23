package contractvalidator

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type cliManifestIndex struct {
	commandsByPath map[string]map[string]any
	flagsByID      map[string]cliFlagRecord
}

type cliFlagRecord struct {
	commandKey string
	recordKey  string
	command    map[string]any
	flag       map[string]any
}

func newCLIManifestIndex(commands map[string]any) cliManifestIndex {
	index := cliManifestIndex{
		commandsByPath: make(map[string]map[string]any),
		flagsByID:      make(map[string]cliFlagRecord),
	}
	for _, commandKey := range sortedStringKeys(commands) {
		command, _ := commands[commandKey].(map[string]any)
		path, _ := command["path"].(string)
		index.commandsByPath[path] = command
		flags, _ := command["flags"].(map[string]any)
		for _, recordKey := range sortedStringKeys(flags) {
			flag, _ := flags[recordKey].(map[string]any)
			id, _ := flag["id"].(string)
			if _, exists := index.flagsByID[id]; !exists && id != "" {
				index.flagsByID[id] = cliFlagRecord{commandKey: commandKey, recordKey: recordKey, command: command, flag: flag}
			}
		}
	}
	return index
}

func cliCommandInheritanceDiagnostics(
	document, commandKey string,
	command map[string]any,
	index cliManifestIndex,
) []Diagnostic {
	flags, _ := command["flags"].(map[string]any)
	var diagnostics []Diagnostic
	for _, recordKey := range sortedStringKeys(flags) {
		flag, _ := flags[recordKey].(map[string]any)
		if flag["scope"] != "inherited" {
			continue
		}
		basePath := []string{"commands", commandKey, "flags", recordKey}
		sourceID, _ := flag["inheritedFromInputId"].(string)
		source, exists := index.flagsByID[sourceID]
		if !exists {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.inheritance.unknown-source", instancePath(append(basePath, "inheritedFromInputId")),
				fmt.Sprintf("inherited input references unknown stable input ID %q", sourceID), document,
			))
			continue
		}
		if source.flag["scope"] != "persistent" {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.inheritance.source-not-persistent", instancePath(append(basePath, "inheritedFromInputId")),
				fmt.Sprintf("inherited input source %q is not persistent", sourceID), document,
			))
			continue
		}
		if !cliCommandIsAncestor(source.command, command) {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.inheritance.non-ancestor", instancePath(append(basePath, "inheritedFromInputId")),
				fmt.Sprintf("persistent input %q does not belong to an ancestor of command %q", sourceID, commandKey), document,
			))
			continue
		}
		for _, field := range cliInheritedSemanticMismatches(source.flag, flag) {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.inheritance.semantic-mismatch", instancePath(append(basePath, field)),
				fmt.Sprintf("inherited input field %q does not preserve persistent source %q", field, sourceID), document,
			))
		}
	}
	return diagnostics
}

func cliInheritedSemanticMismatches(source, inherited map[string]any) []string {
	source = cliComparableInheritedFlag(source)
	inherited = cliComparableInheritedFlag(inherited)
	fields := make(map[string]struct{}, len(source)+len(inherited))
	for field := range source {
		fields[field] = struct{}{}
	}
	for field := range inherited {
		fields[field] = struct{}{}
	}
	var mismatches []string
	for field := range fields {
		if !reflect.DeepEqual(source[field], inherited[field]) {
			mismatches = append(mismatches, field)
		}
	}
	sort.Strings(mismatches)
	return mismatches
}

func cliComparableInheritedFlag(flag map[string]any) map[string]any {
	comparable := make(map[string]any, len(flag))
	for field, value := range flag {
		switch field {
		case "id", "scope", "inheritedFromInputId",
			"kind", "minCardinality", "maxCardinality", "acceptedSources",
			"handlerBindingId", "usage", "sensitivity",
			"default", "changedDefault", "noOptionDefault", "binding",
			"defaultValue", "noOptionDefaultValue":
			continue
		case "lifecycle":
			lifecycle, _ := value.(map[string]any)
			copy := make(map[string]any, len(lifecycle))
			for key, lifecycleValue := range lifecycle {
				if key != "itemId" {
					copy[key] = lifecycleValue
				}
			}
			comparable[field] = copy
		default:
			comparable[field] = value
		}
	}
	comparable["effectiveDefault"] = cliComparableInputValue(flag, "defaultValue", "default")
	comparable["effectiveNoOptionDefault"] = cliComparableInputValue(flag, "noOptionDefaultValue", "noOptionDefault")
	return comparable
}

func cliComparableInputValue(flag map[string]any, typedField, serializedField string) string {
	if typed, exists := flag[typedField].(map[string]any); exists {
		_, value := singleCLIInputValue(typed)
		return fmt.Sprint(value)
	}
	value, _ := flag[serializedField].(string)
	return value
}

type cliSpellingOwner struct {
	identity string
	path     string
	label    string
}

func cliCommandSpellingDiagnostics(
	document, commandKey string,
	command map[string]any,
	index cliManifestIndex,
) []Diagnostic {
	owners := make(map[string][]cliSpellingOwner)
	addCommandFlagSpellings(commandKey, command, owners)
	for _, ancestor := range cliAncestorCommands(command, index) {
		addPersistentFlagSpellings(commandKey, ancestor, owners)
	}

	var diagnostics []Diagnostic
	spellings := make([]string, 0, len(owners))
	for spelling := range owners {
		spellings = append(spellings, spelling)
	}
	sort.Strings(spellings)
	for _, spelling := range spellings {
		entries := uniqueCLISpellingOwners(owners[spelling])
		if len(entries) < 2 {
			continue
		}
		labels := make([]string, 0, len(entries))
		for _, owner := range entries {
			labels = append(labels, owner.label)
		}
		message := fmt.Sprintf("public spelling %q resolves to multiple inputs in command %q: %s", spelling, commandKey, strings.Join(labels, ", "))
		for _, owner := range entries {
			diagnostics = append(diagnostics, newDiagnostic("cli.input.spelling-duplicate", owner.path, message, document))
		}
	}
	return diagnostics
}

func addCommandFlagSpellings(commandKey string, command map[string]any, owners map[string][]cliSpellingOwner) {
	flags, _ := command["flags"].(map[string]any)
	for _, recordKey := range sortedStringKeys(flags) {
		flag, _ := flags[recordKey].(map[string]any)
		identity, _ := flag["id"].(string)
		if flag["scope"] == "inherited" {
			identity, _ = flag["inheritedFromInputId"].(string)
		}
		addFlagSpellings(commandKey, recordKey, identity, flag, owners)
	}
}

func addPersistentFlagSpellings(commandKey string, command map[string]any, owners map[string][]cliSpellingOwner) {
	ancestorKey, _ := command["id"].(string)
	if ancestorKey == "" {
		ancestorKey = commandKey
	}
	flags, _ := command["flags"].(map[string]any)
	for _, recordKey := range sortedStringKeys(flags) {
		flag, _ := flags[recordKey].(map[string]any)
		if flag["scope"] != "persistent" {
			continue
		}
		identity, _ := flag["id"].(string)
		addFlagSpellings(ancestorKey, recordKey, identity, flag, owners)
	}
}

func addFlagSpellings(commandKey, recordKey, identity string, flag map[string]any, owners map[string][]cliSpellingOwner) {
	base := []string{"commands", commandKey, "flags", recordKey}
	id, _ := flag["id"].(string)
	if long, _ := flag["long"].(string); long != "" {
		owners["--"+long] = append(owners["--"+long], cliSpellingOwner{identity, instancePath(append(base, "long")), id})
	}
	aliases, _ := flag["aliases"].([]any)
	for index, value := range aliases {
		if alias, _ := value.(string); alias != "" {
			owners["--"+alias] = append(owners["--"+alias], cliSpellingOwner{identity, instancePath(append(base, "aliases", fmt.Sprint(index))), id})
		}
	}
	if shorthand, _ := flag["shorthand"].(string); shorthand != "" {
		owners["-"+shorthand] = append(owners["-"+shorthand], cliSpellingOwner{identity, instancePath(append(base, "shorthand")), id})
	}
}

func uniqueCLISpellingOwners(owners []cliSpellingOwner) []cliSpellingOwner {
	sort.Slice(owners, func(i, j int) bool { return owners[i].path < owners[j].path })
	seen := make(map[string]bool)
	unique := owners[:0]
	for _, owner := range owners {
		if seen[owner.identity] {
			continue
		}
		seen[owner.identity] = true
		unique = append(unique, owner)
	}
	return unique
}

func cliAncestorCommands(command map[string]any, index cliManifestIndex) []map[string]any {
	path, _ := command["path"].(string)
	parts := strings.Fields(path)
	ancestors := make([]map[string]any, 0, len(parts))
	for length := 1; length < len(parts); length++ {
		if ancestor, exists := index.commandsByPath[strings.Join(parts[:length], " ")]; exists {
			ancestors = append(ancestors, ancestor)
		}
	}
	return ancestors
}

func cliCommandIsAncestor(ancestor, descendant map[string]any) bool {
	ancestorPath, _ := ancestor["path"].(string)
	descendantPath, _ := descendant["path"].(string)
	return ancestorPath != "" && strings.HasPrefix(descendantPath, ancestorPath+" ")
}
