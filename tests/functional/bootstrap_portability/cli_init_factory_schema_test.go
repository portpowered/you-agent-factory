package bootstrap_portability

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestInitFactory_DefaultScaffoldFactoryJSONValidatesAgainstOpenAPISchema proves the
// canonical default init factory document and freshly generated factory.json files
// validate against the repository Factory OpenAPI schema.
func TestInitFactory_DefaultScaffoldFactoryJSONValidatesAgainstOpenAPISchema(t *testing.T) {
	factorySchema := loadFactoryOpenAPISchema(t)

	t.Run("customer default init factory.json", func(t *testing.T) {
		dir := t.TempDir()
		support.RunInitCommand(t, dir)
		factoryJSON, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
		if err != nil {
			t.Fatalf("read factory.json: %v", err)
		}
		assertFactoryJSONValidatesAgainstSchema(t, factorySchema, factoryJSON)
	})

	t.Run("fresh init directory factory.json", func(t *testing.T) {
		dir := t.TempDir()
		support.RunInitCommand(t, dir, "--executor", "claude")

		factoryJSON, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
		if err != nil {
			t.Fatalf("read factory.json: %v", err)
		}
		assertFactoryJSONValidatesAgainstSchema(t, factorySchema, factoryJSON)
	})

	t.Run("fresh init directory flattened factory export", func(t *testing.T) {
		dir := t.TempDir()
		support.RunInitCommand(t, dir)

		flattened, err := support.FlattenFactoryConfig(t, dir)
		if err != nil {
			t.Fatalf("FlattenFactoryConfig(%s): %v", dir, err)
		}
		assertFactoryJSONValidatesAgainstSchema(t, factorySchema, flattened)
	})
}

func loadFactoryOpenAPISchema(t *testing.T) *openapi3.Schema {
	t.Helper()

	openAPIPath := testutil.MustRepoPath(t, "api/openapi.yaml")
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(openAPIPath)
	if err != nil {
		t.Fatalf("load openapi contract from %s: %v", openAPIPath, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}

	schemaRef, ok := doc.Components.Schemas["Factory"]
	if !ok || schemaRef == nil || schemaRef.Value == nil {
		t.Fatal("components.schemas.Factory is missing")
	}
	return schemaRef.Value
}

func assertFactoryJSONValidatesAgainstSchema(t *testing.T, schema *openapi3.Schema, data []byte) {
	t.Helper()

	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}
	if err := schema.VisitJSON(payload); err != nil {
		t.Fatalf("factory.json should validate against OpenAPI Factory schema: %v", err)
	}
}
