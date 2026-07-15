package mcpcontractcheck_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/mcpcontractcheck"
)

const (
	listToolID    = "mcp.tool.you.factory_session.list"
	statusToolID  = "mcp.tool.you.factory_session.status"
	listHandlerID = "mcp.handler.you.factory_session.list"
)

func TestValidateRejectsMissingAndExtraCanonicalRecords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*mcpcontractcheck.Inputs)
		wantCodes []string
		wantText  []string
	}{
		{
			name: "missing discovery", mutate: func(inputs *mcpcontractcheck.Inputs) { inputs.Discovery = nil },
			wantCodes: []string{"mcp.discovery.missing"}, wantText: []string{listToolID, "regenerate discovery"},
		},
		{
			name: "missing registry", mutate: func(inputs *mcpcontractcheck.Inputs) { inputs.Registry = nil },
			wantCodes: []string{"mcp.registry.missing"}, wantText: []string{listToolID, listHandlerID, "register that handler binding"},
		},
		{
			name: "extra discovery", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Discovery = append(inputs.Discovery, tool(statusToolID, "you.factory_session.status", ""))
			},
			wantCodes: []string{"mcp.discovery.extra"}, wantText: []string{statusToolID, "regenerate discovery"},
		},
		{
			name: "extra registry", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Registry = append(inputs.Registry, mcpcontractcheck.HandlerBinding{ToolID: statusToolID, HandlerID: "mcp.handler.you.factory_session.status"})
			},
			wantCodes: []string{"mcp.registry.extra"}, wantText: []string{statusToolID, "remove the binding or add the intended tool"},
		},
		{
			name: "extra catalog", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Catalog = append(inputs.Catalog, tool(statusToolID, "you.factory_session.status", "mcp.handler.you.factory_session.status"))
			},
			wantCodes: []string{"mcp.discovery.missing", "mcp.registry.missing"}, wantText: []string{statusToolID, "generated primary discovery", "handwritten registry"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inputs := cleanInputs()
			test.mutate(&inputs)
			diagnostics := mcpcontractcheck.Validate(inputs)
			assertDiagnosticCodes(t, diagnostics, test.wantCodes)
			for _, text := range test.wantText {
				if !strings.Contains(diagnosticText(diagnostics), text) {
					t.Fatalf("Validate() diagnostics = %+v, want text %q", diagnostics, text)
				}
			}
		})
	}
}

func TestValidateRejectsDuplicateIdentitiesBeforeSetComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*mcpcontractcheck.Inputs)
		wantCode string
	}{
		{"catalog tool ID", func(inputs *mcpcontractcheck.Inputs) {
			inputs.Catalog = append(inputs.Catalog, tool(listToolID, "other.name", "other.handler"))
		}, "mcp.catalog.duplicate_tool_id"},
		{"discovery tool ID", func(inputs *mcpcontractcheck.Inputs) {
			inputs.Discovery = append(inputs.Discovery, tool(listToolID, "other.name", ""))
		}, "mcp.discovery.duplicate_tool_id"},
		{"registry tool ID", func(inputs *mcpcontractcheck.Inputs) {
			inputs.Registry = append(inputs.Registry, mcpcontractcheck.HandlerBinding{ToolID: listToolID, HandlerID: "other.handler"})
		}, "mcp.registry.duplicate_tool_id"},
		{"catalog canonical name", func(inputs *mcpcontractcheck.Inputs) {
			inputs.Catalog = append(inputs.Catalog, tool(statusToolID, "you.factory_session.list", "other.handler"))
		}, "mcp.catalog.duplicate_canonical_name"},
		{"discovery canonical name", func(inputs *mcpcontractcheck.Inputs) {
			inputs.Discovery = append(inputs.Discovery, tool(statusToolID, "you.factory_session.list", ""))
		}, "mcp.discovery.duplicate_canonical_name"},
		{"catalog handler ID", func(inputs *mcpcontractcheck.Inputs) {
			inputs.Catalog = append(inputs.Catalog, tool(statusToolID, "other.name", listHandlerID))
		}, "mcp.catalog.duplicate_handler_id"},
		{"registry handler ID", func(inputs *mcpcontractcheck.Inputs) {
			inputs.Registry = append(inputs.Registry, mcpcontractcheck.HandlerBinding{ToolID: statusToolID, HandlerID: listHandlerID})
		}, "mcp.registry.duplicate_handler_id"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inputs := cleanInputs()
			test.mutate(&inputs)
			diagnostics := mcpcontractcheck.Validate(inputs)
			assertDiagnosticCodes(t, diagnostics, []string{test.wantCode})
			if strings.Contains(diagnosticText(diagnostics), ".missing") || strings.Contains(diagnosticText(diagnostics), ".extra") {
				t.Fatalf("Validate() diagnostics = %+v, set comparison ran after duplicate detection", diagnostics)
			}
		})
	}
}

func TestValidateOrdersFailuresDeterministically(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inputs  mcpcontractcheck.Inputs
		reverse func(*mcpcontractcheck.Inputs)
	}{
		{
			name: "set failures",
			inputs: func() mcpcontractcheck.Inputs {
				inputs := cleanInputs()
				inputs.Discovery = []mcpcontractcheck.ToolRecord{tool("mcp.tool.z", "z", ""), tool("mcp.tool.a", "a", "")}
				return inputs
			}(),
			reverse: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Discovery[0], inputs.Discovery[1] = inputs.Discovery[1], inputs.Discovery[0]
			},
		},
		{
			name: "duplicate failures",
			inputs: mcpcontractcheck.Inputs{Catalog: []mcpcontractcheck.ToolRecord{
				tool("mcp.tool.z", "shared.name", listHandlerID),
				tool("mcp.tool.a", "shared.name", listHandlerID),
			}},
			reverse: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Catalog[0], inputs.Catalog[1] = inputs.Catalog[1], inputs.Catalog[0]
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := mcpcontractcheck.Validate(test.inputs)
			test.reverse(&test.inputs)
			second := mcpcontractcheck.Validate(test.inputs)
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("Validate() ordering changed with input order:\nfirst  = %+v\nsecond = %+v", first, second)
			}
		})
	}
}

func cleanInputs() mcpcontractcheck.Inputs {
	return mcpcontractcheck.Inputs{
		Catalog:   []mcpcontractcheck.ToolRecord{tool(listToolID, "you.factory_session.list", listHandlerID)},
		Discovery: []mcpcontractcheck.ToolRecord{tool(listToolID, "you.factory_session.list", "")},
		Registry:  []mcpcontractcheck.HandlerBinding{{ToolID: listToolID, HandlerID: listHandlerID}},
	}
}

func tool(id, name, handlerID string) mcpcontractcheck.ToolRecord {
	return mcpcontractcheck.ToolRecord{ID: id, Name: name, Description: "description", InputSchema: map[string]any{"type": "object"}, HandlerID: handlerID}
}

func assertDiagnosticCodes(t *testing.T, diagnostics []mcpcontractcheck.Diagnostic, want []string) {
	t.Helper()
	got := make([]string, len(diagnostics))
	for index, diagnostic := range diagnostics {
		got[index] = diagnostic.Code
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Validate() diagnostic codes = %v, want %v; diagnostics = %+v", got, want, diagnostics)
	}
}

func diagnosticText(diagnostics []mcpcontractcheck.Diagnostic) string {
	return strings.TrimSpace(strings.Join(func() []string {
		messages := make([]string, len(diagnostics))
		for index, diagnostic := range diagnostics {
			messages[index] = diagnostic.Code + ": " + diagnostic.Message
		}
		return messages
	}(), "\n"))
}
