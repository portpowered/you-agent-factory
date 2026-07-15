// Package clicontract validates the observable production CLI against its
// canonical and separately approved compatibility contracts.
package clicontract

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
)

const (
	KindUncontractedCommand = "uncontracted-command"
	KindMissingCommand      = "missing-command"
	KindStaleMetadata       = "stale-generated-metadata"
	KindMissingHandler      = "missing-handler"
	KindAliasAsCanonical    = "compatibility-alias-as-canonical"
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
	Canonical              climanifest.Manifest
	Compatibility          climanifest.Manifest
	ApprovedCompatibility  []CompatibilityRecord
	GeneratedCanonical     []climanifest.Manifest
	GeneratedCompatibility []climanifest.Manifest
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
	sortFindings(findings)
	return findings
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

func firstCommandMismatch(want, got climanifest.Command) string {
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
		{"relationships", want.Relationships, got.Relationships}, {"precedence", want.Precedence, got.Precedence},
		{"channels", want.Channels, got.Channels}, {"outputs", want.Outputs, got.Outputs},
		{"exits", want.Exits, got.Exits}, {"sideEffects", want.SideEffects, got.SideEffects},
		{"constraints", want.Constraints, got.Constraints}, {"handler", want.Handler, got.Handler},
	}
	for _, field := range fields {
		if !reflect.DeepEqual(field.want, field.got) {
			return field.name
		}
	}
	return ""
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
