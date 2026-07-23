package contractvalidator

import "testing"

func TestCLIManifestDiagnosticsAcceptRelationshipGraph(t *testing.T) {
	document := cliRelationshipTestDocument()
	command := cliRelationshipTestCommand(document)
	command["relationships"] = map[string]any{
		"example.rel.mutex":       cliTestGroupRelationship("example.rel.mutex", "mutually-exclusive", "example.flag.a", "example.flag.b"),
		"example.rel.together":    cliTestGroupRelationship("example.rel.together", "required-together", "example.flag.c", "example.flag.d"),
		"example.rel.one":         cliTestGroupRelationship("example.rel.one", "at-least-one", "example.flag.a", "example.flag.c"),
		"example.rel.dependency":  cliTestDirectedRelationship("example.rel.dependency", "dependency", "example.flag.a", "example.flag.d"),
		"example.rel.conditional": cliTestDirectedRelationship("example.rel.conditional", "conditional", "example.flag.b", "example.flag.e"),
		"example.rel.conflict":    cliTestGroupRelationship("example.rel.conflict", "conflict", "example.flag.d", "example.flag.e"),
	}

	if diagnostics := cliManifestDiagnostics("contract.json", document); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want a valid multi-input relationship graph", diagnostics)
	}
}

func TestCLIManifestDiagnosticsAcceptPersistentAncestorRelationshipParticipant(t *testing.T) {
	document := cliRelationshipTestDocument()
	commands := document["commands"].(map[string]any)
	commands["example"].(map[string]any)["flags"] = map[string]any{
		"example.flag.global": cliRelationshipTestFlag("example.flag.global", "persistent"),
	}
	child := commands["example.run"].(map[string]any)
	child["relationships"] = map[string]any{
		"example.run.rel.together": cliTestGroupRelationship(
			"example.run.rel.together", "required-together", "example.flag.global", "example.flag.a",
		),
	}

	if diagnostics := cliManifestDiagnostics("contract.json", document); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want persistent ancestor input to be visible", diagnostics)
	}
}

func TestCLIManifestDiagnosticsCoalesceInheritedRelationshipIdentity(t *testing.T) {
	document := cliRelationshipTestDocument()
	commands := document["commands"].(map[string]any)
	commands["example"].(map[string]any)["flags"] = map[string]any{
		"example.flag.global": cliRelationshipTestFlag("example.flag.global", "persistent"),
	}
	child := commands["example.run"].(map[string]any)
	inherited := cliRelationshipTestFlag("example.run.flag.global", "inherited")
	inherited["inheritedFromInputId"] = "example.flag.global"
	child["flags"].(map[string]any)["example.run.flag.global"] = inherited
	child["relationships"] = map[string]any{
		"example.run.rel.self": cliTestDirectedRelationship(
			"example.run.rel.self", "dependency", "example.flag.global", "example.run.flag.global",
		),
	}

	diagnostics := cliManifestDiagnostics("contract.json", document)
	assertCLIDiagnostic(t, diagnostics, "cli.relationship.self-reference", "/commands/example.run/relationships/example.run.rel.self/participants/0/id")
}

func TestCLIManifestDiagnosticsRejectInvalidRelationshipGraphs(t *testing.T) {
	tests := []struct {
		name       string
		relations  map[string]any
		code       string
		pathSuffix string
	}{
		{
			name: "unknown participant",
			relations: map[string]any{"example.rel.unknown": cliTestGroupRelationship(
				"example.rel.unknown", "conflict", "example.flag.a", "example.flag.missing",
			)},
			code: "cli.relationship.unknown-participant", pathSuffix: "/example.rel.unknown/participants/1/id",
		},
		{
			name: "participant type mismatch",
			relations: map[string]any{"example.rel.type": map[string]any{
				"id": "example.rel.type", "kind": "conflict", "participants": []any{
					map[string]any{"type": "argument", "id": "example.flag.a"}, cliTestParticipant("example.flag.b"),
				},
			}},
			code: "cli.relationship.participant-type", pathSuffix: "/example.rel.type/participants/0/id",
		},
		{
			name: "self relation",
			relations: map[string]any{"example.rel.self": cliTestDirectedRelationship(
				"example.rel.self", "dependency", "example.flag.a", "example.flag.a",
			)},
			code: "cli.relationship.self-reference", pathSuffix: "/example.rel.self/participants/0/id",
		},
		{
			name: "duplicate participant identity",
			relations: map[string]any{"example.rel.participant": cliTestGroupRelationship(
				"example.rel.participant", "at-least-one", "example.flag.a", "example.flag.a",
			)},
			code: "cli.relationship.duplicate-participant", pathSuffix: "/example.rel.participant/participants/1/id",
		},
		{
			name: "duplicate equivalent relation",
			relations: map[string]any{
				"example.rel.first":  cliTestGroupRelationship("example.rel.first", "mutually-exclusive", "example.flag.a", "example.flag.b"),
				"example.rel.second": cliTestGroupRelationship("example.rel.second", "mutually-exclusive", "example.flag.b", "example.flag.a"),
			},
			code: "cli.relationship.duplicate", pathSuffix: "/example.rel.first/id",
		},
		{
			name: "contradictory relation set",
			relations: map[string]any{
				"example.rel.conflict": cliTestGroupRelationship("example.rel.conflict", "conflict", "example.flag.a", "example.flag.b"),
				"example.rel.together": cliTestGroupRelationship("example.rel.together", "required-together", "example.flag.b", "example.flag.a"),
			},
			code: "cli.relationship.contradictory", pathSuffix: "/example.rel.together/id",
		},
		{
			name: "dependency contradicts exclusion",
			relations: map[string]any{
				"example.rel.conflict": cliTestGroupRelationship("example.rel.conflict", "conflict", "example.flag.a", "example.flag.b"),
				"example.rel.depends":  cliTestDirectedRelationship("example.rel.depends", "dependency", "example.flag.a", "example.flag.b"),
			},
			code: "cli.relationship.contradictory", pathSuffix: "/example.rel.depends/id",
		},
		{
			name: "dependency and conditional cycle",
			relations: map[string]any{
				"example.rel.depends":     cliTestDirectedRelationship("example.rel.depends", "dependency", "example.flag.a", "example.flag.b"),
				"example.rel.conditional": cliTestDirectedRelationship("example.rel.conditional", "conditional", "example.flag.b", "example.flag.a"),
			},
			code: "cli.relationship.cycle", pathSuffix: "/example.rel.conditional/participants/0/id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := cliRelationshipTestDocument()
			cliRelationshipTestCommand(document)["relationships"] = test.relations
			diagnostics := cliManifestDiagnostics("contract.json", document)
			assertCLIDiagnostic(t, diagnostics, test.code, "/commands/example.run/relationships"+test.pathSuffix)
		})
	}
}

func TestCLIManifestRelationshipDiagnosticsAreDeterministicallyOrdered(t *testing.T) {
	document := cliRelationshipTestDocument()
	cliRelationshipTestCommand(document)["relationships"] = map[string]any{
		"example.rel.z": cliTestGroupRelationship("example.rel.z", "conflict", "example.flag.a", "example.flag.z"),
		"example.rel.a": cliTestGroupRelationship("example.rel.a", "conflict", "example.flag.a", "example.flag.y"),
	}

	diagnostics := cliManifestDiagnostics("contract.json", document)
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want two unknown participants", diagnostics)
	}
	if diagnostics[0].Path != "/commands/example.run/relationships/example.rel.a/participants/1/id" ||
		diagnostics[1].Path != "/commands/example.run/relationships/example.rel.z/participants/1/id" {
		t.Fatalf("diagnostic paths = %q, %q, want lexical relationship order", diagnostics[0].Path, diagnostics[1].Path)
	}
}

func TestCLIManifestDiagnosticsRejectRelationshipParticipantOutsideEffectiveScope(t *testing.T) {
	document := cliRelationshipTestDocument()
	commands := document["commands"].(map[string]any)
	commands["other"] = map[string]any{
		"id": "other", "path": "other",
		"flags": map[string]any{"other.flag.local": cliRelationshipTestFlag("other.flag.local", "local")},
	}
	command := cliRelationshipTestCommand(document)
	command["relationships"] = map[string]any{
		"example.rel.out-of-scope": cliTestGroupRelationship(
			"example.rel.out-of-scope", "conflict", "example.flag.a", "other.flag.local",
		),
	}

	diagnostics := cliManifestDiagnostics("contract.json", document)
	assertCLIDiagnostic(t, diagnostics, "cli.relationship.unknown-participant", "/commands/example.run/relationships/example.rel.out-of-scope/participants/1/id")
}

func cliRelationshipTestDocument() map[string]any {
	flags := make(map[string]any)
	for _, suffix := range []string{"a", "b", "c", "d", "e"} {
		id := "example.flag." + suffix
		flags[id] = cliRelationshipTestFlag(id, "local")
	}
	return map[string]any{"commands": map[string]any{
		"example":     map[string]any{"id": "example", "path": "example"},
		"example.run": map[string]any{"id": "example.run", "path": "example run", "flags": flags},
	}}
}

func cliRelationshipTestCommand(document map[string]any) map[string]any {
	return document["commands"].(map[string]any)["example.run"].(map[string]any)
}

func cliRelationshipTestFlag(id, scope string) map[string]any {
	return map[string]any{"id": id, "scope": scope}
}

func cliTestGroupRelationship(id, kind string, participantIDs ...string) map[string]any {
	participants := make([]any, 0, len(participantIDs))
	for _, participantID := range participantIDs {
		participants = append(participants, cliTestParticipant(participantID))
	}
	return map[string]any{"id": id, "kind": kind, "participants": participants}
}

func cliTestDirectedRelationship(id, kind, when string, targets ...string) map[string]any {
	relationship := cliTestGroupRelationship(id, kind, targets...)
	relationship["when"] = cliTestParticipant(when)
	return relationship
}

func cliTestParticipant(id string) map[string]any {
	return map[string]any{"type": "flag", "id": id}
}
