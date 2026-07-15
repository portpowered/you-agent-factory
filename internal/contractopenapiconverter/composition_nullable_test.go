package contractopenapiconverter_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
)

func TestConvertCompositionNullableSchemaGoldenFixtures(t *testing.T) {
	cases := []string{
		"composition-allof",
		"composition-oneof",
		"composition-nullable",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root, components := loadRefsFixtureInput(t, name)
			first, diagnostics := contractopenapiconverter.ConvertCompositionNullableSchema(root, components)
			if len(diagnostics) != 0 {
				t.Fatalf("ConvertCompositionNullableSchema() diagnostics = %#v, want none", diagnostics)
			}
			firstJSON, err := contractjoiner.MarshalCanonicalJSON(first)
			if err != nil {
				t.Fatalf("MarshalCanonicalJSON() first run error = %v", err)
			}

			second, diagnostics := contractopenapiconverter.ConvertCompositionNullableSchema(root, components)
			if len(diagnostics) != 0 {
				t.Fatalf("ConvertCompositionNullableSchema() second run diagnostics = %#v, want none", diagnostics)
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

func TestConvertCompositionNullableSchemaRejectsAmbiguousNullable(t *testing.T) {
	cases := []struct {
		name     string
		schema   map[string]any
		wantCode string
	}{
		{
			name: "nullable without type",
			schema: map[string]any{
				"nullable": true,
				"enum":     []any{"a", "b"},
			},
			wantCode: "openapi.convert.ambiguous_nullable",
		},
		{
			name: "nullable with allOf",
			schema: map[string]any{
				"nullable": true,
				"type":     "string",
				"allOf": []any{
					map[string]any{"type": "string"},
				},
			},
			wantCode: "openapi.convert.ambiguous_nullable",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			converted, diagnostics := contractopenapiconverter.ConvertCompositionNullableSchema(testCase.schema, map[string]any{})
			if converted != nil {
				t.Fatalf("ConvertCompositionNullableSchema() = %#v, want nil output", converted)
			}
			if len(diagnostics) == 0 {
				t.Fatal("ConvertCompositionNullableSchema() diagnostics empty, want rejection")
			}
			if diagnostics[0].Code != testCase.wantCode {
				t.Fatalf("diagnostic code = %q, want %q", diagnostics[0].Code, testCase.wantCode)
			}
		})
	}
}

func TestConvertRefsSchemaStillRejectsCompositionKeywords(t *testing.T) {
	cases := []string{"allOf", "oneOf", "anyOf", "nullable"}
	for _, keyword := range cases {
		t.Run(keyword, func(t *testing.T) {
			schema := map[string]any{keyword: true}
			if keyword != "nullable" {
				schema = map[string]any{
					keyword: []any{map[string]any{"type": "string"}},
				}
			} else {
				schema = map[string]any{"type": "string", keyword: true}
			}
			converted, diagnostics := contractopenapiconverter.ConvertRefsSchema(schema, map[string]any{})
			if converted != nil {
				t.Fatalf("ConvertRefsSchema() = %#v, want nil output", converted)
			}
			if len(diagnostics) != 1 {
				t.Fatalf("ConvertRefsSchema() diagnostics = %#v, want one rejection", diagnostics)
			}
			if diagnostics[0].Code != "openapi.convert.unsupported_keyword" {
				t.Fatalf("diagnostic code = %q, want openapi.convert.unsupported_keyword", diagnostics[0].Code)
			}
		})
	}
}
