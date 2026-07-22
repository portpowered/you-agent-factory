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

func TestCLIManifestDiagnosticsAcceptDisjointScopesAndValidInheritance(t *testing.T) {
	document := cliScopeTestDocument()
	document["commands"].(map[string]any)["you.inspect"] = map[string]any{
		"id": "you.inspect", "path": "you inspect",
		"flags": map[string]any{
			"you.inspect.flag.output": cliScopeTestFlag("you.inspect.flag.output", "output", "local"),
			"you.inspect.flag.cache":  cliScopeTestFlag("you.inspect.flag.cache", "cache", "persistent"),
		},
	}
	document["commands"].(map[string]any)["you.export"] = map[string]any{
		"id": "you.export", "path": "you export",
		"flags": map[string]any{
			"you.export.flag.output": cliScopeTestFlag("you.export.flag.output", "output", "local"),
			"you.export.flag.cache":  cliScopeTestFlag("you.export.flag.cache", "cache", "persistent"),
		},
	}

	diagnostics := cliManifestDiagnostics("contract.json", document)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want valid inheritance and disjoint local reuse", diagnostics)
	}
}

func TestCLIManifestDiagnosticsRejectEffectiveScopeSpellingCollisions(t *testing.T) {
	document := cliScopeTestDocument()
	commands := document["commands"].(map[string]any)
	run := commands["you.run"].(map[string]any)
	run["flags"].(map[string]any)["you.run.flag.local-long"] = cliScopeTestFlag("you.run.flag.local-long", "verbose", "local")
	aliasCollision := cliScopeTestFlag("you.run.flag.local-alias", "trace", "local")
	aliasCollision["aliases"] = []any{"verbose"}
	run["flags"].(map[string]any)["you.run.flag.local-alias"] = aliasCollision
	shortCollision := cliScopeTestFlag("you.run.flag.local-short", "diagnostics", "local")
	shortCollision["shorthand"] = "v"
	run["flags"].(map[string]any)["you.run.flag.local-short"] = shortCollision

	diagnostics := cliManifestDiagnostics("contract.json", document)
	assertCLIDiagnostic(t, diagnostics, "cli.input.spelling-duplicate", "/commands/you.run/flags/you.run.flag.local-long/long")
	assertCLIDiagnostic(t, diagnostics, "cli.input.spelling-duplicate", "/commands/you.run/flags/you.run.flag.local-alias/aliases/0")
	assertCLIDiagnostic(t, diagnostics, "cli.input.spelling-duplicate", "/commands/you.run/flags/you.run.flag.local-short/shorthand")
}

func TestCLIManifestDiagnosticsRejectInvalidInheritance(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(map[string]any)
		code       string
		pathSuffix string
	}{
		{
			name:   "unknown source",
			mutate: func(flag map[string]any) { flag["inheritedFromInputId"] = "you.flag.missing" },
			code:   "cli.inheritance.unknown-source", pathSuffix: "inheritedFromInputId",
		},
		{
			name:   "local source",
			mutate: func(flag map[string]any) { flag["inheritedFromInputId"] = "you.run.flag.local" },
			code:   "cli.inheritance.source-not-persistent", pathSuffix: "inheritedFromInputId",
		},
		{
			name:   "non ancestor source",
			mutate: func(flag map[string]any) { flag["inheritedFromInputId"] = "you.inspect.flag.persistent" },
			code:   "cli.inheritance.non-ancestor", pathSuffix: "inheritedFromInputId",
		},
		{
			name:   "contradictory redeclaration",
			mutate: func(flag map[string]any) { flag["valueType"] = "string" },
			code:   "cli.inheritance.semantic-mismatch", pathSuffix: "valueType",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := cliScopeTestDocument()
			flag := document["commands"].(map[string]any)["you.run"].(map[string]any)["flags"].(map[string]any)["you.run.flag.verbose"].(map[string]any)
			test.mutate(flag)
			diagnostics := cliManifestDiagnostics("contract.json", document)
			assertCLIDiagnostic(t, diagnostics, test.code, "/commands/you.run/flags/you.run.flag.verbose/"+test.pathSuffix)
		})
	}
}

func cliScopeTestDocument() map[string]any {
	persistent := cliScopeTestFlag("you.flag.verbose", "verbose", "persistent")
	persistent["shorthand"] = "v"
	inherited := cliScopeTestFlag("you.run.flag.verbose", "verbose", "inherited")
	inherited["shorthand"] = "v"
	inherited["inheritedFromInputId"] = "you.flag.verbose"
	return map[string]any{
		"commands": map[string]any{
			"you": map[string]any{
				"id": "you", "path": "you",
				"flags": map[string]any{"you.flag.verbose": persistent},
			},
			"you.run": map[string]any{
				"id": "you.run", "path": "you run",
				"flags": map[string]any{
					"you.run.flag.verbose": inherited,
					"you.run.flag.local":   cliScopeTestFlag("you.run.flag.local", "local", "local"),
				},
			},
			"you.inspect": map[string]any{
				"id": "you.inspect", "path": "you inspect",
				"flags": map[string]any{
					"you.inspect.flag.persistent": cliScopeTestFlag("you.inspect.flag.persistent", "inspect", "persistent"),
				},
			},
		},
	}
}

func cliScopeTestFlag(id, long, scope string) map[string]any {
	return map[string]any{
		"id": id, "long": long, "shorthand": "", "aliases": []any{}, "scope": scope,
		"valueType": "bool", "required": false, "default": "false", "changedDefault": false,
		"noOptionDefault": "true", "repeatable": false, "normalization": "", "completion": "none",
		"binding": "", "visibility": "visible",
		"lifecycle": map[string]any{"formatVersion": "1.0.0", "itemId": id, "state": "active", "since": "1.0.0"},
	}
}

func assertCLIDiagnostic(t *testing.T, diagnostics []Diagnostic, code, path string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && diagnostic.Path == path {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want code=%q path=%q", diagnostics, code, path)
}
