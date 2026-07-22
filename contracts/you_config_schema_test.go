package contracts_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
		schemaResource{
			path: filepath.Join("common", "precedence.schema.json"),
			id:   precedenceSchemaID,
		},
		schemaResource{
			path: filepath.Join("common", "sensitivity.schema.json"),
			id:   sensitivitySchemaID,
		},
	)
}

func TestYouConfigSchemaDeclaresDraft202012AndStableID(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	schema := youConfigSchema(t)

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "defaults only", fixture: filepath.Join("..", "pkg", "services", "operator_settings", "testdata", "fixtures", "valid", "defaults-only.json")},
		{name: "backend scope sibling", fixture: filepath.Join("..", "pkg", "services", "operator_settings", "testdata", "fixtures", "valid", "backend-scope-sibling.json")},
		{name: "worker presets missing", fixture: filepath.Join("..", "pkg", "services", "operator_settings", "testdata", "fixtures", "valid", "worker-presets-missing.json")},
		{name: "load defaults", fixture: filepath.Join("..", "pkg", "services", "operator_settings", "testdata", "fixtures", "valid", "load-defaults.json")},
		{name: "existing backend scope", fixture: filepath.Join("..", "pkg", "services", "operator_settings", "identityinventory", "testdata", "fixtures", "valid", "existing-scope.json")},
		{name: "defaults sibling", fixture: filepath.Join("..", "pkg", "services", "operator_settings", "identityinventory", "testdata", "fixtures", "valid", "defaults-sibling.json")},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			instance := readJSON(t, test.fixture)
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("validate valid fixture %s: %v", test.fixture, err)
			}
		})
	}
}

func TestYouConfigSchemaContractCoversInventoriedTopology(t *testing.T) {
	t.Parallel()
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

	inventory := committedGlobalConfigInventory(t)
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
		if field["parseOwner"] != record.ParseOwner {
			t.Fatalf("field %q parseOwner = %#v, want %q", record.ID, field["parseOwner"], record.ParseOwner)
		}
		if field["persistenceOwner"] != record.PersistenceOwner {
			t.Fatalf("field %q persistenceOwner = %#v, want %q", record.ID, field["persistenceOwner"], record.PersistenceOwner)
		}
		if field["strictness"] != record.Strictness {
			t.Fatalf("field %q strictness = %#v, want %q", record.ID, field["strictness"], record.Strictness)
		}
		if record.Notes != "" {
			notes, _ := field["notes"].(string)
			if notes != record.Notes {
				t.Fatalf("field %q notes = %#v, want %q", record.ID, field["notes"], record.Notes)
			}
		}
	}
}

func TestYouConfigSchemaContractMetadataValidatesThroughCommonVocabulary(t *testing.T) {
	t.Parallel()
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
	precedenceSchema := compileSchema(
		t,
		filepath.Join("common", "precedence.schema.json"),
		precedenceSchemaID,
		schemaResource{
			path: filepath.Join("common", "documentation.schema.json"),
			id:   documentationSchemaID,
		},
	)
	sensitivitySchema := compileSchema(
		t,
		filepath.Join("common", "sensitivity.schema.json"),
		sensitivitySchemaID,
		schemaResource{
			path: filepath.Join("common", "documentation.schema.json"),
			id:   documentationSchemaID,
		},
	)
	fieldContractSchema := compileFieldContractSchema(t, root)

	for _, inventoryID := range inventoriedGlobalConfigFieldIDs {
		inventoryID := inventoryID
		t.Run(inventoryID, func(t *testing.T) {
			t.Parallel()
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
			if err := precedenceSchema.Validate(fieldMap["precedence"]); err != nil {
				t.Fatalf("validate precedence metadata: %v", err)
			}
			if err := sensitivitySchema.Validate(fieldMap["sensitivity"]); err != nil {
				t.Fatalf("validate sensitivity metadata: %v", err)
			}
		})
	}
}

func TestYouConfigSchemaPrecedenceAndSensitivityMetadataAgreesWithInventory(t *testing.T) {
	t.Parallel()
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	contract := document.(map[string]any)["contract"].(map[string]any)
	fields := contract["fields"].(map[string]any)

	precedenceChain, ok := contract["precedenceChain"].(string)
	if !ok || precedenceChain == "" {
		t.Fatal("contract missing precedenceChain metadata")
	}

	inventory := committedGlobalConfigInventory(t)
	if precedenceChain != inventory.PrecedenceChain {
		t.Fatalf("contract precedenceChain = %q, want %q", precedenceChain, inventory.PrecedenceChain)
	}

	for _, record := range inventory.Fields {
		record := record
		t.Run(record.ID, func(t *testing.T) {
			t.Parallel()
			field, ok := fields[record.ID].(map[string]any)
			if !ok {
				t.Fatalf("contract missing field %q", record.ID)
			}

			precedence := field["precedence"].(map[string]any)
			gotLayers := decodeContractValue[[]string](t, precedence["layers"])
			if !slices.Equal(gotLayers, record.PrecedenceLayers) {
				t.Fatalf("field %q precedence layers = %v, want %v", record.ID, gotLayers, record.PrecedenceLayers)
			}
			if record.EnvironmentVariable != "" {
				if precedence["environmentVariable"] != record.EnvironmentVariable {
					t.Fatalf("field %q environmentVariable = %#v, want %q", record.ID, precedence["environmentVariable"], record.EnvironmentVariable)
				}
			} else if precedence["environmentVariable"] != nil {
				t.Fatalf("field %q environmentVariable = %#v, want absent", record.ID, precedence["environmentVariable"])
			}
			if record.FlagName != "" {
				if precedence["flagName"] != record.FlagName {
					t.Fatalf("field %q flagName = %#v, want %q", record.ID, precedence["flagName"], record.FlagName)
				}
			} else if precedence["flagName"] != nil {
				t.Fatalf("field %q flagName = %#v, want absent", record.ID, precedence["flagName"])
			}

			sensitivity := field["sensitivity"].(map[string]any)
			if sensitivity["classification"] != "public" {
				t.Fatalf("field %q sensitivity classification = %#v, want public", record.ID, sensitivity["classification"])
			}
		})
	}
}

func TestYouConfigSchemaClosedObjectBoundaries(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	schema := youConfigSchema(t)
	fixtureRoot := filepath.Join("..", "pkg", "services", "operator_settings", "testdata", "fixtures")

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
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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

func TestYouConfigSchemaContractDocumentsSharedFileSplitAndPersistencePolicy(t *testing.T) {
	t.Parallel()
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	contract := document.(map[string]any)["contract"].(map[string]any)

	topology := committedGlobalConfigInventory(t)
	if contract["sharedFileSplit"] == nil {
		t.Fatal("contract missing sharedFileSplit metadata")
	}
	gotSplit := decodeContractValue[committedGlobalConfigSharedFileSplit](t, contract["sharedFileSplit"])
	gotSplitJSON, err := json.Marshal(gotSplit)
	if err != nil {
		t.Fatalf("marshal decoded sharedFileSplit: %v", err)
	}
	wantSplitJSON, err := json.Marshal(topology.SharedFileSplit)
	if err != nil {
		t.Fatalf("marshal topology sharedFileSplit: %v", err)
	}
	if !bytes.Equal(gotSplitJSON, wantSplitJSON) {
		t.Fatalf("contract sharedFileSplit = %s, want %s", gotSplitJSON, wantSplitJSON)
	}

	gotPolicies := decodeContractValue[[]committedGlobalConfigUnknownPolicy](t, contract["unknownFieldPolicy"])
	if !slices.Equal(gotPolicies, topology.UnknownFieldPolicy) {
		t.Fatalf("contract unknownFieldPolicy = %#v, want %#v", gotPolicies, topology.UnknownFieldPolicy)
	}

	systemInventory := committedIdentityInputInventory(t)
	siblingPreservation, ok := contract["siblingPreservation"].(string)
	if !ok || siblingPreservation == "" {
		t.Fatal("contract missing siblingPreservation metadata")
	}
	if siblingPreservation != systemInventory.SiblingPreservation {
		t.Fatalf("contract siblingPreservation = %q, want %q", siblingPreservation, systemInventory.SiblingPreservation)
	}
}

func TestYouConfigSchemaPersistenceCasesAgreeWithDocumentedSemantics(t *testing.T) {
	t.Parallel()
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	contract := document.(map[string]any)["contract"].(map[string]any)
	fields := contract["fields"].(map[string]any)
	backendScope := fields["backendScopeID"].(map[string]any)

	if backendScope["persistenceOwner"] != "operator_settings" || backendScope["parseOwner"] != "operator_settings" {
		t.Fatalf("backendScopeID ownership = parse %q persist %q, want operator_settings/operator_settings", backendScope["parseOwner"], backendScope["persistenceOwner"])
	}

	defaults := fields["defaults"].(map[string]any)
	if defaults["parseOwner"] != "operator_settings" || defaults["persistenceOwner"] != "none" {
		t.Fatalf("defaults ownership = parse %q persist %q, want operator_settings/none", defaults["parseOwner"], defaults["persistenceOwner"])
	}

	summary := contract["sharedFileSplit"].(map[string]any)["summary"].(string)
	if !strings.Contains(summary, "operator_settings owns backendScopeID identity, defaults, and workerPresets") {
		t.Fatalf("shared file split summary = %q, want unified operator_settings ownership", summary)
	}

	systemInventory := committedIdentityInputInventory(t)
	for _, inputCase := range systemInventory.Cases {
		inputCase := inputCase
		if inputCase.PersistedFileExpectation == nil && inputCase.Entrypoint != "persistBackendScopeID" {
			continue
		}
		t.Run(inputCase.ID, func(t *testing.T) {
			t.Parallel()
			if backendScope["persistenceOwner"] != "operator_settings" {
				t.Fatalf("persistence case %q requires operator_settings persistence owner", inputCase.ID)
			}
			if inputCase.PersistedFileExpectation == nil {
				return
			}
			siblingPreservation := contract["siblingPreservation"].(string)
			if inputCase.PersistedFileExpectation.PreservesDefaults && !strings.Contains(siblingPreservation, "defaults") {
				t.Fatalf("case %q preserves defaults but siblingPreservation omits defaults", inputCase.ID)
			}
			for _, key := range inputCase.PersistedFileExpectation.PreservesSiblingKeys {
				if !strings.Contains(siblingPreservation, "unknown top-level sibling keys") {
					t.Fatalf("case %q preserves sibling %q but siblingPreservation omits unknown sibling preservation", inputCase.ID, key)
				}
			}
		})
	}
}

func TestYouConfigSchemaDoesNotModelEnvOrFlagAsJSONProperties(t *testing.T) {
	t.Parallel()
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	root := document.(map[string]any)
	properties := root["properties"].(map[string]any)

	forbidden := []string{
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER",
		"YOU_DEFAULT_WORKER_MODEL",
		"--default-worker-model-provider",
		"--default-worker-model",
	}
	for _, name := range forbidden {
		if _, ok := properties[name]; ok {
			t.Fatalf("top-level property %q must not model env/flag precedence as JSON", name)
		}
	}

	defaults := properties["defaults"].(map[string]any)
	defaultProperties := defaults["properties"].(map[string]any)
	for _, name := range forbidden {
		if _, ok := defaultProperties[name]; ok {
			t.Fatalf("defaults property %q must not model env/flag precedence as JSON", name)
		}
	}
}

func TestYouConfigSchemaInstancePropertiesMatchInventoriedTopology(t *testing.T) {
	t.Parallel()
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
	documentationSchema, err := readJSONDocument(filepath.Join("common", "documentation.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if err := compiler.AddResource(documentationSchemaID, documentationSchema); err != nil {
		t.Fatalf("add documentation schema: %v", err)
	}
	deprecationsSchema, err := readJSONDocument(filepath.Join("common", "deprecations.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if err := compiler.AddResource(deprecationsSchemaID, deprecationsSchema); err != nil {
		t.Fatalf("add deprecations schema: %v", err)
	}
	precedenceSchema, err := readJSONDocument(filepath.Join("common", "precedence.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if err := compiler.AddResource(precedenceSchemaID, precedenceSchema); err != nil {
		t.Fatalf("add precedence schema: %v", err)
	}
	sensitivitySchema, err := readJSONDocument(filepath.Join("common", "sensitivity.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if err := compiler.AddResource(sensitivitySchemaID, sensitivitySchema); err != nil {
		t.Fatalf("add sensitivity schema: %v", err)
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

func decodeContractValue[T any](t *testing.T, value any) T {
	t.Helper()

	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal contract value: %v", err)
	}
	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode contract value: %v", err)
	}
	return decoded
}
