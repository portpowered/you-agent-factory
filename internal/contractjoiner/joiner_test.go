package contractjoiner_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
)

func TestCanonicalJoinedJSONMatchesGoldenAcrossRepeatedAndShuffledInputs(t *testing.T) {
	input := contractjoiner.Input{
		RepositoryRoot: ".",
		Roots: []string{
			"testdata/canonical/roots/standalone.json",
			"testdata/canonical/roots/catalog.json",
		},
		Components: []string{
			"testdata/canonical/components/scalar.json",
			"testdata/canonical/components/shared.json",
		},
	}

	first := joinAndMarshal(t, input)
	second := joinAndMarshal(t, input)
	shuffled := joinAndMarshal(t, contractjoiner.Input{
		RepositoryRoot: input.RepositoryRoot,
		Roots:          reversed(input.Roots),
		Components:     reversed(input.Components),
	})
	if !reflect.DeepEqual(first, second) {
		t.Fatal("repeated generation produced different paths or bytes")
	}
	if !reflect.DeepEqual(first, shuffled) {
		t.Fatal("shuffled inputs produced different paths or bytes")
	}
	if got, want := serializedDocumentPaths(first), []string{
		"testdata/canonical/roots/catalog.json",
		"testdata/canonical/roots/standalone.json",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical document paths = %v, want %v", got, want)
	}
	for index, document := range first {
		if sha256.Sum256(document.JSON) != sha256.Sum256(second[index].JSON) {
			t.Fatalf("repeated generation digest differs for %s", document.Path)
		}
	}

	golden, err := os.ReadFile("testdata/canonical/golden/catalog.json")
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	if got := first[0].JSON; !bytes.Equal(got, golden) {
		t.Fatalf("canonical catalog differs from golden:\ngot:\n%s\nwant:\n%s", got, golden)
	}
}

type serializedDocument struct {
	Path string
	JSON []byte
}

func TestJoinBuildsPortableDocumentsFromNestedSharedComponents(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "contracts/roots/alpha.json", `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$id":"https://schemas.example.test/alpha.json",
  "properties":{
    "primary":{"$ref":"../components/collection.json#/$defs/widget"},
    "secondary":{"$ref":"../components/widget.json"}
  }
}`)
	writeFile(t, root, "contracts/roots/beta.json", `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$id":"https://schemas.example.test/beta.json",
  "items":{"$ref":"../components/widget.json"}
}`)
	writeFile(t, root, "contracts/components/collection.json", `{
  "$id":"https://schemas.example.test/collection.json",
  "$defs":{"widget":{"$ref":"widget.json"}}
}`)
	writeFile(t, root, "contracts/components/widget.json", `{
  "$id":"https://schemas.example.test/widget.json",
  "type":"object",
  "properties":{"name":{"type":"string"}}
}`)

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/roots/beta.json", "contracts/roots/alpha.json"},
		Components: []string{
			"contracts/components/widget.json",
			"contracts/components/collection.json",
		},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	if got, want := documentPaths(documents), []string{"contracts/roots/alpha.json", "contracts/roots/beta.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Join() document paths = %v, want %v", got, want)
	}
	alpha := object(t, documents[0].Value)
	if got := alpha["$id"]; got != "https://schemas.example.test/alpha.json" {
		t.Fatalf("joined root $id = %v", got)
	}
	properties := object(t, alpha["properties"])
	primary := object(t, properties["primary"])
	secondary := object(t, properties["secondary"])
	assertPortableWidget(t, primary)
	assertPortableWidget(t, secondary)
	if !reflect.DeepEqual(primary, secondary) {
		t.Fatalf("shared component joined inconsistently:\nprimary=%v\nsecondary=%v", primary, secondary)
	}
	if got := primary["$id"]; got != "https://schemas.example.test/widget.json" {
		t.Fatalf("joined component $id = %v", got)
	}
	beta := object(t, documents[1].Value)
	assertPortableWidget(t, object(t, beta["items"]))
	assertNoReferences(t, documents)
}

func TestJoinPropagatesComponentEditsOnlyToConsumersAndPreservesInputs(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"contracts/roots/consumer-a.json",
		"contracts/roots/consumer-b.json",
		"contracts/roots/unaffected.json",
		"contracts/components/shared.json",
	}
	writeFile(t, root, paths[0], `{"value":{"$ref":"../components/shared.json"}}`)
	writeFile(t, root, paths[1], `{"nested":{"value":{"$ref":"../components/shared.json"}}}`)
	writeFile(t, root, paths[2], `{"type":"boolean"}`)
	writeFile(t, root, paths[3], `{"$id":"https://schemas.example.test/shared.json","type":"string"}`)
	beforeInputs := readFiles(t, root, paths)

	input := contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          paths[:3],
		Components:     paths[3:],
	}
	before, diagnostics := contractjoiner.Join(input)
	if len(diagnostics) != 0 {
		t.Fatalf("first Join() diagnostics = %+v", diagnostics)
	}
	if got := readFiles(t, root, paths); !reflect.DeepEqual(got, beforeInputs) {
		t.Fatal("Join() mutated authored inputs")
	}

	writeFile(t, root, paths[3], `{"$id":"https://schemas.example.test/shared.json","type":"number"}`)
	after, diagnostics := contractjoiner.Join(input)
	if len(diagnostics) != 0 {
		t.Fatalf("second Join() diagnostics = %+v", diagnostics)
	}
	beforeJSON := documentJSON(t, before)
	afterJSON := documentJSON(t, after)
	for _, consumer := range paths[:2] {
		if reflect.DeepEqual(beforeJSON[consumer], afterJSON[consumer]) {
			t.Fatalf("component edit did not change consumer %s", consumer)
		}
	}
	if !reflect.DeepEqual(beforeJSON[paths[2]], afterJSON[paths[2]]) {
		t.Fatalf("component edit changed unaffected document %s", paths[2])
	}
	if got := readFiles(t, root, paths[:3]); !reflect.DeepEqual(got, beforeInputs[:3]) {
		t.Fatal("regeneration mutated authored roots")
	}
}

func assertPortableWidget(t *testing.T, value map[string]any) {
	t.Helper()
	if got := value["type"]; got != "object" {
		t.Fatalf("joined component type = %v, want object", got)
	}
	name := object(t, object(t, value["properties"])["name"])
	if got := name["type"]; got != "string" {
		t.Fatalf("nested property type = %v, want string", got)
	}
}

func assertNoReferences(t *testing.T, documents []contractjoiner.Document) {
	t.Helper()
	for _, document := range documents {
		walkJSON(document.Value, func(value map[string]any) {
			if reference, ok := value["$ref"]; ok {
				t.Fatalf("joined document %s retained $ref %v", document.Path, reference)
			}
		})
	}
}

func walkJSON(value any, visit func(map[string]any)) {
	switch typed := value.(type) {
	case map[string]any:
		visit(typed)
		for _, child := range typed {
			walkJSON(child, visit)
		}
	case []any:
		for _, child := range typed {
			walkJSON(child, visit)
		}
	}
}

func object(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %T, want JSON object", value)
	}
	return result
}

func documentPaths(documents []contractjoiner.Document) []string {
	paths := make([]string, len(documents))
	for i, document := range documents {
		paths[i] = document.Path
	}
	return paths
}

func documentJSON(t *testing.T, documents []contractjoiner.Document) map[string][]byte {
	t.Helper()
	result := make(map[string][]byte, len(documents))
	for _, document := range documents {
		value, err := json.Marshal(document.Value)
		if err != nil {
			t.Fatalf("marshal %s: %v", document.Path, err)
		}
		result[document.Path] = value
	}
	return result
}

func joinAndMarshal(t *testing.T, input contractjoiner.Input) []serializedDocument {
	t.Helper()
	documents, diagnostics := contractjoiner.Join(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	result := make([]serializedDocument, 0, len(documents))
	for _, document := range documents {
		contents, err := contractjoiner.MarshalCanonicalJSON(document.Value)
		if err != nil {
			t.Fatalf("MarshalCanonicalJSON(%s): %v", document.Path, err)
		}
		result = append(result, serializedDocument{Path: document.Path, JSON: contents})
	}
	return result
}

func serializedDocumentPaths(documents []serializedDocument) []string {
	paths := make([]string, len(documents))
	for index, document := range documents {
		paths[index] = document.Path
	}
	return paths
}

func reversed(values []string) []string {
	result := append([]string(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func readFiles(t *testing.T, root string, paths []string) [][]byte {
	t.Helper()
	result := make([][]byte, len(paths))
	for i, path := range paths {
		value, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		result[i] = value
	}
	return result
}

func writeFile(t *testing.T, root, path, contents string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(absolute, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
