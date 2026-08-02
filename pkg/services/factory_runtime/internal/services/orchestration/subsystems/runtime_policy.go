package subsystems

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// workstationPropagationMode is Runtime-owned routing policy. Definitions
// resolves and detaches authored invocation facts; Runtime applies the
// propagation mode while it creates the next marking.
func workstationPropagationMode(workstation *factorydefinitions.FactoryWorkstationConfig) factorydefinitions.WorkPropagationMode {
	if workstation == nil || workstation.WorkPropagation == nil {
		return factorydefinitions.WorkPropagationModeOutputAsPayload
	}
	mode := strings.TrimSpace(string(workstation.WorkPropagation.Mode))
	if mode == "" {
		return factorydefinitions.WorkPropagationModeOutputAsPayload
	}
	return factorydefinitions.WorkPropagationMode(mode)
}

func usesDecisionEnvelopeOutcome(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return workstation != nil && strings.TrimSpace(workstation.OutcomeFormat) == factorydefinitions.WorkstationOutcomeFormatDecisionEnvelope
}

func usesGoalRoutingDecisionEnvelope(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return usesDecisionEnvelopeOutcome(workstation) && len(workstation.ClassificationRoutes) > 0
}

type quorumLineageInput struct {
	WorkID     string
	WorkTypeID string
}

func isPackagedQuorumFactory(cfg *factorydefinitions.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.Name) == factorydefinitions.PackagedQuorumFactoryName ||
		strings.TrimSpace(cfg.Project) == factorydefinitions.PackagedQuorumFactoryProject
}

func packagedQuorumRelations(
	workstationName string,
	outputParentID string,
	outputWorkTypeID string,
	inputs []quorumLineageInput,
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
		return packagedQuorumBranchDependencies(inputs)
	default:
		return nil
	}
}

func packagedQuorumBranchDependencies(inputs []quorumLineageInput) []work.Relation {
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
