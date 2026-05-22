package factory

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

func generatedFactory(payload interfaces.InitialStructurePayload) factoryapi.Factory {
	resources := generatedResources(payload.Resources)
	workTypes := generatedWorkTypes(payload.WorkTypes)
	workers := generatedWorkers(payload.Workers)
	workstations := generatedWorkstations(payload.Workstations, payload.Places)

	return factoryapi.Factory{
		Name:         generatedFactoryName(payload.Name),
		Resources:    slicePtr(resources),
		WorkTypes:    slicePtr(workTypes),
		Workers:      slicePtr(workers),
		Workstations: slicePtr(workstations),
	}
}

func generatedFactoryName(name string) factoryapi.FactoryName {
	if strings.TrimSpace(name) == "" {
		return "factory"
	}
	return factoryapi.FactoryName(name)
}

func generatedResources(resources []interfaces.FactoryResource) []factoryapi.Resource {
	out := make([]factoryapi.Resource, 0, len(resources))
	for _, resource := range resources {
		name := resource.Name
		if name == "" {
			name = resource.ID
		}
		out = append(out, factoryapi.Resource{Name: name, Capacity: resource.Capacity})
	}
	return out
}

func generatedWorkTypes(workTypes []interfaces.FactoryWorkType) []factoryapi.WorkType {
	out := make([]factoryapi.WorkType, 0, len(workTypes))
	for _, workType := range workTypes {
		name := workType.Name
		if name == "" {
			name = workType.ID
		}
		states := make([]factoryapi.WorkState, 0, len(workType.States))
		for _, stateDef := range workType.States {
			states = append(states, factoryapi.WorkState{
				Name: stateDef.Value,
				Type: generatedWorkStateType(stateDef.Category),
			})
		}
		out = append(out, factoryapi.WorkType{Name: name, States: states})
	}
	return out
}

func generatedWorkStateType(category string) factoryapi.WorkStateType {
	switch state.StateCategory(category) {
	case state.StateCategoryInitial:
		return factoryapi.WorkStateTypeINITIAL
	case state.StateCategoryTerminal:
		return factoryapi.WorkStateTypeTERMINAL
	case state.StateCategoryFailed:
		return factoryapi.WorkStateTypeFAILED
	default:
		return factoryapi.WorkStateTypePROCESSING
	}
}

func generatedWorkStatePtr(name string) *factoryapi.WorkState {
	if name == "" {
		return nil
	}
	return &factoryapi.WorkState{Name: name, Type: inferredGeneratedWorkStateType(name)}
}

func inferredGeneratedWorkStateType(name string) factoryapi.WorkStateType {
	switch name {
	case "init":
		return factoryapi.WorkStateTypeINITIAL
	case "complete", "done":
		return factoryapi.WorkStateTypeTERMINAL
	case "failed":
		return factoryapi.WorkStateTypeFAILED
	default:
		return factoryapi.WorkStateTypePROCESSING
	}
}

func generatedWorkers(workers []interfaces.FactoryWorker) []factoryapi.Worker {
	out := make([]factoryapi.Worker, 0, len(workers))
	for _, worker := range workers {
		name := worker.Name
		if name == "" {
			name = worker.ID
		}
		out = append(out, factoryapi.Worker{
			Name:             name,
			ExecutorProvider: interfaces.GeneratedPublicFactoryWorkerProviderPtr(worker.Provider),
			ModelProvider:    interfaces.GeneratedPublicFactoryWorkerModelProviderPtr(worker.ModelProvider),
			Model:            stringPtrIfNotEmpty(worker.Model),
			Type:             interfaces.GeneratedPublicFactoryWorkerTypePtr(worker.Config["type"]),
		})
	}
	return out
}

func generatedWorkstations(workstations []interfaces.FactoryWorkstation, places []interfaces.FactoryPlace) []factoryapi.Workstation {
	placesByID := make(map[string]interfaces.FactoryPlace, len(places))
	for _, place := range places {
		placesByID[place.ID] = place
	}
	out := make([]factoryapi.Workstation, 0, len(workstations))
	for _, workstation := range workstations {
		name := workstation.Name
		if name == "" {
			name = workstation.ID
		}
		converted := factoryapi.Workstation{
			Id:          stringPtrIfNotEmpty(workstation.ID),
			Name:        name,
			Worker:      workstation.WorkerID,
			Type:        interfaces.GeneratedPublicFactoryWorkstationTypePtr(workstation.Config["type"]),
			Inputs:      generatedWorkstationIOs(workstation.InputPlaceIDs, placesByID),
			Outputs:     generatedWorkstationIOsPtr(workstation.OutputPlaceIDs, placesByID),
			OnContinue:  generatedWorkstationIOsPtr(workstation.ContinuePlaceIDs, placesByID),
			OnRejection: generatedWorkstationIOsPtr(workstation.RejectionPlaceIDs, placesByID),
			OnFailure:   generatedWorkstationIOsPtr(workstation.FailurePlaceIDs, placesByID),
		}
		if workstation.Kind != "" {
			converted.Behavior = interfaces.GeneratedPublicWorkstationKindPtr(interfaces.WorkstationKind(workstation.Kind))
		}
		out = append(out, converted)
	}
	return out
}

func generatedWorkstationIOs(placeIDs []string, places map[string]interfaces.FactoryPlace) []factoryapi.WorkstationIO {
	out := make([]factoryapi.WorkstationIO, 0, len(placeIDs))
	for _, placeID := range placeIDs {
		place, ok := places[placeID]
		if !ok {
			workType, stateValue := splitPlaceID(placeID)
			place = interfaces.FactoryPlace{TypeID: workType, State: stateValue}
		}
		out = append(out, factoryapi.WorkstationIO{WorkType: place.TypeID, State: place.State})
	}
	return out
}

func generatedWorkstationIOsPtr(placeIDs []string, places map[string]interfaces.FactoryPlace) *[]factoryapi.WorkstationIO {
	ios := generatedWorkstationIOs(placeIDs, places)
	if len(ios) == 0 {
		return nil
	}
	return &ios
}

func splitPlaceID(placeID string) (string, string) {
	before, after, ok := strings.Cut(placeID, ":")
	if !ok {
		return placeID, ""
	}
	return before, after
}

func generatedWorksPtr(items []interfaces.FactoryWorkItem) *[]factoryapi.Work {
	works := generatedWorks(items)
	return slicePtr(works)
}

func generatedWorks(items []interfaces.FactoryWorkItem) []factoryapi.Work {
	out := make([]factoryapi.Work, 0, len(items))
	for _, item := range items {
		out = append(out, generatedWork(item))
	}
	return out
}

func generatedWork(item interfaces.FactoryWorkItem) factoryapi.Work {
	name := item.DisplayName
	if name == "" {
		name = item.ID
	}
	currentChainingTraceID := item.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = item.TraceID
	}
	return factoryapi.Work{
		Name:                     name,
		WorkId:                   stringPtrIfNotEmpty(item.ID),
		WorkTypeName:             stringPtrIfNotEmpty(item.WorkTypeID),
		State:                    generatedWorkStatePtr(item.State),
		ChainingTraceDepth:       intPtrIfPositive(item.ChainingTraceDepth),
		CurrentChainingTraceId:   stringPtrIfNotEmpty(currentChainingTraceID),
		PreviousChainingTraceIds: stringSlicePtr(item.PreviousChainingTraceIDs),
		TraceId:                  stringPtrIfNotEmpty(item.TraceID),
		Content:                  workcontent.GeneratedPtrFromParts(item.Content),
		Tags:                     generatedStringMapPtr(item.Tags),
	}
}

func generatedDispatchConsumedWorkRefsFromTokens(tokens []interfaces.Token) []factoryapi.DispatchConsumedWorkRef {
	out := make([]factoryapi.DispatchConsumedWorkRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		workID := token.Color.WorkID
		if workID == "" {
			workID = token.ID
		}
		if workID == "" {
			continue
		}
		out = append(out, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
	}
	return out
}

func generatedDispatchRequestEventMetadataPtr(replayKey string, selection interfaces.ResolvedRunnerSelection) *factoryapi.DispatchRequestEventMetadata {
	if replayKey == "" && selection.RunnerID == "" && selection.Source == "" {
		return nil
	}
	return &factoryapi.DispatchRequestEventMetadata{
		ReplayKey:             stringPtrIfNotEmpty(replayKey),
		RunnerId:              interfaces.GeneratedPublicFactoryRunnerIDPtr(selection.RunnerID),
		RunnerSelectionSource: interfaces.GeneratedPublicFactoryRunnerSelectionSourcePtr(string(selection.Source)),
	}
}

func generatedFactoryRelationsPtr(relations []interfaces.FactoryRelation) *[]factoryapi.Relation {
	out := make([]factoryapi.Relation, 0, len(relations))
	for _, relation := range relations {
		out = append(out, generatedFactoryRelation(relation))
	}
	return slicePtr(out)
}

func generatedFactoryRelation(relation interfaces.FactoryRelation) factoryapi.Relation {
	targetName := relation.TargetWorkName
	if targetName == "" {
		targetName = relation.TargetWorkID
	}
	return factoryapi.Relation{
		Type:           factoryapi.RelationType(relation.Type),
		SourceWorkName: relation.SourceWorkName,
		TargetWorkName: targetName,
		TargetWorkId:   stringPtrIfNotEmpty(relation.TargetWorkID),
		RequiredState:  stringPtrIfNotEmpty(relation.RequiredState),
	}
}

func (h *FactoryEventHistory) generatedResourcesPtr(tokens []interfaces.Token) *[]factoryapi.Resource {
	resources := make([]factoryapi.Resource, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType != interfaces.DataTypeResource {
			continue
		}
		resources = append(resources, h.generatedResource(token.Color.WorkTypeID))
	}
	return slicePtr(resources)
}

func (h *FactoryEventHistory) generatedOutputResourcesPtr(mutations []interfaces.TokenMutationRecord) *[]factoryapi.Resource {
	resources := make([]factoryapi.Resource, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.Token == nil || mutation.Token.Color.DataType != interfaces.DataTypeResource {
			continue
		}
		resources = append(resources, h.generatedResource(mutation.Token.Color.WorkTypeID))
	}
	return slicePtr(resources)
}

func (h *FactoryEventHistory) generatedResource(resourceID string) factoryapi.Resource {
	resource := factoryapi.Resource{Name: resourceID}
	if h.net != nil && h.net.Resources != nil {
		if def := h.net.Resources[resourceID]; def != nil {
			resource.Name = def.Name
			if resource.Name == "" {
				resource.Name = def.ID
			}
			resource.Capacity = def.Capacity
		}
	}
	return resource
}

func generatedFactoryStatePtr(stateValue interfaces.FactoryState) *factoryapi.FactoryState {
	if stateValue == "" {
		return nil
	}
	converted := factoryapi.FactoryState(stateValue)
	return &converted
}
