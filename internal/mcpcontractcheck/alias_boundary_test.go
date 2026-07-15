package mcpcontractcheck_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/mcpcontractcheck"
)

const (
	statusAliasID   = "mcp.alias.you.workflow.status"
	statusAliasName = "you.workflow.status"
)

func TestValidateRejectsInvalidRetainedAliasMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*mcpcontractcheck.Inputs)
		wantCode string
		wantText []string
	}{
		{
			name: "missing canonical target", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Aliases[0].CanonicalToolID = ""
			},
			wantCode: "mcp.alias.missing_target", wantText: []string{statusAliasID, statusAliasName, "lifecycle.successor.targetItemId"},
		},
		{
			name: "unknown canonical target", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Aliases[0].CanonicalToolID = statusToolID
			},
			wantCode: "mcp.alias.unknown_target", wantText: []string{statusAliasID, statusAliasName, statusToolID, "contracts/mcp/deprecated.json"},
		},
		{
			name: "conflicting duplicate ID", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Aliases = append(inputs.Aliases, mcpcontractcheck.AliasBinding{
					ID: statusAliasID, Name: "you.workflow.other", CanonicalToolID: statusToolID,
				})
			},
			wantCode: "mcp.alias.conflicting_mapping", wantText: []string{statusAliasID, listToolID, statusToolID, "one alias inventory mapping"},
		},
		{
			name: "conflicting duplicate name", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Aliases = append(inputs.Aliases, mcpcontractcheck.AliasBinding{
					ID: "mcp.alias.you.workflow.other", Name: statusAliasName, CanonicalToolID: statusToolID,
				})
			},
			wantCode: "mcp.alias.conflicting_mapping", wantText: []string{statusAliasName, listToolID, statusToolID, "one alias inventory mapping"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inputs := cleanAliasInputs()
			test.mutate(&inputs)

			diagnostics := mcpcontractcheck.Validate(inputs)

			assertDiagnosticCodes(t, diagnostics, []string{test.wantCode})
			for _, want := range test.wantText {
				if !strings.Contains(diagnostics[0].Message, want) {
					t.Fatalf("Validate() message = %q, want text %q", diagnostics[0].Message, want)
				}
			}
		})
	}
}

func TestValidateRejectsAliasIdentityOnCanonicalSurfaces(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*mcpcontractcheck.Inputs)
		surface string
	}{
		{
			name: "alias name in catalog", surface: "catalog", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Catalog[0].Name = statusAliasName
			},
		},
		{
			name: "alias identity in catalog", surface: "catalog", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Catalog[0].ID = statusAliasID
				inputs.Discovery[0].ID = statusAliasID
				inputs.Registry[0].ToolID = statusAliasID
			},
		},
		{
			name: "alias name in discovery", surface: "discovery", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Discovery[0].Name = statusAliasName
			},
		},
		{
			name: "alias identity in discovery", surface: "discovery", mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.Discovery[0].ID = statusAliasID
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inputs := cleanAliasInputs()
			test.mutate(&inputs)

			diagnostics := mcpcontractcheck.Validate(inputs)
			aliasDiagnostic := findDiagnostic(t, diagnostics, "mcp.alias.canonical", test.surface)
			if aliasDiagnostic.ToolID != listToolID {
				t.Fatalf("alias diagnostic ToolID = %q, want canonical target %q", aliasDiagnostic.ToolID, listToolID)
			}
			for _, want := range []string{statusAliasName, statusAliasID, listToolID, test.surface, "retained compatibility path"} {
				if !strings.Contains(aliasDiagnostic.Message, want) {
					t.Fatalf("Validate() message = %q, want text %q", aliasDiagnostic.Message, want)
				}
			}
		})
	}
}

func TestValidateRejectsRuntimeAliasDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*mcpcontractcheck.Inputs)
		wantCode string
	}{
		{
			name: "retained alias missing runtime route", wantCode: "mcp.alias.runtime_missing",
			mutate: func(inputs *mcpcontractcheck.Inputs) { inputs.RuntimeAliases = nil },
		},
		{
			name: "runtime route targets wrong canonical name", wantCode: "mcp.alias.runtime_target_mismatch",
			mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.RuntimeAliases[0].CanonicalName = "you.factory_session.other"
			},
		},
		{
			name: "runtime alias absent from retained inventory", wantCode: "mcp.alias.runtime_uninventoried",
			mutate: func(inputs *mcpcontractcheck.Inputs) {
				inputs.RuntimeAliases = append(inputs.RuntimeAliases, mcpcontractcheck.RuntimeAliasBinding{
					Name: "you.workflow.other", CanonicalName: "you.factory_session.list",
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inputs := cleanAliasInputs()
			test.mutate(&inputs)
			diagnostics := mcpcontractcheck.Validate(inputs)
			assertDiagnosticCodes(t, diagnostics, []string{test.wantCode})
		})
	}
}

func cleanAliasInputs() mcpcontractcheck.Inputs {
	inputs := cleanInputs()
	inputs.Aliases = []mcpcontractcheck.AliasBinding{{
		ID: statusAliasID, Name: statusAliasName, CanonicalToolID: listToolID,
	}}
	inputs.RuntimeAliases = []mcpcontractcheck.RuntimeAliasBinding{{
		Name: statusAliasName, CanonicalName: "you.factory_session.list",
	}}
	return inputs
}

func findDiagnostic(t *testing.T, diagnostics []mcpcontractcheck.Diagnostic, code, surface string) mcpcontractcheck.Diagnostic {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && diagnostic.Surface == surface {
			return diagnostic
		}
	}
	t.Fatalf("Validate() diagnostics = %+v, want code %q on %s", diagnostics, code, surface)
	return mcpcontractcheck.Diagnostic{}
}
