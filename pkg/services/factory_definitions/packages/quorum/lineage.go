package quorum

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/quorumpolicy"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	PackagedSplitWorkstationName = factorydefinitions.PackagedQuorumSplitWorkstationName
	PackagedMergeWorkstationName = factorydefinitions.PackagedQuorumMergeWorkstationName
)

func ApplyWorkRelations(
	output *workerexecution.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	inputs []workerexecution.Color,
) {
	if output == nil || workstation == nil {
		return
	}
	lineage := make([]factorydefinitions.QuorumLineageInput, len(inputs))
	for index, input := range inputs {
		lineage[index] = factorydefinitions.QuorumLineageInput{
			WorkID: input.WorkID, WorkTypeID: input.WorkTypeID,
		}
	}
	output.Color.Relations = quorumpolicy.WorkRelations(
		workstation.Name,
		output.Color.ParentID,
		output.Color.WorkTypeID,
		lineage,
	)
}
