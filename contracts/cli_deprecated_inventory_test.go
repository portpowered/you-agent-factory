package contracts_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const cliDeprecatedInventoryPath = "cli/deprecated.json"

var cliCompatibilitySuccessorByIDCandidate = map[string]string{
	"you.workflow.preview":    "api.route.post.factories.preview",
	"you.workflow.validate":   "api.route.post.factories.preview",
	"you.workflow.run":        "api.route.post.factory-sessions.sync",
	"you.workflow.start":      "api.route.post.factory-sessions.async",
	"you.workflow.status":     "api.route.get.factory-sessions.session-id",
	"you.workflow.result":     "api.route.get.factory-sessions.session-id.results",
	"you.workflow.dispatches": "api.route.get.factory-sessions.session-id.dispatches",
	"you.workflow.artifacts":  "api.route.get.factory-sessions.session-id.artifacts",
	"you.workflow.events":     "api.route.get.factory-sessions.session-id.events",
}

func TestCLIDeprecatedInventoryValidatesAgainstSchema(t *testing.T) {
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

	instance := readJSON(t, cliDeprecatedInventoryPath)
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("validate CLI deprecated inventory: %v", err)
	}
	diagnostics := contractvalidator.CompatibilityInventorySemanticsDiagnostics(cliDeprecatedInventoryPath, instance)
	if len(diagnostics) != 0 {
		t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
	}
}

func TestCLIDeprecatedInventoryClassifiesEveryBaselineCompatibilityCommand(t *testing.T) {
	baseline := readCLICommandIdentityBaseline(t)
	inventory := readCLIDeprecatedInventory(t)

	if inventory.Family != "cli" {
		t.Fatalf("inventory family = %q, want cli", inventory.Family)
	}

	compatibilityCommands := baselineCompatibilityCommands(baseline)
	if len(compatibilityCommands) == 0 {
		t.Fatal("expected at least one baseline CLI compatibility command")
	}

	for _, command := range compatibilityCommands {
		recordKey := cliCompatibilityRecordKey(command.IDCandidate)
		record, ok := inventory.Records[recordKey]
		if !ok {
			t.Fatalf("missing inventory record for baseline command %q", command.Path)
		}
		if record.PublicName != command.Path {
			t.Fatalf("record publicName = %q, want %q", record.PublicName, command.Path)
		}
		if record.Classification == "" {
			t.Fatalf("record for %q missing classification", command.Path)
		}
		wantSuccessor, ok := cliCompatibilitySuccessorByIDCandidate[command.IDCandidate]
		if !ok {
			t.Fatalf("missing successor mapping for baseline command idCandidate %q", command.IDCandidate)
		}
		if record.Lifecycle.Successor.TargetItemID != wantSuccessor {
			t.Fatalf(
				"record successor targetItemId = %q, want %q",
				record.Lifecycle.Successor.TargetItemID,
				wantSuccessor,
			)
		}
		if record.Lifecycle.Successor.CanonicalEnglish == "" {
			t.Fatalf("record for %q missing successor migration guidance", command.Path)
		}
		if record.Evidence.Summary == "" {
			t.Fatalf("record for %q missing evidence summary", command.Path)
		}
		if len(record.RemovalGates) == 0 {
			t.Fatalf("record for %q missing removal gates", command.Path)
		}
		if record.ApprovalStatus == "" {
			t.Fatalf("record for %q missing approval status", command.Path)
		}
	}

	if len(inventory.Records) != len(compatibilityCommands) {
		t.Fatalf(
			"inventory record count = %d, want %d baseline compatibility commands",
			len(inventory.Records),
			len(compatibilityCommands),
		)
	}
}

type cliCommandIdentityBaselineDocument struct {
	Commands []cliCommandIdentityRecord `json:"commands"`
}

type cliCommandIdentityRecord struct {
	Path        string `json:"path"`
	IDCandidate string `json:"idCandidate"`
}

type cliDeprecatedInventoryDocument struct {
	Family  string                          `json:"family"`
	Records map[string]cliCompatibilityRecord `json:"records"`
}

type cliCompatibilityRecord struct {
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

func readCLICommandIdentityBaseline(t *testing.T) cliCommandIdentityBaselineDocument {
	t.Helper()
	return decodeContractJSON[cliCommandIdentityBaselineDocument](t, filepath.Join("testdata", "baseline", "cli-commands.json"))
}

func readCLIDeprecatedInventory(t *testing.T) cliDeprecatedInventoryDocument {
	t.Helper()
	return decodeContractJSON[cliDeprecatedInventoryDocument](t, cliDeprecatedInventoryPath)
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
