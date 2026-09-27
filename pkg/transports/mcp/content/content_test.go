package content

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
	"gopkg.in/yaml.v3"
)

func TestOperatorConfigSchemaMatchesOpenAPI(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	definitions := map[string]any{}
	pending := []string{"GlobalConfig"}
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if _, seen := definitions[name]; seen {
			continue
		}
		source, found := contract.Components.Schemas[name]
		if !found {
			t.Fatalf("OpenAPI schema %q not found", name)
		}
		converted := localSchemaRefs(source)
		definitions[name] = converted
		pending = append(pending, referencedSchemas(converted)...)
	}
	root, ok := definitions["GlobalConfig"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI GlobalConfig is not an object")
	}
	delete(definitions, "GlobalConfig")
	want := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "you://operator/config/schema",
		"title":   "Operator configuration",
		"$defs":   definitions,
	}
	for key, value := range root {
		want[key] = value
	}
	var got map[string]any
	if err := json.Unmarshal(SchemaBytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jsonRoundTrip(t, want), got) {
		t.Fatal("embedded operator config schema differs from api/openapi.yaml; regenerate it")
	}
}

func TestSkillFrontmatterMatchesEmbeddedDocument(t *testing.T) {
	t.Parallel()
	metadata, err := SkillFrontmatter()
	if err != nil {
		t.Fatal(err)
	}
	if metadata["name"] != "subagent-configuration" || metadata["description"] == "" {
		t.Fatalf("skill frontmatter = %#v", metadata)
	}
	skills := mcpgenerated.PrimarySkills()
	if len(skills) != 1 || !reflect.DeepEqual(metadata, skills[0].Frontmatter) {
		t.Fatalf("embedded skill frontmatter = %#v, generated manifest = %#v", metadata, skills)
	}
}

func localSchemaRefs(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		converted := make(map[string]any, len(typed))
		for key, child := range typed {
			if key == "$ref" {
				if ref, ok := child.(string); ok {
					converted[key] = strings.Replace(ref, "#/components/schemas/", "#/$defs/", 1)
					continue
				}
			}
			converted[key] = localSchemaRefs(child)
		}
		return converted
	case []any:
		converted := make([]any, len(typed))
		for index, child := range typed {
			converted[index] = localSchemaRefs(child)
		}
		return converted
	default:
		return value
	}
}

func referencedSchemas(value any) []string {
	var names []string
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok && strings.HasPrefix(ref, "#/$defs/") {
			names = append(names, strings.TrimPrefix(ref, "#/$defs/"))
		}
		for _, child := range typed {
			names = append(names, referencedSchemas(child)...)
		}
	case []any:
		for _, child := range typed {
			names = append(names, referencedSchemas(child)...)
		}
	}
	return names
}

func jsonRoundTrip(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
