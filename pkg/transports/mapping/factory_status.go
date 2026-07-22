package apisurface

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryStatusToAPI maps the detached Factory Runtime result onto its HTTP
// representation without applying categorization or resource policy.
func FactoryStatusToAPI(status factoryruntime.FactoryStatus) factoryapi.StatusResponse {
	categories := factoryapi.StatusCategories{
		Failed:     status.Categories.Failed,
		Initial:    status.Categories.Initial,
		Processing: status.Categories.Processing,
		Terminal:   status.Categories.Terminal,
	}
	response := factoryapi.StatusResponse{
		Categories:    categories,
		FactoryState:  status.FactoryState,
		RuntimeStatus: status.RuntimeStatus,
		TotalTokens:   status.TotalTokens,
	}
	if status.LifecycleControlStatus != "" {
		lifecycle := factoryapi.FactorySessionDurableLifecycleStatus(status.LifecycleControlStatus)
		response.LifecycleControlStatus = &lifecycle
	}
	if len(status.Resources) > 0 {
		resources := make([]factoryapi.ResourceUsage, 0, len(status.Resources))
		for _, resource := range status.Resources {
			resources = append(resources, factoryapi.ResourceUsage{
				Available: resource.Available,
				Name:      resource.Name,
				Total:     resource.Total,
			})
		}
		response.Resources = &resources
	}
	return response
}
