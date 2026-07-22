package contracts_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var mcpRegistryValidFixturePaths = []string{
	"contracts/mcp/tools.json",
	"contracts/testdata/mcp/valid-input-closed-nested.json",
	"contracts/testdata/mcp/valid-text-success-result.json",
	"contracts/testdata/mcp/valid-text-error-result.json",
	"contracts/testdata/mcp/valid-domain-failure-result.json",
	"contracts/testdata/mcp/valid-protocol-failures.json",
	"contracts/testdata/mcp/valid-handler-binding.json",
}

const (
	toolCatalogSchemaID       = "https://schemas.portpowered.com/you/contracts/mcp/tool-catalog.schema.json"
	mcpContentSchemaID        = "https://schemas.portpowered.com/you/contracts/mcp/protocol/content.schema.json"
	mcpCallToolResultSchemaID = "https://schemas.portpowered.com/you/contracts/mcp/protocol/call-tool-result.schema.json"
	mcpDomainToolResponseID   = "https://schemas.portpowered.com/you/contracts/mcp/protocol/domain-tool-response.schema.json"
	mcpJSONRPCErrorSchemaID   = "https://schemas.portpowered.com/you/contracts/mcp/protocol/json-rpc-error.schema.json"
)

var mcpValidCatalogFixtures = []struct {
	name    string
	fixture string
}{
	{name: "minimal tool metadata", fixture: "valid-minimal.json"},
	{name: "closed nested input object", fixture: "valid-input-closed-nested.json"},
	{name: "canonical text success result", fixture: "valid-text-success-result.json"},
	{name: "canonical text error result", fixture: "valid-text-error-result.json"},
	{name: "domain failure paired with text error envelope", fixture: "valid-domain-failure-result.json"},
	{name: "JSON-RPC protocol failures", fixture: "valid-protocol-failures.json"},
	{name: "handler binding", fixture: "valid-handler-binding.json"},
}

var mcpInvalidCatalogSchemaFixtures = []struct {
	name     string
	fixture  string
	wantPath string
}{
	{
		name:     "open nested input object",
		fixture:  "invalid-open-nested-input.json",
		wantPath: "/tools/mcp.tool.you.factory_session.start_async/input/schema/properties/source",
	},
	{
		name:     "unsupported task behavior",
		fixture:  "invalid-unsupported-task.json",
		wantPath: "/tools/mcp.tool.you.factory_session.list/execution/mode",
	},
	{
		name:     "result image content",
		fixture:  "invalid-result-image.json",
		wantPath: "/tools/mcp.tool.you.factory_session.list/result/examples/0/content/0/type",
	},
	{
		name:     "result audio content",
		fixture:  "invalid-result-audio.json",
		wantPath: "/tools/mcp.tool.you.factory_session.list/result/examples/0/content/0/type",
	},
	{
		name:     "result embedded resource content",
		fixture:  "invalid-result-embedded-resource.json",
		wantPath: "/tools/mcp.tool.you.factory_session.list/result/examples/0/content/0/type",
	},
	{
		name:     "result structured content field",
		fixture:  "invalid-result-structured-content.json",
		wantPath: "/tools/mcp.tool.you.factory_session.list/result/examples/0",
	},
	{
		name:     "result output schema field",
		fixture:  "invalid-result-output-schema.json",
		wantPath: "/tools/mcp.tool.you.factory_session.list/result/examples/0",
	},
	{
		name:     "domain failure confused with protocol error",
		fixture:  "invalid-domain-confused-with-protocol-error.json",
		wantPath: "/tools/mcp.tool.you.factory_session.get/result/domain/failures/mcp.failure.you.factory_session.get.protocol_confusion/examples/0",
	},
	{
		name:     "broken handler ID",
		fixture:  "invalid-handler-id.json",
		wantPath: "/tools/mcp.tool.you.factory_session.control/handler/id",
	},
	{
		name:     "unknown protocol version",
		fixture:  "invalid-unknown-protocol-version.json",
		wantPath: "/protocolVersion",
	},
	{
		name:     "malformed lifecycle record",
		fixture:  "invalid-malformed-lifecycle.json",
		wantPath: "/tools/mcp.tool.you.factory_session.control/lifecycle",
	},
	{
		name:     "malformed documentation record",
		fixture:  "invalid-malformed-documentation.json",
		wantPath: "/tools/mcp.tool.you.factory_session.get/documentation/documentation/title/id",
	},
	{
		name:     "broken shared schema reference",
		fixture:  "invalid-broken-shared-schema-ref.json",
		wantPath: "/tools/mcp.tool.you.factory_session.start_async/transports/$ref",
	},
}

func toolCatalogSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return compileSchema(
		t,
		filepath.Join("mcp", "tool-catalog.schema.json"),
		toolCatalogSchemaID,
		schemaResource{path: filepath.Join("common", "documentation.schema.json"), id: documentationSchemaID},
		schemaResource{path: filepath.Join("common", "deprecations.schema.json"), id: deprecationsSchemaID},
		schemaResource{path: filepath.Join("mcp", "protocol", "content.schema.json"), id: mcpContentSchemaID},
		schemaResource{path: filepath.Join("mcp", "protocol", "call-tool-result.schema.json"), id: mcpCallToolResultSchemaID},
		schemaResource{path: filepath.Join("mcp", "protocol", "domain-tool-response.schema.json"), id: mcpDomainToolResponseID},
		schemaResource{path: filepath.Join("mcp", "protocol", "json-rpc-error.schema.json"), id: mcpJSONRPCErrorSchemaID},
	)
}

func TestMCPToolCatalogSchemaValidFixtureMatrix(t *testing.T) {
	schema := toolCatalogSchema(t)

	for _, test := range mcpValidCatalogFixtures {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "mcp", test.fixture))
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("validate valid fixture %s: %v", test.fixture, err)
			}
		})
	}
}

func TestMCPToolCatalogSchemaInvalidFixtureMatrix(t *testing.T) {
	schema := toolCatalogSchema(t)

	for _, test := range mcpInvalidCatalogSchemaFixtures {
		t.Run(test.name, func(t *testing.T) {
			instance := readJSON(t, filepath.Join("testdata", "mcp", test.fixture))
			if test.fixture == "invalid-broken-shared-schema-ref.json" {
				root, err := filepath.Abs("..")
				if err != nil {
					t.Fatalf("repository root: %v", err)
				}
				diagnostics := contractvalidator.Validate(root, mcpCatalogFixtureRegistry(test.fixture), "mcp", "1.0.0")
				if len(diagnostics) == 0 {
					t.Fatal("expected broken shared-schema reference diagnostics, got none")
				}
				found := false
				document := filepath.ToSlash(filepath.Join("contracts", "testdata", "mcp", test.fixture))
				for _, diagnostic := range diagnostics {
					if diagnostic.Code == "reference.fragment" && diagnostic.Path == test.wantPath && diagnostic.Document == document {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("diagnostics = %+v, want code=reference.fragment path=%q document=%q", diagnostics, test.wantPath, document)
				}
				return
			}
			err := schema.Validate(instance)
			if err == nil {
				t.Fatal("expected fixture validation to fail")
			}
			if paths := validationPaths(t, err); !slices.Contains(paths, test.wantPath) {
				t.Fatalf("validation paths = %v, want %q", paths, test.wantPath)
			}
		})
	}
}

func TestMCPToolCatalogContractValidatorInvalidFixtureMatrix(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}

	tests := []struct {
		name     string
		fixture  string
		code     string
		wantPath string
	}{
		{
			name:     "duplicate stable documentation IDs",
			fixture:  "invalid-duplicate-stable-id.json",
			code:     "identity.duplicate",
			wantPath: "/tools/mcp.tool.you.factory_session.list_dispatches/documentation/documentation/title/id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := contractvalidator.Validate(root, mcpCatalogFixtureRegistry(test.fixture), "mcp", "1.0.0")
			if len(diagnostics) == 0 {
				t.Fatal("expected diagnostics, got none")
			}
			found := false
			document := filepath.ToSlash(filepath.Join("contracts", "testdata", "mcp", test.fixture))
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == test.code && diagnostic.Path == test.wantPath && diagnostic.Document == document {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("diagnostics = %+v, want code=%q path=%q document=%q", diagnostics, test.code, test.wantPath, document)
			}
		})
	}
}

func TestMCPToolCatalogFixtureDirectoryCoverage(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("testdata", "mcp"))
	if err != nil {
		t.Fatalf("read fixture directory: %v", err)
	}

	classified := make(map[string]struct{})
	for _, fixture := range mcpValidCatalogFixtures {
		classified[fixture.fixture] = struct{}{}
	}
	for _, fixture := range mcpInvalidCatalogSchemaFixtures {
		classified[fixture.fixture] = struct{}{}
	}
	classified["invalid-duplicate-stable-id.json"] = struct{}{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			t.Fatalf("unexpected non-json fixture %q", name)
		}
		if _, ok := classified[name]; !ok {
			t.Fatalf("fixture %q is not covered by the MCP tool-catalog fixture matrix", name)
		}
	}
	if len(classified) != len(entries) {
		t.Fatalf("classified fixture count = %d, want %d files on disk", len(classified), len(entries))
	}
}

func TestMCPToolCatalogRegistryMatchesValidFixtureMatrix(t *testing.T) {
	want := make([]string, 0, len(mcpValidCatalogFixtures))
	for _, fixture := range mcpValidCatalogFixtures {
		if fixture.fixture == "valid-minimal.json" {
			continue
		}
		want = append(want, fixture.fixture)
	}
	slices.Sort(want)

	registered := make([]string, 0, len(mcpRegistryValidFixturePaths))
	for _, path := range mcpRegistryValidFixturePaths {
		if path == "contracts/mcp/tools.json" {
			continue
		}
		registered = append(registered, filepath.Base(path))
		if _, err := os.Stat(strings.TrimPrefix(path, "contracts/")); err != nil {
			t.Fatalf("registered fixture %s missing on disk: %v", path, err)
		}
	}
	slices.Sort(registered)

	if !slices.Equal(registered, want) {
		t.Fatalf("registered valid fixtures = %v, want %v", registered, want)
	}
	if _, err := os.Stat(filepath.Join("mcp", "tools.json")); err != nil {
		t.Fatalf("authored catalog contracts/mcp/tools.json must exist: %v", err)
	}
}

func TestMCPToolCatalogAuthoredCatalogBoundary(t *testing.T) {
	if _, err := os.Stat(filepath.Join("mcp", "tools.json")); err != nil {
		t.Fatalf("contracts/mcp/tools.json must exist as the authored MCP tool catalog: %v", err)
	}
	if _, err := os.Stat(filepath.Join("mcp", "deprecated.json")); err != nil {
		t.Fatalf("contracts/mcp/deprecated.json must remain the compatibility alias inventory: %v", err)
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	diagnostics := contractvalidator.Validate(root, contractvalidator.MCPRegistry(), "mcp", "1.0.0")
	if len(diagnostics) != 0 {
		t.Fatalf("authored catalog validation diagnostics = %+v", diagnostics)
	}
}

func mcpCatalogFixtureRegistry(fixture string) contractvalidator.Registry {
	const (
		contentID            = "https://schemas.portpowered.com/you/contracts/mcp/protocol/content.schema.json"
		callToolResultID     = "https://schemas.portpowered.com/you/contracts/mcp/protocol/call-tool-result.schema.json"
		domainToolResponseID = "https://schemas.portpowered.com/you/contracts/mcp/protocol/domain-tool-response.schema.json"
		jsonRPCErrorID       = "https://schemas.portpowered.com/you/contracts/mcp/protocol/json-rpc-error.schema.json"
		documentationID      = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
		deprecationsID       = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
	)
	return contractvalidator.NewRegistry(contractvalidator.Entry{
		Family:        "mcp",
		FormatVersion: "1.0.0",
		Schemas: []contractvalidator.Schema{
			{ID: documentationID, Path: "contracts/common/documentation.schema.json"},
			{ID: deprecationsID, Path: "contracts/common/deprecations.schema.json"},
			{ID: contentID, Path: "contracts/mcp/protocol/content.schema.json"},
			{ID: callToolResultID, Path: "contracts/mcp/protocol/call-tool-result.schema.json"},
			{ID: domainToolResponseID, Path: "contracts/mcp/protocol/domain-tool-response.schema.json"},
			{ID: jsonRPCErrorID, Path: "contracts/mcp/protocol/json-rpc-error.schema.json"},
			{ID: toolCatalogSchemaID, Path: "contracts/mcp/tool-catalog.schema.json"},
		},
		Documents: []contractvalidator.Document{{
			Path:     filepath.ToSlash(filepath.Join("contracts", "testdata", "mcp", fixture)),
			SchemaID: toolCatalogSchemaID,
		}},
	})
}
