package mcpcontractcheck_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/mcpcontractcheck"
)

func TestValidateRejectsStaleGeneratedDiscoveryMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*mcpcontractcheck.ToolRecord)
		wantField string
		wantValue string
	}{
		{
			name: "canonical name", wantField: "name", wantValue: "you.factory_session.stale",
			mutate: func(record *mcpcontractcheck.ToolRecord) { record.Name = "you.factory_session.stale" },
		},
		{
			name: "description", wantField: "description", wantValue: "stale description",
			mutate: func(record *mcpcontractcheck.ToolRecord) { record.Description = "stale description" },
		},
		{
			name: "input schema", wantField: "inputSchema",
			mutate: func(record *mcpcontractcheck.ToolRecord) {
				record.InputSchema = map[string]any{"type": "object", "required": []any{"stale"}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inputs := cleanInputs()
			test.mutate(&inputs.Discovery[0])
			before := cloneInputs(t, inputs)

			diagnostics := mcpcontractcheck.Validate(inputs)

			assertDiagnosticCodes(t, diagnostics, []string{"mcp.discovery.metadata_mismatch"})
			message := diagnostics[0].Message
			for _, text := range []string{listToolID, test.wantField, "update the authored catalog if intent changed", "regenerate discovery"} {
				if !strings.Contains(message, text) {
					t.Fatalf("Validate() message = %q, want text %q", message, text)
				}
			}
			if test.wantValue != "" && !strings.Contains(message, test.wantValue) {
				t.Fatalf("Validate() message = %q, want stale value %q", message, test.wantValue)
			}
			if !reflect.DeepEqual(inputs, before) {
				t.Fatalf("Validate() rewrote stale inputs:\ngot  = %#v\nwant = %#v", inputs, before)
			}
		})
	}
}

func TestValidateRejectsStaleHandlerBinding(t *testing.T) {
	t.Parallel()

	inputs := cleanInputs()
	const staleHandlerID = "mcp.handler.you.factory_session.stale"
	inputs.Registry[0].HandlerID = staleHandlerID

	diagnostics := mcpcontractcheck.Validate(inputs)

	assertDiagnosticCodes(t, diagnostics, []string{"mcp.registry.handler_mismatch"})
	for _, text := range []string{listToolID, staleHandlerID, listHandlerID, "update the authored catalog handler ID if intent changed", "repair the handwritten registry binding"} {
		if !strings.Contains(diagnostics[0].Message, text) {
			t.Fatalf("Validate() message = %q, want text %q", diagnostics[0].Message, text)
		}
	}
}

func cloneInputs(t *testing.T, inputs mcpcontractcheck.Inputs) mcpcontractcheck.Inputs {
	t.Helper()

	clone := inputs
	clone.Catalog = append([]mcpcontractcheck.ToolRecord(nil), inputs.Catalog...)
	clone.Discovery = append([]mcpcontractcheck.ToolRecord(nil), inputs.Discovery...)
	clone.Registry = append([]mcpcontractcheck.HandlerBinding(nil), inputs.Registry...)
	clone.Aliases = append([]mcpcontractcheck.AliasBinding(nil), inputs.Aliases...)
	return clone
}
