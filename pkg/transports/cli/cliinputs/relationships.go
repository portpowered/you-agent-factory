package cliinputs

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Cobra stores flag-group relationships as per-flag annotations; mirror the
// keys from github.com/spf13/cobra/flag_groups.go for read-only inventory.
const (
	cobraRequiredAsGroupAnnotation   = "cobra_annotation_required_if_others_set"
	cobraOneRequiredAnnotation       = "cobra_annotation_one_required"
	cobraMutuallyExclusiveAnnotation = "cobra_annotation_mutually_exclusive"
)

type flagGroupSpec struct {
	annotation string
	kind       string
}

var flagGroupSpecs = []flagGroupSpec{
	{annotation: cobraMutuallyExclusiveAnnotation, kind: relationshipKindMutuallyExclusive},
	{annotation: cobraRequiredAsGroupAnnotation, kind: relationshipKindRequiredTogether},
	{annotation: cobraOneRequiredAnnotation, kind: relationshipKindAtLeastOne},
}

func collectRelationshipRecords(root *cobra.Command) []RelationshipRecord {
	records := []RelationshipRecord{}
	collectCommandRelationships(root, &records)
	return records
}

func collectCommandRelationships(cmd *cobra.Command, records *[]RelationshipRecord) {
	*records = append(*records, relationshipRecordsForCommand(cmd)...)

	children := append([]*cobra.Command(nil), cmd.Commands()...)
	sort.Slice(children, func(i, j int) bool {
		return children[i].CommandPath() < children[j].CommandPath()
	})
	for _, child := range children {
		collectCommandRelationships(child, records)
	}
}

func relationshipRecordsForCommand(cmd *cobra.Command) []RelationshipRecord {
	flags := cmd.Flags()
	if flags == nil {
		return nil
	}

	join := commandJoin(cmd)
	records := make([]RelationshipRecord, 0)
	for _, spec := range flagGroupSpecs {
		for _, participants := range collectFlagGroups(flags, spec.annotation) {
			records = append(records, RelationshipRecord{
				CommandJoin:  join,
				IDCandidate:  relationshipIDCandidate(join.CommandIDCandidate, spec.kind, participants),
				Kind:         spec.kind,
				Participants: participants,
			})
		}
	}
	return records
}

func collectFlagGroups(flags *pflag.FlagSet, annotation string) [][]string {
	seen := map[string]bool{}
	groups := [][]string{}

	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Annotations == nil {
			return
		}
		for _, groupKey := range flag.Annotations[annotation] {
			if seen[groupKey] {
				continue
			}
			participants := strings.Fields(groupKey)
			if !flagGroupParticipantsDefined(flags, participants) {
				continue
			}
			sort.Strings(participants)
			seen[groupKey] = true
			groups = append(groups, participants)
		}
	})

	sort.Slice(groups, func(i, j int) bool {
		return strings.Join(groups[i], "\x00") < strings.Join(groups[j], "\x00")
	})
	return groups
}

func flagGroupParticipantsDefined(flags *pflag.FlagSet, participants []string) bool {
	for _, name := range participants {
		if flags.Lookup(name) == nil {
			return false
		}
	}
	return len(participants) > 0
}

func relationshipIDCandidate(commandID, kind string, participants []string) string {
	return fmt.Sprintf("%s.rel.%s.%s", commandID, relationshipKindToken(kind), strings.Join(participants, "-"))
}

func relationshipKindToken(kind string) string {
	switch kind {
	case relationshipKindMutuallyExclusive:
		return "mutex"
	default:
		return kind
	}
}
