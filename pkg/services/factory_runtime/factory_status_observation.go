package factory

// FactoryStatusFromObservation maps a detached orchestration-neutral observation
// into the Factory Runtime status read model. Peers on the focused observation
// path consume this helper instead of projecting Petri markings directly.
func FactoryStatusFromObservation(observation Observation) FactoryStatus {
	resources := make([]FactoryResourceUsage, 0, len(observation.Resources))
	for _, resource := range observation.Resources {
		resources = append(resources, FactoryResourceUsage{
			Available: resource.AvailableCount,
			Name:      resource.ResourceID,
			Total:     resource.AvailableCount + resource.InUseCount,
		})
	}
	return FactoryStatus{
		Categories: FactoryStatusCategories{
			Failed:     observation.Progress.WorkCategories.Failed,
			Initial:    observation.Progress.WorkCategories.Initial,
			Processing: observation.Progress.WorkCategories.Processing,
			Terminal:   observation.Progress.WorkCategories.Terminal,
		},
		FactoryState:           observation.Health.FactoryState,
		LifecycleControlStatus: observation.Health.LifecycleControlStatus,
		Resources:              resources,
		RuntimeStatus:          string(observation.Status),
		TotalTokens:            observation.Progress.TotalWorkCount,
	}
}
