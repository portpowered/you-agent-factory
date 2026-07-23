package contractvalidator

import (
	"fmt"
	"sort"
	"strings"
)

type cliRelationshipParticipant struct {
	id       string
	identity string
	kind     string
	path     []string
}

type cliRelationshipInput struct {
	identity string
	kind     string
}

type cliRelationshipRecord struct {
	key          string
	kind         string
	path         []string
	participants []cliRelationshipParticipant
	when         *cliRelationshipParticipant
}

type cliRelationshipEdge struct {
	from string
	to   string
	path []string
}

func cliCommandRelationshipDiagnostics(
	document, commandKey string,
	command map[string]any,
	index cliManifestIndex,
) []Diagnostic {
	relationships := collectCLIRelationships(commandKey, command)
	if len(relationships) == 0 {
		return nil
	}

	known := cliEffectiveRelationshipInputs(command, index)
	resolveCLIRelationshipIdentities(relationships, known)
	var diagnostics []Diagnostic
	for _, relationship := range relationships {
		diagnostics = append(diagnostics, cliRelationshipReferenceDiagnostics(document, commandKey, relationship, known)...)
		diagnostics = append(diagnostics, cliRelationshipSelfDiagnostics(document, relationship)...)
	}
	diagnostics = append(diagnostics, cliDuplicateRelationshipDiagnostics(document, relationships)...)
	diagnostics = append(diagnostics, cliContradictoryRelationshipDiagnostics(document, relationships)...)
	diagnostics = append(diagnostics, cliRelationshipCycleDiagnostics(document, relationships, known)...)
	return diagnostics
}

func collectCLIRelationships(commandKey string, command map[string]any) []cliRelationshipRecord {
	values, _ := command["relationships"].(map[string]any)
	relationships := make([]cliRelationshipRecord, 0, len(values))
	for _, key := range sortedStringKeys(values) {
		value, _ := values[key].(map[string]any)
		base := []string{"commands", commandKey, "relationships", key}
		relationship := cliRelationshipRecord{key: key, path: base}
		relationship.kind, _ = value["kind"].(string)
		participants, _ := value["participants"].([]any)
		for participantIndex, participant := range participants {
			relationship.participants = append(relationship.participants, cliRelationshipParticipantValue(
				participant, append(base, "participants", fmt.Sprint(participantIndex)),
			))
		}
		if when, exists := value["when"]; exists {
			participant := cliRelationshipParticipantValue(when, append(base, "when"))
			relationship.when = &participant
		}
		relationships = append(relationships, relationship)
	}
	return relationships
}

func cliRelationshipParticipantValue(value any, path []string) cliRelationshipParticipant {
	participant, _ := value.(map[string]any)
	id, _ := participant["id"].(string)
	kind, _ := participant["type"].(string)
	return cliRelationshipParticipant{id: id, kind: kind, path: path}
}

func cliEffectiveRelationshipInputs(
	command map[string]any,
	index cliManifestIndex,
) map[string]cliRelationshipInput {
	known := make(map[string]cliRelationshipInput)
	add := func(owner map[string]any, persistentOnly bool) {
		for _, field := range []string{"arguments", "flags"} {
			records, _ := owner[field].(map[string]any)
			for _, recordKey := range sortedStringKeys(records) {
				record, _ := records[recordKey].(map[string]any)
				if persistentOnly && record["scope"] != "persistent" {
					continue
				}
				id, _ := record["id"].(string)
				if id != "" {
					identity := id
					if sourceID, _ := record["inheritedFromInputId"].(string); sourceID != "" {
						identity = sourceID
					}
					known[id] = cliRelationshipInput{identity: identity, kind: strings.TrimSuffix(field, "s")}
				}
			}
		}
	}
	add(command, false)
	for _, ancestor := range cliAncestorCommands(command, index) {
		add(ancestor, true)
	}
	return known
}

func resolveCLIRelationshipIdentities(relationships []cliRelationshipRecord, known map[string]cliRelationshipInput) {
	for relationshipIndex := range relationships {
		relationship := &relationships[relationshipIndex]
		for participantIndex := range relationship.participants {
			participant := &relationship.participants[participantIndex]
			participant.identity = known[participant.id].identity
		}
		if relationship.when != nil {
			relationship.when.identity = known[relationship.when.id].identity
		}
	}
}

func cliRelationshipReferenceDiagnostics(
	document, commandKey string,
	relationship cliRelationshipRecord,
	known map[string]cliRelationshipInput,
) []Diagnostic {
	participants := append([]cliRelationshipParticipant(nil), relationship.participants...)
	if relationship.when != nil {
		participants = append(participants, *relationship.when)
	}
	var diagnostics []Diagnostic
	for _, participant := range participants {
		path := instancePath(append(participant.path, "id"))
		input, exists := known[participant.id]
		if !exists {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.relationship.unknown-participant", path,
				fmt.Sprintf("relationship participant %q is not visible in the effective scope of command %q", participant.id, commandKey),
				document,
			))
			continue
		}
		if participant.kind != input.kind {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.relationship.participant-type", path,
				fmt.Sprintf("relationship participant %q is declared as %s but references a %s", participant.id, participant.kind, input.kind),
				document,
			))
		}
	}
	return diagnostics
}

func cliRelationshipSelfDiagnostics(document string, relationship cliRelationshipRecord) []Diagnostic {
	seen := make(map[string]bool)
	var diagnostics []Diagnostic
	for _, participant := range relationship.participants {
		identity := cliRelationshipSemanticID(participant)
		if seen[identity] {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.relationship.duplicate-participant", instancePath(append(participant.path, "id")),
				fmt.Sprintf("relationship %q names input %q more than once", relationship.key, participant.id), document,
			))
		}
		seen[identity] = true
		if relationship.when != nil && cliRelationshipSemanticID(*relationship.when) == identity {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.relationship.self-reference", instancePath(append(participant.path, "id")),
				fmt.Sprintf("relationship %q cannot make input %q depend on itself", relationship.key, participant.id), document,
			))
		}
	}
	return diagnostics
}

func cliDuplicateRelationshipDiagnostics(document string, relationships []cliRelationshipRecord) []Diagnostic {
	owners := make(map[string][]cliRelationshipRecord)
	for _, relationship := range relationships {
		signature := relationship.kind + ":" + cliRelationshipDirectionSignature(relationship)
		owners[signature] = append(owners[signature], relationship)
	}
	var diagnostics []Diagnostic
	signatures := make([]string, 0, len(owners))
	for signature := range owners {
		signatures = append(signatures, signature)
	}
	sort.Strings(signatures)
	for _, signature := range signatures {
		duplicates := owners[signature]
		if len(duplicates) < 2 {
			continue
		}
		ids := make([]string, 0, len(duplicates))
		for _, duplicate := range duplicates {
			ids = append(ids, duplicate.key)
		}
		for _, duplicate := range duplicates {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.relationship.duplicate", instancePath(append(duplicate.path, "id")),
				fmt.Sprintf("relationship %q duplicates equivalent relationship set %s", duplicate.key, strings.Join(ids, ", ")), document,
			))
		}
	}
	return diagnostics
}

func cliRelationshipDirectionSignature(relationship cliRelationshipRecord) string {
	participants := make([]string, 0, len(relationship.participants))
	for _, participant := range relationship.participants {
		participants = append(participants, participant.kind+":"+cliRelationshipSemanticID(participant))
	}
	sort.Strings(participants)
	when := ""
	if relationship.when != nil {
		when = relationship.when.kind + ":" + cliRelationshipSemanticID(*relationship.when) + "->"
	}
	return when + strings.Join(participants, ",")
}

func cliContradictoryRelationshipDiagnostics(document string, relationships []cliRelationshipRecord) []Diagnostic {
	var diagnostics []Diagnostic
	for rightIndex := 1; rightIndex < len(relationships); rightIndex++ {
		right := relationships[rightIndex]
		for leftIndex := 0; leftIndex < rightIndex; leftIndex++ {
			left := relationships[leftIndex]
			if !cliRelationshipsContradict(left, right) {
				continue
			}
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.relationship.contradictory", instancePath(append(right.path, "id")),
				fmt.Sprintf("relationship %q contradicts relationship %q", right.key, left.key), document,
			))
		}
	}
	return diagnostics
}

func cliRelationshipsContradict(left, right cliRelationshipRecord) bool {
	if cliGroupRelationshipContradiction(left, right) {
		return true
	}
	for _, pair := range [][2]cliRelationshipRecord{{left, right}, {right, left}} {
		directed, exclusion := pair[0], pair[1]
		if !cliRelationshipIsDirected(directed.kind) || !cliRelationshipIsExclusion(exclusion.kind) || directed.when == nil {
			continue
		}
		excluded := cliRelationshipParticipantIDs(exclusion)
		for _, target := range directed.participants {
			if excluded[cliRelationshipSemanticID(*directed.when)] && excluded[cliRelationshipSemanticID(target)] {
				return true
			}
		}
	}
	return false
}

func cliGroupRelationshipContradiction(left, right cliRelationshipRecord) bool {
	if cliRelationshipDirectionSignature(left) != cliRelationshipDirectionSignature(right) {
		return false
	}
	return left.kind == "required-together" && cliRelationshipIsExclusion(right.kind) ||
		right.kind == "required-together" && cliRelationshipIsExclusion(left.kind)
}

func cliRelationshipIsExclusion(kind string) bool {
	return kind == "mutually-exclusive" || kind == "conflict"
}

func cliRelationshipIsDirected(kind string) bool {
	return kind == "dependency" || kind == "conditional"
}

func cliRelationshipParticipantIDs(relationship cliRelationshipRecord) map[string]bool {
	ids := make(map[string]bool, len(relationship.participants))
	for _, participant := range relationship.participants {
		ids[cliRelationshipSemanticID(participant)] = true
	}
	return ids
}

func cliRelationshipCycleDiagnostics(
	document string,
	relationships []cliRelationshipRecord,
	known map[string]cliRelationshipInput,
) []Diagnostic {
	var edges []cliRelationshipEdge
	adjacency := make(map[string][]string)
	for _, relationship := range relationships {
		if !cliRelationshipIsDirected(relationship.kind) || relationship.when == nil || !cliRelationshipParticipantKnown(*relationship.when, known) {
			continue
		}
		for _, participant := range relationship.participants {
			if cliRelationshipSemanticID(participant) == cliRelationshipSemanticID(*relationship.when) || !cliRelationshipParticipantKnown(participant, known) {
				continue
			}
			edge := cliRelationshipEdge{
				from: cliRelationshipSemanticID(*relationship.when),
				to:   cliRelationshipSemanticID(participant),
				path: participant.path,
			}
			edges = append(edges, edge)
			adjacency[edge.from] = append(adjacency[edge.from], edge.to)
		}
	}
	for from := range adjacency {
		sort.Strings(adjacency[from])
	}

	var diagnostics []Diagnostic
	for _, edge := range edges {
		if cliRelationshipReachable(edge.to, edge.from, adjacency, make(map[string]bool)) {
			diagnostics = append(diagnostics, newDiagnostic(
				"cli.relationship.cycle", instancePath(append(edge.path, "id")),
				fmt.Sprintf("dependency edge %q -> %q participates in a relationship cycle", edge.from, edge.to), document,
			))
		}
	}
	return diagnostics
}

func cliRelationshipParticipantKnown(participant cliRelationshipParticipant, known map[string]cliRelationshipInput) bool {
	input, exists := known[participant.id]
	return exists && input.kind == participant.kind
}

func cliRelationshipSemanticID(participant cliRelationshipParticipant) string {
	if participant.identity != "" {
		return participant.identity
	}
	return participant.id
}

func cliRelationshipReachable(current, target string, adjacency map[string][]string, visited map[string]bool) bool {
	if current == target {
		return true
	}
	if visited[current] {
		return false
	}
	visited[current] = true
	for _, next := range adjacency[current] {
		if cliRelationshipReachable(next, target, adjacency, visited) {
			return true
		}
	}
	return false
}
