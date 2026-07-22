package factorysession

import (
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sessionBudgetsToAPI(budgets *factorysessionexecution.SessionBudgets) *factoryapi.FactorySessionBudgets {
	if budgets == nil || budgets.MaxAgents <= 0 {
		return nil
	}
	value := budgets.MaxAgents
	return &factoryapi.FactorySessionBudgets{MaxAgents: &value}
}

func sessionBudgetsFromAPI(budgets *factoryapi.FactorySessionBudgets) *factorysessionexecution.SessionBudgets {
	if budgets == nil || budgets.MaxAgents == nil || *budgets.MaxAgents <= 0 {
		return nil
	}
	return &factorysessionexecution.SessionBudgets{MaxAgents: *budgets.MaxAgents}
}

func sessionUsageToAPI(usage factorysessionexecution.SessionUsage) factoryapi.FactorySessionUsage {
	resources := usage.Resources
	if resources == nil {
		resources = []factorysessionexecution.ResourceUsage{}
	}
	out := make([]factoryapi.ResourceUsage, 0, len(resources))
	for _, resource := range resources {
		out = append(out, factoryapi.ResourceUsage{
			Name:      resource.Name,
			Available: resource.Available,
			Total:     resource.Total,
		})
	}
	return factoryapi.FactorySessionUsage{Resources: out}
}

func sessionUsageFromAPI(usage *factoryapi.FactorySessionUsage) factorysessionexecution.SessionUsage {
	if usage == nil || len(usage.Resources) == 0 {
		return factorysessionexecution.EmptySessionUsage()
	}
	out := factorysessionexecution.SessionUsage{Resources: make([]factorysessionexecution.ResourceUsage, 0, len(usage.Resources))}
	for _, resource := range usage.Resources {
		out.Resources = append(out.Resources, factorysessionexecution.ResourceUsage{
			Name:      resource.Name,
			Available: resource.Available,
			Total:     resource.Total,
		})
	}
	return out
}

func providerSessionRefsToAPI(refs []factorysessionexecution.ProviderSessionRef) *[]factoryapi.LoadableProviderSessionRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]factoryapi.LoadableProviderSessionRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, factoryapi.LoadableProviderSessionRef{
			Provider: factoryapi.LoadableProviderSessionProvider(ref.Provider),
			Kind:     factoryapi.LoadableProviderSessionKind(ref.Kind),
			Id:       ref.ID,
		})
	}
	return &out
}

func providerSessionRefsFromAPI(refs *[]factoryapi.LoadableProviderSessionRef) []factorysessionexecution.ProviderSessionRef {
	if refs == nil || len(*refs) == 0 {
		return nil
	}
	out := make([]factorysessionexecution.ProviderSessionRef, 0, len(*refs))
	for _, ref := range *refs {
		out = append(out, factorysessionexecution.ProviderSessionRef{
			Provider: string(ref.Provider),
			Kind:     string(ref.Kind),
			ID:       ref.Id,
		})
	}
	return out
}
