package factorystatus

import (
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/legacysnapshot"
)

// ProjectFromSnapshot maps a migration-only engine snapshot into the detached
// Factory Runtime status read model. Peers on the published observation path
// should use factory.FactoryStatusFromObservation instead.
func ProjectFromSnapshot(snapshot *legacysnapshot.Snapshot) factory.FactoryStatus {
	if snapshot == nil {
		return factory.FactoryStatus{}
	}

	categories, resources := projectFactoryStatusTokens(&snapshot.Marking, snapshot.Topology)
	return factory.FactoryStatus{
		Categories:             categories,
		FactoryState:           snapshot.FactoryState,
		LifecycleControlStatus: strings.TrimSpace(snapshot.LifecycleControlStatus),
		Resources:              resources,
		RuntimeStatus:          string(snapshot.RuntimeStatus),
		TotalTokens:            countFactoryStatusTokens(&snapshot.Marking),
	}
}

func projectFactoryStatusTokens(marking *factory.PetriMarkingSnapshot, net *factory.Net) (factory.FactoryStatusCategories, []factory.FactoryResourceUsage) {
	var categories factory.FactoryStatusCategories
	resourceCounts := make(map[string]int)
	resourceTotals := factoryStatusResourceTotals(net)

	if marking == nil {
		return categories, factoryStatusResourceUsage(resourceCounts, resourceTotals)
	}

	for _, token := range marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		if token.Color.DataType == factory.RuntimeTokenDataTypeResource {
			resourceID, resourceState := factory.SplitPlaceID(token.PlaceID)
			if _, exists := resourceTotals[resourceID]; !exists {
				resourceTotals[resourceID]++
			}
			if resourceState == interfaces.ResourceStateAvailable {
				resourceCounts[resourceID]++
			}
			continue
		}

		category := factory.StateCategoryProcessing
		if net != nil {
			category = net.StateCategoryForPlace(token.PlaceID)
		}
		switch category {
		case factory.StateCategoryFailed:
			categories.Failed++
		case factory.StateCategoryTerminal:
			categories.Terminal++
		case factory.StateCategoryInitial:
			categories.Initial++
		default:
			categories.Processing++
		}
	}

	return categories, factoryStatusResourceUsage(resourceCounts, resourceTotals)
}

func countFactoryStatusTokens(marking *factory.PetriMarkingSnapshot) int {
	if marking == nil {
		return 0
	}
	count := 0
	for _, token := range marking.Tokens {
		if token != nil && !interfaces.IsSystemTimeToken(token) {
			count++
		}
	}
	return count
}

func factoryStatusResourceTotals(net *factory.Net) map[string]int {
	totals := make(map[string]int)
	if net == nil {
		return totals
	}
	for id, resource := range net.Resources {
		if resource != nil {
			totals[id] = resource.Capacity
		}
	}
	return totals
}

func factoryStatusResourceUsage(counts, totals map[string]int) []factory.FactoryResourceUsage {
	ids := make([]string, 0, len(totals))
	for id := range totals {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	resources := make([]factory.FactoryResourceUsage, 0, len(ids))
	for _, id := range ids {
		resources = append(resources, factory.FactoryResourceUsage{
			Available: counts[id],
			Name:      id,
			Total:     totals[id],
		})
	}
	return resources
}
