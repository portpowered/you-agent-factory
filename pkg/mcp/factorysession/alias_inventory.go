package factorysession

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	// AliasInventoryBaselineRelativePath is the reviewed MCP compatibility alias inventory fixture.
	AliasInventoryBaselineRelativePath = "contracts/testdata/baseline/mcp-aliases.json"
)

// AliasInventory is a pure, read-only projection of workflow compatibility aliases.
type AliasInventory struct {
	FormatVersion   string                `json:"formatVersion"`
	ProtocolVersion string                `json:"protocolVersion"`
	Aliases         []AliasInventoryEntry `json:"aliases"`
}

// AliasInventoryEntry records one compatibility alias mapped to its canonical tool.
type AliasInventoryEntry struct {
	Name              string `json:"name"`
	CanonicalName     string `json:"canonicalName"`
	CompatibilityOnly bool   `json:"compatibilityOnly"`
	Description       string `json:"description"`
}

// ProjectAliasInventory builds a sorted alias inventory from DiscoverCompatibilityAliases.
func ProjectAliasInventory() (AliasInventory, error) {
	return ProjectAliasInventoryFromDiscovered(DiscoverCompatibilityAliases())
}

// ProjectAliasInventoryFromDiscovered builds one alias inventory document from an
// explicit compatibility-alias slice. Production callers should use ProjectAliasInventory.
func ProjectAliasInventoryFromDiscovered(aliases []CompatibilityAlias) (AliasInventory, error) {
	entries := make([]AliasInventoryEntry, 0, len(aliases))
	for _, alias := range aliases {
		entries = append(entries, AliasInventoryEntry{
			Name:              alias.Name,
			CanonicalName:     alias.CanonicalName,
			CompatibilityOnly: alias.CompatibilityOnly,
			Description:       alias.Description,
		})
	}
	slices.SortFunc(entries, func(left, right AliasInventoryEntry) int {
		return strings.Compare(left.Name, right.Name)
	})
	return AliasInventory{
		FormatVersion:   ToolInventoryFormatVersion,
		ProtocolVersion: ToolInventoryProtocolVersion,
		Aliases:         entries,
	}, nil
}

// MarshalAliasInventoryJSON encodes one alias inventory document with stable map key order.
func MarshalAliasInventoryJSON(inventory AliasInventory) ([]byte, error) {
	return json.Marshal(inventory)
}

// VerifyProjectedAliasInventory projects the alias inventory and fails when any alias
// does not resolve exactly once to a discovered canonical tool or appears in the
// canonical tool inventory baseline.
func VerifyProjectedAliasInventory() error {
	inventory, err := ProjectAliasInventory()
	if err != nil {
		return err
	}
	return VerifyAliasInventory(inventory)
}

// VerifyAliasInventory fails when any alias does not resolve to a discovered canonical
// tool, when ResolveToolName disagrees with the recorded mapping, or when an alias name
// appears in the canonical tool inventory.
func VerifyAliasInventory(inventory AliasInventory) error {
	canonicalNames := canonicalToolNameSet()
	toolInventory, err := ProjectToolInventory()
	if err != nil {
		return err
	}
	canonicalInventoryNames := make(map[string]struct{}, len(toolInventory.Tools))
	for _, tool := range toolInventory.Tools {
		canonicalInventoryNames[tool.Name] = struct{}{}
	}

	seenAliasNames := make(map[string]struct{}, len(inventory.Aliases))
	for _, alias := range inventory.Aliases {
		if _, duplicate := seenAliasNames[alias.Name]; duplicate {
			return fmt.Errorf("compatibility alias %q appears more than once in alias inventory", alias.Name)
		}
		seenAliasNames[alias.Name] = struct{}{}

		if _, inCanonicalInventory := canonicalInventoryNames[alias.Name]; inCanonicalInventory {
			return fmt.Errorf("compatibility alias %q must not appear in canonical tool inventory", alias.Name)
		}
		if !alias.CompatibilityOnly {
			return fmt.Errorf("compatibility alias %q must be marked compatibility-only", alias.Name)
		}
		if ResolveToolName(alias.Name) != alias.CanonicalName {
			return fmt.Errorf(
				"compatibility alias %q resolves to %q, want %q",
				alias.Name,
				ResolveToolName(alias.Name),
				alias.CanonicalName,
			)
		}
		if _, ok := canonicalNames[alias.CanonicalName]; !ok {
			return fmt.Errorf(
				"compatibility alias %q canonical target %q is not discoverable",
				alias.Name,
				alias.CanonicalName,
			)
		}
	}
	return nil
}

func canonicalToolNameSet() map[string]struct{} {
	tools := DiscoverTools()
	names := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		names[tool.Name] = struct{}{}
	}
	return names
}
