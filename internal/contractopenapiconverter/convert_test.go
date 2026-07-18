package contractopenapiconverter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func TestConvertCoreSchemaGoldenFixtures(t *testing.T) {
	cases := []string{
		"primitive-string",
		"object-required",
		"array-items",
		"enum-string",
		"core-shapes-all",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			input := loadFixtureInput(t, name)
			first, diagnostics := contractopenapiconverter.ConvertCoreSchema(input)
			if len(diagnostics) != 0 {
				t.Fatalf("ConvertCoreSchema() diagnostics = %#v, want none", diagnostics)
			}
			firstJSON, err := contractjoiner.MarshalCanonicalJSON(first)
			if err != nil {
				t.Fatalf("MarshalCanonicalJSON() first run error = %v", err)
			}

			second, diagnostics := contractopenapiconverter.ConvertCoreSchema(input)
			if len(diagnostics) != 0 {
				t.Fatalf("ConvertCoreSchema() second run diagnostics = %#v, want none", diagnostics)
			}
			secondJSON, err := contractjoiner.MarshalCanonicalJSON(second)
			if err != nil {
				t.Fatalf("MarshalCanonicalJSON() second run error = %v", err)
			}
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatal("repeated conversion produced different bytes")
			}

			goldenPath := filepath.Join("testdata", "golden", name+".json")
			golden := readGoldenBytes(t, goldenPath)
			if !bytes.Equal(firstJSON, golden) {
				t.Fatalf("converted output differs from golden %s:\ngot:\n%s\nwant:\n%s", goldenPath, firstJSON, golden)
			}

			compileDraft202012Schema(t, first)
		})
	}
}

func TestConvertCoreSchemaRejectsUnsupportedKeywords(t *testing.T) {
	cases := []struct {
		name    string
		schema  map[string]any
		wantKey string
	}{
		{
			name:    "nullable",
			schema:  map[string]any{"type": "string", "nullable": true},
			wantKey: "nullable",
		},
		{
			name:    "ref",
			schema:  map[string]any{"$ref": "#/components/schemas/Widget"},
			wantKey: "$ref",
		},
		{
			name:    "vendor extension",
			schema:  map[string]any{"type": "string", "x-enum-varnames": []any{"A"}},
			wantKey: "x-enum-varnames",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			converted, diagnostics := contractopenapiconverter.ConvertCoreSchema(testCase.schema)
			if converted != nil {
				t.Fatalf("ConvertCoreSchema() = %#v, want nil output", converted)
			}
			if len(diagnostics) != 1 {
				t.Fatalf("ConvertCoreSchema() diagnostics = %#v, want one rejection", diagnostics)
			}
			if diagnostics[0].Code != "openapi.convert.unsupported_keyword" {
				t.Fatalf("diagnostic code = %q, want openapi.convert.unsupported_keyword", diagnostics[0].Code)
			}
			if got := diagnostics[0].Message; !bytes.Contains([]byte(got), []byte(testCase.wantKey)) {
				t.Fatalf("diagnostic message = %q, want keyword %q", got, testCase.wantKey)
			}
		})
	}
}

func TestConvertCoreSchemaPreservesOpenAPIExclusiveBounds(t *testing.T) {
	input := map[string]any{
		"type":             "number",
		"minimum":          0,
		"exclusiveMinimum": true,
		"maximum":          10000,
	}
	converted, diagnostics := contractopenapiconverter.ConvertCoreSchema(input)
	if len(diagnostics) != 0 {
		t.Fatalf("ConvertCoreSchema() diagnostics = %#v, want none", diagnostics)
	}
	if _, exists := converted["minimum"]; exists {
		t.Fatalf("converted schema = %#v, minimum must be replaced by the Draft 2020-12 exclusive bound", converted)
	}
	if converted["exclusiveMinimum"] != 0 {
		t.Fatalf("exclusiveMinimum = %#v, want numeric zero", converted["exclusiveMinimum"])
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("exclusive.json", converted); err != nil {
		t.Fatalf("AddResource(): %v", err)
	}
	schema, err := compiler.Compile("exclusive.json")
	if err != nil {
		t.Fatalf("Compile(): %v", err)
	}
	if err := schema.Validate(0); err == nil {
		t.Fatal("Validate(0) error = nil, want exclusive lower-bound rejection")
	}
	if err := schema.Validate(0.5); err != nil {
		t.Fatalf("Validate(0.5): %v", err)
	}

	failClosed, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(input, nil)
	if len(diagnostics) != 0 {
		t.Fatalf("ConvertFailClosedSchema() diagnostics = %#v, want none", diagnostics)
	}
	if _, exists := failClosed["minimum"]; exists || failClosed["exclusiveMinimum"] != 0 {
		t.Fatalf("fail-closed converted schema = %#v, want numeric exclusive lower bound", failClosed)
	}
}

func loadFixtureInput(t *testing.T, name string) map[string]any {
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
	return value
}

func compileDraft202012Schema(t *testing.T, schema map[string]any) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	document := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$defs": map[string]any{
			"fixture": schema,
		},
	}
	if err := compiler.AddResource("fixture.json", document); err != nil {
		t.Fatalf("AddResource(): %v", err)
	}
	if _, err := compiler.Compile("fixture.json#/$defs/fixture"); err != nil {
		t.Fatalf("Compile(): %v", err)
	}
}
