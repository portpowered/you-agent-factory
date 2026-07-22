package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	mcpDeprecatedInventoryPath = "mcp/deprecated.json"
	cliDeprecatedInventoryPath = "cli/deprecated.json"
	apiDeprecatedInventoryPath = "api/deprecated.json"
)

var supportedCompatibilityClassifications = map[string]struct{}{
	"retain-temporarily":  {},
	"remove-now":          {},
	"separately-approved": {},
}

func TestCompatibilityInventoryBaselineCoverage(t *testing.T) {
	t.Run("mcp", func(t *testing.T) {
		assertMCPCompatibilityInventoryBaselineCoverage(t)
	})
	t.Run("cli", func(t *testing.T) {
		assertCLICompatibilityInventoryBaselineCoverage(t)
	})
	t.Run("api", func(t *testing.T) {
		assertAPICompatibilityInventoryBaselineCoverage(t)
	})
}

func assertMCPCompatibilityInventoryBaselineCoverage(t *testing.T) {
	t.Helper()

	inventory := readCompatibilityInventoryDocument(t, mcpDeprecatedInventoryPath)

	if inventory.Family != "mcp" {
		t.Fatalf("inventory family = %q, want mcp", inventory.Family)
	}
	if len(inventory.Records) != 0 {
		t.Fatalf("MCP workflow alias records = %#v, want none", inventory.Records)
	}
}

func assertCLICompatibilityInventoryBaselineCoverage(t *testing.T) {
	t.Helper()

	inventory := readCompatibilityInventoryDocument(t, cliDeprecatedInventoryPath)

	if inventory.Family != "cli" {
		t.Fatalf("inventory family = %q, want cli", inventory.Family)
	}

	if len(inventory.Records) != 0 {
		t.Fatalf("CLI workflow alias records = %#v, want none", inventory.Records)
	}
}

func assertAPICompatibilityInventoryBaselineCoverage(t *testing.T) {
	t.Helper()

	baseline := readAPICompatibilitySurfacesBaseline(t)
	inventory := readCompatibilityInventoryDocument(t, apiDeprecatedInventoryPath)

	if inventory.Family != "api" {
		t.Fatalf("inventory family = %q, want api", inventory.Family)
	}
	if len(baseline.Surfaces) == 0 {
		t.Fatal("expected at least one baseline API compatibility surface")
	}

	for _, surface := range baseline.Surfaces {
		if !surface.CompatibilityOnly {
			t.Fatalf("baseline surface %q is not marked compatibility-only", surface.ItemID)
		}
		record, ok := inventory.Records[surface.ItemID]
		if !ok {
			t.Fatalf("missing inventory record for baseline surface %q", surface.ItemID)
		}
		assertClassifiedCompatibilityRecord(t, inventory.Family, surface.ItemID, record, classifiedRecordExpectation{
			publicName:            surface.PublicName,
			successorTargetItemID: surface.CanonicalSuccessorItemID,
		})
	}

	if len(inventory.Records) != len(baseline.Surfaces) {
		t.Fatalf(
			"inventory record count = %d, want %d baseline compatibility surfaces",
			len(inventory.Records),
			len(baseline.Surfaces),
		)
	}
}

type classifiedRecordExpectation struct {
	publicName            string
	successorTargetItemID string
}

func assertClassifiedCompatibilityRecord(
	t *testing.T,
	inventoryFamily string,
	recordKey string,
	record compatibilityInventoryRecord,
	expectation classifiedRecordExpectation,
) {
	t.Helper()

	if record.ItemID == "" {
		t.Fatalf("record %q missing stable itemId", recordKey)
	}
	if record.ItemID != recordKey {
		t.Fatalf("record itemId = %q, want %q", record.ItemID, recordKey)
	}
	if record.Family == "" {
		t.Fatalf("record %q missing family", recordKey)
	}
	if record.Family != inventoryFamily {
		t.Fatalf("record family = %q, want %q", record.Family, inventoryFamily)
	}
	if record.PublicName == "" {
		t.Fatalf("record %q missing publicName", recordKey)
	}
	if record.PublicName != expectation.publicName {
		t.Fatalf("record publicName = %q, want %q", record.PublicName, expectation.publicName)
	}
	if record.Classification == "" {
		t.Fatalf("record %q missing classification", recordKey)
	}
	if _, ok := supportedCompatibilityClassifications[record.Classification]; !ok {
		t.Fatalf("record %q classification = %q, want one of retain-temporarily, remove-now, separately-approved", recordKey, record.Classification)
	}
	if record.Lifecycle.Since == "" {
		t.Fatalf("record %q missing lifecycle.since version", recordKey)
	}
	if record.Lifecycle.Deprecated == "" {
		t.Fatalf("record %q missing lifecycle.deprecated version", recordKey)
	}
	if record.Lifecycle.Successor.TargetItemID == "" {
		t.Fatalf("record %q missing successor targetItemId", recordKey)
	}
	if record.Lifecycle.Successor.TargetItemID != expectation.successorTargetItemID {
		t.Fatalf(
			"record successor targetItemId = %q, want %q",
			record.Lifecycle.Successor.TargetItemID,
			expectation.successorTargetItemID,
		)
	}
	if record.Lifecycle.Successor.CanonicalEnglish == "" {
		t.Fatalf("record %q missing successor migration guidance", recordKey)
	}
	if record.Evidence.Summary == "" {
		t.Fatalf("record %q missing evidence summary", recordKey)
	}
	if len(record.RemovalGates) == 0 {
		t.Fatalf("record %q missing removal gates", recordKey)
	}
	for _, gate := range record.RemovalGates {
		if gate.ID == "" {
			t.Fatalf("record %q has a removal gate missing id", recordKey)
		}
		if gate.Description == "" {
			t.Fatalf("record %q removal gate %q missing description", recordKey, gate.ID)
		}
		if gate.Status == "" {
			t.Fatalf("record %q removal gate %q missing status", recordKey, gate.ID)
		}
	}
	if record.ApprovalStatus == "" {
		t.Fatalf("record %q missing approval status", recordKey)
	}
}

type compatibilityInventoryDocument struct {
	Family  string                                  `json:"family"`
	Records map[string]compatibilityInventoryRecord `json:"records"`
}

type compatibilityInventoryRecord struct {
	ItemID         string `json:"itemId"`
	Family         string `json:"family"`
	PublicName     string `json:"publicName"`
	Classification string `json:"classification"`
	ApprovalStatus string `json:"approvalStatus"`
	Lifecycle      struct {
		Since      string `json:"since"`
		Deprecated string `json:"deprecated"`
		Successor  struct {
			TargetItemID     string `json:"targetItemId"`
			CanonicalEnglish string `json:"canonicalEnglish"`
		} `json:"successor"`
	} `json:"lifecycle"`
	Evidence struct {
		Summary string `json:"summary"`
	} `json:"evidence"`
	RemovalGates []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Status      string `json:"status"`
	} `json:"removalGates"`
}

type mcpBaselineAliasesDocument struct {
	Aliases []mcpBaselineAlias `json:"aliases"`
}

type mcpBaselineAlias struct {
	Name              string `json:"name"`
	CanonicalName     string `json:"canonicalName"`
	CompatibilityOnly bool   `json:"compatibilityOnly"`
}

type cliCommandIdentityBaselineDocument struct {
	Commands []cliCommandIdentityRecord `json:"commands"`
}

type cliCommandIdentityRecord struct {
	Path        string `json:"path"`
	IDCandidate string `json:"idCandidate"`
}

type apiCompatibilitySurfacesBaselineDocument struct {
	Surfaces []apiCompatibilitySurfaceRecord `json:"surfaces"`
}

type apiCompatibilitySurfaceRecord struct {
	ItemID                   string `json:"itemId"`
	PublicName               string `json:"publicName"`
	CanonicalSuccessorItemID string `json:"canonicalSuccessorItemId"`
	CompatibilityOnly        bool   `json:"compatibilityOnly"`
}

func readCompatibilityInventoryDocument(t *testing.T, path string) compatibilityInventoryDocument {
	t.Helper()
	return decodeContractJSON[compatibilityInventoryDocument](t, path)
}

func readMCPBaselineAliases(t *testing.T) mcpBaselineAliasesDocument {
	t.Helper()
	return decodeContractJSON[mcpBaselineAliasesDocument](t, filepath.Join("testdata", "baseline", "mcp-aliases.json"))
}

func readCLICommandIdentityBaseline(t *testing.T) cliCommandIdentityBaselineDocument {
	t.Helper()
	return decodeContractJSON[cliCommandIdentityBaselineDocument](t, filepath.Join("testdata", "baseline", "cli-commands.json"))
}

func readAPICompatibilitySurfacesBaseline(t *testing.T) apiCompatibilitySurfacesBaselineDocument {
	t.Helper()
	return decodeContractJSON[apiCompatibilitySurfacesBaselineDocument](t, filepath.Join("testdata", "baseline", "api-compatibility-surfaces.json"))
}

func baselineCompatibilityCommands(baseline cliCommandIdentityBaselineDocument) []cliCommandIdentityRecord {
	var commands []cliCommandIdentityRecord
	for _, command := range baseline.Commands {
		if !isCLICompatibilityCommandIDCandidate(command.IDCandidate) {
			continue
		}
		commands = append(commands, command)
	}
	return commands
}

func isCLICompatibilityCommandIDCandidate(idCandidate string) bool {
	return strings.HasPrefix(idCandidate, "you.workflow.") && idCandidate != "you.workflow"
}

func cliCompatibilityRecordKey(idCandidate string) string {
	return "cli.command." + strings.TrimPrefix(idCandidate, "you.")
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
