// Package quorumpolicy implements packaged Quorum identity and lineage policy
// owned by nested invocation_policy.
package quorumpolicy

import (
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service implements the fixed packaged Quorum identity and lineage policy.
type Service struct{}

var _ factoryeffects.QuorumPolicyService = Service{}

// NewService returns the canonical packaged Quorum policy.
func NewService() factoryeffects.QuorumPolicyService {
	return Service{}
}

func (Service) IsPackagedQuorumFactory(cfg *factorydefinitions.FactoryConfig) bool {
	return IsPackagedQuorumFactory(cfg)
}

func (Service) WorkRelations(
	workstationName string,
	outputParentID string,
	outputWorkTypeID string,
	inputs []factorydefinitions.QuorumLineageInput,
) []work.Relation {
	return WorkRelations(workstationName, outputParentID, outputWorkTypeID, inputs)
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
	inputs []factorydefinitions.QuorumLineageInput,
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

func dependenciesForBranchInputs(inputs []factorydefinitions.QuorumLineageInput) []work.Relation {
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
