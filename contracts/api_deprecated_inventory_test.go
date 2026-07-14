package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const apiDeprecatedInventoryPath = "api/deprecated.json"

func TestAPIDeprecatedInventoryValidatesAgainstSchema(t *testing.T) {
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

	instance := readJSON(t, apiDeprecatedInventoryPath)
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("validate API deprecated inventory: %v", err)
	}
	diagnostics := contractvalidator.CompatibilityInventorySemanticsDiagnostics(apiDeprecatedInventoryPath, instance)
	if len(diagnostics) != 0 {
		t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
	}
}

func TestAPIDeprecatedInventoryClassifiesEveryBaselineCompatibilitySurface(t *testing.T) {
	baseline := readAPICompatibilitySurfacesBaseline(t)
	inventory := readAPIDeprecatedInventory(t)

	if inventory.Family != "api" {
		t.Fatalf("inventory family = %q, want api", inventory.Family)
	}

	for _, surface := range baseline.Surfaces {
		if !surface.CompatibilityOnly {
			t.Fatalf("baseline surface %q is not marked compatibility-only", surface.ItemID)
		}
		record, ok := inventory.Records[surface.ItemID]
		if !ok {
			t.Fatalf("missing inventory record for baseline surface %q", surface.ItemID)
		}
		if record.PublicName != surface.PublicName {
			t.Fatalf("record publicName = %q, want %q", record.PublicName, surface.PublicName)
		}
		if record.Classification == "" {
			t.Fatalf("record for %q missing classification", surface.ItemID)
		}
		if record.Lifecycle.Successor.TargetItemID != surface.CanonicalSuccessorItemID {
			t.Fatalf(
				"record successor targetItemId = %q, want %q",
				record.Lifecycle.Successor.TargetItemID,
				surface.CanonicalSuccessorItemID,
			)
		}
		if record.Lifecycle.Successor.CanonicalEnglish == "" {
			t.Fatalf("record for %q missing successor migration guidance", surface.ItemID)
		}
		if record.Evidence.Summary == "" {
			t.Fatalf("record for %q missing evidence summary", surface.ItemID)
		}
		if len(record.RemovalGates) == 0 {
			t.Fatalf("record for %q missing removal gates", surface.ItemID)
		}
		if record.ApprovalStatus == "" {
			t.Fatalf("record for %q missing approval status", surface.ItemID)
		}
	}

	if len(inventory.Records) != len(baseline.Surfaces) {
		t.Fatalf(
			"inventory record count = %d, want %d baseline compatibility surfaces",
			len(inventory.Records),
			len(baseline.Surfaces),
		)
	}
}

type apiCompatibilitySurfacesBaselineDocument struct {
	Surfaces []apiCompatibilitySurfaceRecord `json:"surfaces"`
}

type apiCompatibilitySurfaceRecord struct {
	ItemID                     string `json:"itemId"`
	PublicName                 string `json:"publicName"`
	CanonicalSuccessorItemID   string `json:"canonicalSuccessorItemId"`
	CompatibilityOnly          bool   `json:"compatibilityOnly"`
}

type apiDeprecatedInventoryDocument struct {
	Family  string                         `json:"family"`
	Records map[string]apiCompatibilityRecord `json:"records"`
}

type apiCompatibilityRecord struct {
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

func readAPICompatibilitySurfacesBaseline(t *testing.T) apiCompatibilitySurfacesBaselineDocument {
	t.Helper()
	return decodeContractJSON[apiCompatibilitySurfacesBaselineDocument](t, filepath.Join("testdata", "baseline", "api-compatibility-surfaces.json"))
}

func readAPIDeprecatedInventory(t *testing.T) apiDeprecatedInventoryDocument {
	t.Helper()
	return decodeContractJSON[apiDeprecatedInventoryDocument](t, apiDeprecatedInventoryPath)
}
