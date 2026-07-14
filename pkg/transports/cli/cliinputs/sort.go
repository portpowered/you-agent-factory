package cliinputs

import (
	"sort"
	"strings"
)

// Inventory collections are sorted before Walk returns so committed baselines are
// reviewer-verifiable and byte-identical on repeat:
//
//   - arguments[]: commandPath, position, name, idCandidate
//   - flags[]:     commandPath, long, idCandidate
//   - relationships[]: commandPath, kind, participants (lexicographic join), idCandidate

func sortArgumentRecords(records []ArgumentRecord) {
	sort.Slice(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if left.CommandPath != right.CommandPath {
			return left.CommandPath < right.CommandPath
		}
		if left.Position != right.Position {
			return left.Position < right.Position
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.IDCandidate < right.IDCandidate
	})
}

func sortFlagRecords(records []FlagRecord) {
	sort.Slice(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if left.CommandPath != right.CommandPath {
			return left.CommandPath < right.CommandPath
		}
		if left.Long != right.Long {
			return left.Long < right.Long
		}
		return left.IDCandidate < right.IDCandidate
	})
}

func sortRelationshipRecords(records []RelationshipRecord) {
	sort.Slice(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if left.CommandPath != right.CommandPath {
			return left.CommandPath < right.CommandPath
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		leftParticipants := strings.Join(left.Participants, "\x00")
		rightParticipants := strings.Join(right.Participants, "\x00")
		if leftParticipants != rightParticipants {
			return leftParticipants < rightParticipants
		}
		return left.IDCandidate < right.IDCandidate
	})
}

func sortInventoryCollections(inv *Inventory) {
	sortArgumentRecords(inv.Arguments)
	sortFlagRecords(inv.Flags)
	sortRelationshipRecords(inv.Relationships)
}
