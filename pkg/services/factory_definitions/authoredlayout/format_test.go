package authoredlayout

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestFactorySourceLoaderProducesEquivalentJSONAndYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "equivalent.json")
	yamlPath := filepath.Join(dir, "equivalent.yml")
	jsonDocument := `{
		"name":"equivalent",
		"description":{
			"type":"LOCALIZABLE_ASSET",
			"value":"Base",
			"locales":["fr-FR"],
			"values":{"fr-FR":"Français"}
		},
		"examples":[{
			"name":"multiline",
			"description":{"type":"LOCALIZABLE_ASSET","value":"Example"},
			"args":{"prompt":"first line\nsecond line\n"}
		}]
	}`
	yamlDocument := `name: equivalent
description:
  type: LOCALIZABLE_ASSET
  value: Base
  locales: [fr-FR]
  values:
    fr-FR: Français
examples:
  - name: multiline
    description:
      type: LOCALIZABLE_ASSET
      value: Example
    args:
      prompt: |
        first line
        second line
`
	writeTestSource(t, jsonPath, jsonDocument)
	writeTestSource(t, yamlPath, yamlDocument)

	load := NewFactorySourceLoader(localTestFileSystem{})
	jsonBytes, err := load(jsonPath)
	if err != nil {
		t.Fatalf("load JSON: %v", err)
	}
	yamlBytes, err := load(yamlPath)
	if err != nil {
		t.Fatalf("load YAML: %v", err)
	}
	jsonFactory, err := factoryconfig.DecodeAuthoredFactoryAPI(jsonBytes)
	if err != nil {
		t.Fatalf("map JSON: %v", err)
	}
	yamlFactory, err := factoryconfig.DecodeAuthoredFactoryAPI(yamlBytes)
	if err != nil {
		t.Fatalf("map YAML: %v", err)
	}
	if !reflect.DeepEqual(jsonFactory, yamlFactory) {
		t.Fatalf("mapped factories differ:\nJSON: %#v\nYAML: %#v", jsonFactory, yamlFactory)
	}
	if yamlFactory.Examples == nil || len(*yamlFactory.Examples) != 1 {
		t.Fatalf("examples = %#v", yamlFactory.Examples)
	}
	prompt, err := (*yamlFactory.Examples)[0].Args["prompt"].AsFactoryInvocationArguments0()
	if err != nil {
		t.Fatalf("decode multiline example: %v", err)
	}
	if prompt != "first line\nsecond line\n" {
		t.Fatalf("multiline example = %q", prompt)
	}
}

func TestFactorySourceLoaderRejectsUnsafeOrInvalidYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "empty", body: "", wantErr: "empty"},
		{name: "malformed", body: "name: [", wantErr: "did not find expected node content"},
		{name: "multiple documents", body: "name: one\n---\nname: two\n", wantErr: "multiple YAML documents"},
		{name: "duplicate root key", body: "name: one\nname: two\n", wantErr: `duplicate YAML mapping key "name"`},
		{name: "duplicate nested key", body: "name: one\nmetadata:\n  key: one\n  key: two\n", wantErr: `duplicate YAML mapping key "key"`},
		{name: "non-string key", body: "name: one\n1: value\n", wantErr: "mapping keys must be strings"},
		{name: "non-finite number", body: "name: one\nvalue: .nan\n", wantErr: "non-finite"},
		{name: "timestamp", body: "name: one\nvalue: 2026-07-23T12:00:00Z\n", wantErr: `tag "!!timestamp" is not JSON-compatible`},
		{name: "custom tag", body: "name: !customer one\n", wantErr: `tag "!customer" is not JSON-compatible`},
		{name: "binary", body: "name: one\nvalue: !!binary SGVsbG8=\n", wantErr: `tag "!!binary" is not JSON-compatible`},
		{name: "alias", body: "name: &factory one\ncopy: *factory\n", wantErr: "anchors and aliases are not supported"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "factory.yaml")
			writeTestSource(t, path, test.body)
			_, err := NewFactorySourceLoader(localTestFileSystem{})(path)
			if err == nil {
				t.Fatal("expected strict YAML error")
			}
			for _, want := range []string{path, "as YAML", test.wantErr} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want substring %q", err, want)
				}
			}
		})
	}
}

func TestFactorySourceLoaderRejectsUnsupportedExplicitExtension(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "factory.toml")
	writeTestSource(t, path, `name = "unsupported"`)
	_, err := NewFactorySourceLoader(localTestFileSystem{})(path)
	if err == nil {
		t.Fatal("expected unsupported extension error")
	}
	for _, want := range []string{path, ".json", ".yaml", ".yml"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err, want)
		}
	}
}

func TestFactorySourceLoaderReportsJSONSourceContext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "factory.json")
	writeTestSource(t, path, `{"name":`)
	_, err := NewFactorySourceLoader(localTestFileSystem{})(path)
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
	for _, want := range []string{path, "as JSON", "unexpected end of JSON input"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err, want)
		}
	}
}

func writeTestSource(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
