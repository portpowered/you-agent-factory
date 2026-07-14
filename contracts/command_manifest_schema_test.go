package contracts_test

import (
	"encoding/json"
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

func TestCommandManifestSchemaProductionRootManifest(t *testing.T) {
	schema := commandManifestSchema(t)
	instance := readJSON(t, filepath.Join("cli", "commands.json"))
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("validate production root manifest: %v", err)
	}
}

func TestCommandManifestSchemaProductionSessionFamily(t *testing.T) {
	instance := readJSON(t, filepath.Join("cli", "commands.json"))
	commands, ok := instance.(map[string]any)["commands"].(map[string]any)
	if !ok {
		t.Fatal("production manifest missing commands map")
	}

	session, ok := commands["you.session"].(map[string]any)
	if !ok {
		t.Fatal("production manifest missing you.session command")
	}
	if runnable, _ := session["runnable"].(bool); runnable {
		t.Fatal("you.session must be a non-runnable parent command")
	}

	show, ok := commands["you.session.show"].(map[string]any)
	if !ok {
		t.Fatal("production manifest missing you.session.show command")
	}
	if got, _ := show["path"].(string); got != "you session show" {
		t.Fatalf("you.session.show path = %q, want you session show", got)
	}

	args, ok := show["arguments"].(map[string]any)
	if !ok {
		t.Fatal("you.session.show missing arguments map")
	}
	arg, ok := args["you.session.show.arg.0"].(map[string]any)
	if !ok {
		t.Fatal("you.session.show missing session-id argument record")
	}
	if got, _ := arg["name"].(string); got != "session-id" {
		t.Fatalf("you.session.show argument name = %q, want session-id", got)
	}
	if required, _ := arg["required"].(bool); required {
		t.Fatal("you.session.show session-id argument must be optional")
	}
	switch max := arg["maxCardinality"].(type) {
	case float64:
		if int(max) != 1 {
			t.Fatalf("you.session.show session-id maxCardinality = %v, want 1", max)
		}
	case int:
		if max != 1 {
			t.Fatalf("you.session.show session-id maxCardinality = %d, want 1", max)
		}
	case int64:
		if max != 1 {
			t.Fatalf("you.session.show session-id maxCardinality = %d, want 1", max)
		}
	case json.Number:
		if got, err := max.Int64(); err != nil || got != 1 {
			t.Fatalf("you.session.show session-id maxCardinality = %v, want 1", max)
		}
	default:
		t.Fatalf("you.session.show session-id maxCardinality = %T(%v), want 1", arg["maxCardinality"], arg["maxCardinality"])
	}

	flags, ok := show["flags"].(map[string]any)
	if !ok {
		t.Fatal("you.session.show missing flags map")
	}
	port, ok := flags["you.session.show.flag.port"].(map[string]any)
	if !ok {
		t.Fatal("you.session.show missing hidden --port flag")
	}
	if got, _ := port["scope"].(string); got != "local" {
		t.Fatalf("you.session.show --port scope = %q, want local", got)
	}
	if got, _ := port["visibility"].(string); got != "hidden" {
		t.Fatalf("you.session.show --port visibility = %q, want hidden", got)
	}

	handler, ok := show["handler"].(map[string]any)
	if !ok {
		t.Fatal("you.session.show missing handler binding")
	}
	if got, _ := handler["id"].(string); got != "you.session.show.handler" {
		t.Fatalf("you.session.show handler id = %q, want you.session.show.handler", got)
	}
	if got, _ := handler["operationId"].(string); got != "getFactorySession" {
		t.Fatalf("you.session.show handler operationId = %q, want getFactorySession", got)
	}
}

func TestCommandManifestSchemaProductionModelsFamily(t *testing.T) {
	instance := readJSON(t, filepath.Join("cli", "commands.json"))
	commands, ok := instance.(map[string]any)["commands"].(map[string]any)
	if !ok {
		t.Fatal("production manifest missing commands map")
	}

	models, ok := commands["you.models"].(map[string]any)
	if !ok {
		t.Fatal("production manifest missing you.models command")
	}
	if runnable, _ := models["runnable"].(bool); runnable {
		t.Fatal("you.models must be a non-runnable parent command")
	}
	if got, _ := models["path"].(string); got != "you models" {
		t.Fatalf("you.models path = %q, want you models", got)
	}

	leafCases := []struct {
		commandID   string
		path        string
		operationID string
		requiresArg bool
	}{
		{commandID: "you.models.list", path: "you models list", operationID: "listModels"},
		{commandID: "you.models.inspect", path: "you models inspect", operationID: "getModel", requiresArg: true},
		{commandID: "you.models.invoke", path: "you models invoke", operationID: "invokeModel", requiresArg: true},
		{commandID: "you.models.pull", path: "you models pull", operationID: "pullModel", requiresArg: true},
	}

	for _, leaf := range leafCases {
		record, ok := commands[leaf.commandID].(map[string]any)
		if !ok {
			t.Fatalf("production manifest missing %s command", leaf.commandID)
		}
		if got, _ := record["path"].(string); got != leaf.path {
			t.Fatalf("%s path = %q, want %q", leaf.commandID, got, leaf.path)
		}
		if runnable, _ := record["runnable"].(bool); !runnable {
			t.Fatalf("%s must be runnable", leaf.commandID)
		}
		if leaf.requiresArg {
			args, ok := record["arguments"].(map[string]any)
			if !ok {
				t.Fatalf("%s missing arguments map", leaf.commandID)
			}
			arg, ok := args[leaf.commandID+".arg.0"].(map[string]any)
			if !ok {
				t.Fatalf("%s missing model-name argument record", leaf.commandID)
			}
			if got, _ := arg["name"].(string); got != "model-name" {
				t.Fatalf("%s argument name = %q, want model-name", leaf.commandID, got)
			}
			if required, _ := arg["required"].(bool); !required {
				t.Fatalf("%s model-name argument must be required", leaf.commandID)
			}
		} else if _, ok := record["arguments"]; ok {
			t.Fatalf("%s must not declare positional arguments", leaf.commandID)
		}

		flags, ok := record["flags"].(map[string]any)
		if !ok {
			t.Fatalf("%s missing flags map", leaf.commandID)
		}
		port, ok := flags[leaf.commandID+".flag.port"].(map[string]any)
		if !ok {
			t.Fatalf("%s missing hidden --port flag", leaf.commandID)
		}
		if got, _ := port["visibility"].(string); got != "hidden" {
			t.Fatalf("%s --port visibility = %q, want hidden", leaf.commandID, got)
		}

		handler, ok := record["handler"].(map[string]any)
		if !ok {
			t.Fatalf("%s missing handler binding", leaf.commandID)
		}
		if got, _ := handler["id"].(string); got != leaf.commandID+".handler" {
			t.Fatalf("%s handler id = %q, want %s.handler", leaf.commandID, got, leaf.commandID)
		}
		if got, _ := handler["operationId"].(string); got != leaf.operationID {
			t.Fatalf("%s handler operationId = %q, want %s", leaf.commandID, got, leaf.operationID)
		}
	}

	invoke, ok := commands["you.models.invoke"].(map[string]any)
	if !ok {
		t.Fatal("production manifest missing you.models.invoke command")
	}
	invokeFlags, ok := invoke["flags"].(map[string]any)
	if !ok {
		t.Fatal("you.models.invoke missing flags map")
	}
	for _, flagID := range []string{
		"you.models.invoke.flag.operation",
		"you.models.invoke.flag.text",
		"you.models.invoke.flag.output",
	} {
		if _, ok := invokeFlags[flagID]; !ok {
			t.Fatalf("you.models.invoke missing %s", flagID)
		}
	}
	operation, ok := invokeFlags["you.models.invoke.flag.operation"].(map[string]any)
	if !ok {
		t.Fatal("you.models.invoke missing --operation flag")
	}
	if got, _ := operation["default"].(string); got != "TTS" {
		t.Fatalf("you.models.invoke --operation default = %q, want TTS", got)
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
