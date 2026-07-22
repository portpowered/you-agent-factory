package contractvalidator

import "testing"

func TestCLIManifestDiagnosticsValidateCanonicalPrecedence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		code   string
		path   string
	}{
		{
			name: "missing precedence",
			mutate: func(command map[string]any) {
				delete(command, "precedence")
			},
			code: "cli.precedence.missing", path: "/commands/example/precedence",
		},
		{
			name: "duplicate and missing tier",
			mutate: func(command map[string]any) {
				command["precedence"].(map[string]any)["order"].([]any)[5] = "manifest-default"
			},
			code: "cli.precedence.duplicate", path: "/commands/example/precedence/order/5",
		},
		{
			name: "unknown tier",
			mutate: func(command map[string]any) {
				command["precedence"].(map[string]any)["order"].([]any)[2] = "unknown"
			},
			code: "cli.precedence.unknown", path: "/commands/example/precedence/order/2",
		},
		{
			name: "reordered tier",
			mutate: func(command map[string]any) {
				order := command["precedence"].(map[string]any)["order"].([]any)
				order[0], order[1] = order[1], order[0]
			},
			code: "cli.precedence.order", path: "/commands/example/precedence/order/0",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := cliCanonicalInputTestDocument()
			command := document["commands"].(map[string]any)["example"].(map[string]any)
			test.mutate(command)
			diagnostics := cliManifestDiagnostics("contract.json", document)
			assertCLIDiagnostic(t, diagnostics, test.code, test.path)
		})
	}
}

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

func TestCLIManifestDiagnosticsAcceptTypedValuesAndExplicitBindings(t *testing.T) {
	diagnostics := cliManifestDiagnostics("contract.json", cliCanonicalInputTestDocument())
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want a valid canonical input contract", diagnostics)
	}
}

func TestCLIManifestDiagnosticsAcceptRequiredInputWithoutDefault(t *testing.T) {
	document := cliCanonicalInputTestDocument()
	input := cliCanonicalTestFlag(document, "example.flag.name")
	input["required"] = true
	input["minCardinality"] = float64(1)
	delete(input, "defaultValue")
	delete(input, "noOptionDefaultValue")
	input["acceptedSources"] = []any{"cli", "environment"}

	diagnostics := cliManifestDiagnostics("contract.json", document)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want required input without default to be valid", diagnostics)
	}
}

func TestCLIManifestDiagnosticsRejectInvalidTypedValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		code   string
		field  string
	}{
		{name: "wrong typed member", mutate: func(input map[string]any) { input["defaultValue"] = map[string]any{"int": float64(1)} }, code: "cli.input.value-type", field: "defaultValue"},
		{name: "choice outside enum", mutate: func(input map[string]any) { input["defaultValue"] = map[string]any{"string": "other"} }, code: "cli.input.value-choice", field: "defaultValue"},
		{name: "unnormalized default", mutate: func(input map[string]any) { input["defaultValue"] = map[string]any{"string": " Worker "} }, code: "cli.input.value-normalization", field: "defaultValue"},
		{name: "invalid no-option type", mutate: func(input map[string]any) {
			input["valueType"] = "int"
			input["defaultValue"] = map[string]any{"int": float64(1)}
			input["noOptionDefaultValue"] = map[string]any{"int": float64(1)}
		}, code: "cli.input.no-option-invalid", field: "noOptionDefaultValue"},
		{name: "normalization on integer", mutate: func(input map[string]any) {
			input["valueType"] = "int"
			input["defaultValue"] = map[string]any{"int": float64(1)}
			delete(input, "noOptionDefaultValue")
		}, code: "cli.input.normalization-value-type", field: "normalization"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := cliCanonicalInputTestDocument()
			input := cliCanonicalTestFlag(document, "example.flag.name")
			test.mutate(input)
			diagnostics := cliManifestDiagnostics("contract.json", document)
			assertCLIDiagnostic(t, diagnostics, test.code, "/commands/example/flags/example.flag.name/"+test.field)
		})
	}
}

func TestCLIManifestDiagnosticsRejectDefaultSourceMismatches(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		code   string
		path   string
	}{
		{
			name: "typed default without manifest source",
			mutate: func(input map[string]any) {
				input["acceptedSources"] = []any{"cli", "environment"}
			},
			code: "cli.input.default-source-missing",
			path: "/commands/example/flags/example.flag.name/defaultValue",
		},
		{
			name: "manifest source without typed default",
			mutate: func(input map[string]any) {
				delete(input, "defaultValue")
			},
			code: "cli.input.default-value-missing",
			path: "/commands/example/flags/example.flag.name/acceptedSources/2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := cliCanonicalInputTestDocument()
			test.mutate(cliCanonicalTestFlag(document, "example.flag.name"))
			diagnostics := cliManifestDiagnostics("contract.json", document)
			assertCLIDiagnostic(t, diagnostics, test.code, test.path)
		})
	}
}

func TestCLIManifestDiagnosticsRejectImpossibleCardinality(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		code   string
		field  string
	}{
		{name: "required with zero minimum", mutate: func(input map[string]any) { input["required"] = true }, code: "cli.input.cardinality-required", field: "minCardinality"},
		{name: "minimum above maximum", mutate: func(input map[string]any) { input["minCardinality"] = float64(2) }, code: "cli.input.cardinality-range", field: "maxCardinality"},
		{name: "scalar repeated", mutate: func(input map[string]any) { input["repeatable"] = true; input["maxCardinality"] = float64(-1) }, code: "cli.input.cardinality-value-type", field: "valueType"},
		{name: "repeatable capped at one", mutate: func(input map[string]any) {
			input["repeatable"] = true
			input["valueType"] = "stringArray"
			input["defaultValue"] = map[string]any{"stringArray": []any{"worker"}}
		}, code: "cli.input.cardinality-repeatable", field: "maxCardinality"},
		{name: "default exceeds maximum", mutate: func(input map[string]any) {
			input["valueType"] = "stringArray"
			input["defaultValue"] = map[string]any{"stringArray": []any{"worker", "worker"}}
			delete(input, "noOptionDefaultValue")
		}, code: "cli.input.default-cardinality", field: "defaultValue"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := cliCanonicalInputTestDocument()
			test.mutate(cliCanonicalTestFlag(document, "example.flag.name"))
			diagnostics := cliManifestDiagnostics("contract.json", document)
			assertCLIDiagnostic(t, diagnostics, test.code, "/commands/example/flags/example.flag.name/"+test.field)
		})
	}
}

func TestCLIManifestDiagnosticsRejectInvalidSourceAndHandlerBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		code   string
		path   string
	}{
		{
			name: "unknown source target",
			mutate: func(command map[string]any) {
				command["sourceBindings"].(map[string]any)["example.source.env"].(map[string]any)["inputId"] = "example.flag.missing"
			},
			code: "cli.source.unknown-input", path: "/commands/example/sourceBindings/example.source.env/inputId",
		},
		{
			name: "source not accepted",
			mutate: func(command map[string]any) {
				cliCanonicalTestFlagCommand(command, "example.flag.name")["acceptedSources"] = []any{"cli"}
			},
			code: "cli.source.not-accepted", path: "/commands/example/sourceBindings/example.source.env/inputId",
		},
		{
			name:   "missing source binding",
			mutate: func(command map[string]any) { command["sourceBindings"] = map[string]any{} },
			code:   "cli.source.missing-binding", path: "/commands/example/flags/example.flag.name/acceptedSources/1",
		},
		{
			name: "stdin cannot target boolean",
			mutate: func(command map[string]any) {
				input := cliCanonicalTestFlagCommand(command, "example.flag.name")
				input["valueType"] = "bool"
				input["defaultValue"] = map[string]any{"boolean": false}
				input["noOptionDefaultValue"] = map[string]any{"boolean": true}
				input["enum"] = []any{}
				input["normalization"] = ""
				input["acceptedSources"] = []any{"cli", "stdin"}
				command["sourceBindings"].(map[string]any)["example.source.env"] = map[string]any{"id": "example.source.stdin", "source": "stdin", "inputId": "example.flag.name"}
			},
			code: "cli.source.stdin-shape", path: "/commands/example/sourceBindings/example.source.env/inputId",
		},
		{
			name: "unknown handler declaration target",
			mutate: func(command map[string]any) {
				command["handlerBindings"].(map[string]any)["example.binding.name"].(map[string]any)["inputId"] = "example.flag.missing"
			},
			code: "cli.binding.unknown-input", path: "/commands/example/handlerBindings/example.binding.name/inputId",
		},
		{
			name: "unknown handler reference",
			mutate: func(command map[string]any) {
				cliCanonicalTestFlagCommand(command, "example.flag.name")["handlerBindingId"] = "example.binding.missing"
			},
			code: "cli.binding.unknown-handler", path: "/commands/example/flags/example.flag.name/handlerBindingId",
		},
		{
			name: "handler binding claimed twice",
			mutate: func(command map[string]any) {
				cliCanonicalTestFlagCommand(command, "example.flag.other")["handlerBindingId"] = "example.binding.name"
			},
			code: "cli.binding.multiple-targets", path: "/commands/example/flags/example.flag.name/handlerBindingId",
		},
		{
			name: "external key targets twice",
			mutate: func(command map[string]any) {
				cliCanonicalTestFlagCommand(command, "example.flag.other")["acceptedSources"] = []any{"cli", "environment"}
				command["sourceBindings"].(map[string]any)["example.source.other"] = map[string]any{"id": "example.source.other", "source": "environment", "externalKey": "EXAMPLE_NAME", "inputId": "example.flag.other"}
			},
			code: "cli.source.multiple-targets", path: "/commands/example/sourceBindings/example.source.env/inputId",
		},
		{
			name: "same tier binds one input twice",
			mutate: func(command map[string]any) {
				command["sourceBindings"].(map[string]any)["example.source.env.second"] = map[string]any{"id": "example.source.env.second", "source": "environment", "externalKey": "EXAMPLE_NAME_SECOND", "inputId": "example.flag.name"}
			},
			code: "cli.source.multiple-bindings", path: "/commands/example/sourceBindings/example.source.env.second/inputId",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := cliCanonicalInputTestDocument()
			command := document["commands"].(map[string]any)["example"].(map[string]any)
			test.mutate(command)
			diagnostics := cliManifestDiagnostics("contract.json", document)
			assertCLIDiagnostic(t, diagnostics, test.code, test.path)
		})
	}
}

func cliCanonicalInputTestDocument() map[string]any {
	name := map[string]any{
		"id": "example.flag.name", "valueType": "string", "required": false,
		"minCardinality": float64(0), "maxCardinality": float64(1), "repeatable": false,
		"defaultValue": map[string]any{"string": "worker"}, "noOptionDefaultValue": map[string]any{"string": "worker"},
		"enum": []any{"worker"}, "normalization": "lowercase-trim", "acceptedSources": []any{"cli", "environment", "manifest-default"},
		"handlerBindingId": "example.binding.name",
	}
	other := map[string]any{
		"id": "example.flag.other", "valueType": "string", "required": false,
		"minCardinality": float64(0), "maxCardinality": float64(1), "repeatable": false,
		"defaultValue": map[string]any{"string": "other"}, "normalization": "", "acceptedSources": []any{"cli", "manifest-default"},
		"handlerBindingId": "example.binding.other",
	}
	return map[string]any{"commands": map[string]any{"example": map[string]any{
		"id": "example", "path": "example", "completeness": "authoritative", "flags": map[string]any{"example.flag.name": name, "example.flag.other": other},
		"precedence":     canonicalCLIPrecedenceTestValue(),
		"sourceBindings": map[string]any{"example.source.env": map[string]any{"id": "example.source.env", "source": "environment", "externalKey": "EXAMPLE_NAME", "inputId": "example.flag.name"}},
		"handlerBindings": map[string]any{
			"example.binding.name":  map[string]any{"id": "example.binding.name", "inputId": "example.flag.name"},
			"example.binding.other": map[string]any{"id": "example.binding.other", "inputId": "example.flag.other"},
		},
	}}}
}

func canonicalCLIPrecedenceTestValue() map[string]any {
	return map[string]any{
		"order":       []any{"cli", "stdin", "environment", "operator-config", "manifest-default", "factory-signature-default"},
		"withinTier":  map[string]any{"scalar": "last", "repeated": "append"},
		"acrossTiers": "replace", "multipleBindings": "reject",
	}
}

func cliCanonicalTestFlag(document map[string]any, id string) map[string]any {
	command := document["commands"].(map[string]any)["example"].(map[string]any)
	return cliCanonicalTestFlagCommand(command, id)
}

func cliCanonicalTestFlagCommand(command map[string]any, id string) map[string]any {
	return command["flags"].(map[string]any)[id].(map[string]any)
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
