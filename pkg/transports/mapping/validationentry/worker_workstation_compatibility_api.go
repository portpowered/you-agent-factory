package validationentry

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// submittedDefinitionTaxonomyFromAPI copies the exact public taxonomy into a
// detached Factory Definitions request. It deliberately makes no compatibility
// decision and constructs no validation target.
func submittedDefinitionTaxonomyFromAPI(factory factoryapi.Factory) interfaces.SubmittedDefinitionTaxonomy {
	var taxonomy interfaces.SubmittedDefinitionTaxonomy
	if factory.Workers != nil {
		taxonomy.Workers = make([]interfaces.SubmittedWorkerTaxonomy, 0, len(*factory.Workers))
		for _, worker := range *factory.Workers {
			workerType := ""
			if worker.Type != nil {
				workerType = string(*worker.Type)
			}
			taxonomy.Workers = append(taxonomy.Workers, interfaces.SubmittedWorkerTaxonomy{
				Name: worker.Name, Type: workerType,
			})
		}
	}
	if factory.Workstations != nil {
		taxonomy.Workstations = make([]interfaces.SubmittedWorkstationTaxonomy, 0, len(*factory.Workstations))
		for index, workstation := range *factory.Workstations {
			workstationType := ""
			if workstation.Type != nil {
				workstationType = string(*workstation.Type)
			}
			behavior := interfaces.WorkstationKind("")
			if workstation.Behavior != nil {
				behavior = interfaces.WorkstationKind(*workstation.Behavior)
			}
			taxonomy.Workstations = append(taxonomy.Workstations, interfaces.SubmittedWorkstationTaxonomy{
				Name: workstation.Name, Type: workstationType, Behavior: behavior,
				Worker: workstation.Worker, Index: index,
			})
		}
	}
	return taxonomy
}
