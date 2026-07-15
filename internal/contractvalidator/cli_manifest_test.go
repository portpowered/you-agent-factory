package contractvalidator

import "testing"

func TestCLIManifestDiagnosticsRejectUnknownRelationshipParticipant(t *testing.T) {
	document := map[string]any{
		"commands": map[string]any{
			"you.mcp.serve": map[string]any{
				"flags": map[string]any{
					"you.mcp.serve.flag.runtime": map[string]any{"id": "you.mcp.serve.flag.runtime"},
				},
				"relationships": map[string]any{
					"you.mcp.serve.relationship.runtime-source": map[string]any{
						"participants": []any{
							map[string]any{"type": "flag", "id": "you.mcp.serve.flag.runtime"},
							map[string]any{"type": "flag", "id": "you.mcp.serve.flag.fixture-catalog"},
						},
					},
				},
			},
		},
	}

	diagnostics := cliManifestDiagnostics("contracts/cli/commands.json", document)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one unknown participant", diagnostics)
	}
	if got := diagnostics[0].Code; got != "cli.relationship.unknown-participant" {
		t.Fatalf("diagnostic code = %q, want cli.relationship.unknown-participant", got)
	}
	wantPath := "/commands/you.mcp.serve/relationships/you.mcp.serve.relationship.runtime-source/participants/1/id"
	if got := diagnostics[0].Path; got != wantPath {
		t.Fatalf("diagnostic path = %q, want %q", got, wantPath)
	}
}
