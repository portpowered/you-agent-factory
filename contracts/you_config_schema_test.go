package contracts_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const youConfigSchemaID = "https://schemas.portpowered.com/you/config/you-config.schema.json"

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

func TestYouConfigSchemaRuntimeObservabilitySettings(t *testing.T) {
	t.Parallel()
	schema := youConfigSchema(t)

	tests := []struct {
		name       string
		document   string
		wantAccept bool
	}{
		{name: "partial", document: `{"runtime":{"logging":{"compress":true}}}`, wantAccept: true},
		{name: "complete", document: `{"runtime":{"logging":{"directory":"logs","maxSizeMB":1,"maxBackups":2,"maxAgeDays":3,"compress":true},"metrics":{"directory":"metrics","maxSizeMB":4,"maxBackups":5,"maxAgeDays":6,"compress":false}}}`, wantAccept: true},
		{name: "unknown setting", document: `{"runtime":{"logging":{"unknown":true}}}`},
		{name: "empty directory", document: `{"runtime":{"metrics":{"directory":" "}}}`},
		{name: "non-positive size", document: `{"runtime":{"logging":{"maxSizeMB":0}}}`},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			instance, err := jsonschema.UnmarshalJSON(strings.NewReader(test.document))
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			err = schema.Validate(instance)
			if test.wantAccept && err != nil {
				t.Fatalf("schema rejected runtime settings: %v", err)
			}
			if !test.wantAccept && err == nil {
				t.Fatal("schema accepted invalid runtime settings")
			}
		})
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
