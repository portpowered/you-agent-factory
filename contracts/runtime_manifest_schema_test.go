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
	t.Parallel()
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "nested namespace", fixture: "valid-nested-namespace.json"},
		{name: "value globals", fixture: "valid-value.json"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
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
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
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
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
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
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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

func TestRuntimeManifestSchemaSignatureAndSerializableFixtures(t *testing.T) {
	t.Parallel()
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		valid    bool
		wantPath string
	}{
		{name: "closed serializable values", fixture: "valid-closed-serializable-value.json", valid: true},
		{name: "shared schema references", fixture: "valid-shared-schema-refs.json", valid: true},
		{
			name:     "duplicate parameter positions",
			fixture:  "invalid-signature-duplicate-position.json",
			wantPath: "/symbols/example.bad.duplicate-position/parameters/1/position",
		},
		{
			name:     "rest parameter not last",
			fixture:  "invalid-signature-rest-not-last.json",
			wantPath: "/symbols/example.bad.rest-not-last/parameters/0/rest",
		},
		{
			name:     "parameter position gap",
			fixture:  "invalid-signature-position-gap.json",
			wantPath: "/symbols/example.bad.position-gap/parameters/0/position",
		},
		{
			name:     "open serializable value schema",
			fixture:  "invalid-open-serializable-value.json",
			wantPath: "/symbols/example.bad.open-serializable/parameters/0/serializableValue",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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
			if err != nil {
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
	t.Parallel()
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
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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

func TestRuntimeManifestSchemaSupportedSurfaceFixtures(t *testing.T) {
	t.Parallel()
	schema := runtimeManifestSchema(t)

	tests := []struct {
		name     string
		fixture  string
		wantPath string
		wantCode string
	}{
		{
			name:     "context global",
			fixture:  "invalid-unsupported-context-global.json",
			wantPath: "/symbols/example.context/path",
			wantCode: "javascript.surface.forbidden_global",
		},
		{
			name:     "orchestrator global",
			fixture:  "invalid-unsupported-orchestrator-global.json",
			wantPath: "/symbols/example.orchestrator/path",
			wantCode: "javascript.surface.forbidden_global",
		},
		{
			name:     "comparison-project helper",
			fixture:  "invalid-unsupported-comparison-helper.json",
			wantPath: "/symbols/example.workflow.sleep/path",
			wantCode: "javascript.surface.unsupported_helper",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			instance := readJSON(t, filepath.Join("testdata", "javascript", test.fixture))
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("schema validation should pass before semantics: %v", err)
			}
			diagnostics := contractvalidator.RuntimeManifestSemanticsDiagnostics(test.fixture, instance)
			if len(diagnostics) == 0 {
				t.Fatal("expected semantic validation to fail")
			}
			found := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == test.wantCode && diagnostic.Path == test.wantPath {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("semantic diagnostics = %#v, want code=%q path=%q", diagnostics, test.wantCode, test.wantPath)
			}
		})
	}
}
