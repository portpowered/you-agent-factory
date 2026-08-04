package contractopenapiconverter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	"github.com/portpowered/infinite-you/internal/contractvalidator"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func TestConvertFailClosedSchemaRejectionFixtures(t *testing.T) {
	cases := []string{
		"reject-unsupported-keyword",
		"reject-external-ref",
		"reject-repository-escape",
		"reject-reference-cycle",
		"reject-ambiguous-discriminator",
		"reject-ambiguous-composition",
		"reject-ambiguous-default",
		"reject-ambiguous-nullable-ref",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root, components, want := loadRejectFixture(t, name)
			converted, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(root, components)
			if converted != nil {
				t.Fatalf("ConvertFailClosedSchema() = %#v, want nil output", converted)
			}
			assertDiagnosticIdentity(t, diagnostics, want)
		})
	}
}

func TestConvertFailClosedSchemaDiagnosticsStableAcrossRuns(t *testing.T) {
	root, components, want := loadRejectFixture(t, "reject-ambiguous-composition")
	for run := 0; run < 2; run++ {
		converted, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(root, components)
		if converted != nil {
			t.Fatalf("run %d: ConvertFailClosedSchema() = %#v, want nil output", run, converted)
		}
		assertDiagnosticIdentity(t, diagnostics, want)
	}
}

func TestConvertFailClosedSchemaAcceptsCompositionNullableGoldenFixtures(t *testing.T) {
	cases := []string{
		"composition-allof",
		"composition-oneof",
		"composition-nullable",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root, components := loadRefsFixtureInput(t, name)
			converted, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(root, components)
			if len(diagnostics) != 0 {
				t.Fatalf("ConvertFailClosedSchema() diagnostics = %#v, want none", diagnostics)
			}
			if converted == nil {
				t.Fatal("ConvertFailClosedSchema() = nil, want converted schema")
			}
		})
	}
}

func TestConvertFailClosedSchemaPreservesNegatedDispatchIdentityConstraint(t *testing.T) {
	source := map[string]any{
		"type":     "object",
		"required": []any{"type", "context"},
		"properties": map[string]any{
			"type":    map[string]any{"type": "string", "enum": []any{"DISPATCH_WORKER_SESSION_ASSOCIATION"}},
			"context": map[string]any{"type": "object"},
		},
		"not": map[string]any{
			"type":     "object",
			"required": []any{"type", "context"},
			"properties": map[string]any{
				"type": map[string]any{"type": "string", "enum": []any{"DISPATCH_WORKER_SESSION_ASSOCIATION"}},
				"context": map[string]any{
					"not": map[string]any{
						"type":     "object",
						"required": []any{"dispatchId"},
						"properties": map[string]any{
							"dispatchId": map[string]any{"type": "string", "minLength": 1},
						},
					},
				},
			},
		},
	}
	converted, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(source, nil)
	if len(diagnostics) != 0 {
		t.Fatalf("ConvertFailClosedSchema() diagnostics = %#v, want none", diagnostics)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("association.json", converted); err != nil {
		t.Fatalf("add converted schema: %v", err)
	}
	schema, err := compiler.Compile("association.json")
	if err != nil {
		t.Fatalf("compile converted schema: %v", err)
	}
	if err := schema.Validate(map[string]any{
		"type":    "DISPATCH_WORKER_SESSION_ASSOCIATION",
		"context": map[string]any{"dispatchId": "dispatch-actual-7"},
	}); err != nil {
		t.Fatalf("non-empty dispatch identity should validate: %v", err)
	}
	for name, event := range map[string]map[string]any{
		"missing dispatch ID": {"type": "DISPATCH_WORKER_SESSION_ASSOCIATION", "context": map[string]any{}},
		"empty dispatch ID":   {"type": "DISPATCH_WORKER_SESSION_ASSOCIATION", "context": map[string]any{"dispatchId": ""}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := schema.Validate(event); err == nil {
				t.Fatalf("association with %s should not validate", name)
			}
		})
	}
}

type expectedDiagnostic struct {
	Code     string
	Path     string
	Document string
	Message  string
}

func loadRejectFixture(t *testing.T, name string) (map[string]any, map[string]any, expectedDiagnostic) {
	t.Helper()
	path := filepath.Join("testdata", "inputs", name+".yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	var value map[string]any
	if err := yaml.Unmarshal(payload, &value); err != nil {
		t.Fatalf("decode fixture %s: %v", path, err)
	}
	root, ok := value["root"].(map[string]any)
	if !ok {
		t.Fatalf("fixture %s missing root object", name)
	}
	components := map[string]any{}
	if rawComponents, ok := value["components"].(map[string]any); ok {
		components = rawComponents
	}
	expectValue, ok := value["expect"].(map[string]any)
	if !ok {
		t.Fatalf("fixture %s missing expect object", name)
	}
	want := expectedDiagnostic{
		Code:     stringField(t, name, expectValue, "code"),
		Path:     stringField(t, name, expectValue, "path"),
		Document: stringField(t, name, expectValue, "document"),
		Message:  stringField(t, name, expectValue, "message"),
	}
	return root, components, want
}

func stringField(t *testing.T, fixtureName string, object map[string]any, key string) string {
	t.Helper()
	value, ok := object[key].(string)
	if !ok || value == "" {
		t.Fatalf("fixture %s expect.%s must be a non-empty string", fixtureName, key)
	}
	return value
}

func assertDiagnosticIdentity(t *testing.T, diagnostics []contractvalidator.Diagnostic, want expectedDiagnostic) {
	t.Helper()
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one rejection", diagnostics)
	}
	got := diagnostics[0]
	if got.Code != want.Code {
		t.Fatalf("diagnostic code = %q, want %q", got.Code, want.Code)
	}
	if got.Path != want.Path {
		t.Fatalf("diagnostic path = %q, want %q", got.Path, want.Path)
	}
	if got.Document != want.Document {
		t.Fatalf("diagnostic document = %q, want %q", got.Document, want.Document)
	}
	if got.Message != want.Message {
		t.Fatalf("diagnostic message = %q, want %q", got.Message, want.Message)
	}
}
