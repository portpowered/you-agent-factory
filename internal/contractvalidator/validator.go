// Package contractvalidator loads and validates repository-authored contract
// documents. It is build tooling and must not be imported by runtime packages.
package contractvalidator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const rootPath = "/"

// Diagnostic is the stable error contract returned by the validator. Path uses
// JSON Pointer escaping with "/" as the documented document-root convention.
// Diagnostics are ordered by document, path, code, then message.
type Diagnostic struct {
	Code     string `json:"code"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Document string `json:"document"`
}

// Schema identifies one schema resource registered for a contract family.
type Schema struct {
	ID   string
	Path string
}

// Document identifies one authored document and the schema that validates it.
type Document struct {
	Path     string
	SchemaID string
}

// Entry is one exact family and format-version validation set.
type Entry struct {
	Family        string
	FormatVersion string
	Schemas       []Schema
	Documents     []Document
}

// Registry contains only deliberately supported contract family versions.
// NewRegistry copies its input so callers cannot mutate a validation run by
// retaining the supplied slices.
type Registry struct {
	entries []Entry
}

// NewRegistry constructs an explicit registry without filesystem discovery.
func NewRegistry(entries ...Entry) Registry {
	copied := make([]Entry, len(entries))
	for i, entry := range entries {
		copied[i] = cloneEntry(entry)
	}
	return Registry{entries: copied}
}

// MergeRegistries combines multiple registries without mutating the inputs.
func MergeRegistries(registries ...Registry) Registry {
	var entries []Entry
	for _, registry := range registries {
		entries = append(entries, registry.entries...)
	}
	return Registry{entries: entries}
}

// DefaultRegistry registers every contract family validated by make contracts-validate.
func DefaultRegistry() Registry {
	return MergeRegistries(CommonRegistry(), CLIRegistry(), CompatibilityInventoryRegistry(), MCPRegistry(), JavaScriptRegistry())
}

// CommonRegistry registers the common schemas and their merged valid fixtures.
func CommonRegistry() Registry {
	const (
		documentationID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
		deprecationsID  = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
		precedenceID    = "https://schemas.portpowered.com/you/contracts/common/precedence.schema.json"
		sensitivityID   = "https://schemas.portpowered.com/you/contracts/common/sensitivity.schema.json"
	)
	return NewRegistry(Entry{
		Family:        "common",
		FormatVersion: "1.0.0",
		Schemas: []Schema{
			{ID: documentationID, Path: "contracts/common/documentation.schema.json"},
			{ID: deprecationsID, Path: "contracts/common/deprecations.schema.json"},
			{ID: precedenceID, Path: "contracts/common/precedence.schema.json"},
			{ID: sensitivityID, Path: "contracts/common/sensitivity.schema.json"},
		},
		Documents: []Document{
			{Path: "contracts/testdata/common/documentation/valid-public.json", SchemaID: documentationID},
			{Path: "contracts/testdata/common/documentation/valid-internal.json", SchemaID: documentationID},
			{Path: "contracts/testdata/common/deprecations/valid-active.json", SchemaID: deprecationsID},
			{Path: "contracts/testdata/common/deprecations/valid-deprecated.json", SchemaID: deprecationsID},
			{Path: "contracts/testdata/common/deprecations/valid-removed.json", SchemaID: deprecationsID},
			{Path: "contracts/testdata/common/precedence/valid-file-env-flag.json", SchemaID: precedenceID},
			{Path: "contracts/testdata/common/precedence/valid-file-only.json", SchemaID: precedenceID},
			{Path: "contracts/testdata/common/precedence/valid-no-layers.json", SchemaID: precedenceID},
			{Path: "contracts/testdata/common/sensitivity/valid-public.json", SchemaID: sensitivityID},
		},
	})
}

// MCPRegistry registers the MCP tool-catalog schema and its valid fixtures.
func MCPRegistry() Registry {
	const (
		toolCatalogID        = "https://schemas.portpowered.com/you/contracts/mcp/tool-catalog.schema.json"
		contentID            = "https://schemas.portpowered.com/you/contracts/mcp/protocol/content.schema.json"
		callToolResultID     = "https://schemas.portpowered.com/you/contracts/mcp/protocol/call-tool-result.schema.json"
		domainToolResponseID = "https://schemas.portpowered.com/you/contracts/mcp/protocol/domain-tool-response.schema.json"
		jsonRPCErrorID       = "https://schemas.portpowered.com/you/contracts/mcp/protocol/json-rpc-error.schema.json"
		documentationID      = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
		deprecationsID       = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
	)
	return NewRegistry(Entry{
		Family:        "mcp",
		FormatVersion: "1.0.0",
		Schemas: []Schema{
			{ID: documentationID, Path: "contracts/common/documentation.schema.json"},
			{ID: deprecationsID, Path: "contracts/common/deprecations.schema.json"},
			{ID: contentID, Path: "contracts/mcp/protocol/content.schema.json"},
			{ID: callToolResultID, Path: "contracts/mcp/protocol/call-tool-result.schema.json"},
			{ID: domainToolResponseID, Path: "contracts/mcp/protocol/domain-tool-response.schema.json"},
			{ID: jsonRPCErrorID, Path: "contracts/mcp/protocol/json-rpc-error.schema.json"},
			{ID: toolCatalogID, Path: "contracts/mcp/tool-catalog.schema.json"},
		},
		Documents: []Document{
			{Path: "contracts/mcp/tools.json", SchemaID: toolCatalogID},
			{Path: "contracts/testdata/mcp/valid-input-closed-nested.json", SchemaID: toolCatalogID},
			{Path: "contracts/testdata/mcp/valid-text-success-result.json", SchemaID: toolCatalogID},
			{Path: "contracts/testdata/mcp/valid-text-error-result.json", SchemaID: toolCatalogID},
			{Path: "contracts/testdata/mcp/valid-domain-failure-result.json", SchemaID: toolCatalogID},
			{Path: "contracts/testdata/mcp/valid-protocol-failures.json", SchemaID: toolCatalogID},
			{Path: "contracts/testdata/mcp/valid-handler-binding.json", SchemaID: toolCatalogID},
		},
	})
}

// CLIRegistry registers the CLI command-manifest schema and its valid fixtures.
func CLIRegistry() Registry {
	const (
		commandManifestID = "https://schemas.portpowered.com/you/contracts/cli/command-manifest.schema.json"
		documentationID   = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
		deprecationsID    = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
	)
	return NewRegistry(Entry{
		Family:        "cli",
		FormatVersion: "1.0.0",
		Schemas: []Schema{
			{ID: documentationID, Path: "contracts/common/documentation.schema.json"},
			{ID: deprecationsID, Path: "contracts/common/deprecations.schema.json"},
			{ID: commandManifestID, Path: "contracts/cli/command-manifest.schema.json"},
		},
		Documents: []Document{
			{Path: "contracts/cli/commands.json", SchemaID: commandManifestID},
			{Path: "contracts/cli/deprecated-commands.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-identity.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-optional-argument.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-variadic-argument.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-persistent-flag.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-inherited-flag.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-no-option-flag.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-mutex-relationship.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-required-together-relationship.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-conditional-relationship.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-precedence.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-handler-binding.json", SchemaID: commandManifestID},
			{Path: "contracts/testdata/cli/valid-canonical-input-kinds.json", SchemaID: commandManifestID},
		},
	})
}

// Validate selects one exact registry entry and validates all of its documents.
func Validate(repositoryRoot string, registry Registry, family, formatVersion string) []Diagnostic {
	entry, diagnostic := registry.selectEntry(family, formatVersion)
	if diagnostic != nil {
		return []Diagnostic{*diagnostic}
	}
	diagnostics := validateEntry(repositoryRoot, entry)
	sortDiagnostics(diagnostics)
	return diagnostics
}

// ValidateAll validates every family/version entry deliberately present in the
// registry and returns one deterministically ordered diagnostic collection.
func ValidateAll(repositoryRoot string, registry Registry) []Diagnostic {
	var diagnostics []Diagnostic
	for _, entry := range registry.entries {
		diagnostics = append(diagnostics, validateEntry(repositoryRoot, cloneEntry(entry))...)
	}
	sortDiagnostics(diagnostics)
	return diagnostics
}

const toolCatalogSchemaID = "https://schemas.portpowered.com/you/contracts/mcp/tool-catalog.schema.json"

func validateEntry(repositoryRoot string, entry Entry) []Diagnostic {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(disabledURLLoader{})
	for _, resource := range entry.Schemas {
		value, issue := loadJSON(repositoryRoot, resource.Path, "schema")
		if issue != nil {
			return []Diagnostic{*issue}
		}
		if err := compiler.AddResource(resource.ID, value); err != nil {
			return []Diagnostic{newDiagnostic("schema.register", rootPath, "registered schema could not be loaded", resource.Path)}
		}
	}

	compiled := make(map[string]*jsonschema.Schema, len(entry.Schemas))
	for _, resource := range entry.Schemas {
		schema, err := compiler.Compile(resource.ID)
		if err != nil {
			return []Diagnostic{newDiagnostic("schema.compile", rootPath, "registered schema could not be compiled", resource.Path)}
		}
		compiled[resource.ID] = schema
	}

	var diagnostics []Diagnostic
	loadedDocuments := make(map[string]loadedDocument)
	for _, document := range entry.Documents {
		value, issue := loadJSON(repositoryRoot, document.Path, "document")
		if issue != nil {
			diagnostics = append(diagnostics, *issue)
			continue
		}
		value, sourceDocuments, referenceDiagnostics := resolveReferences(repositoryRoot, document.Path, value)
		if len(referenceDiagnostics) != 0 {
			diagnostics = append(diagnostics, referenceDiagnostics...)
			continue
		}
		schema, ok := compiled[document.SchemaID]
		if !ok {
			diagnostics = append(diagnostics, newDiagnostic("schema.unregistered", rootPath, "document references an unregistered schema", document.Path))
			continue
		}
		if err := schema.Validate(value); err != nil {
			diagnostics = append(diagnostics, validationDiagnostics(document.Path, err)...)
			continue
		}
		if document.SchemaID == compatibilityInventorySchemaID {
			diagnostics = append(diagnostics, compatibilityInventorySemanticsDiagnostics(document.Path, value)...)
		}
		if document.SchemaID == commandManifestSchemaID {
			diagnostics = append(diagnostics, cliManifestDiagnostics(document.Path, value)...)
		}
		if document.SchemaID == runtimeManifestSchemaID {
			diagnostics = append(diagnostics, runtimeManifestDiagnostics(document.Path, value)...)
			diagnostics = append(diagnostics, javascriptCatalogPathCompletenessDiagnostics(document.Path, value)...)
			diagnostics = append(diagnostics, javascriptCatalogCallBehaviorParityDiagnostics(document.Path, value)...)
		}
		if document.SchemaID == toolCatalogSchemaID {
			diagnostics = append(diagnostics, mcpToolCatalogIdentityDiagnostics(document.Path, value)...)
			diagnostics = append(diagnostics, mcpToolCatalogInputSchemaDiagnostics(document.Path, value)...)
			diagnostics = append(diagnostics, mcpToolCatalogPublicationDiagnostics(document.Path, value)...)
		}
		for _, source := range sourceDocuments {
			loadedDocuments[source.path] = source
		}
	}
	uniqueDocuments := make([]loadedDocument, 0, len(loadedDocuments))
	for _, document := range loadedDocuments {
		uniqueDocuments = append(uniqueDocuments, document)
	}
	diagnostics = append(diagnostics, duplicateStableIDDiagnostics(uniqueDocuments)...)
	return diagnostics
}

func (registry Registry) selectEntry(family, formatVersion string) (Entry, *Diagnostic) {
	familyFound := false
	for _, entry := range registry.entries {
		if entry.Family != family {
			continue
		}
		familyFound = true
		if entry.FormatVersion == formatVersion {
			selected := cloneEntry(entry)
			return selected, nil
		}
	}
	if !familyFound {
		diagnostic := newDiagnostic("registry.unknown_family", rootPath, fmt.Sprintf("contract family %q is not registered", family), "registry")
		return Entry{}, &diagnostic
	}
	diagnostic := newDiagnostic("registry.unknown_version", rootPath, fmt.Sprintf("format version %q is not registered for family %q", formatVersion, family), "registry")
	return Entry{}, &diagnostic
}

func loadJSON(repositoryRoot, document, kind string) (any, *Diagnostic) {
	document = normalizeRepositoryPath(document)
	root, err := canonicalRoot(repositoryRoot)
	if err != nil {
		diagnostic := newDiagnostic(kind+".read", rootPath, "registered "+kind+" could not be read", document)
		return nil, &diagnostic
	}
	path := filepath.Join(root, filepath.FromSlash(document))
	if !containedBy(root, path) {
		diagnostic := newDiagnostic(kind+".read", rootPath, "registered "+kind+" could not be read", document)
		return nil, &diagnostic
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil || !containedBy(root, path) {
		diagnostic := newDiagnostic(kind+".read", rootPath, "registered "+kind+" could not be read", document)
		return nil, &diagnostic
	}
	file, err := os.Open(path)
	if err != nil {
		diagnostic := newDiagnostic(kind+".read", rootPath, "registered "+kind+" could not be read", document)
		return nil, &diagnostic
	}
	defer file.Close()
	value, err := jsonschema.UnmarshalJSON(file)
	if err != nil {
		diagnostic := newDiagnostic(kind+".parse", rootPath, "registered "+kind+" is not valid JSON", document)
		return nil, &diagnostic
	}
	return value, nil
}

func validationDiagnostics(document string, err error) []Diagnostic {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return []Diagnostic{newDiagnostic("schema.validation", rootPath, "document does not conform to its registered schema", document)}
	}
	var diagnostics []Diagnostic
	collectValidationDiagnostics(document, validationErr, &diagnostics)
	return diagnostics
}

func collectValidationDiagnostics(document string, err *jsonschema.ValidationError, diagnostics *[]Diagnostic) {
	if len(err.Causes) == 0 {
		*diagnostics = append(*diagnostics, newDiagnostic("schema.validation", instancePath(err.InstanceLocation), "document does not conform to its registered schema", document))
		return
	}
	for _, cause := range err.Causes {
		collectValidationDiagnostics(document, cause, diagnostics)
	}
}

func instancePath(segments []string) string {
	if len(segments) == 0 {
		return rootPath
	}
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = strings.NewReplacer("~", "~0", "/", "~1").Replace(segment)
	}
	return rootPath + strings.Join(escaped, rootPath)
}

func newDiagnostic(code, path, message, document string) Diagnostic {
	return Diagnostic{Code: code, Path: path, Message: message, Document: normalizeRepositoryPath(document)}
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.Document != right.Document {
			return left.Document < right.Document
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		return left.Message < right.Message
	})
}

// SortDiagnostics applies the validator's documented total order in place.
// Build tooling that aggregates diagnostics from multiple validation runs uses
// this to preserve one stable error contract.
func SortDiagnostics(diagnostics []Diagnostic) {
	sortDiagnostics(diagnostics)
}

func cloneEntry(entry Entry) Entry {
	entry.Schemas = append([]Schema(nil), entry.Schemas...)
	entry.Documents = append([]Document(nil), entry.Documents...)
	return entry
}
