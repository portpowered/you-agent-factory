package contractvalidator

import (
	"fmt"
	"sort"
)

const commandManifestSchemaID = "https://schemas.portpowered.com/you/contracts/cli/command-manifest.schema.json"

func cliManifestDiagnostics(document string, value any) []Diagnostic {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	commands, ok := root["commands"].(map[string]any)
	if !ok {
		return nil
	}

	index := newCLIManifestIndex(commands)
	var diagnostics []Diagnostic
	for _, commandKey := range sortedStringKeys(commands) {
		command, ok := commands[commandKey].(map[string]any)
		if !ok {
			continue
		}
		diagnostics = append(diagnostics, cliCommandInputAmbiguityDiagnostics(document, commandKey, command)...)
		diagnostics = append(diagnostics, cliCommandInheritanceDiagnostics(document, commandKey, command, index)...)
		diagnostics = append(diagnostics, cliCommandSpellingDiagnostics(document, commandKey, command, index)...)
		diagnostics = append(diagnostics, cliCommandValueAndBindingDiagnostics(document, commandKey, command)...)
		diagnostics = append(diagnostics, cliCommandRelationshipDiagnostics(document, commandKey, command, index)...)
	}
	sortDiagnostics(diagnostics)
	return diagnostics
}

func cliCommandInputAmbiguityDiagnostics(document, commandKey string, command map[string]any) []Diagnostic {
	argumentIDs := collectCLIRecordIDPaths(commandKey, command, "arguments")
	flagIDs := collectCLIRecordIDPaths(commandKey, command, "flags")

	var diagnostics []Diagnostic
	sharedIDs := make([]string, 0)
	for id := range argumentIDs {
		if _, ok := flagIDs[id]; ok {
			sharedIDs = append(sharedIDs, id)
		}
	}
	sort.Strings(sharedIDs)

	for _, id := range sharedIDs {
		message := fmt.Sprintf("stable ID %q is claimed by both a flag and an argument", id)
		diagnostics = append(diagnostics,
			newDiagnostic("cli.input.ambiguous", argumentIDs[id], message, document),
			newDiagnostic("cli.input.ambiguous", flagIDs[id], message, document),
		)
	}
	return diagnostics
}

func collectCLIRecordIDPaths(commandKey string, command map[string]any, field string) map[string]string {
	records, ok := command[field].(map[string]any)
	if !ok {
		return nil
	}

	ids := make(map[string]string, len(records))
	for _, recordKey := range sortedStringKeys(records) {
		record, ok := records[recordKey].(map[string]any)
		if !ok {
			continue
		}
		id, ok := record["id"].(string)
		if !ok || id == "" {
			continue
		}
		ids[id] = instancePath([]string{"commands", commandKey, field, recordKey, "id"})
	}
	return ids
}

func sortedStringKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
