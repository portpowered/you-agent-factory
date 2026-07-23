package contracts_test

import (
	"path/filepath"
	"slices"
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
		{name: "unknown top-level field", fixture: filepath.Join(fixtureRoot, "invalid", "unknown-top-level.json"), wantPath: ""},
		{name: "unknown nested defaults field", fixture: filepath.Join(fixtureRoot, "invalid", "unknown-nested-defaults.json"), wantPath: "/defaults"},
		{name: "preset empty id", fixture: filepath.Join(fixtureRoot, "invalid", "preset-empty-id.json"), wantPath: "/workerPresets/0/id"},
		{name: "preset missing provider", fixture: filepath.Join(fixtureRoot, "invalid", "preset-missing-provider.json"), wantPath: "/workerPresets/0"},
		{name: "preset symbolic provider", fixture: filepath.Join(fixtureRoot, "invalid", "preset-symbolic-provider.json"), wantPath: "/workerPresets/0/modelProvider"},
		{name: "preset unsupported provider", fixture: filepath.Join(fixtureRoot, "invalid", "preset-unsupported-provider.json"), wantPath: "/workerPresets/0/modelProvider"},
		{name: "preset unsupported reasoning effort", fixture: filepath.Join(fixtureRoot, "invalid", "preset-unsupported-reasoning.json"), wantPath: "/workerPresets/0/reasoningEffort"},
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
