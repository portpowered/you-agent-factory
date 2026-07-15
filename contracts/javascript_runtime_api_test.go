package contracts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func javascriptManifestFixtureRegistry(fixture string) contractvalidator.Registry {
	const (
		documentationID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
		deprecationsID  = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
	)
	return contractvalidator.NewRegistry(contractvalidator.Entry{
		Family:        "javascript",
		FormatVersion: "1.0.0",
		Schemas: []contractvalidator.Schema{
			{ID: documentationID, Path: "contracts/common/documentation.schema.json"},
			{ID: deprecationsID, Path: "contracts/common/deprecations.schema.json"},
			{ID: runtimeManifestSchemaID, Path: "contracts/javascript/runtime-manifest.schema.json"},
		},
		Documents: []contractvalidator.Document{{
			Path:     fixture,
			SchemaID: runtimeManifestSchemaID,
		}},
	})
}

func TestJavaScriptRuntimeAPIAuthoredCatalogBoundary(t *testing.T) {
	t.Parallel()

	catalogPath := filepath.Join("javascript", "runtime-api.json")
	if _, err := os.Stat(catalogPath); err != nil {
		t.Fatalf("contracts/javascript/runtime-api.json must exist as the authored catalog: %v", err)
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	diagnostics := contractvalidator.Validate(root, contractvalidator.JavaScriptRegistry(), "javascript", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("authored catalog validation diagnostics = %+v", diagnostics)
	}
}

func TestJavaScriptRuntimeAPIBrokenSharedSchemaReference(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	const (
		fixture  = "contracts/testdata/javascript/invalid-broken-shared-schema-ref.json"
		wantPath = "/symbols/javascript.workflow.checkpoint/parameters/0/serializableValue/$ref"
	)
	diagnostics := contractvalidator.Validate(root, javascriptManifestFixtureRegistry(fixture), "javascript", "1.0.0")
	if len(diagnostics) == 0 {
		t.Fatal("expected broken shared-schema reference diagnostics, got none")
	}
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "reference.fragment" && diagnostic.Path == wantPath && diagnostic.Document == fixture {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want code=reference.fragment path=%q document=%q", diagnostics, wantPath, fixture)
	}
}

func TestJavaScriptRuntimeAPIOpenSharedSchema(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	const (
		fixture  = "contracts/testdata/javascript/invalid-open-shared-schema.json"
		wantPath = "/sharedSchemas/javascript.schema.open_object/schema"
	)
	diagnostics := contractvalidator.Validate(root, javascriptManifestFixtureRegistry(fixture), "javascript", "1.0.0")
	if len(diagnostics) == 0 {
		t.Fatal("expected open shared-schema diagnostics, got none")
	}
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "javascript.serializable_value.open" && diagnostic.Path == wantPath && diagnostic.Document == fixture {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want code=javascript.serializable_value.open path=%q document=%q", diagnostics, wantPath, fixture)
	}
}
