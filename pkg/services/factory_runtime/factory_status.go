package factory

import (
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// FactoryStatus is the detached Factory Runtime status read model. It contains
// no marking, topology, token, or implementation references.
type FactoryStatus struct {
	Categories             FactoryStatusCategories
	FactoryState           string
	LifecycleControlStatus string
	Resources              []FactoryResourceUsage
	RuntimeStatus          string
	TotalTokens            int
}

// FactoryStatusCategories counts public Work by runtime state category.
type FactoryStatusCategories struct {
	Failed     int
	Initial    int
	Processing int
	Terminal   int
}

// FactoryResourceUsage is the detached availability projection for one
// runtime resource.
type FactoryResourceUsage struct {
	Available int
	Name      string
	Total     int
}

// FactoryStatusProjector owns the exact status projection operation injected
// into consumers. Transport packages receive this role through composition and
// never categorize runtime tokens themselves.
type FactoryStatusProjector interface {
	ProjectFactoryStatus(*StateSnapshot) FactoryStatus
	ProjectFactoryStatusFromObservation(Observation) FactoryStatus
}

type factoryStatusProjector struct{}

// NewFactoryStatusProjector constructs the stateless Factory Runtime status
// projection operation. Application composition owns this constructor.
func NewFactoryStatusProjector() FactoryStatusProjector {
	return factoryStatusProjector{}
}

func (factoryStatusProjector) ProjectFactoryStatus(snapshot *StateSnapshot) FactoryStatus {
	if snapshot == nil {
		return FactoryStatus{}
	}

	categories, resources := projectFactoryStatusTokens(&snapshot.Marking, snapshot.Topology)
	return FactoryStatus{
		Categories:             categories,
		FactoryState:           snapshot.FactoryState,
		LifecycleControlStatus: strings.TrimSpace(snapshot.LifecycleControlStatus),
		Resources:              resources,
		RuntimeStatus:          string(snapshot.RuntimeStatus),
		TotalTokens:            countFactoryStatusTokens(&snapshot.Marking),
	}
}

func (factoryStatusProjector) ProjectFactoryStatusFromObservation(observation Observation) FactoryStatus {
	return FactoryStatusFromObservation(observation)
}

func projectFactoryStatusTokens(marking *PetriMarkingSnapshot, net *Net) (FactoryStatusCategories, []FactoryResourceUsage) {
	var categories FactoryStatusCategories
	resourceCounts := make(map[string]int)
	resourceTotals := factoryStatusResourceTotals(net)

	if marking == nil {
		return categories, factoryStatusResourceUsage(resourceCounts, resourceTotals)
	}

	for _, token := range marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		if token.Color.DataType == RuntimeTokenDataTypeResource {
			resourceID, resourceState := SplitPlaceID(token.PlaceID)
			if _, exists := resourceTotals[resourceID]; !exists {
				resourceTotals[resourceID]++
			}
			if resourceState == interfaces.ResourceStateAvailable {
				resourceCounts[resourceID]++
			}
			continue
		}

		category := StateCategoryProcessing
		if net != nil {
			category = net.StateCategoryForPlace(token.PlaceID)
		}
		switch category {
		case StateCategoryFailed:
			categories.Failed++
		case StateCategoryTerminal:
			categories.Terminal++
		case StateCategoryInitial:
			categories.Initial++
		default:
			categories.Processing++
		}
	}

	return categories, factoryStatusResourceUsage(resourceCounts, resourceTotals)
}

func countFactoryStatusTokens(marking *PetriMarkingSnapshot) int {
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

func factoryStatusResourceTotals(net *Net) map[string]int {
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

func factoryStatusResourceUsage(counts, totals map[string]int) []FactoryResourceUsage {
	ids := make([]string, 0, len(totals))
	for id := range totals {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	resources := make([]FactoryResourceUsage, 0, len(ids))
	for _, id := range ids {
		resources = append(resources, FactoryResourceUsage{
			Available: counts[id],
			Name:      id,
			Total:     totals[id],
		})
	}
	return resources
}
