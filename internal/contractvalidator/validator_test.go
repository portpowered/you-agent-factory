package contractvalidator_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func TestCommonRegistryValidFixtures(t *testing.T) {
	root := repositoryRoot(t)
	diagnostics := contractvalidator.Validate(root, contractvalidator.CommonRegistry(), "common", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("Validate() diagnostics = %+v, want none", diagnostics)
	}
}

func TestValidateRejectsUnknownRegistrySelection(t *testing.T) {
	tests := []struct {
		name    string
		family  string
		version string
		code    string
	}{
		{name: "unknown family", family: "other", version: "1.0.0", code: "registry.unknown_family"},
		{name: "unknown version", family: "common", version: "2.0.0", code: "registry.unknown_version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := contractvalidator.Validate(t.TempDir(), contractvalidator.CommonRegistry(), test.family, test.version)
			assertSingleDiagnostic(t, diagnostics, test.code, "registry", "/")
		})
	}
}

func TestValidateNormalizesDocumentFailures(t *testing.T) {
	const schemaID = "https://example.test/schema.json"
	tests := []struct {
		name     string
		document string
		contents *string
		code     string
		path     string
	}{
		{name: "unreadable", document: "missing.json", code: "document.read", path: "/"},
		{name: "malformed", document: "malformed.json", contents: pointer("{"), code: "document.parse", path: "/"},
		{name: "schema validation", document: "invalid.json", contents: pointer(`{"value": 1}`), code: "schema.validation", path: "/value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "schema.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"value":{"type":"string"}}}`)
			if test.contents != nil {
				writeFile(t, root, test.document, *test.contents)
			}
			registry := contractvalidator.NewRegistry(contractvalidator.Entry{
				Family: "test", FormatVersion: "1.0.0",
				Schemas:   []contractvalidator.Schema{{ID: schemaID, Path: "schema.json"}},
				Documents: []contractvalidator.Document{{Path: test.document, SchemaID: schemaID}},
			})
			diagnostics := contractvalidator.Validate(root, registry, "test", "1.0.0")
			assertSingleDiagnostic(t, diagnostics, test.code, test.document, test.path)
		})
	}
}

func TestValidateResolvesNestedRepositoryReferenceWithFragment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "schema.json", componentSchema)
	writeFile(t, root, "nested/root.json", `{"component":{"$ref":"../shared/components.json#/$defs/component"}}`)
	writeFile(t, root, "shared/components.json", `{"$defs":{"component":{"id":"shared-component"}}}`)

	diagnostics := contractvalidator.Validate(root, referenceRegistry("nested/root.json"), "test", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("Validate() diagnostics = %+v, want none", diagnostics)
	}
}

func TestValidateRejectsBrokenAndEscapingReferences(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	writeFile(t, root, "schema.json", componentSchema)
	writeFile(t, parent, "outside.json", `{`)
	writeFile(t, parent, "repository-copy/outside.json", `{`)

	tests := []struct {
		name      string
		reference string
		code      string
	}{
		{name: "missing", reference: "../missing.json", code: "reference.missing"},
		{name: "parent escape", reference: "../../outside.json", code: "reference.escape"},
		{name: "sibling prefix escape", reference: "../../repository-copy/outside.json", code: "reference.escape"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeReferenceDocument(t, root, "nested/root.json", test.reference)
			diagnostics := contractvalidator.Validate(root, referenceRegistry("nested/root.json"), "test", "1.0.0")
			assertSingleDiagnostic(t, diagnostics, test.code, "nested/root.json", "/component/$ref")
		})
	}
}

func TestValidateRejectsNonRepositoryRelativeReferences(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "schema.json", componentSchema)
	absPath := filepath.Join(t.TempDir(), "outside.json")
	tests := []struct {
		name      string
		reference string
	}{
		{name: "absolute path", reference: absPath},
		{name: "file URL", reference: "file:///tmp/component.json"},
		{name: "network URL", reference: "https://example.test/component.json"},
		{name: "other scheme", reference: "urn:example:component"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeReferenceDocument(t, root, "root.json", test.reference)
			diagnostics := contractvalidator.Validate(root, referenceRegistry("root.json"), "test", "1.0.0")
			assertSingleDiagnostic(t, diagnostics, "reference.unsupported", "root.json", "/component/$ref")
		})
	}
}

func TestValidateRejectsReferenceThroughSymlinkOutsideRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	outside := filepath.Join(parent, "outside")
	writeFile(t, root, "schema.json", componentSchema)
	writeFile(t, root, "root.json", `{"component":{"$ref":"linked/component.json"}}`)
	writeFile(t, outside, "component.json", `{"id":"outside"}`)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	diagnostics := contractvalidator.Validate(root, referenceRegistry("root.json"), "test", "1.0.0")
	assertSingleDiagnostic(t, diagnostics, "reference.escape", "root.json", "/component/$ref")
}

func TestValidateNormalizesWindowsAndSlashReferencePaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "schema.json", componentSchema)
	writeReferenceDocument(t, root, "nested/root.json", `..\missing.json`)

	slashDiagnostics := contractvalidator.Validate(root, referenceRegistry("nested/root.json"), "test", "1.0.0")
	windowsDiagnostics := contractvalidator.Validate(root, referenceRegistry(`nested\root.json`), "test", "1.0.0")
	if fmt.Sprint(slashDiagnostics) != fmt.Sprint(windowsDiagnostics) {
		t.Fatalf("path-style diagnostics differ:\nslash:   %+v\nwindows: %+v", slashDiagnostics, windowsDiagnostics)
	}
	assertSingleDiagnostic(t, windowsDiagnostics, "reference.missing", "nested/root.json", "/component/$ref")
}

const componentSchema = `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "type":"object",
  "required":["component"],
  "properties":{"component":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}
}`

func referenceRegistry(document string) contractvalidator.Registry {
	return contractvalidator.NewRegistry(contractvalidator.Entry{
		Family: "test", FormatVersion: "1.0.0",
		Schemas:   []contractvalidator.Schema{{ID: "https://example.test/schema.json", Path: "schema.json"}},
		Documents: []contractvalidator.Document{{Path: document, SchemaID: "https://example.test/schema.json"}},
	})
}

func writeReferenceDocument(t *testing.T, root, path, reference string) {
	t.Helper()
	reference = strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(reference)
	writeFile(t, root, path, `{"component":{"$ref":"`+reference+`"}}`)
}

func assertSingleDiagnostic(t *testing.T, diagnostics []contractvalidator.Diagnostic, code, document, path string) {
	t.Helper()
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one", diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.Code != code || diagnostic.Document != document || diagnostic.Path != path || diagnostic.Message == "" {
		t.Fatalf("diagnostic = %+v, want code=%q document=%q path=%q and a message", diagnostic, code, document, path)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	return root
}

func writeFile(t *testing.T, root, path, contents string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func pointer(value string) *string { return &value }
