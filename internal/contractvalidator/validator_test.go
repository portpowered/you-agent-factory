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

func TestCompatibilityInventoryRegistryValidFixtures(t *testing.T) {
	root := repositoryRoot(t)
	diagnostics := contractvalidator.Validate(root, contractvalidator.CompatibilityInventoryRegistry(), "compatibility-inventory", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("Validate() diagnostics = %+v, want none", diagnostics)
	}
}

func TestCLIRegistryValidFixtures(t *testing.T) {
	root := repositoryRoot(t)
	diagnostics := contractvalidator.Validate(root, contractvalidator.CLIRegistry(), "cli", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("Validate() diagnostics = %+v, want none", diagnostics)
	}
}

func TestJavaScriptRegistryValidFixtures(t *testing.T) {
	root := repositoryRoot(t)
	diagnostics := contractvalidator.Validate(root, contractvalidator.JavaScriptRegistry(), "javascript", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("Validate() diagnostics = %+v, want none", diagnostics)
	}
}

func TestValidateCLIInvalidManifestDiagnostics(t *testing.T) {
	root := repositoryRoot(t)

	tests := []struct {
		name     string
		fixture  string
		code     string
		wantPath string
	}{
		{
			name:     "duplicate stable documentation IDs",
			fixture:  "contracts/testdata/cli/invalid-duplicate-stable-id.json",
			code:     "identity.duplicate",
			wantPath: "/commands/example.factory.alpha/documentation/documentation/title/id",
		},
		{
			name:     "ambiguous flag and argument stable ID",
			fixture:  "contracts/testdata/cli/invalid-ambiguous-flag-argument.json",
			code:     "cli.input.ambiguous",
			wantPath: "/commands/example.factory.sync/arguments/example.factory.sync.target/id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := contractvalidator.Validate(root, cliManifestFixtureRegistry(test.fixture), "cli", "1.0.0")
			if len(diagnostics) == 0 {
				t.Fatal("expected diagnostics, got none")
			}
			found := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == test.code && diagnostic.Path == test.wantPath && diagnostic.Document == test.fixture {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("diagnostics = %+v, want code=%q path=%q document=%q", diagnostics, test.code, test.wantPath, test.fixture)
			}
		})
	}
}

func cliManifestFixtureRegistry(fixture string) contractvalidator.Registry {
	const (
		commandManifestID = "https://schemas.portpowered.com/you/contracts/cli/command-manifest.schema.json"
		documentationID   = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
		deprecationsID    = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
	)
	return contractvalidator.NewRegistry(contractvalidator.Entry{
		Family:        "cli",
		FormatVersion: "1.0.0",
		Schemas: []contractvalidator.Schema{
			{ID: documentationID, Path: "contracts/common/documentation.schema.json"},
			{ID: deprecationsID, Path: "contracts/common/deprecations.schema.json"},
			{ID: commandManifestID, Path: "contracts/cli/command-manifest.schema.json"},
		},
		Documents: []contractvalidator.Document{{Path: fixture, SchemaID: commandManifestID}},
	})
}

func TestDefaultRegistryValidFixtures(t *testing.T) {
	root := repositoryRoot(t)
	diagnostics := contractvalidator.ValidateAll(root, contractvalidator.DefaultRegistry())
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateAll() diagnostics = %+v, want none", diagnostics)
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

func TestValidateAcceptsUniqueStableIDs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "schema.json", identitySchema)
	writeFile(t, root, "first.json", identityDocument("component.first"))
	writeFile(t, root, "second.json", identityDocument("component.second"))

	diagnostics := contractvalidator.Validate(root, identityRegistry("first.json", "second.json"), "test", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("Validate() diagnostics = %+v, want none", diagnostics)
	}
}

func TestValidateReportsEveryDuplicateStableIDOccurrence(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "schema.json", identitySchema)
	writeFile(t, root, "first.json", identityDocument("component.shared"))
	writeFile(t, root, "nested/second.json", identityDocument("component.shared"))

	diagnostics := contractvalidator.Validate(root, identityRegistry("first.json", "nested/second.json"), "test", "1.0.0")
	want := []contractvalidator.Diagnostic{
		{Code: "identity.duplicate", Path: "/identity/id", Message: `stable ID "component.shared" appears more than once`, Document: "first.json"},
		{Code: "identity.duplicate", Path: "/identity/id", Message: `stable ID "component.shared" appears more than once`, Document: "nested/second.json"},
	}
	if fmt.Sprint(diagnostics) != fmt.Sprint(want) {
		t.Fatalf("Validate() diagnostics = %+v, want %+v", diagnostics, want)
	}
}

func TestValidateDuplicateStableIDsAreIndependentOfDocumentOrder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "schema.json", identitySchema)
	writeFile(t, root, "alpha.json", identityDocument("duplicate.alpha"))
	writeFile(t, root, "beta.json", identityDocument("duplicate.beta"))
	writeFile(t, root, "other-alpha.json", identityDocument("duplicate.alpha"))
	writeFile(t, root, "other-beta.json", identityDocument("duplicate.beta"))

	forward := contractvalidator.Validate(root, identityRegistry("alpha.json", "beta.json", "other-alpha.json", "other-beta.json"), "test", "1.0.0")
	reversed := contractvalidator.Validate(root, identityRegistry("other-beta.json", "other-alpha.json", "beta.json", "alpha.json"), "test", "1.0.0")
	if fmt.Sprint(forward) != fmt.Sprint(reversed) {
		t.Fatalf("document order changed diagnostics:\nforward: %+v\nreverse: %+v", forward, reversed)
	}
	if len(forward) != 4 {
		t.Fatalf("Validate() diagnostics = %+v, want four occurrences from two duplicate groups", forward)
	}
}

func TestValidateAllIsIndependentOfRegistryOrderAndPathStyle(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "schema.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["name"]}`)
	writeFile(t, root, "alpha/invalid.json", `{}`)
	writeFile(t, root, "zeta/invalid.json", `{}`)

	entry := func(family, document string) contractvalidator.Entry {
		return contractvalidator.Entry{
			Family: family, FormatVersion: "1.0.0",
			Schemas:   []contractvalidator.Schema{{ID: "https://example.test/schema.json", Path: "schema.json"}},
			Documents: []contractvalidator.Document{{Path: document, SchemaID: "https://example.test/schema.json"}},
		}
	}
	forward := contractvalidator.ValidateAll(root, contractvalidator.NewRegistry(
		entry("zeta", `zeta\invalid.json`),
		entry("alpha", "alpha/invalid.json"),
	))
	reversed := contractvalidator.ValidateAll(root, contractvalidator.NewRegistry(
		entry("alpha", `alpha\invalid.json`),
		entry("zeta", "zeta/invalid.json"),
	))
	want := []contractvalidator.Diagnostic{
		{Code: "schema.validation", Path: "/", Message: "document does not conform to its registered schema", Document: "alpha/invalid.json"},
		{Code: "schema.validation", Path: "/", Message: "document does not conform to its registered schema", Document: "zeta/invalid.json"},
	}
	if fmt.Sprint(forward) != fmt.Sprint(want) || fmt.Sprint(reversed) != fmt.Sprint(want) {
		t.Fatalf("ValidateAll() diagnostics differ:\nforward: %+v\nreverse: %+v\nwant:    %+v", forward, reversed, want)
	}
}

const componentSchema = `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "type":"object",
  "required":["component"],
  "properties":{"component":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}
}`

const identitySchema = `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "type":"object",
  "required":["identity"],
  "properties":{"identity":{
    "type":"object",
    "required":["id","canonicalEnglish"],
    "properties":{"id":{"type":"string"},"canonicalEnglish":{"type":"string"}}
  }}
}`

func referenceRegistry(document string) contractvalidator.Registry {
	return contractvalidator.NewRegistry(contractvalidator.Entry{
		Family: "test", FormatVersion: "1.0.0",
		Schemas:   []contractvalidator.Schema{{ID: "https://example.test/schema.json", Path: "schema.json"}},
		Documents: []contractvalidator.Document{{Path: document, SchemaID: "https://example.test/schema.json"}},
	})
}

func identityRegistry(documents ...string) contractvalidator.Registry {
	registered := make([]contractvalidator.Document, 0, len(documents))
	for _, document := range documents {
		registered = append(registered, contractvalidator.Document{Path: document, SchemaID: "https://example.test/identity.schema.json"})
	}
	return contractvalidator.NewRegistry(contractvalidator.Entry{
		Family: "test", FormatVersion: "1.0.0",
		Schemas:   []contractvalidator.Schema{{ID: "https://example.test/identity.schema.json", Path: "schema.json"}},
		Documents: registered,
	})
}

func identityDocument(id string) string {
	return fmt.Sprintf(`{"identity":{"id":%q,"canonicalEnglish":"Component documentation"}}`, id)
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
