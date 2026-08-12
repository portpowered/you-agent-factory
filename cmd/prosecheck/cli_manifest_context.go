package main

import (
	"slices"
	"strings"
	"unicode"
)

type cliManifestContext struct {
	commandID string
	inputID   string
	jsonPath  string
	surfaces  []string
	literals  []string
}

func cliManifestSurfaceSelectors(policy Policy, commandID string) []string {
	surfaces := []string{surfaceCLIHelp}
	for _, term := range policy.Terms {
		if cliCommandSubjectMatches(commandID, term) {
			surfaces = append(surfaces, term.Surfaces...)
		}
	}
	return uniqueCLIStrings(surfaces)
}

func cliCommandSubjectMatches(commandID string, term Term) bool {
	words := strings.FieldsFunc(strings.ToLower(commandID), func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsDigit(value)
	})
	if len(words) == 0 {
		return false
	}
	for _, spelling := range termSpellings(term) {
		spellingWords := strings.FieldsFunc(strings.ToLower(spelling), func(value rune) bool {
			return !unicode.IsLetter(value) && !unicode.IsDigit(value)
		})
		if len(spellingWords) == 0 || len(words) < len(spellingWords) {
			continue
		}
		for start := 0; start <= len(words)-len(spellingWords); start++ {
			matched := true
			for index, word := range spellingWords {
				if words[start+index] != word {
					matched = false
					break
				}
			}
			if matched {
				return true
			}
		}
	}
	return false
}

func (c *cliManifestCollector) commandLiterals(command *cliJSONValue) []string {
	literals := cliManifestLiterals(command)
	literals = append(literals, cliPolicyLiterals(c.policy)...)
	return uniqueCLIStrings(literals)
}

func (c *cliManifestCollector) inputLiterals(context cliManifestContext, input *cliJSONValue) []string {
	literals := append([]string(nil), context.literals...)
	literals = append(literals, cliManifestLiterals(input)...)
	return uniqueCLIStrings(literals)
}

func cliManifestLiterals(root *cliJSONValue) []string {
	var literals []string
	var visit func(*cliJSONValue, string)
	visit = func(value *cliJSONValue, key string) {
		if value == nil {
			return
		}
		switch value.kind {
		case cliJSONString:
			if cliLiteralField(key) {
				literals = append(literals, value.stringValue)
			}
		case cliJSONObject:
			for _, field := range value.fields {
				visit(field.value, field.key)
			}
		case cliJSONArray:
			for _, item := range value.elements {
				visit(item, key)
			}
		}
	}
	visit(root, "")
	return uniqueCLIStrings(literals)
}

func cliLiteralField(key string) bool {
	switch key {
	case "id", "name", "path", "long", "shorthand", "aliases", "targetItemId", "operationId", "code", "default", "provider", "model", "providerName", "modelName", "event", "schema":
		return true
	default:
		return false
	}
}

func cliPolicyLiterals(policy Policy) []string {
	var literals []string
	for _, term := range policy.Terms {
		if term.Category != "command" && term.Category != "protected-literal" {
			continue
		}
		literals = append(literals, term.Canonical)
		for _, form := range term.ApprovedForms {
			literals = append(literals, form)
		}
	}
	return uniqueCLIStrings(literals)
}

func uniqueCLIStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	slices.SortStableFunc(result, func(left, right string) int {
		if len(left) != len(right) {
			return len(right) - len(left)
		}
		return strings.Compare(left, right)
	})
	return result
}
