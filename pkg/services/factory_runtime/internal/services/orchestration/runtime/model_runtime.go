package runtime

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// modelRuntimeInputForSelection projects the session-owned Models scope into
// the detached Workers request selected by Factory Runtime. Runtime does not
// invoke Models or choose a backend here; it only carries the opened scope and
// authored worker/resource facts to Workers, whose inference runner owns the
// local-vs-provider decision.
func modelRuntimeInputForSelection(
	cfg *runtimeConfig,
	selection runtimeExecutionSelection,
) *workers.ModelRuntimeInput {
	if cfg == nil || cfg.modelRuntimeScope.IsZero() ||
		!strings.EqualFold(
			strings.TrimSpace(selection.modelLocality),
			modelprovider.RuntimeModelLocalityLocal,
		) || strings.TrimSpace(selection.model) == "" {
		return nil
	}

	worker := modelprovider.LocalWorker{
		Name:          strings.TrimSpace(selection.workerName),
		Type:          strings.TrimSpace(selection.workerType),
		Model:         strings.TrimSpace(selection.model),
		ModelLocality: strings.TrimSpace(selection.modelLocality),
	}
	var resources []modelprovider.LocalResource
	if lookup, ok := runtimeDefinitionLookup(cfg); ok {
		if definition, found := lookup.Worker(worker.Name); found && definition != nil {
			worker.Resources = localResourcesFromFactory(definition.Resources)
		}
		if factoryLookup, found := lookup.(interfaces.RuntimeFactoryConfigLookup); found {
			if factoryConfig := factoryLookup.FactoryConfig(); factoryConfig != nil {
				resources = localResourcesFromFactory(factoryConfig.Resources)
			}
		}
	}

	return &workers.ModelRuntimeInput{
		Scope:     cfg.modelRuntimeScope,
		Worker:    worker,
		Resources: resources,
	}
}

func localResourcesFromFactory(
	resources []interfaces.ResourceConfig,
) []modelprovider.LocalResource {
	if len(resources) == 0 {
		return nil
	}
	projected := make([]modelprovider.LocalResource, len(resources))
	for index, resource := range resources {
		projected[index] = modelprovider.LocalResource{
			ID:         resource.ID,
			Name:       resource.Name,
			Type:       resource.Type,
			Capacity:   resource.Capacity,
			Model:      resource.Model,
			Backend:    resource.Backend,
			LoadPolicy: resource.LoadPolicy,
			Provider:   resource.Provider,
		}
	}
	return projected
}
