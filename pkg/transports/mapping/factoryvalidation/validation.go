// Package factoryvalidation maps public Factory validation payloads onto the
// Factory Definitions-owned validation operation.
package factoryvalidation

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// ValidateFactoryAPI maps one public validation request and invokes the exact
// Factory Definitions validation operation. The operation owns its fixed
// topology profile; transport callers cannot select validation phases.
func ValidateFactoryAPI(
	ctx context.Context,
	factory factoryapi.Factory,
	operation factorydefinitions.SubmittedDefinitionValidationOperation,
) (factorydefinitions.ValidationResult, error) {
	if operation == nil {
		return factorydefinitions.ValidationResult{}, fmt.Errorf("Factory Definition validation operation is required")
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		return factorydefinitions.ValidationResult{}, err
	}
	return operation.ValidateSubmittedDefinition(ctx, factorydefinitions.SubmittedDefinitionValidationRequest{
		Config:   &cfg,
		Taxonomy: submittedDefinitionTaxonomyFromAPI(factory),
	})
}

func submittedDefinitionTaxonomyFromAPI(factory factoryapi.Factory) factorydefinitions.SubmittedDefinitionTaxonomy {
	var taxonomy factorydefinitions.SubmittedDefinitionTaxonomy
	if factory.Workers != nil {
		taxonomy.Workers = make([]factorydefinitions.SubmittedWorkerTaxonomy, 0, len(*factory.Workers))
		for _, worker := range *factory.Workers {
			workerType := ""
			if worker.Type != nil {
				workerType = string(*worker.Type)
			}
			taxonomy.Workers = append(taxonomy.Workers, factorydefinitions.SubmittedWorkerTaxonomy{
				Name: worker.Name, Type: workerType,
			})
		}
	}
	if factory.Workstations != nil {
		taxonomy.Workstations = make([]factorydefinitions.SubmittedWorkstationTaxonomy, 0, len(*factory.Workstations))
		for index, workstation := range *factory.Workstations {
			workstationType := ""
			if workstation.Type != nil {
				workstationType = string(*workstation.Type)
			}
			behavior := factorydefinitions.WorkstationKind("")
			if workstation.Behavior != nil {
				behavior = factorydefinitions.WorkstationKind(*workstation.Behavior)
			}
			taxonomy.Workstations = append(taxonomy.Workstations, factorydefinitions.SubmittedWorkstationTaxonomy{
				Name: workstation.Name, Type: workstationType, Behavior: behavior,
				Worker: optionalString(workstation.Worker), Index: index,
			})
		}
	}
	return taxonomy
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
