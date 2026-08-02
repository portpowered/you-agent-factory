// Package quorumpolicy implements packaged Quorum identity and lineage policy
// owned by nested invocation_policy.
package quorumpolicy

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// LineageInput is the private Work identity used by the packaged Quorum
// implementation. It is not a Definitions root contract.
type LineageInput struct {
	WorkID     string
	WorkTypeID string
}

func IsPackagedQuorumFactory(cfg *factorydefinitions.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.Name) == factorydefinitions.PackagedQuorumFactoryName ||
		strings.TrimSpace(cfg.Project) == factorydefinitions.PackagedQuorumFactoryProject
}

func WorkRelations(
	workstationName string,
	outputParentID string,
	outputWorkTypeID string,
	inputs []LineageInput,
) []work.Relation {
	switch workstationName {
	case factorydefinitions.PackagedQuorumSplitWorkstationName:
		if outputParentID == "" || outputWorkTypeID == "task" {
			return nil
		}
		return []work.Relation{{
			Type:         work.RelationParentChild,
			TargetWorkID: outputParentID,
		}}
	case factorydefinitions.PackagedQuorumMergeWorkstationName:
		if outputWorkTypeID != "quorum-merge" {
			return nil
		}
		return dependenciesForBranchInputs(inputs)
	default:
		return nil
	}
}

func dependenciesForBranchInputs(inputs []LineageInput) []work.Relation {
	relations := make([]work.Relation, 0, 2)
	for _, input := range inputs {
		if input.WorkID == "" {
			continue
		}
		switch input.WorkTypeID {
		case "quorum-branch-a", "quorum-branch-b":
			relations = append(relations, work.Relation{
				Type:          work.RelationDependsOn,
				TargetWorkID:  input.WorkID,
				RequiredState: "complete",
			})
		}
	}
	return relations
}
