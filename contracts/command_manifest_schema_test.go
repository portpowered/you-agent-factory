package contracts_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const commandManifestSchemaID = "https://schemas.portpowered.com/you/contracts/cli/command-manifest.schema.json"

func commandManifestSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return compileSchema(
		t,
		filepath.Join("cli", "command-manifest.schema.json"),
		commandManifestSchemaID,
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

func TestCommandManifestSchemaValidIdentityFixture(t *testing.T) {
	schema := commandManifestSchema(t)
	instance := readJSON(t, filepath.Join("testdata", "cli", "valid-identity.json"))
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("validate valid identity fixture: %v", err)
	}
}

func TestCommandManifestSchemaArgumentFixtures(t *testing.T) {
	schema := commandManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "valid optional positional argument", fixture: "valid-optional-argument.json", valid: true},
		{name: "valid variadic argument", fixture: "valid-variadic-argument.json", valid: true},
		{
			name:     "variadic argument with bounded max cardinality",
			fixture:  "invalid-argument-cardinality.json",
			wantPath: "/commands/example.factory.collect/arguments/example.factory.collect.arg.items/maxCardinality",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "cli", test.fixture))
			err := schema.Validate(instance)
			if test.valid {
				if err != nil {
					t.Fatalf("validate valid fixture: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected fixture validation to fail")
			}
			if paths := validationPaths(t, err); !slices.Contains(paths, test.wantPath) {
				t.Fatalf("validation paths = %v, want %q", paths, test.wantPath)
			}
		})
	}
}

func TestCommandManifestSchemaFlagFixtures(t *testing.T) {
	schema := commandManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "valid persistent flag", fixture: "valid-persistent-flag.json", valid: true},
		{name: "valid inherited flag", fixture: "valid-inherited-flag.json", valid: true},
		{name: "valid no-option flag", fixture: "valid-no-option-flag.json", valid: true},
		{
			name:     "unknown flag property",
			fixture:  "invalid-flag-unknown-property.json",
			wantPath: "/commands/example.factory.inspect/flags/example.factory.inspect.flag.output",
		},
		{
			name:     "no-option default on string flag",
			fixture:  "invalid-flag-scope-value.json",
			wantPath: "/commands/example.factory.publish/flags/example.factory.publish.flag.channel/valueType",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "cli", test.fixture))
			err := schema.Validate(instance)
			if test.valid {
				if err != nil {
					t.Fatalf("validate valid fixture: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected fixture validation to fail")
			}
			if paths := validationPaths(t, err); !slices.Contains(paths, test.wantPath) {
				t.Fatalf("validation paths = %v, want %q", paths, test.wantPath)
			}
		})
	}
}

func TestCommandManifestSchemaRelationshipFixtures(t *testing.T) {
	schema := commandManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "valid mutex relationship", fixture: "valid-mutex-relationship.json", valid: true},
		{name: "valid required-together relationship", fixture: "valid-required-together-relationship.json", valid: true},
		{name: "valid conditional relationship", fixture: "valid-conditional-relationship.json", valid: true},
		{
			name:     "mutex relationship with one participant",
			fixture:  "invalid-relationship-impossible.json",
			wantPath: "/commands/example.factory.export/relationships/example.factory.export.rel.mutex.archive/participants",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "cli", test.fixture))
			err := schema.Validate(instance)
			if test.valid {
				if err != nil {
					t.Fatalf("validate valid fixture: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected fixture validation to fail")
			}
			if paths := validationPaths(t, err); !slices.Contains(paths, test.wantPath) {
				t.Fatalf("validation paths = %v, want %q", paths, test.wantPath)
			}
		})
	}
}

func TestCommandManifestSchemaValidFixtureMatrix(t *testing.T) {
	schema := commandManifestSchema(t)

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "identity metadata", fixture: "valid-identity.json"},
		{name: "optional positional argument", fixture: "valid-optional-argument.json"},
		{name: "variadic argument", fixture: "valid-variadic-argument.json"},
		{name: "persistent flag", fixture: "valid-persistent-flag.json"},
		{name: "inherited flag", fixture: "valid-inherited-flag.json"},
		{name: "no-option flag", fixture: "valid-no-option-flag.json"},
		{name: "mutex relationship", fixture: "valid-mutex-relationship.json"},
		{name: "required-together relationship", fixture: "valid-required-together-relationship.json"},
		{name: "conditional relationship", fixture: "valid-conditional-relationship.json"},
		{name: "precedence and execution metadata", fixture: "valid-precedence.json"},
		{name: "handler binding", fixture: "valid-handler-binding.json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "cli", test.fixture))
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("validate valid fixture %s: %v", test.fixture, err)
			}
		})
	}
}

func TestCommandManifestSchemaInvalidFixtureMatrix(t *testing.T) {
	schema := commandManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		wantPath string
	}{
		{
			name:     "invalid argument cardinality",
			fixture:  "invalid-argument-cardinality.json",
			wantPath: "/commands/example.factory.collect/arguments/example.factory.collect.arg.items/maxCardinality",
		},
		{
			name:     "unknown flag property",
			fixture:  "invalid-flag-unknown-property.json",
			wantPath: "/commands/example.factory.inspect/flags/example.factory.inspect.flag.output",
		},
		{
			name:     "no-option default on string flag",
			fixture:  "invalid-flag-scope-value.json",
			wantPath: "/commands/example.factory.publish/flags/example.factory.publish.flag.channel/valueType",
		},
		{
			name:     "impossible mutex relationship",
			fixture:  "invalid-relationship-impossible.json",
			wantPath: "/commands/example.factory.export/relationships/example.factory.export.rel.mutex.archive/participants",
		},
		{
			name:     "invalid handler id",
			fixture:  "invalid-handler-id.json",
			wantPath: "/commands/example.factory.dispatch/handler/id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "cli", test.fixture))
			err := schema.Validate(instance)
			if err == nil {
				t.Fatal("expected fixture validation to fail")
			}
			if paths := validationPaths(t, err); !slices.Contains(paths, test.wantPath) {
				t.Fatalf("validation paths = %v, want %q", paths, test.wantPath)
			}
		})
	}
}

func TestCommandManifestSchemaExecutionMetadataFixtures(t *testing.T) {
	schema := commandManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "valid precedence and execution metadata", fixture: "valid-precedence.json", valid: true},
		{name: "valid handler binding", fixture: "valid-handler-binding.json", valid: true},
		{
			name:     "invalid handler id",
			fixture:  "invalid-handler-id.json",
			wantPath: "/commands/example.factory.dispatch/handler/id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "cli", test.fixture))
			err := schema.Validate(instance)
			if test.valid {
				if err != nil {
					t.Fatalf("validate valid fixture: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected fixture validation to fail")
			}
			if paths := validationPaths(t, err); !slices.Contains(paths, test.wantPath) {
				t.Fatalf("validation paths = %v, want %q", paths, test.wantPath)
			}
		})
	}
}
