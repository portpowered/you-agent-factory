package contracts_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/globalconfiginventory"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const youConfigSchemaID = "https://schemas.portpowered.com/you/config/you-config.schema.json"

var inventoriedGlobalConfigFieldIDs = []string{
	"backendScopeID",
	"defaults",
	"defaults.workerModelProvider",
	"defaults.workerModel",
	"workerPresets",
	"workerPresets[].id",
	"workerPresets[].modelProvider",
	"workerPresets[].model",
	"workerPresets[].reasoningEffort",
}

func youConfigSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return compileSchema(
		t,
		filepath.Join("config", "you-config.schema.json"),
		youConfigSchemaID,
		schemaResource{
			path: filepath.Join("common", "documentation.schema.json"),
			id:   documentationSchemaID,
		},
		schemaResource{
			path: filepath.Join("common", "deprecations.schema.json"),
			id:   deprecationsSchemaID,
		},
	)
}

func TestYouConfigSchemaDeclaresDraft202012AndStableID(t *testing.T) {
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	root, ok := document.(map[string]any)
	if !ok {
		t.Fatalf("schema root type = %T, want map[string]any", document)
	}
	if root["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %#v, want Draft 2020-12", root["$schema"])
	}
	if root["$id"] != youConfigSchemaID {
		t.Fatalf("$id = %#v, want %q", root["$id"], youConfigSchemaID)
	}
}

func TestYouConfigSchemaValidFixtures(t *testing.T) {
	schema := youConfigSchema(t)

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "defaults only", fixture: filepath.Join("..", "pkg", "config", "operatorconfig", "testdata", "fixtures", "valid", "defaults-only.json")},
		{name: "backend scope sibling", fixture: filepath.Join("..", "pkg", "config", "operatorconfig", "testdata", "fixtures", "valid", "backend-scope-sibling.json")},
		{name: "worker presets missing", fixture: filepath.Join("..", "pkg", "config", "operatorconfig", "testdata", "fixtures", "valid", "worker-presets-missing.json")},
		{name: "load defaults", fixture: filepath.Join("..", "pkg", "config", "operatorconfig", "testdata", "fixtures", "valid", "load-defaults.json")},
		{name: "existing backend scope", fixture: filepath.Join("..", "pkg", "config", "systemconfig", "testdata", "fixtures", "valid", "existing-scope.json")},
		{name: "defaults sibling", fixture: filepath.Join("..", "pkg", "config", "systemconfig", "testdata", "fixtures", "valid", "defaults-sibling.json")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, test.fixture)
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("validate valid fixture %s: %v", test.fixture, err)
			}
		})
	}
}

func TestYouConfigSchemaContractCoversInventoriedTopology(t *testing.T) {
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	root, ok := document.(map[string]any)
	if !ok {
		t.Fatalf("schema root type = %T, want map[string]any", document)
	}

	contract, ok := root["contract"].(map[string]any)
	if !ok {
		t.Fatal("schema missing contract annotation")
	}
	fields, ok := contract["fields"].(map[string]any)
	if !ok {
		t.Fatal("contract missing fields map")
	}

	gotIDs := make([]string, 0, len(fields))
	for key, value := range fields {
		field, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("contract field %q type = %T, want map[string]any", key, value)
		}
		inventoryID, ok := field["inventoryId"].(string)
		if !ok || inventoryID == "" {
			t.Fatalf("contract field %q missing inventoryId", key)
		}
		if inventoryID != key {
			t.Fatalf("contract field key %q inventoryId = %q, want matching keys", key, inventoryID)
		}
		gotIDs = append(gotIDs, inventoryID)
	}
	slices.Sort(gotIDs)

	wantIDs := append([]string(nil), inventoriedGlobalConfigFieldIDs...)
	slices.Sort(wantIDs)
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("contract field inventory IDs = %v, want %v", gotIDs, wantIDs)
	}

	inventory := globalconfiginventory.ProjectTopologyInventory()
	for _, record := range inventory.Fields {
		field, ok := fields[record.ID].(map[string]any)
		if !ok {
			t.Fatalf("contract missing inventoried field %q", record.ID)
		}
		if field["jsonPath"] != record.JSONPath {
			t.Fatalf("field %q jsonPath = %#v, want %q", record.ID, field["jsonPath"], record.JSONPath)
		}
		if field["defaultEmptyBehavior"] != record.DefaultEmptyBehavior {
			t.Fatalf("field %q defaultEmptyBehavior = %#v, want %q", record.ID, field["defaultEmptyBehavior"], record.DefaultEmptyBehavior)
		}
	}
}

func TestYouConfigSchemaContractMetadataValidatesThroughCommonVocabulary(t *testing.T) {
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	root := document.(map[string]any)
	contract := root["contract"].(map[string]any)
	fields := contract["fields"].(map[string]any)

	documentationSchema := compileSchema(
		t,
		filepath.Join("common", "documentation.schema.json"),
		documentationSchemaID,
	)
	lifecycleSchema := compileSchema(
		t,
		filepath.Join("common", "deprecations.schema.json"),
		deprecationsSchemaID,
		schemaResource{
			path: filepath.Join("common", "documentation.schema.json"),
			id:   documentationSchemaID,
		},
	)
	fieldContractSchema := compileFieldContractSchema(t, root)

	for _, inventoryID := range inventoriedGlobalConfigFieldIDs {
		t.Run(inventoryID, func(t *testing.T) {
			field := fields[inventoryID]
			if err := fieldContractSchema.Validate(field); err != nil {
				t.Fatalf("validate field contract: %v", err)
			}
			fieldMap := field.(map[string]any)
			if err := documentationSchema.Validate(fieldMap["documentation"]); err != nil {
				t.Fatalf("validate documentation metadata: %v", err)
			}
			if err := lifecycleSchema.Validate(fieldMap["lifecycle"]); err != nil {
				t.Fatalf("validate lifecycle metadata: %v", err)
			}
		})
	}
}

func TestYouConfigSchemaClosedObjectBoundaries(t *testing.T) {
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	root := document.(map[string]any)

	if root["additionalProperties"] != false {
		t.Fatalf("root additionalProperties = %#v, want false", root["additionalProperties"])
	}

	defaults := root["properties"].(map[string]any)["defaults"].(map[string]any)
	if defaults["additionalProperties"] != false {
		t.Fatalf("defaults additionalProperties = %#v, want false", defaults["additionalProperties"])
	}

	workerPreset := root["$defs"].(map[string]any)["workerPreset"].(map[string]any)
	if workerPreset["additionalProperties"] != false {
		t.Fatalf("workerPreset additionalProperties = %#v, want false", workerPreset["additionalProperties"])
	}
}

func TestYouConfigSchemaRejectsInvalidFixtures(t *testing.T) {
	schema := youConfigSchema(t)
	fixtureRoot := filepath.Join("..", "pkg", "config", "operatorconfig", "testdata", "fixtures")

	tests := []struct {
		name     string
		fixture  string
		wantPath string
	}{
		{
			name:     "unknown top-level field",
			fixture:  filepath.Join(fixtureRoot, "invalid", "unknown-top-level.json"),
			wantPath: "",
		},
		{
			name:     "unknown nested defaults field",
			fixture:  filepath.Join(fixtureRoot, "invalid", "unknown-nested-defaults.json"),
			wantPath: "/defaults",
		},
		{
			name:     "preset empty id",
			fixture:  filepath.Join(fixtureRoot, "invalid", "preset-empty-id.json"),
			wantPath: "/workerPresets/0/id",
		},
		{
			name:     "preset missing provider",
			fixture:  filepath.Join(fixtureRoot, "invalid", "preset-missing-provider.json"),
			wantPath: "/workerPresets/0",
		},
		{
			name:     "preset symbolic provider",
			fixture:  filepath.Join(fixtureRoot, "invalid", "preset-symbolic-provider.json"),
			wantPath: "/workerPresets/0/modelProvider",
		},
		{
			name:     "preset unsupported provider",
			fixture:  filepath.Join(fixtureRoot, "invalid", "preset-unsupported-provider.json"),
			wantPath: "/workerPresets/0/modelProvider",
		},
		{
			name:     "preset unsupported reasoning effort",
			fixture:  filepath.Join(fixtureRoot, "invalid", "preset-unsupported-reasoning.json"),
			wantPath: "/workerPresets/0/reasoningEffort",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, test.fixture)
			err := schema.Validate(instance)
			if err == nil {
				t.Fatalf("validate invalid fixture %s: expected rejection", test.fixture)
			}
			if paths := validationPaths(t, err); !slices.Contains(paths, test.wantPath) {
				t.Fatalf("validation paths = %v, want %q", paths, test.wantPath)
			}
		})
	}
}

func TestYouConfigSchemaInstancePropertiesMatchInventoriedTopology(t *testing.T) {
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	root := document.(map[string]any)
	properties := root["properties"].(map[string]any)

	wantTopLevel := []string{"backendScopeID", "defaults", "workerPresets"}
	gotTopLevel := make([]string, 0, len(properties))
	for name := range properties {
		gotTopLevel = append(gotTopLevel, name)
	}
	slices.Sort(gotTopLevel)
	slices.Sort(wantTopLevel)
	if !slices.Equal(gotTopLevel, wantTopLevel) {
		t.Fatalf("top-level properties = %v, want %v", gotTopLevel, wantTopLevel)
	}

	defaults := properties["defaults"].(map[string]any)
	defaultProperties := defaults["properties"].(map[string]any)
	wantDefaults := []string{"workerModelProvider", "workerModel"}
	gotDefaults := make([]string, 0, len(defaultProperties))
	for name := range defaultProperties {
		gotDefaults = append(gotDefaults, name)
	}
	slices.Sort(gotDefaults)
	slices.Sort(wantDefaults)
	if !slices.Equal(gotDefaults, wantDefaults) {
		t.Fatalf("defaults properties = %v, want %v", gotDefaults, wantDefaults)
	}

	workerPreset := root["$defs"].(map[string]any)["workerPreset"].(map[string]any)
	presetProperties := workerPreset["properties"].(map[string]any)
	wantPreset := []string{"id", "modelProvider", "model", "reasoningEffort"}
	gotPreset := make([]string, 0, len(presetProperties))
	for name := range presetProperties {
		gotPreset = append(gotPreset, name)
	}
	slices.Sort(gotPreset)
	slices.Sort(wantPreset)
	if !slices.Equal(gotPreset, wantPreset) {
		t.Fatalf("workerPreset properties = %v, want %v", gotPreset, wantPreset)
	}
}

func compileFieldContractSchema(t *testing.T, schemaRoot map[string]any) *jsonschema.Schema {
	t.Helper()

	defs := schemaRoot["$defs"].(map[string]any)
	fieldContract := defs["fieldContract"]
	wrapped := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://schemas.portpowered.com/you/config/you-config.field-contract.schema.json",
		"$ref":    "#/$defs/fieldContract",
		"$defs": map[string]any{
			"fieldContract": fieldContract,
		},
	}
	payload, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("marshal field contract schema: %v", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(documentationSchemaID, readJSON(t, filepath.Join("common", "documentation.schema.json"))); err != nil {
		t.Fatalf("add documentation schema: %v", err)
	}
	if err := compiler.AddResource(deprecationsSchemaID, readJSON(t, filepath.Join("common", "deprecations.schema.json"))); err != nil {
		t.Fatalf("add deprecations schema: %v", err)
	}
	wrappedDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode field contract schema: %v", err)
	}
	if err := compiler.AddResource(wrapped["$id"].(string), wrappedDocument); err != nil {
		t.Fatalf("add field contract schema: %v", err)
	}
	compiled, err := compiler.Compile(wrapped["$id"].(string))
	if err != nil {
		t.Fatalf("compile field contract schema: %v", err)
	}
	return compiled
}
