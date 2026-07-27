// Package clicontract validates the observable production CLI against its
// canonical and separately approved compatibility contracts.
package clicontract

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
)

const (
	KindUncontractedCommand = "uncontracted-command"
	KindMissingCommand      = "missing-command"
	KindStaleMetadata       = "stale-generated-metadata"
	KindMissingHandler      = "missing-handler"
	KindAliasAsCanonical    = "compatibility-alias-as-canonical"
	KindUncontractedGlobal  = "uncontracted-root-global"
	KindMissingGlobal       = "missing-root-global"
	KindRootGlobalDrift     = "root-global-metadata-drift"
	KindPublishedDrift      = "published-metadata-drift"
)

// CompatibilityRecord identifies one approved compatibility-only CLI path.
type CompatibilityRecord struct {
	InventoryID    string
	StableID       string
	Path           string
	Classification string
}

// Input contains immutable snapshots used by the pure validator.
type Input struct {
	Production             commandidentity.Inventory
	ProductionInputs       cliinputs.Inventory
	Canonical              climanifest.Manifest
	Compatibility          climanifest.Manifest
	ApprovedCompatibility  []CompatibilityRecord
	GeneratedCanonical     []climanifest.Manifest
	GeneratedCompatibility []climanifest.Manifest
	PublishedCanonical     climanifest.Manifest
}

// Finding is one deterministic whole-tree contract violation.
type Finding struct {
	Kind     string
	StableID string
	Path     string
	Field    string
	Detail   string
}

func (f Finding) Error() string {
	location := fmt.Sprintf("stable ID %q path %q", f.StableID, f.Path)
	if f.Field != "" {
		location += fmt.Sprintf(" field %q", f.Field)
	}
	return fmt.Sprintf("%s: %s: %s", f.Kind, location, f.Detail)
}

type expectedCommand struct {
	id       string
	path     string
	record   *climanifest.Command
	approved bool
}

// Validate compares snapshots without executing commands or mutating inputs.
func Validate(input Input) []Finding {
	findings := make([]Finding, 0)
	expected := make(map[string]expectedCommand)
	approved := approvedIndex(input.ApprovedCompatibility)

	for key, command := range input.Canonical.Commands {
		command := command
		if compatibility, ok := approved[command.Path]; ok || approvedStableID(approved, command.ID) {
			if !ok {
				compatibility = CompatibilityRecord{StableID: command.ID, Path: command.Path}
			}
			findings = append(findings, newFinding(
				KindAliasAsCanonical, compatibility.StableID, compatibility.Path, "classification",
				"approved compatibility command is present in the canonical manifest",
			))
		}
		if key != command.ID {
			findings = append(findings, newFinding(KindStaleMetadata, key, command.Path, "id", "canonical map key differs from command ID"))
		}
		expected[command.Path] = expectedCommand{id: command.ID, path: command.Path, record: &command}
	}

	for _, compatibility := range input.ApprovedCompatibility {
		addExpectedCompatibilityAncestors(expected, compatibility.Path)
		expected[compatibility.Path] = expectedCommand{
			id: compatibility.StableID, path: compatibility.Path, approved: true,
		}
	}
	for id, command := range input.Compatibility.Commands {
		command := command
		approval, ok := approved[command.Path]
		if !ok || approval.StableID != command.ID {
			findings = append(findings, newFinding(
				KindUncontractedCommand, id, command.Path, "classification",
				"compatibility manifest command is not separately approved",
			))
			continue
		}
		expected[command.Path] = expectedCommand{id: command.ID, path: command.Path, record: &command, approved: true}
	}

	seen := make(map[string]bool, len(input.Production.Commands))
	for _, actual := range input.Production.Commands {
		want, ok := expected[actual.Path]
		if !ok || want.id != actual.IDCandidate {
			findings = append(findings, newFinding(
				KindUncontractedCommand, actual.IDCandidate, actual.Path, "",
				"public production command is absent from canonical and approved compatibility contracts",
			))
			continue
		}
		seen[actual.Path] = true
		findings = append(findings, validateProductionCommand(actual, want)...)
	}
	for path, want := range expected {
		if !seen[path] {
			findings = append(findings, newFinding(
				KindMissingCommand, want.id, path, "", "contracted command is absent from the production tree",
			))
		}
	}

	findings = append(findings, validateGeneratedCanonical(input.Canonical, approved, input.GeneratedCanonical)...)
	findings = append(findings, validateGeneratedCompatibility(input.Compatibility, approved, input.GeneratedCompatibility)...)
	findings = append(findings, validatePublishedCanonical(input.Canonical, input.PublishedCanonical)...)
	findings = append(findings, validateRootGlobals(input.Canonical, input.ProductionInputs)...)
	findings = append(findings, validateMigratedInputs(input.Canonical, input.ProductionInputs)...)
	sortFindings(findings)
	return findings
}

func validateRootGlobals(
	canonical climanifest.Manifest,
	production cliinputs.Inventory,
) []Finding {
	root, ok := canonical.Commands[canonical.RootPath]
	if !ok {
		return []Finding{newFinding(
			KindMissingGlobal,
			canonical.RootPath,
			canonical.RootPath,
			"root",
			"canonical root command is missing",
		)}
	}
	expected := make(map[string]climanifest.Flag)
	for id, flag := range root.Flags {
		if flag.Scope == "persistent" && flag.Lifecycle.State == "active" {
			expected[id] = flag
		}
	}
	actual := make(map[string]cliinputs.FlagRecord)
	findings := make([]Finding, 0)
	for _, flag := range production.Flags {
		if flag.CommandPath != root.Path || flag.Scope != "persistent" {
			continue
		}
		if _, exists := actual[flag.IDCandidate]; exists {
			findings = append(findings, newFinding(
				KindUncontractedGlobal,
				flag.IDCandidate,
				root.Path,
				"long",
				"executable root contains duplicate persistent input identity",
			))
			continue
		}
		actual[flag.IDCandidate] = flag
	}
	for id, got := range actual {
		want, exists := expected[id]
		if !exists {
			findings = append(findings, newFinding(
				KindUncontractedGlobal,
				id,
				root.Path,
				"long",
				fmt.Sprintf("executable root persistent flag %q is absent from the authored manifest", got.Long),
			))
			continue
		}
		if field := firstRootGlobalMismatch(want, got); field != "" {
			findings = append(findings, newFinding(
				KindRootGlobalDrift,
				id,
				root.Path,
				field,
				"executable root persistent flag metadata differs from the authored manifest",
			))
		}
	}
	for id, want := range expected {
		if _, exists := actual[id]; !exists {
			findings = append(findings, newFinding(
				KindMissingGlobal,
				id,
				root.Path,
				"long",
				fmt.Sprintf("authored root persistent flag %q is absent from the executable root", want.Long),
			))
		}
	}
	return findings
}

func firstRootGlobalMismatch(want climanifest.Flag, got cliinputs.FlagRecord) string {
	aliases := append([]string(nil), want.Aliases...)
	if aliases == nil {
		aliases = []string{}
	}
	sort.Strings(aliases)
	fields := []struct {
		name string
		want any
		got  any
	}{
		{"long", want.Long, got.Long},
		{"shorthand", want.Shorthand, got.Shorthand},
		{"aliases", aliases, got.Aliases},
		{"scope", want.Scope, got.Scope},
		{"valueType", want.ValueType, got.ValueType},
		{"required", want.Required, got.Required},
		{"default", manifestInputValueString(want.DefaultValue, want.Default), got.Default},
		{"changedDefault", want.ChangedDefault, got.ChangedDefault},
		{"noOptionDefault", manifestInputValueString(want.NoOptionValue, want.NoOptionDefault), got.NoOptionDefault},
		{"repeatable", want.Repeatable, got.Repeatable},
		{"normalization", want.Normalization, got.Normalization},
		{"completion", want.Completion, got.CompletionKind},
		{"visibility", want.Visibility, got.Visibility},
		{"deprecated", want.Lifecycle.State == "deprecated", got.Deprecated},
		{"deprecatedMessage", want.Lifecycle.Deprecated, got.DeprecatedMessage},
	}
	for _, field := range fields {
		if !reflect.DeepEqual(field.want, field.got) {
			return field.name
		}
	}
	return ""
}

func manifestInputValueString(value *climanifest.InputValue, fallback string) string {
	if value == nil {
		return fallback
	}
	switch {
	case value.Boolean != nil:
		return fmt.Sprint(*value.Boolean)
	case value.String != nil:
		return *value.String
	case value.Int != nil:
		return fmt.Sprint(*value.Int)
	case value.Int64 != nil:
		return fmt.Sprint(*value.Int64)
	case value.StringArray != nil:
		return fmt.Sprint(*value.StringArray)
	default:
		return fallback
	}
}

func validateProductionCommand(actual commandidentity.CommandRecord, want expectedCommand) []Finding {
	findings := make([]Finding, 0)
	requiresHandler := actual.Runnable
	if want.record != nil && want.record.Runnable {
		requiresHandler = true
	}
	if requiresHandler && !actual.HandlerPresent {
		findings = append(findings, newFinding(KindMissingHandler, want.id, want.path, "handler", "runnable production command has no handwritten handler"))
	}
	if want.record == nil {
		return findings
	}
	record := *want.record
	if record.Runnable && (record.Handler == nil || strings.TrimSpace(record.Handler.ID) == "") {
		findings = append(findings, newFinding(KindMissingHandler, want.id, want.path, "handler.id", "runnable contracted command has no stable handler ID"))
	}
	return findings
}

func validateGeneratedCanonical(canonical climanifest.Manifest, approved map[string]CompatibilityRecord, manifests []climanifest.Manifest) []Finding {
	findings := make([]Finding, 0)
	covered := make(map[string]bool, len(canonical.Commands))
	for _, manifest := range manifests {
		for id, generated := range manifest.Commands {
			if approval, ok := approved[generated.Path]; ok || approvedStableID(approved, id) {
				if !ok {
					approval = CompatibilityRecord{StableID: id, Path: generated.Path}
				}
				findings = append(findings, newFinding(KindAliasAsCanonical, approval.StableID, approval.Path, "classification", "compatibility command is present in a canonical generated family"))
				continue
			}
			want, ok := canonical.Commands[id]
			if !ok {
				findings = append(findings, newFinding(KindUncontractedCommand, id, generated.Path, "generated", "canonical generated family contains an uncontracted command"))
				continue
			}
			covered[id] = true
			if field := firstCommandMismatch(want, generated); field != "" {
				findings = append(findings, newFinding(KindStaleMetadata, id, want.Path, field, "generated canonical metadata differs from the authored contract"))
			}
		}
	}
	for id, command := range canonical.Commands {
		if !covered[id] {
			findings = append(findings, newFinding(KindStaleMetadata, id, command.Path, "generated", "canonical command is missing from generated family metadata"))
		}
	}
	return findings
}

func validateGeneratedCompatibility(compatibility climanifest.Manifest, approved map[string]CompatibilityRecord, manifests []climanifest.Manifest) []Finding {
	findings := make([]Finding, 0)
	covered := make(map[string]bool, len(compatibility.Commands))
	for _, manifest := range manifests {
		for id, generated := range manifest.Commands {
			approval, ok := approved[generated.Path]
			if !ok || approval.StableID != id {
				findings = append(findings, newFinding(KindUncontractedCommand, id, generated.Path, "classification", "generated compatibility command is not separately approved"))
				continue
			}
			want, ok := compatibility.Commands[id]
			if !ok {
				findings = append(findings, newFinding(KindStaleMetadata, id, generated.Path, "generated", "approved command is absent from the compatibility metadata manifest"))
				continue
			}
			covered[id] = true
			if field := firstCommandMismatch(want, generated); field != "" {
				findings = append(findings, newFinding(KindStaleMetadata, id, want.Path, field, "generated compatibility metadata differs from the authored contract"))
			}
		}
	}
	for id, command := range compatibility.Commands {
		if !covered[id] {
			findings = append(findings, newFinding(KindStaleMetadata, id, command.Path, "generated", "compatibility command is missing from generated family metadata"))
		}
	}
	return findings
}

func validatePublishedCanonical(canonical, published climanifest.Manifest) []Finding {
	if published.RootPath == "" && len(published.Commands) == 0 {
		return nil
	}
	findings := make([]Finding, 0)
	if published.FormatVersion != canonical.FormatVersion {
		findings = append(findings, newFinding(
			KindPublishedDrift, canonical.RootPath, canonical.RootPath, "formatVersion",
			"published CLI format version differs from the authored contract",
		))
	}
	if published.RootPath != canonical.RootPath {
		findings = append(findings, newFinding(
			KindPublishedDrift, canonical.RootPath, canonical.RootPath, "rootPath",
			"published CLI root differs from the authored contract",
		))
	}
	for id, command := range published.Commands {
		want, ok := canonical.Commands[id]
		if !ok {
			findings = append(findings, newFinding(
				KindPublishedDrift, id, command.Path, "published",
				"published CLI contains an uncontracted command",
			))
			continue
		}
		if field := firstCommandMismatch(want, command); field != "" {
			findings = append(findings, newFinding(
				KindPublishedDrift, id, want.Path, field,
				"published CLI metadata differs from the authored contract",
			))
		}
	}
	for id, command := range canonical.Commands {
		if _, ok := published.Commands[id]; !ok {
			findings = append(findings, newFinding(
				KindPublishedDrift, id, command.Path, "published",
				"authored command is missing from the published CLI contract",
			))
		}
	}
	return findings
}

func firstCommandMismatch(want, got climanifest.Command) string {
	want = normalizeCommandSets(want)
	got = normalizeCommandSets(got)
	fields := []struct {
		name string
		want any
		got  any
	}{
		{"id", want.ID, got.ID}, {"name", want.Name, got.Name}, {"path", want.Path, got.Path},
		{"aliases", want.Aliases, got.Aliases}, {"completeness", want.Completeness, got.Completeness},
		{"groupId", want.GroupID, got.GroupID}, {"documentation", want.Documentation, got.Documentation},
		{"lifecycle", want.Lifecycle, got.Lifecycle}, {"visibility", want.Visibility, got.Visibility},
		{"runnable", want.Runnable, got.Runnable}, {"usage", want.Usage, got.Usage},
		{"arguments", want.Arguments, got.Arguments}, {"flags", want.Flags, got.Flags},
		{"sourceBindings", want.SourceBindings, got.SourceBindings},
		{"handlerBindings", want.HandlerBindings, got.HandlerBindings},
		{"relationships", want.Relationships, got.Relationships}, {"precedence", want.Precedence, got.Precedence},
		{"channels", want.Channels, got.Channels}, {"outputs", want.Outputs, got.Outputs},
		{"errors", want.Errors, got.Errors}, {"exits", want.Exits, got.Exits},
		{"sideEffects", want.SideEffects, got.SideEffects},
		{"constraints", want.Constraints, got.Constraints}, {"handler", want.Handler, got.Handler},
		{"rootLifecycle", want.RootLifecycle, got.RootLifecycle},
	}
	for _, field := range fields {
		if !reflect.DeepEqual(field.want, field.got) {
			return field.name
		}
	}
	return ""
}

func normalizeCommandSets(command climanifest.Command) climanifest.Command {
	command.Aliases = normalizedStrings(command.Aliases)
	command.Documentation.Examples = normalizedStrings(command.Documentation.Examples)
	command.Channels.Input = normalizedStrings(command.Channels.Input)
	command.Channels.Output = normalizedStrings(command.Channels.Output)
	command.Constraints.Platforms = normalizedStrings(command.Constraints.Platforms)
	command.Constraints.Runtime = normalizedStrings(command.Constraints.Runtime)
	command.Arguments = cloneArgumentsWithNormalizedSets(command.Arguments)
	command.Flags = cloneFlagsWithNormalizedSets(command.Flags)
	command.Relationships = cloneRelationshipsWithNormalizedSets(command.Relationships)
	return command
}

func cloneArgumentsWithNormalizedSets(source map[string]climanifest.Argument) map[string]climanifest.Argument {
	if source == nil {
		return nil
	}
	result := make(map[string]climanifest.Argument, len(source))
	for id, argument := range source {
		argument.Enum = normalizedStrings(argument.Enum)
		argument.Channels = normalizedStrings(argument.Channels)
		argument.AcceptedSources = normalizedStrings(argument.AcceptedSources)
		result[id] = argument
	}
	return result
}

func cloneFlagsWithNormalizedSets(source map[string]climanifest.Flag) map[string]climanifest.Flag {
	if source == nil {
		return nil
	}
	result := make(map[string]climanifest.Flag, len(source))
	for id, flag := range source {
		flag.Aliases = normalizedStrings(flag.Aliases)
		flag.Enum = normalizedStrings(flag.Enum)
		flag.AcceptedSources = normalizedStrings(flag.AcceptedSources)
		result[id] = flag
	}
	return result
}

func cloneRelationshipsWithNormalizedSets(source map[string]climanifest.Relationship) map[string]climanifest.Relationship {
	if source == nil {
		return nil
	}
	result := make(map[string]climanifest.Relationship, len(source))
	for id, relationship := range source {
		relationship.Participants = append([]climanifest.ParticipantRef(nil), relationship.Participants...)
		sort.Slice(relationship.Participants, func(left, right int) bool {
			if relationship.Participants[left].Type != relationship.Participants[right].Type {
				return relationship.Participants[left].Type < relationship.Participants[right].Type
			}
			return relationship.Participants[left].ID < relationship.Participants[right].ID
		})
		result[id] = relationship
	}
	return result
}

func approvedIndex(records []CompatibilityRecord) map[string]CompatibilityRecord {
	result := make(map[string]CompatibilityRecord, len(records))
	for _, record := range records {
		result[record.Path] = record
	}
	return result
}

func approvedStableID(records map[string]CompatibilityRecord, stableID string) bool {
	for _, record := range records {
		if record.StableID == stableID {
			return true
		}
	}
	return false
}

func addExpectedCompatibilityAncestors(expected map[string]expectedCommand, path string) {
	parts := strings.Fields(path)
	for size := 1; size < len(parts); size++ {
		ancestor := strings.Join(parts[:size], " ")
		if _, exists := expected[ancestor]; exists {
			continue
		}
		expected[ancestor] = expectedCommand{id: strings.Join(parts[:size], "."), path: ancestor, approved: true}
	}
}

func newFinding(kind, stableID, path, field, detail string) Finding {
	return Finding{Kind: kind, StableID: stableID, Path: path, Field: field, Detail: detail}
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		left := findings[i]
		right := findings[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.StableID != right.StableID {
			return left.StableID < right.StableID
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Field != right.Field {
			return left.Field < right.Field
		}
		return left.Detail < right.Detail
	})
}
