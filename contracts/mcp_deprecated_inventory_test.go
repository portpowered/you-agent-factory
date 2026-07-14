package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const mcpDeprecatedInventoryPath = "mcp/deprecated.json"

func TestMCPDeprecatedInventoryValidatesAgainstSchema(t *testing.T) {
	schema := compileSchema(
		t,
		"compatibility-inventory.schema.json",
		compatibilityInventorySchemaID,
		schemaResource{
			path: filepath.Join("common", "compatibility-inventory.schema.json"),
			id:   compatibilityVocabularySchemaID,
		},
		schemaResource{
			path: filepath.Join("common", "documentation.schema.json"),
			id:   documentationSchemaID,
		},
		schemaResource{
			path: filepath.Join("common", "deprecations.schema.json"),
			id:   deprecationsSchemaID,
		},
	)

	instance := readJSON(t, mcpDeprecatedInventoryPath)
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("validate MCP deprecated inventory: %v", err)
	}
	diagnostics := contractvalidator.CompatibilityInventorySemanticsDiagnostics(mcpDeprecatedInventoryPath, instance)
	if len(diagnostics) != 0 {
		t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
	}
}

func TestMCPDeprecatedInventoryClassifiesEveryBaselineAlias(t *testing.T) {
	baseline := readMCPBaselineAliases(t)
	inventory := readMCPDeprecatedInventory(t)

	if inventory.Family != "mcp" {
		t.Fatalf("inventory family = %q, want mcp", inventory.Family)
	}

	for _, alias := range baseline.Aliases {
		if !alias.CompatibilityOnly {
			t.Fatalf("baseline alias %q is not marked compatibility-only", alias.Name)
		}
		record, ok := inventory.Records["mcp.alias."+alias.Name]
		if !ok {
			t.Fatalf("missing inventory record for baseline alias %q", alias.Name)
		}
		if record.PublicName != alias.Name {
			t.Fatalf("record publicName = %q, want %q", record.PublicName, alias.Name)
		}
		if record.Classification == "" {
			t.Fatalf("record for %q missing classification", alias.Name)
		}
		if record.Lifecycle.Successor.TargetItemID != "mcp.tool."+alias.CanonicalName {
			t.Fatalf(
				"record successor targetItemId = %q, want %q",
				record.Lifecycle.Successor.TargetItemID,
				"mcp.tool."+alias.CanonicalName,
			)
		}
		if record.Lifecycle.Successor.CanonicalEnglish == "" {
			t.Fatalf("record for %q missing successor migration guidance", alias.Name)
		}
		if record.Evidence.Summary == "" {
			t.Fatalf("record for %q missing evidence summary", alias.Name)
		}
		if len(record.RemovalGates) == 0 {
			t.Fatalf("record for %q missing removal gates", alias.Name)
		}
		if record.ApprovalStatus == "" {
			t.Fatalf("record for %q missing approval status", alias.Name)
		}
	}

	if len(inventory.Records) != len(baseline.Aliases) {
		t.Fatalf("inventory record count = %d, want %d baseline aliases", len(inventory.Records), len(baseline.Aliases))
	}
}

type mcpBaselineAliasesDocument struct {
	Aliases []mcpBaselineAlias `json:"aliases"`
}

type mcpBaselineAlias struct {
	Name              string `json:"name"`
	CanonicalName     string `json:"canonicalName"`
	CompatibilityOnly bool   `json:"compatibilityOnly"`
}

type mcpDeprecatedInventoryDocument struct {
	Family  string                           `json:"family"`
	Records map[string]mcpCompatibilityRecord `json:"records"`
}

type mcpCompatibilityRecord struct {
	PublicName     string `json:"publicName"`
	Classification string `json:"classification"`
	ApprovalStatus string `json:"approvalStatus"`
	Lifecycle      struct {
		Successor struct {
			TargetItemID     string `json:"targetItemId"`
			CanonicalEnglish string `json:"canonicalEnglish"`
		} `json:"successor"`
	} `json:"lifecycle"`
	Evidence struct {
		Summary string `json:"summary"`
	} `json:"evidence"`
	RemovalGates []struct {
		ID string `json:"id"`
	} `json:"removalGates"`
}

func readMCPBaselineAliases(t *testing.T) mcpBaselineAliasesDocument {
	t.Helper()
	return decodeContractJSON[mcpBaselineAliasesDocument](t, filepath.Join("testdata", "baseline", "mcp-aliases.json"))
}

func readMCPDeprecatedInventory(t *testing.T) mcpDeprecatedInventoryDocument {
	t.Helper()
	return decodeContractJSON[mcpDeprecatedInventoryDocument](t, mcpDeprecatedInventoryPath)
}

func decodeContractJSON[T any](t *testing.T, path string) T {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value T
	if err := json.Unmarshal(contents, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}
