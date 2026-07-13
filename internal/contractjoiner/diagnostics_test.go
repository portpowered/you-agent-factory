package contractjoiner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func TestJoinRejectsUnsafeReferenceGraphsWithoutPartialDocuments(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	writeFile(t, root, "roots/direct-cycle.json", `{"node":{"$ref":"#"}}`)
	writeFile(t, root, "roots/indirect-cycle.json", `{"$ref":"../components/first.json"}`)
	writeFile(t, root, "components/first.json", `{"$ref":"second.json"}`)
	writeFile(t, root, "components/second.json", `{"$ref":"first.json"}`)
	writeFile(t, root, "components/target.json", `{"type":"string"}`)
	writeFile(t, root, "components/pointers.json", `{"foo~bar":{"type":"number"},"foo~2bar":{"type":"string"}}`)
	writeFile(t, parent, "outside.json", `{}`)
	writeFile(t, parent, "repository-copy/outside.json", `{}`)
	absPath := filepath.Join(parent, "outside.json")

	tests := []struct {
		name       string
		rootPath   string
		contents   string
		components []string
		code       string
		document   string
		path       string
	}{
		{name: "direct cycle", rootPath: "roots/direct-cycle.json", components: nil, code: "reference.cycle", document: "roots/direct-cycle.json", path: "/node/$ref"},
		{name: "indirect cycle", rootPath: "roots/indirect-cycle.json", components: []string{"components/first.json", "components/second.json"}, code: "reference.cycle", document: "components/second.json", path: "/$ref"},
		{name: "network URL", rootPath: "roots/network.json", contents: `{"value":{"$ref":"https://example.test/schema.json"}}`, code: "reference.unsupported", document: "roots/network.json", path: "/value/$ref"},
		{name: "file URL", rootPath: "roots/file-url.json", contents: `{"value":{"$ref":"file:///tmp/schema.json"}}`, code: "reference.unsupported", document: "roots/file-url.json", path: "/value/$ref"},
		{name: "absolute path", rootPath: "roots/absolute.json", contents: fmt.Sprintf(`{"value":{"$ref":%q}}`, absPath), code: "reference.unsupported", document: "roots/absolute.json", path: "/value/$ref"},
		{name: "empty reference", rootPath: "roots/empty.json", contents: `{"value":{"$ref":""}}`, code: "reference.invalid", document: "roots/empty.json", path: "/value/$ref"},
		{name: "non-string reference", rootPath: "roots/non-string.json", contents: `{"value":{"$ref":42}}`, code: "reference.invalid", document: "roots/non-string.json", path: "/value/$ref"},
		{name: "query", rootPath: "roots/query.json", contents: `{"value":{"$ref":"../components/target.json?version=1"}}`, components: []string{"components/target.json"}, code: "reference.unsupported", document: "roots/query.json", path: "/value/$ref"},
		{name: "unsupported fragment", rootPath: "roots/fragment.json", contents: `{"value":{"$ref":"../components/target.json#anchor"}}`, components: []string{"components/target.json"}, code: "reference.fragment", document: "roots/fragment.json", path: "/value/$ref"},
		{name: "bare JSON Pointer escape", rootPath: "roots/bare-pointer-escape.json", contents: `{"value":{"$ref":"../components/pointers.json#/foo~bar"}}`, components: []string{"components/pointers.json"}, code: "reference.fragment", document: "roots/bare-pointer-escape.json", path: "/value/$ref"},
		{name: "invalid JSON Pointer escape with matching key", rootPath: "roots/invalid-pointer-escape.json", contents: `{"value":{"$ref":"../components/pointers.json#/foo~2bar"}}`, components: []string{"components/pointers.json"}, code: "reference.fragment", document: "roots/invalid-pointer-escape.json", path: "/value/$ref"},
		{name: "external dynamic reference", rootPath: "roots/dynamic-external.json", contents: `{"nested":{"$dynamicRef":"https://example.test/external.json#node"}}`, code: "reference.unsupported", document: "roots/dynamic-external.json", path: "/nested/$dynamicRef"},
		{name: "repository-relative dynamic reference", rootPath: "roots/dynamic-relative.json", contents: `{"nested":{"$dynamicRef":"../components/target.json#node"}}`, components: []string{"components/target.json"}, code: "reference.unsupported", document: "roots/dynamic-relative.json", path: "/nested/$dynamicRef"},
		{name: "legacy recursive reference", rootPath: "roots/recursive.json", contents: `{"nested":{"$recursiveRef":"#"}}`, code: "reference.unsupported", document: "roots/recursive.json", path: "/nested/$recursiveRef"},
		{name: "missing target", rootPath: "roots/missing.json", contents: `{"value":{"$ref":"../components/missing.json"}}`, components: []string{"components/missing.json"}, code: "reference.missing", document: "roots/missing.json", path: "/value/$ref"},
		{name: "repository escape", rootPath: "roots/escape.json", contents: `{"value":{"$ref":"../../outside.json"}}`, code: "reference.escape", document: "roots/escape.json", path: "/value/$ref"},
		{name: "sibling prefix escape", rootPath: "roots/sibling.json", contents: `{"value":{"$ref":"../../repository-copy/outside.json"}}`, code: "reference.escape", document: "roots/sibling.json", path: "/value/$ref"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.contents != "" {
				writeFile(t, root, test.rootPath, test.contents)
			}
			documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
				RepositoryRoot: root,
				Roots:          []string{test.rootPath},
				Components:     test.components,
			})
			if documents != nil {
				t.Fatalf("Join() documents = %+v, want no partial output", documents)
			}
			assertJoinDiagnostic(t, diagnostics, test.code, test.document, test.path, root)
		})
	}
}

func TestJoinRejectsMalformedJSONPointerEscapesDeterministically(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "roots/a.json", `{"value":{"$ref":"../components/pointers.json#/foo~2bar"}}`)
	writeFile(t, root, "roots/b.json", `{"value":{"$ref":"../components/pointers.json#/foo~bar"}}`)
	writeFile(t, root, "components/pointers.json", `{"foo~bar":{"type":"number"},"foo~2bar":{"type":"string"}}`)
	input := contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"roots/a.json", "roots/b.json"},
		Components:     []string{"components/pointers.json"},
	}

	documents, forward := contractjoiner.Join(input)
	shuffledDocuments, shuffled := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          reversed(input.Roots),
		Components:     reversed(input.Components),
	})
	if documents != nil || shuffledDocuments != nil {
		t.Fatalf("Join() returned partial documents: forward=%+v shuffled=%+v", documents, shuffledDocuments)
	}
	if !reflect.DeepEqual(forward, shuffled) {
		t.Fatalf("input order changed malformed-pointer diagnostics:\nforward: %+v\nshuffled: %+v", forward, shuffled)
	}
	want := []contractvalidator.Diagnostic{
		{Code: "reference.fragment", Path: "/value/$ref", Message: `reference "../components/pointers.json#/foo~2bar" has an unresolved fragment`, Document: "roots/a.json"},
		{Code: "reference.fragment", Path: "/value/$ref", Message: `reference "../components/pointers.json#/foo~bar" has an unresolved fragment`, Document: "roots/b.json"},
	}
	if !reflect.DeepEqual(forward, want) {
		t.Fatalf("Join() diagnostics = %+v, want %+v", forward, want)
	}
}

func TestJoinResolvesValidJSONPointerEscapes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "roots/root.json", `{
  "properties":{
    "tilde":{"$ref":"../components/pointers.json#/foo~0bar"},
    "slash":{"$ref":"../components/pointers.json#/foo~1bar"}
  }
}`)
	writeFile(t, root, "components/pointers.json", `{"foo~bar":{"type":"number"},"foo/bar":{"type":"string"}}`)

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"roots/root.json"},
		Components:     []string{"components/pointers.json"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Join() diagnostics = %+v, want none", diagnostics)
	}
	if len(documents) != 1 {
		t.Fatalf("Join() documents = %+v, want one", documents)
	}
	rootObject := documents[0].Value.(map[string]any)
	properties := rootObject["properties"].(map[string]any)
	if got := properties["tilde"].(map[string]any)["type"]; got != "number" {
		t.Fatalf("escaped tilde target type = %v, want number", got)
	}
	if got := properties["slash"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("escaped slash target type = %v, want string", got)
	}
}

func TestJoinDiagnosticsUseTotalOrderIndependentOfInputOrder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "roots/a.json", `{"$ref":"../z-components/first.json"}`)
	writeFile(t, root, "roots/b.json", `{"value":{"$dynamicRef":"https://example.test/external.json#node"}}`)
	writeFile(t, root, "z-components/first.json", `{"$ref":"second.json"}`)
	writeFile(t, root, "z-components/second.json", `{"$ref":"first.json"}`)
	input := contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"roots/a.json", "roots/b.json"},
		Components:     []string{"z-components/first.json", "z-components/second.json"},
	}

	_, forward := contractjoiner.Join(input)
	_, shuffled := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: input.RepositoryRoot,
		Roots:          reversed(input.Roots),
		Components:     reversed(input.Components),
	})
	if !reflect.DeepEqual(forward, shuffled) {
		t.Fatalf("input order changed diagnostics:\nforward: %+v\nshuffled: %+v", forward, shuffled)
	}
	want := []contractvalidator.Diagnostic{
		{Code: "reference.unsupported", Path: "/value/$dynamicRef", Message: `reference keyword "$dynamicRef" is not supported`, Document: "roots/b.json"},
		{Code: "reference.cycle", Path: "/$ref", Message: `reference "first.json" forms a cycle`, Document: "z-components/second.json"},
	}
	if !reflect.DeepEqual(forward, want) {
		t.Fatalf("Join() diagnostics = %+v, want %+v", forward, want)
	}
}

func TestJoinRejectsDistinctResourcesWithTheSameResolvedIDDeterministically(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "roots/root.json", `{
  "$id":"https://schemas.example.test/root.json",
  "properties":{
    "a":{"$ref":"../components/a.json"},
    "b":{"$ref":"../components/b.json"}
  }
}`)
	writeFile(t, root, "components/a.json", `{"$id":"https://schemas.example.test/item.json","type":"string"}`)
	writeFile(t, root, "components/b.json", `{"$id":"https://schemas.example.test/item.json","type":"integer"}`)
	input := contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"roots/root.json"},
		Components:     []string{"components/a.json", "components/b.json"},
	}

	documents, forward := contractjoiner.Join(input)
	shuffledDocuments, shuffled := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          input.Roots,
		Components:     reversed(input.Components),
	})
	if documents != nil || shuffledDocuments != nil {
		t.Fatalf("Join() returned partial documents: forward=%+v shuffled=%+v", documents, shuffledDocuments)
	}
	if !reflect.DeepEqual(forward, shuffled) {
		t.Fatalf("input order changed collision diagnostics:\nforward: %+v\nshuffled: %+v", forward, shuffled)
	}
	want := []contractvalidator.Diagnostic{{
		Code:     "reference.resource_collision",
		Path:     "/$id",
		Message:  `stable resource URI "https://schemas.example.test/item.json" is declared by multiple authored resources`,
		Document: "components/b.json",
	}}
	if !reflect.DeepEqual(forward, want) {
		t.Fatalf("Join() diagnostics = %+v, want %+v", forward, want)
	}
}

func TestJoinRejectsAuthoredInputsInsideGeneratedOutput(t *testing.T) {
	root := t.TempDir()
	generated := "packages/api/generated/joined/contract.json"
	writeFile(t, root, generated, `{}`)
	writeFile(t, root, "roots/safe.json", `{}`)

	for _, inputPath := range []string{
		generated,
		`packages\api\generated\joined\contract.json`,
		"packages/api/components/../generated/joined/contract.json",
	} {
		t.Run(inputPath, func(t *testing.T) {
			documents, diagnostics := contractjoiner.Join(contractjoiner.Input{RepositoryRoot: root, Roots: []string{inputPath}})
			if documents != nil {
				t.Fatalf("Join() documents = %+v, want none", documents)
			}
			assertJoinDiagnostic(t, diagnostics, "join.input.generated", generated, "/", root)
		})
	}

	_, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"roots/safe.json"},
		Components:     []string{generated},
	})
	assertJoinDiagnostic(t, diagnostics, "join.input.generated", generated, "/", root)
}

func TestJoinRejectsSymlinkAliasIntoGeneratedOutput(t *testing.T) {
	root := t.TempDir()
	generatedDirectory := filepath.Join(root, "packages", "api", "generated", "joined")
	writeFile(t, root, "packages/api/generated/joined/contract.json", `{}`)
	if err := os.MkdirAll(filepath.Join(root, "aliases"), 0o700); err != nil {
		t.Fatalf("create aliases directory: %v", err)
	}
	if err := os.Symlink(generatedDirectory, filepath.Join(root, "aliases", "joined")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"aliases/joined/contract.json"},
	})
	if documents != nil {
		t.Fatalf("Join() documents = %+v, want none", documents)
	}
	assertJoinDiagnostic(t, diagnostics, "join.input.generated", "aliases/joined/contract.json", "/", root)
}

func TestJoinRejectsAuthoredSymlinkOutsideRepository(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	writeFile(t, parent, "outside/contract.json", `{}`)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := os.Symlink(filepath.Join(parent, "outside"), filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	documents, diagnostics := contractjoiner.Join(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"linked/contract.json"},
	})
	if documents != nil {
		t.Fatalf("Join() documents = %+v, want none", documents)
	}
	assertJoinDiagnostic(t, diagnostics, "join.input.escape", "linked/contract.json", "/", root)
}

func assertJoinDiagnostic(t *testing.T, diagnostics []contractvalidator.Diagnostic, code, document, path, repositoryRoot string) {
	t.Helper()
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one", diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.Code != code || diagnostic.Document != document || diagnostic.Path != path || diagnostic.Message == "" {
		t.Fatalf("diagnostic = %+v, want code=%q document=%q path=%q and a message", diagnostic, code, document, path)
	}
	if strings.Contains(diagnostic.Message, repositoryRoot) || strings.Contains(diagnostic.Document, `\`) {
		t.Fatalf("diagnostic leaks an absolute path or uses platform separators: %+v", diagnostic)
	}
}
