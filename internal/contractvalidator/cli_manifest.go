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
		diagnostics = append(diagnostics, cliCommandPrecedenceDiagnostics(document, commandKey, command)...)
	}
	sortDiagnostics(diagnostics)
	return diagnostics
}

var canonicalCLIPrecedence = []string{
	"cli",
	"stdin",
	"environment",
	"operator-config",
	"manifest-default",
	"factory-signature-default",
}

func cliCommandPrecedenceDiagnostics(document, commandKey string, command map[string]any) []Diagnostic {
	precedence, exists := command["precedence"].(map[string]any)
	if !exists {
		if command["completeness"] == "authoritative" || cliCommandHasCanonicalInputs(commandKey, command) {
			return []Diagnostic{newDiagnostic("cli.precedence.missing", instancePath([]string{"commands", commandKey, "precedence"}), "command requiring canonical source resolution is missing source precedence", document)}
		}
		return nil
	}
	order, _ := precedence["order"].([]any)
	if len(order) != len(canonicalCLIPrecedence) {
		return []Diagnostic{newDiagnostic("cli.precedence.incomplete", instancePath([]string{"commands", commandKey, "precedence", "order"}), "source precedence must contain every canonical tier exactly once", document)}
	}
	seen := make(map[string]int, len(order))
	known := make(map[string]bool, len(canonicalCLIPrecedence))
	for _, source := range canonicalCLIPrecedence {
		known[source] = true
	}
	var diagnostics []Diagnostic
	for index, raw := range order {
		source, _ := raw.(string)
		path := instancePath([]string{"commands", commandKey, "precedence", "order", fmt.Sprint(index)})
		if !known[source] {
			diagnostics = append(diagnostics, newDiagnostic("cli.precedence.unknown", path, fmt.Sprintf("source tier %q is not canonical", source), document))
			continue
		}
		if first, duplicate := seen[source]; duplicate {
			diagnostics = append(diagnostics, newDiagnostic("cli.precedence.duplicate", path, fmt.Sprintf("source tier %q duplicates index %d", source, first), document))
			continue
		}
		seen[source] = index
		if source != canonicalCLIPrecedence[index] {
			diagnostics = append(diagnostics, newDiagnostic("cli.precedence.order", path, fmt.Sprintf("source tier %q must be %q at index %d", source, canonicalCLIPrecedence[index], index), document))
		}
	}
	for _, source := range canonicalCLIPrecedence {
		if _, exists := seen[source]; !exists {
			diagnostics = append(diagnostics, newDiagnostic("cli.precedence.missing-tier", instancePath([]string{"commands", commandKey, "precedence", "order"}), fmt.Sprintf("source precedence is missing tier %q", source), document))
		}
	}
	return diagnostics
}

func cliCommandHasCanonicalInputs(commandKey string, command map[string]any) bool {
	for _, input := range collectCLIInputs(commandKey, command) {
		if cliInputIsCanonical(input.record) {
			return true
		}
	}
	return false
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
