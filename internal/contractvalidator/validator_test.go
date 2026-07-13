package contractvalidator_test

import (
	"os"
	"path/filepath"
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
	if err := os.WriteFile(filepath.Join(root, path), []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func pointer(value string) *string { return &value }
