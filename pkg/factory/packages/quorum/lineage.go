package quorum

import "github.com/portpowered/infinite-you/pkg/interfaces"

const (
	// PackagedSplitWorkstationName is the logical fan-out boundary for quorum Work.
	PackagedSplitWorkstationName = "split-quorum"
	// PackagedMergeWorkstationName is the gated fan-in boundary for quorum Work.
	PackagedMergeWorkstationName = "merge-quorum"
)

// ApplyWorkRelations records the public lineage edges introduced by the fixed
// quorum topology. The split creates two children of the request; the merge
// result depends on both branch results in its authored input order.
func ApplyWorkRelations(output *interfaces.Token, workstation *interfaces.FactoryWorkstationConfig, inputs []interfaces.TokenColor) {
	if output == nil || workstation == nil {
		return
	}
	switch workstation.Name {
	case PackagedSplitWorkstationName:
		if output.Color.ParentID != "" && output.Color.WorkTypeID != "task" {
			output.Color.Relations = []interfaces.Relation{{
				Type:         interfaces.RelationParentChild,
				TargetWorkID: output.Color.ParentID,
			}}
		}
	case PackagedMergeWorkstationName:
		if output.Color.WorkTypeID != "quorum-merge" {
			return
		}
		output.Color.Relations = dependenciesForBranchInputs(inputs)
	}
}

func dependenciesForBranchInputs(inputs []interfaces.TokenColor) []interfaces.Relation {
	relations := make([]interfaces.Relation, 0, 2)
	for _, input := range inputs {
		if input.WorkID == "" {
			continue
		}
		switch input.WorkTypeID {
		case "quorum-branch-a", "quorum-branch-b":
			relations = append(relations, interfaces.Relation{
				Type:          interfaces.RelationDependsOn,
				TargetWorkID:  input.WorkID,
				RequiredState: "complete",
			})
		}
	}
	return relations
}
