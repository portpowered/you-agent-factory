package rootobservation

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/factorystatus"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/legacysnapshot"
)

// Project maps a legacy engine snapshot into the published plain observation
// vocabulary. Marking, topology, tokens, and enabled transitions are
// intentionally omitted from the peer-facing result. This helper lives under
// factory_runtime/internal so raw engine-snapshot types stay off the public
// Runtime package surface enforced by pkg-boundary.
func Project(
	snap *legacysnapshot.Snapshot,
	scope factory.ObservationScope,
) factory.Observation {
	if snap == nil {
		return factory.Observation{}
	}

	status := factorystatus.ProjectFromSnapshot(snap)
	full := factory.Observation{
		Status: observationStatusFromRuntime(snap.RuntimeStatus),
		Progress: factory.ObservationProgress{
			InFlightDispatchCount: snap.InFlightCount,
			TickCount:             snap.TickCount,
			TotalWorkCount:        status.TotalTokens,
			WorkCategories: factory.ObservationWorkCategories{
				Failed:     status.Categories.Failed,
				Initial:    status.Categories.Initial,
				Processing: status.Categories.Processing,
				Terminal:   status.Categories.Terminal,
			},
		},
		InFlightDispatches: projectInFlightDispatches(snap.Dispatches),
		Results:            projectResultViews(snap.DispatchHistory),
		Resources:          projectResourceViews(snap, status.Resources),
		Health: factory.ObservationHealth{
			FactoryState:           snap.FactoryState,
			LifecycleControlStatus: snap.LifecycleControlStatus,
			StreamGenerationID:     snap.StreamGenerationID,
			Uptime:                 snap.Uptime,
			ActiveThrottlePauses: append(
				[]interfaces.ActiveThrottlePause(nil),
				snap.ActiveThrottlePauses...,
			),
		},
	}
	return filterObservationScope(full, scope)
}

func projectResourceViews(
	snap *legacysnapshot.Snapshot,
	resources []factory.FactoryResourceUsage,
) []factory.ObservationResourceView {
	if len(resources) == 0 {
		return nil
	}
	out := make([]factory.ObservationResourceView, 0, len(resources))
	for _, resource := range resources {
		name := resource.Name
		if snap.Topology != nil {
			if definition := snap.Topology.Resources[resource.Name]; definition != nil && definition.Name != "" {
				name = definition.Name
			}
		}
		out = append(out, factory.ObservationResourceView{
			ResourceID:     resource.Name,
			ResourceName:   name,
			ResourceType:   "RUNTIME",
			InUseCount:     resource.Total - resource.Available,
			AvailableCount: resource.Available,
		})
	}
	return out
}

func observationStatusFromRuntime(status interfaces.RuntimeStatus) factory.ObservationStatus {
	switch status {
	case interfaces.RuntimeStatusActive:
		return factory.ObservationStatusActive
	case interfaces.RuntimeStatusIdle:
		return factory.ObservationStatusIdle
	case interfaces.RuntimeStatusFinished:
		return factory.ObservationStatusFinished
	default:
		return factory.ObservationStatusIdle
	}
}

func projectInFlightDispatches(
	dispatches map[string]*interfaces.DispatchEntry,
) []factory.ObservationDispatchSummary {
	if len(dispatches) == 0 {
		return nil
	}
	out := make([]factory.ObservationDispatchSummary, 0, len(dispatches))
	for _, entry := range dispatches {
		if entry == nil {
			continue
		}
		workIDs := make([]string, 0, len(entry.ConsumedTokens))
		for _, tok := range entry.ConsumedTokens {
			if workID := tok.Color.WorkID; workID != "" {
				workIDs = append(workIDs, workID)
			}
		}
		out = append(out, factory.ObservationDispatchSummary{
			DispatchID:      entry.DispatchID,
			WorkIDs:         workIDs,
			WorkstationName: entry.WorkstationName,
			Status:          "IN_FLIGHT",
		})
	}
	return out
}

func projectResultViews(history []interfaces.CompletedDispatch) []factory.ObservationResultView {
	if len(history) == 0 {
		return nil
	}
	out := make([]factory.ObservationResultView, 0, len(history))
	for _, completed := range history {
		workID := ""
		if len(completed.ConsumedTokens) > 0 {
			workID = completed.ConsumedTokens[0].Color.WorkID
		}
		out = append(out, factory.ObservationResultView{
			DispatchID: completed.DispatchID,
			WorkID:     workID,
			Outcome:    string(completed.Outcome),
		})
	}
	return out
}

func filterObservationScope(full factory.Observation, scope factory.ObservationScope) factory.Observation {
	switch scope {
	case "", factory.ObservationScopeFull:
		return full
	case factory.ObservationScopeStatus:
		return factory.Observation{Status: full.Status}
	case factory.ObservationScopeProgress:
		return factory.Observation{Progress: full.Progress}
	case factory.ObservationScopeDispatches:
		return factory.Observation{InFlightDispatches: full.InFlightDispatches}
	case factory.ObservationScopeResults:
		return factory.Observation{Results: full.Results}
	case factory.ObservationScopeResources:
		return factory.Observation{Resources: full.Resources}
	case factory.ObservationScopeHealth:
		return factory.Observation{Health: full.Health}
	default:
		return full
	}
}
