package clicontract

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

const (
	KindUncontractedInput = "uncontracted-input"
	KindMissingInput      = "missing-input"
	KindInputDrift        = "input-metadata-drift"
)

func validateMigratedInputs(
	canonical climanifest.Manifest,
	production cliinputs.Inventory,
) []Finding {
	findings := make([]Finding, 0)
	findings = append(findings, validateMigratedArguments(canonical, production.Arguments)...)
	findings = append(findings, validateMigratedFlags(canonical, production.Flags)...)
	findings = append(findings, validateMigratedRelationships(canonical, production.Relationships)...)
	return findings
}

func validateMigratedArguments(
	canonical climanifest.Manifest,
	actual []cliinputs.ArgumentRecord,
) []Finding {
	expected := make(map[string]climanifest.Argument)
	paths := migratedCommandPaths(canonical)
	for id, command := range canonical.Commands {
		if !migratedCommandID(id) {
			continue
		}
		for inputID, argument := range command.Arguments {
			expected[inputID] = argument
		}
	}
	findings := make([]Finding, 0)
	seen := make(map[string]bool)
	for _, got := range actual {
		if !paths[got.CommandPath] {
			continue
		}
		want, ok := expected[got.IDCandidate]
		if !ok {
			findings = append(findings, newFinding(
				KindUncontractedInput, got.IDCandidate, got.CommandPath, "argument",
				"constructed positional argument is absent from the authored manifest",
			))
			continue
		}
		seen[got.IDCandidate] = true
		if field := firstArgumentMismatch(want, got); field != "" {
			findings = append(findings, newFinding(
				KindInputDrift, want.ID, got.CommandPath, field,
				"constructed positional argument metadata differs from the authored manifest",
			))
		}
	}
	for inputID, want := range expected {
		if !seen[inputID] {
			findings = append(findings, newFinding(
				KindMissingInput, want.ID, commandPathForInput(canonical, inputID), "argument",
				"authored positional argument is absent from the constructed command",
			))
		}
	}
	return findings
}

func validateMigratedFlags(
	canonical climanifest.Manifest,
	actual []cliinputs.FlagRecord,
) []Finding {
	expected := make(map[string]climanifest.Flag)
	paths := migratedCommandPaths(canonical)
	for id, command := range canonical.Commands {
		if !migratedCommandID(id) {
			continue
		}
		for inputID, flag := range command.Flags {
			expected[inputID] = flag
		}
	}
	findings := make([]Finding, 0)
	seen := make(map[string]bool)
	for _, got := range actual {
		if !paths[got.CommandPath] {
			continue
		}
		want, ok := expected[got.IDCandidate]
		if !ok {
			findings = append(findings, newFinding(
				KindUncontractedInput, got.IDCandidate, got.CommandPath, "flag",
				"constructed flag is absent from the authored manifest",
			))
			continue
		}
		seen[got.IDCandidate] = true
		if field := firstMigratedFlagMismatch(want, got); field != "" {
			findings = append(findings, newFinding(
				KindInputDrift, want.ID, got.CommandPath, field,
				"constructed flag metadata differs from the authored manifest",
			))
		}
	}
	for inputID, want := range expected {
		if !seen[inputID] {
			findings = append(findings, newFinding(
				KindMissingInput, want.ID, commandPathForInput(canonical, inputID), "flag",
				"authored flag is absent from the constructed command",
			))
		}
	}
	return findings
}

func firstArgumentMismatch(want climanifest.Argument, got cliinputs.ArgumentRecord) string {
	fields := []struct {
		name string
		want any
		got  any
	}{
		{"name", want.Name, got.Name},
		{"position", want.Position, got.Position},
		{"kind", want.Kind, got.Kind},
		{"valueType", want.ValueType, got.ValueType},
		{"required", want.Required, got.Required},
		{"minCardinality", want.MinCardinality, got.MinCardinality},
		{"maxCardinality", want.MaxCardinality, got.MaxCardinality},
		{"variadic", want.Variadic, got.Variadic},
		{"enum", normalizedStrings(want.Enum), normalizedStrings(got.Enum)},
		{"pattern", want.Pattern, got.Pattern},
		{"completion", want.Completion, got.CompletionKind},
		{"acceptedSources", normalizedStrings(want.AcceptedSources), normalizedStrings(got.InputChannels)},
		{"doubleDash", want.DoubleDash, got.DoubleDashHandling},
	}
	return firstMismatch(fields)
}

func firstMigratedFlagMismatch(want climanifest.Flag, got cliinputs.FlagRecord) string {
	fields := []struct {
		name string
		want any
		got  any
	}{
		{"long", want.Long, got.Long},
		{"shorthand", want.Shorthand, got.Shorthand},
		{"aliases", normalizedStrings(want.Aliases), normalizedStrings(got.Aliases)},
		{"scope", want.Scope, got.Scope},
		{"valueType", want.ValueType, got.ValueType},
		{"required", want.Required, got.Required},
		{"default", manifestInputValueString(want.DefaultValue, want.Default), got.Default},
		{"changedDefault", want.ChangedDefault, got.ChangedDefault},
		{"noOptionDefault", manifestInputValueString(want.NoOptionValue, want.NoOptionDefault), got.NoOptionDefault},
		{"repeatable", want.Repeatable, got.Repeatable},
		{"enum", normalizedStrings(want.Enum), normalizedStrings(got.Enum)},
		{"normalization", want.Normalization, got.Normalization},
		{"completion", want.Completion, got.CompletionKind},
		{"visibility", want.Visibility, got.Visibility},
		{"deprecated", want.Lifecycle.State == "deprecated", got.Deprecated},
		{"deprecatedMessage", want.Lifecycle.Deprecated, got.DeprecatedMessage},
	}
	return firstMismatch(fields)
}

func firstMismatch(fields []struct {
	name string
	want any
	got  any
}) string {
	for _, field := range fields {
		if !reflect.DeepEqual(field.want, field.got) {
			return field.name
		}
	}
	return ""
}

func validateMigratedRelationships(
	canonical climanifest.Manifest,
	actual []cliinputs.RelationshipRecord,
) []Finding {
	expected := make(map[string]expectedRelationship)
	paths := migratedCommandPaths(canonical)
	for id, command := range canonical.Commands {
		if !migratedCommandID(id) {
			continue
		}
		for _, relationship := range command.Relationships {
			item := expectedRelationship{
				id: relationship.ID, path: command.Path, kind: relationship.Kind,
				participants: relationshipParticipantNames(command, relationship),
			}
			expected[item.key()] = item
		}
	}
	findings := make([]Finding, 0)
	seen := make(map[string]bool)
	for _, got := range actual {
		if !paths[got.CommandPath] {
			continue
		}
		key := relationshipKey(got.CommandPath, got.Kind, got.Participants)
		_, ok := expected[key]
		if !ok {
			findings = append(findings, newFinding(
				KindUncontractedInput, got.IDCandidate, got.CommandPath, "relationship",
				"constructed input relationship is absent from the authored manifest",
			))
			continue
		}
		seen[key] = true
	}
	for key, want := range expected {
		if !seen[key] {
			findings = append(findings, newFinding(
				KindMissingInput, want.id, want.path, "relationship",
				"authored input relationship is absent from the constructed command",
			))
		}
	}
	return findings
}

type expectedRelationship struct {
	id           string
	path         string
	kind         string
	participants []string
}

func (relationship expectedRelationship) key() string {
	return relationshipKey(relationship.path, relationship.kind, relationship.participants)
}

func relationshipKey(path, kind string, participants []string) string {
	return path + "\x00" + kind + "\x00" + strings.Join(normalizedStrings(participants), "\x00")
}

func relationshipParticipantNames(
	command climanifest.Command,
	relationship climanifest.Relationship,
) []string {
	names := make([]string, 0, len(relationship.Participants))
	for _, participant := range relationship.Participants {
		switch participant.Type {
		case "flag":
			if flag, ok := command.Flags[participant.ID]; ok {
				names = append(names, flag.Long)
			}
		case "argument":
			if argument, ok := command.Arguments[participant.ID]; ok {
				names = append(names, argument.Name)
			}
		}
	}
	return normalizedStrings(names)
}

func migratedCommandPaths(canonical climanifest.Manifest) map[string]bool {
	paths := make(map[string]bool)
	for id, command := range canonical.Commands {
		if migratedCommandID(id) {
			paths[command.Path] = true
		}
	}
	return paths
}

func migratedCommandID(id string) bool {
	return id == "you.docs" ||
		id == "you.models" || strings.HasPrefix(id, "you.models.") ||
		id == "you.mcp" || strings.HasPrefix(id, "you.mcp.")
}

func commandPathForInput(canonical climanifest.Manifest, inputID string) string {
	for _, command := range canonical.Commands {
		if _, ok := command.Arguments[inputID]; ok {
			return command.Path
		}
		if _, ok := command.Flags[inputID]; ok {
			return command.Path
		}
	}
	return fmt.Sprintf("unknown input %s", inputID)
}

func normalizedStrings(values []string) []string {
	result := append([]string(nil), values...)
	if result == nil {
		result = []string{}
	}
	sort.Strings(result)
	return result
}
