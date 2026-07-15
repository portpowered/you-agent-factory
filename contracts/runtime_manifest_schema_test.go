package contracts_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const runtimeManifestSchemaID = "https://schemas.portpowered.com/you/contracts/javascript/runtime-manifest.schema.json"

func runtimeManifestSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return compileSchema(
		t,
		filepath.Join("javascript", "runtime-manifest.schema.json"),
		runtimeManifestSchemaID,
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

func TestRuntimeManifestSchemaValidFixtures(t *testing.T) {
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "nested namespace", fixture: "valid-nested-namespace.json"},
		{name: "value globals", fixture: "valid-value.json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "javascript", test.fixture))
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("validate valid fixture %s: %v", test.fixture, err)
			}
			diagnostics := contractvalidator.RuntimeManifestSemanticsDiagnostics(test.fixture, instance)
			if len(diagnostics) != 0 {
				t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
			}
		})
	}
}

func TestRuntimeManifestSchemaCallableFixtures(t *testing.T) {
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "valid synchronous function", fixture: "valid-synchronous-function.json", valid: true},
		{name: "valid asynchronous function", fixture: "valid-asynchronous-function.json", valid: true},
		{
			name:     "async callable with synchronous return",
			fixture:  "invalid-impossible-async-return.json",
			wantPath: "/symbols/example.bad.async-sync-return/return/kind",
		},
		{
			name:     "sync callable with promise return",
			fixture:  "invalid-impossible-async-return.json",
			wantPath: "/symbols/example.bad.sync-promise-return/return/kind",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "javascript", test.fixture))
			err := schema.Validate(instance)
			if test.valid {
				if err != nil {
					t.Fatalf("validate valid fixture: %v", err)
				}
				diagnostics := contractvalidator.RuntimeManifestSemanticsDiagnostics(test.fixture, instance)
				if len(diagnostics) != 0 {
					t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
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

func TestRuntimeManifestSchemaCallableMetadataFixtures(t *testing.T) {
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "callback signature", fixture: "valid-callback.json"},
		{name: "emitted records and errors", fixture: "valid-emitted-record.json"},
		{name: "policy checks", fixture: "valid-policy.json"},
		{name: "resume and determinism notes", fixture: "valid-resume.json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "javascript", test.fixture))
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("validate valid fixture %s: %v", test.fixture, err)
			}
			diagnostics := contractvalidator.RuntimeManifestSemanticsDiagnostics(test.fixture, instance)
			if len(diagnostics) != 0 {
				t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
			}
		})
	}
}

func TestRuntimeManifestSchemaSymbolIntegrityFixtures(t *testing.T) {
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		wantPath string
	}{
		{
			name:     "duplicate symbol paths",
			fixture:  "invalid-duplicate-symbol-path.json",
			wantPath: "/symbols/example.workflow/path",
		},
		{
			name:     "broken parent reference",
			fixture:  "invalid-broken-parent.json",
			wantPath: "/symbols/example.workflow.ops/parent",
		},
		{
			name:     "unresolved namespace member",
			fixture:  "invalid-unresolved-member.json",
			wantPath: "/symbols/example.workflow/members/2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixturePath := filepath.Join("testdata", "javascript", test.fixture)
			instance := readJSON(t, fixturePath)
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("schema validation should pass before semantics: %v", err)
			}
			diagnostics := contractvalidator.RuntimeManifestSemanticsDiagnostics(test.fixture, instance)
			if len(diagnostics) == 0 {
				t.Fatal("expected semantic validation to fail")
			}
			paths := semanticDiagnosticPaths(diagnostics)
			if !slices.Contains(paths, test.wantPath) {
				t.Fatalf("semantic paths = %v, want %q", paths, test.wantPath)
			}
		})
	}
}

func TestRuntimeManifestSchemaObjectBindingFixtures(t *testing.T) {
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "lifecycle constraints and root metadata", fixture: "valid-lifecycle-constraints.json", valid: true},
		{
			name:     "active lifecycle with deprecated version",
			fixture:  "invalid-malformed-lifecycle.json",
			wantPath: "/symbols/example.malformed-lifecycle.args/lifecycle/deprecated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "javascript", test.fixture))
			err := schema.Validate(instance)
			if test.valid {
				if err != nil {
					t.Fatalf("validate valid fixture: %v", err)
				}
				diagnostics := contractvalidator.RuntimeManifestSemanticsDiagnostics(test.fixture, instance)
				if len(diagnostics) != 0 {
					t.Fatalf("semantic diagnostics = %#v, want none", diagnostics)
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
