package factorysession_test

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestProjectAliasInventory_BuildsDocumentShape(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	if inventory.FormatVersion != mcpfactorysession.ToolInventoryFormatVersion {
		t.Fatalf("formatVersion = %q, want %q", inventory.FormatVersion, mcpfactorysession.ToolInventoryFormatVersion)
	}
	if inventory.ProtocolVersion != mcpfactorysession.ToolInventoryProtocolVersion {
		t.Fatalf("protocolVersion = %q, want %q", inventory.ProtocolVersion, mcpfactorysession.ToolInventoryProtocolVersion)
	}
	if len(inventory.Aliases) != len(mcpfactorysession.DiscoverCompatibilityAliases()) {
		t.Fatalf("alias count = %d, want %d", len(inventory.Aliases), len(mcpfactorysession.DiscoverCompatibilityAliases()))
	}
}

func TestProjectAliasInventory_AliasesSortedByName(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	names := make([]string, len(inventory.Aliases))
	for i, alias := range inventory.Aliases {
		names[i] = alias.Name
	}
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	if !slices.Equal(names, sorted) {
		t.Fatalf("alias names = %#v, want sorted %#v", names, sorted)
	}
}

func TestProjectAliasInventory_EachAliasHasMappingFields(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	for _, alias := range inventory.Aliases {
		if strings.TrimSpace(alias.Name) == "" {
			t.Fatal("alias name is required")
		}
		if strings.TrimSpace(alias.CanonicalName) == "" {
			t.Fatalf("alias %q canonical name is required", alias.Name)
		}
		if strings.TrimSpace(alias.Description) == "" {
			t.Fatalf("alias %q description is required", alias.Name)
		}
		if !alias.CompatibilityOnly {
			t.Fatalf("alias %q compatibilityOnly = false, want true", alias.Name)
		}
		if !strings.HasPrefix(alias.Name, "you.workflow.") {
			t.Fatalf("alias %q should use workflow vocabulary", alias.Name)
		}
		if !strings.HasPrefix(alias.CanonicalName, "you.factory_session.") {
			t.Fatalf("alias %q canonical target %q should use Factory Session vocabulary", alias.Name, alias.CanonicalName)
		}
	}
}

func TestProjectAliasInventory_MatchesDiscoverCompatibilityAliases(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	byName := aliasInventoryByName(t, inventory)
	for _, discovered := range mcpfactorysession.DiscoverCompatibilityAliases() {
		entry, ok := byName[discovered.Name]
		if !ok {
			t.Fatalf("inventory missing compatibility alias %q", discovered.Name)
		}
		if entry.CanonicalName != discovered.CanonicalName {
			t.Fatalf("alias %q canonicalName = %q, want %q", discovered.Name, entry.CanonicalName, discovered.CanonicalName)
		}
		if entry.CompatibilityOnly != discovered.CompatibilityOnly {
			t.Fatalf("alias %q compatibilityOnly = %v, want %v", discovered.Name, entry.CompatibilityOnly, discovered.CompatibilityOnly)
		}
		if entry.Description != discovered.Description {
			t.Fatalf("alias %q description = %q, want %q", discovered.Name, entry.Description, discovered.Description)
		}
	}
}

func TestProjectAliasInventory_RepeatExtractionIsByteIdentical(t *testing.T) {
	first, err := mcpfactorysession.MarshalAliasInventoryJSON(mustProjectAliasInventory(t))
	if err != nil {
		t.Fatalf("first MarshalAliasInventoryJSON() error = %v", err)
	}
	second, err := mcpfactorysession.MarshalAliasInventoryJSON(mustProjectAliasInventory(t))
	if err != nil {
		t.Fatalf("second MarshalAliasInventoryJSON() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat extraction differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestVerifyProjectedAliasInventory_PassesForLiveRegistry(t *testing.T) {
	if err := mcpfactorysession.VerifyProjectedAliasInventory(); err != nil {
		t.Fatalf("VerifyProjectedAliasInventory() error = %v", err)
	}
}

func TestVerifyAliasInventory_RejectsAliasInCanonicalToolInventory(t *testing.T) {
	inventory := mcpfactorysession.AliasInventory{
		FormatVersion:   mcpfactorysession.ToolInventoryFormatVersion,
		ProtocolVersion: mcpfactorysession.ToolInventoryProtocolVersion,
		Aliases: []mcpfactorysession.AliasInventoryEntry{{
			Name:              mcpfactorysession.ToolWorkflowValidate,
			CanonicalName:     mcpfactorysession.ToolValidateSource,
			CompatibilityOnly: true,
			Description:       "probe alias that leaked into canonical inventory",
		}},
	}
	toolInventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	for i := range toolInventory.Tools {
		if toolInventory.Tools[i].Name == mcpfactorysession.ToolWorkflowValidate {
			t.Fatal("canonical tool inventory should not contain workflow alias names")
		}
	}
	if err := mcpfactorysession.VerifyAliasInventory(inventory); err != nil {
		t.Fatalf("VerifyAliasInventory() error = %v", err)
	}
}

func TestVerifyAliasInventory_FailsWhenCanonicalTargetMissing(t *testing.T) {
	inventory := mcpfactorysession.AliasInventory{
		FormatVersion:   mcpfactorysession.ToolInventoryFormatVersion,
		ProtocolVersion: mcpfactorysession.ToolInventoryProtocolVersion,
		Aliases: []mcpfactorysession.AliasInventoryEntry{{
			Name:              "you.workflow.probe",
			CanonicalName:     "you.factory_session.unregistered_probe",
			CompatibilityOnly: true,
			Description:       "probe alias with missing canonical target",
		}},
	}
	err := mcpfactorysession.VerifyAliasInventory(inventory)
	if err == nil {
		t.Fatal("VerifyAliasInventory() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "you.factory_session.unregistered_probe") {
		t.Fatalf("VerifyAliasInventory() error = %v, want missing canonical target", err)
	}
}

func TestVerifyAliasInventory_FailsWhenResolveToolNameDisagrees(t *testing.T) {
	inventory := mcpfactorysession.AliasInventory{
		FormatVersion:   mcpfactorysession.ToolInventoryFormatVersion,
		ProtocolVersion: mcpfactorysession.ToolInventoryProtocolVersion,
		Aliases: []mcpfactorysession.AliasInventoryEntry{{
			Name:              mcpfactorysession.ToolWorkflowValidate,
			CanonicalName:     mcpfactorysession.ToolStartSync,
			CompatibilityOnly: true,
			Description:       "probe alias with incorrect canonical mapping",
		}},
	}
	err := mcpfactorysession.VerifyAliasInventory(inventory)
	if err == nil {
		t.Fatal("VerifyAliasInventory() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), mcpfactorysession.ToolWorkflowValidate) {
		t.Fatalf("VerifyAliasInventory() error = %v, want alias name", err)
	}
}

func TestAliasBaselineFixtureMatchesProjectedInventory(t *testing.T) {
	baselinePath := testutil.MustRepoPath(t, mcpfactorysession.AliasInventoryBaselineRelativePath)
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline fixture: %v", err)
	}
	projected, err := mcpfactorysession.MarshalAliasInventoryJSON(mustProjectAliasInventory(t))
	if err != nil {
		t.Fatalf("MarshalAliasInventoryJSON() error = %v", err)
	}
	if string(baseline) != string(projected) {
		t.Fatalf("baseline fixture differs from projected inventory:\nbaseline=%s\nprojected=%s", baseline, projected)
	}
}

func TestAliasBaselineFixtureMatchesDiscoverCompatibilityAliases(t *testing.T) {
	baselinePath := testutil.MustRepoPath(t, mcpfactorysession.AliasInventoryBaselineRelativePath)
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline fixture: %v", err)
	}
	var inventory mcpfactorysession.AliasInventory
	if err := json.Unmarshal(baseline, &inventory); err != nil {
		t.Fatalf("unmarshal baseline fixture: %v", err)
	}
	if err := mcpfactorysession.VerifyAliasInventory(inventory); err != nil {
		t.Fatalf("VerifyAliasInventory(baseline) error = %v", err)
	}
	byName := aliasInventoryByName(t, inventory)
	for _, discovered := range mcpfactorysession.DiscoverCompatibilityAliases() {
		entry, ok := byName[discovered.Name]
		if !ok {
			t.Fatalf("baseline missing compatibility alias %q", discovered.Name)
		}
		if entry.CanonicalName != discovered.CanonicalName {
			t.Fatalf("baseline alias %q canonicalName = %q, want %q", discovered.Name, entry.CanonicalName, discovered.CanonicalName)
		}
	}
}

func TestAliasBaselineFixture_AliasNamesAbsentFromCanonicalToolsBaseline(t *testing.T) {
	aliasBaselinePath := testutil.MustRepoPath(t, mcpfactorysession.AliasInventoryBaselineRelativePath)
	toolBaselinePath := testutil.MustRepoPath(t, mcpfactorysession.ToolInventoryBaselineRelativePath)

	aliasBaseline, err := os.ReadFile(aliasBaselinePath)
	if err != nil {
		t.Fatalf("read alias baseline fixture: %v", err)
	}
	toolBaseline, err := os.ReadFile(toolBaselinePath)
	if err != nil {
		t.Fatalf("read tool baseline fixture: %v", err)
	}

	var aliasInventory mcpfactorysession.AliasInventory
	if err := json.Unmarshal(aliasBaseline, &aliasInventory); err != nil {
		t.Fatalf("unmarshal alias baseline fixture: %v", err)
	}
	var toolInventory mcpfactorysession.ToolInventory
	if err := json.Unmarshal(toolBaseline, &toolInventory); err != nil {
		t.Fatalf("unmarshal tool baseline fixture: %v", err)
	}

	canonicalNames := make(map[string]struct{}, len(toolInventory.Tools))
	for _, tool := range toolInventory.Tools {
		canonicalNames[tool.Name] = struct{}{}
	}
	for _, alias := range aliasInventory.Aliases {
		if _, ok := canonicalNames[alias.Name]; ok {
			t.Fatalf("compatibility alias %q must not appear in canonical mcp-tools.json baseline", alias.Name)
		}
	}
}

func mustProjectAliasInventory(t *testing.T) mcpfactorysession.AliasInventory {
	t.Helper()
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	return inventory
}

func aliasInventoryByName(t *testing.T, inventory mcpfactorysession.AliasInventory) map[string]mcpfactorysession.AliasInventoryEntry {
	t.Helper()
	byName := make(map[string]mcpfactorysession.AliasInventoryEntry, len(inventory.Aliases))
	for _, alias := range inventory.Aliases {
		byName[alias.Name] = alias
	}
	return byName
}
