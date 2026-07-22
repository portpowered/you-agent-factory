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

func TestConvertRefsSchemaGoldenFixtures(t *testing.T) {
	cases := []string{
		"refs-nested",
		"refs-shared",
		"refs-transitive",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root, components := loadRefsFixtureInput(t, name)
			first, diagnostics := contractopenapiconverter.ConvertRefsSchema(root, components)
			if len(diagnostics) != 0 {
				t.Fatalf("ConvertRefsSchema() diagnostics = %#v, want none", diagnostics)
			}
			firstJSON, err := contractjoiner.MarshalCanonicalJSON(first)
			if err != nil {
				t.Fatalf("MarshalCanonicalJSON() first run error = %v", err)
			}

			second, diagnostics := contractopenapiconverter.ConvertRefsSchema(root, components)
			if len(diagnostics) != 0 {
				t.Fatalf("ConvertRefsSchema() second run diagnostics = %#v, want none", diagnostics)
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

			compileDraft202012Document(t, first)
		})
	}
}

func TestConvertRefsSchemaDefsOrderIndependentOfDiscovery(t *testing.T) {
	root := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"zulu":  map[string]any{"$ref": "#/components/schemas/Zulu"},
			"alpha": map[string]any{"$ref": "#/components/schemas/Alpha"},
		},
	}
	components := map[string]any{
		"Alpha": map[string]any{"type": "string"},
		"Zulu":  map[string]any{"type": "integer"},
	}

	first, diagnostics := contractopenapiconverter.ConvertRefsSchema(root, components)
	if len(diagnostics) != 0 {
		t.Fatalf("ConvertRefsSchema() diagnostics = %#v, want none", diagnostics)
	}
	firstJSON, err := contractjoiner.MarshalCanonicalJSON(first)
	if err != nil {
		t.Fatalf("MarshalCanonicalJSON() error = %v", err)
	}

	reorderedRoot := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"alpha": map[string]any{"$ref": "#/components/schemas/Alpha"},
			"zulu":  map[string]any{"$ref": "#/components/schemas/Zulu"},
		},
	}
	reorderedComponents := map[string]any{
		"Zulu":  map[string]any{"type": "integer"},
		"Alpha": map[string]any{"type": "string"},
	}
	second, diagnostics := contractopenapiconverter.ConvertRefsSchema(reorderedRoot, reorderedComponents)
	if len(diagnostics) != 0 {
		t.Fatalf("ConvertRefsSchema() reordered diagnostics = %#v, want none", diagnostics)
	}
	secondJSON, err := contractjoiner.MarshalCanonicalJSON(second)
	if err != nil {
		t.Fatalf("MarshalCanonicalJSON() reordered error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("discovery-order change produced different bytes:\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
}

func TestConvertRefsSchemaRejectsUnsupportedReferences(t *testing.T) {
	cases := []struct {
		name       string
		root       map[string]any
		components map[string]any
		wantCode   string
	}{
		{
			name: "external url",
			root: map[string]any{
				"$ref": "https://example.com/schema.json",
			},
			wantCode: "openapi.convert.unsupported_reference",
		},
		{
			name: "repository escape",
			root: map[string]any{
				"$ref": "../other/schema.json",
			},
			wantCode: "openapi.convert.unsupported_reference",
		},
		{
			name: "absolute path",
			root: map[string]any{
				"$ref": "/components/schemas/Widget",
			},
			wantCode: "openapi.convert.unsupported_reference",
		},
		{
			name: "wrong fragment prefix",
			root: map[string]any{
				"$ref": "#/definitions/Widget",
			},
			wantCode: "openapi.convert.unsupported_reference",
		},
		{
			name: "missing component",
			root: map[string]any{
				"$ref": "#/components/schemas/Missing",
			},
			components: map[string]any{},
			wantCode:   "openapi.convert.missing_component",
		},
		{
			name: "reference cycle",
			root: map[string]any{
				"$ref": "#/components/schemas/Alpha",
			},
			components: map[string]any{
				"Alpha": map[string]any{"$ref": "#/components/schemas/Beta"},
				"Beta":  map[string]any{"$ref": "#/components/schemas/Alpha"},
			},
			wantCode: "openapi.convert.reference_cycle",
		},
		{
			name: "ref with siblings",
			root: map[string]any{
				"$ref": "#/components/schemas/Widget",
				"type": "object",
			},
			components: map[string]any{
				"Widget": map[string]any{"type": "string"},
			},
			wantCode: "openapi.convert.unsupported_reference",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			converted, diagnostics := contractopenapiconverter.ConvertRefsSchema(testCase.root, testCase.components)
			if converted != nil {
				t.Fatalf("ConvertRefsSchema() = %#v, want nil output", converted)
			}
			if len(diagnostics) == 0 {
				t.Fatal("ConvertRefsSchema() diagnostics empty, want rejection")
			}
			if diagnostics[0].Code != testCase.wantCode {
				t.Fatalf("diagnostic code = %q, want %q", diagnostics[0].Code, testCase.wantCode)
			}
		})
	}
}

func loadRefsFixtureInput(t *testing.T, name string) (map[string]any, map[string]any) {
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
	components, ok := value["components"].(map[string]any)
	if !ok {
		t.Fatalf("fixture %s missing components object", name)
	}
	return root, components
}

func compileDraft202012Document(t *testing.T, schema map[string]any) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	document := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
	}
	for key, value := range schema {
		document[key] = value
	}
	if err := compiler.AddResource("fixture.json", document); err != nil {
		t.Fatalf("AddResource(): %v", err)
	}
	if _, err := compiler.Compile("fixture.json"); err != nil {
		t.Fatalf("Compile(): %v", err)
	}
}
