package contractjoiner_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/santhosh-tekuri/jsonschema/v6"
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
	compileJoinedSchema(t, joinDocuments(t, input)[0].Value)
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
	if got := secondary["$ref"]; got != "https://schemas.example.test/widget.json" {
		t.Fatalf("repeated shared component reference = %v, want stable internal resource ID", got)
	}
	if got := primary["$id"]; got != "https://schemas.example.test/widget.json" {
		t.Fatalf("joined component $id = %v", got)
	}
	if got := countResourceID(alpha, "https://schemas.example.test/widget.json"); got != 1 {
		t.Fatalf("joined widget resource count = %d, want 1", got)
	}
	beta := object(t, documents[1].Value)
	assertPortableWidget(t, object(t, beta["items"]))
	compileJoinedSchema(t, alpha)
	compileJoinedSchema(t, beta)
}

func TestJoinDeduplicatesSharedStableIDResourceAndPreservesValidation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "contracts/root.json", `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$id":"https://schemas.example.test/root.json",
  "type":"object",
  "required":["a","b"],
  "properties":{
    "a":{"$ref":"component.json"},
    "b":{"$ref":"component.json"}
  }
}`)
	writeFile(t, root, "contracts/component.json", `{
  "$id":"https://schemas.example.test/component.json",
  "type":"string",
  "minLength":2
}`)

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/root.json"},
		Components:     []string{"contracts/component.json"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	joined := documents[0].Value
	if got := countResourceID(joined, "https://schemas.example.test/component.json"); got != 1 {
		t.Fatalf("joined component resource count = %d, want 1", got)
	}

	schema := compileJoinedSchema(t, joined)
	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "both shared uses pass", value: map[string]any{"a": "aa", "b": "bb"}, valid: true},
		{name: "first use fails", value: map[string]any{"a": "a", "b": "bb"}, valid: false},
		{name: "second use fails", value: map[string]any{"a": "aa", "b": 2}, valid: false},
		{name: "required use fails", value: map[string]any{"a": "aa"}, valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := schema.Validate(test.value)
			if (err == nil) != test.valid {
				t.Fatalf("Validate(%v) error = %v, want valid %t", test.value, err, test.valid)
			}
		})
	}
}

func TestJoinPreservesRelativeResourceIDWhenInliningAcrossBases(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "contracts/roots/root.json", `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$id":"https://schemas.example.test/roots/root.json",
  "type":"object",
  "required":["first","second"],
  "properties":{
    "first":{"$ref":"../components/component.json"},
    "second":{"$ref":"../components/component.json"}
  }
}`)
	writeFile(t, root, "contracts/components/component.json", `{
  "$id":"component.json",
  "type":"string",
  "minLength":2
}`)

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/roots/root.json"},
		Components:     []string{"contracts/components/component.json"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	joined := documents[0].Value
	if got := countResourceID(joined, "component.json"); got != 1 {
		t.Fatalf("joined authored relative resource count = %d, want 1", got)
	}
	properties := object(t, object(t, joined)["properties"])
	if got := object(t, properties["second"])["$ref"]; got != "https://schemas.example.test/components/component.json" {
		t.Fatalf("repeated relative resource reference = %v, want canonical embedded resource URI", got)
	}

	schema := compileJoinedSchema(t, joined)
	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "both uses pass", value: map[string]any{"first": "aa", "second": "bb"}, valid: true},
		{name: "first use fails", value: map[string]any{"first": "a", "second": "bb"}, valid: false},
		{name: "repeated use fails", value: map[string]any{"first": "aa", "second": 2}, valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := schema.Validate(test.value)
			if (err == nil) != test.valid {
				t.Fatalf("Validate(%v) error = %v, want valid %t", test.value, err, test.valid)
			}
		})
	}
}

func TestJoinPreservesRetrievalBaseForNestedRelativeResourceID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "contracts/roots/root.json", `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$id":"https://schemas.example.test/roots/root.json",
  "type":"object",
  "required":["container"],
  "properties":{"container":{"$ref":"../components/container.json"}}
}`)
	writeFile(t, root, "contracts/components/container.json", `{
  "type":"object",
  "required":["first","second"],
  "$defs":{"item":{"$id":"item.json","type":"string","minLength":2}},
  "properties":{
    "first":{"$ref":"#/$defs/item"},
    "second":{"$ref":"#/$defs/item"}
  }
}`)

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/roots/root.json"},
		Components:     []string{"contracts/components/container.json"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	joined := documents[0].Value
	if got := countResourceID(joined, "item.json"); got != 1 {
		t.Fatalf("joined nested relative resource count = %d, want 1", got)
	}
	referenceCount := 0
	walkJSON(joined, func(value map[string]any) {
		reference, ok := value["$ref"]
		if !ok {
			return
		}
		referenceCount++
		if reference != "https://schemas.example.test/components/item.json" {
			t.Fatalf("joined nested resource reference = %v, want canonical embedded resource URI", reference)
		}
	})
	if referenceCount != 2 {
		t.Fatalf("joined nested resource reference count = %d, want 2", referenceCount)
	}

	schema := compileJoinedSchema(t, joined)
	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "both uses pass", value: map[string]any{"container": map[string]any{"first": "aa", "second": "bb"}}, valid: true},
		{name: "first use fails", value: map[string]any{"container": map[string]any{"first": "a", "second": "bb"}}, valid: false},
		{name: "second use fails", value: map[string]any{"container": map[string]any{"first": "aa", "second": 2}}, valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := schema.Validate(test.value)
			if (err == nil) != test.valid {
				t.Fatalf("Validate(%v) error = %v, want valid %t", test.value, err, test.valid)
			}
		})
	}
}

func TestJoinDeduplicatesRelocatedIDLessResourceWithNestedRelativeID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "contracts/roots/root.json", `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$id":"https://schemas.example.test/roots/root.json",
  "type":"object",
  "required":["first","second"],
  "properties":{
    "first":{"$ref":"../components/container.json"},
    "second":{"$ref":"../components/container.json"}
  }
}`)
	writeFile(t, root, "contracts/components/container.json", `{
  "type":"object",
  "required":["value"],
  "$defs":{"item":{"$id":"item.json","type":"string","minLength":2}},
  "properties":{"value":{"$ref":"#/$defs/item"}}
}`)

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/roots/root.json"},
		Components:     []string{"contracts/components/container.json"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	joined := documents[0].Value
	relocationID := "https://schemas.example.test/components/container.json?you-join-source=contracts%2Fcomponents%2Fcontainer.json%23"
	if got := countResourceID(joined, relocationID); got != 1 {
		t.Fatalf("joined synthetic relocation resource count = %d, want 1", got)
	}
	if got := countResourceID(joined, "item.json"); got != 1 {
		t.Fatalf("joined authored nested resource count = %d, want 1", got)
	}
	properties := object(t, object(t, joined)["properties"])
	if got := object(t, properties["second"])["$ref"]; got != relocationID {
		t.Fatalf("repeated relocated resource reference = %v, want %s", got, relocationID)
	}

	schema := compileJoinedSchema(t, joined)
	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "both relocated uses pass", value: map[string]any{"first": map[string]any{"value": "aa"}, "second": map[string]any{"value": "bb"}}, valid: true},
		{name: "first relocated use fails", value: map[string]any{"first": map[string]any{"value": "a"}, "second": map[string]any{"value": "bb"}}, valid: false},
		{name: "second relocated use fails", value: map[string]any{"first": map[string]any{"value": "aa"}, "second": map[string]any{"value": 2}}, valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := schema.Validate(test.value)
			if (err == nil) != test.valid {
				t.Fatalf("Validate(%v) error = %v, want valid %t", test.value, err, test.valid)
			}
		})
	}
}

func TestJoinResolvesFragmentsWithinSelectedSchemaResourceScope(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "contracts/roots/root.json", `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$id":"https://schemas.example.test/roots/root.json",
  "type":"object",
  "required":["group"],
  "properties":{"group":{"$ref":"../components/component.json#/$defs/group"}}
}`)
	writeFile(t, root, "contracts/components/component.json", `{
  "$id":"https://schemas.example.test/catalog/component.json",
  "$defs":{
    "group":{
      "$id":"groups/group.json",
      "type":"object",
      "required":["first","second"],
      "$defs":{"item":{"$id":"item.json","type":"string","minLength":2}},
      "properties":{
        "first":{"$ref":"#/$defs/item"},
        "second":{"$ref":"#/$defs/item"}
      }
    }
  }
}`)

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/roots/root.json"},
		Components:     []string{"contracts/components/component.json"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	joined := documents[0].Value
	if got := countResourceID(joined, "groups/group.json"); got != 1 {
		t.Fatalf("joined selected resource count = %d, want 1", got)
	}
	if got := countResourceID(joined, "item.json"); got != 1 {
		t.Fatalf("joined selected descendant resource count = %d, want 1", got)
	}
	wantItemURI := "https://schemas.example.test/catalog/groups/item.json"
	itemReferenceCount := 0
	walkJSON(joined, func(value map[string]any) {
		if value["$ref"] == wantItemURI {
			itemReferenceCount++
		}
	})
	if itemReferenceCount != 2 {
		t.Fatalf("joined selected descendant reference count = %d, want 2 references to %s", itemReferenceCount, wantItemURI)
	}

	schema := compileJoinedSchema(t, joined)
	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "both scoped fragment uses pass", value: map[string]any{"group": map[string]any{"first": "aa", "second": "bb"}}, valid: true},
		{name: "first scoped fragment use fails", value: map[string]any{"group": map[string]any{"first": "a", "second": "bb"}}, valid: false},
		{name: "second scoped fragment use fails", value: map[string]any{"group": map[string]any{"first": "aa", "second": 2}}, valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := schema.Validate(test.value)
			if (err == nil) != test.valid {
				t.Fatalf("Validate(%v) error = %v, want valid %t", test.value, err, test.valid)
			}
		})
	}
}

func TestJoinDistinguishesRelativeResourceIDsUnderDifferentBases(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"contracts/root.json",
		"contracts/a/base.json",
		"contracts/a/item.json",
		"contracts/b/base.json",
		"contracts/b/item.json",
	}
	writeFile(t, root, paths[0], `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$id":"https://schemas.example.test/root.json",
  "type":"object",
  "required":["a","b"],
  "properties":{
    "a":{"$ref":"a/base.json"},
    "b":{"$ref":"b/base.json"}
  }
}`)
	writeFile(t, root, paths[1], `{
  "$id":"https://schemas.example.test/a/",
  "type":"object",
  "required":["first","second"],
  "properties":{"first":{"$ref":"item.json"},"second":{"$ref":"item.json"}}
}`)
	writeFile(t, root, paths[2], `{"$id":"item.json","type":"string","minLength":2}`)
	writeFile(t, root, paths[3], `{
  "$id":"https://schemas.example.test/b/",
  "type":"object",
  "required":["first","second"],
  "properties":{"first":{"$ref":"item.json"},"second":{"$ref":"item.json"}}
}`)
	writeFile(t, root, paths[4], `{"$id":"item.json","type":"integer","minimum":10}`)

	input := contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          paths[:1],
		Components:     paths[1:],
	}
	before := joinDocuments(t, input)[0].Value
	if got := countResourceID(before, "item.json"); got != 2 {
		t.Fatalf("distinct relative item resource count = %d, want 2", got)
	}
	joinedProperties := object(t, object(t, before)["properties"])
	for property, want := range map[string]string{
		"a": "https://schemas.example.test/a/item.json",
		"b": "https://schemas.example.test/b/item.json",
	} {
		baseProperties := object(t, object(t, joinedProperties[property])["properties"])
		if got := object(t, baseProperties["second"])["$ref"]; got != want {
			t.Fatalf("joined %s repeated resource reference = %v, want %s", property, got, want)
		}
	}
	assertDistinctRelativeResourcesValidate(t, before)

	beforeProperties := joinedProperties
	beforeA := canonicalValue(t, beforeProperties["a"])
	beforeB := canonicalValue(t, beforeProperties["b"])

	writeFile(t, root, paths[2], `{"$id":"item.json","type":"string","minLength":3}`)
	aEdited := joinDocuments(t, input)[0].Value
	aProperties := object(t, object(t, aEdited)["properties"])
	if bytes.Equal(beforeA, canonicalValue(t, aProperties["a"])) {
		t.Fatal("editing a/item.json did not change its consumer")
	}
	if !bytes.Equal(beforeB, canonicalValue(t, aProperties["b"])) {
		t.Fatal("editing a/item.json changed the distinct b resource")
	}

	writeFile(t, root, paths[4], `{"$id":"item.json","type":"integer","minimum":20}`)
	bEdited := joinDocuments(t, input)[0].Value
	bProperties := object(t, object(t, bEdited)["properties"])
	if !bytes.Equal(canonicalValue(t, aProperties["a"]), canonicalValue(t, bProperties["a"])) {
		t.Fatal("editing b/item.json changed the distinct a resource")
	}
	if bytes.Equal(canonicalValue(t, aProperties["b"]), canonicalValue(t, bProperties["b"])) {
		t.Fatal("editing b/item.json did not change its consumer")
	}
}

func assertDistinctRelativeResourcesValidate(t *testing.T, value any) {
	t.Helper()
	schema := compileJoinedSchema(t, value)
	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "both resources pass", value: map[string]any{"a": map[string]any{"first": "aa", "second": "bb"}, "b": map[string]any{"first": 10, "second": 11}}, valid: true},
		{name: "a first fails", value: map[string]any{"a": map[string]any{"first": "a", "second": "bb"}, "b": map[string]any{"first": 10, "second": 11}}, valid: false},
		{name: "a repeated use fails", value: map[string]any{"a": map[string]any{"first": "aa", "second": 2}, "b": map[string]any{"first": 10, "second": 11}}, valid: false},
		{name: "b first fails", value: map[string]any{"a": map[string]any{"first": "aa", "second": "bb"}, "b": map[string]any{"first": 9, "second": 11}}, valid: false},
		{name: "b repeated use fails", value: map[string]any{"a": map[string]any{"first": "aa", "second": "bb"}, "b": map[string]any{"first": 10, "second": "wrong"}}, valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := schema.Validate(test.value)
			if (err == nil) != test.valid {
				t.Fatalf("Validate(%v) error = %v, want valid %t", test.value, err, test.valid)
			}
		})
	}
}

func canonicalValue(t *testing.T, value any) []byte {
	t.Helper()
	contents, err := contractjoiner.MarshalCanonicalJSON(value)
	if err != nil {
		t.Fatalf("MarshalCanonicalJSON() error = %v", err)
	}
	return contents
}

func TestJoinPreservesAdjacentReferenceKeywordsAndTheirSemantics(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "contracts/root.json", `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$id":"https://schemas.example.test/stable-root.json",
  "$ref":"component.json",
  "maxProperties":1,
  "allOf":[{"required":["name"]}]
}`)
	writeFile(t, root, "contracts/component.json", `{
  "$id":"https://schemas.example.test/component.json",
  "type":"object",
  "properties":{"name":{"type":"string"}}
}`)

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/root.json"},
		Components:     []string{"contracts/component.json"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	joined := object(t, documents[0].Value)
	if got := joined["$id"]; got != "https://schemas.example.test/stable-root.json" {
		t.Fatalf("joined adjacent $id = %v, want stable root ID", got)
	}
	if got := joined["maxProperties"]; got != json.Number("1") {
		t.Fatalf("joined adjacent assertion = %v, want maxProperties 1", got)
	}
	schema := compileJoinedSchema(t, joined)
	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "both conjuncts pass", value: map[string]any{"name": "widget"}, valid: true},
		{name: "referenced type fails", value: "widget", valid: false},
		{name: "adjacent required fails", value: map[string]any{}, valid: false},
		{name: "adjacent maximum fails", value: map[string]any{"name": "widget", "extra": true}, valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := schema.Validate(test.value)
			if (err == nil) != test.valid {
				t.Fatalf("Validate(%v) error = %v, want valid %t", test.value, err, test.valid)
			}
		})
	}
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

func countResourceID(value any, identifier string) int {
	count := 0
	walkJSON(value, func(object map[string]any) {
		if object["$id"] == identifier {
			count++
		}
	})
	return count
}

type rejectingLoader struct{}

func (rejectingLoader) Load(resourceURL string) (any, error) {
	return nil, fmt.Errorf("unexpected external schema load: %s", resourceURL)
}

func compileJoinedSchema(t *testing.T, value any) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(rejectingLoader{})
	if err := compiler.AddResource("joined.json", value); err != nil {
		t.Fatalf("add joined schema: %v", err)
	}
	schema, err := compiler.Compile("joined.json")
	if err != nil {
		t.Fatalf("compile joined schema without external loading: %v", err)
	}
	return schema
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
	documents := joinDocuments(t, input)
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

func joinDocuments(t *testing.T, input contractjoiner.Input) []contractjoiner.Document {
	t.Helper()
	documents, diagnostics := contractjoiner.Join(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	return documents
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
