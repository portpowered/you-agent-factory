package events

import (
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func cloneFactoryEvents(events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
	clones := make([]interfaces.FactoryEvent, len(events))
	for index, event := range events {
		clones[index] = event.Clone()
	}
	return clones
}

// RecordInferenceEvent appends worker-owned provider facts to canonical
// history while Factory owns the envelope, vocabulary, and ordering.
func (h *FactoryEventHistory) RecordInferenceEvent(event workerexecution.InferenceEvent) {
	if h == nil || strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.DispatchID) == "" {
		return
	}
	eventType, payload := inferenceFactoryEventPayload(event)
	if eventType == "" || payload == nil {
		return
	}
	h.appendEvent(domainFactoryEvent(
		eventType,
		event.ID,
		interfaces.FactoryEventContext{
			Tick:       event.Tick,
			EventTime:  interfaces.CanonicalEventTime(event.EventTime),
			DispatchID: stringPtrIfNotEmpty(event.DispatchID),
			RequestID:  stringPtrIfNotEmpty(event.RequestID),
			TraceIDs:   stringSlicePtr(event.TraceIDs),
			WorkIDs:    stringSlicePtr(event.WorkIDs),
		},
		payload,
	))
}

func inferenceFactoryEventPayload(event workerexecution.InferenceEvent) (interfaces.FactoryEventType, any) {
	switch event.Kind {
	case workerexecution.InferenceEventKindRequest:
		if event.Request != nil && event.Response == nil {
			return interfaces.FactoryEventTypeInferenceRequest, *event.Request
		}
	case workerexecution.InferenceEventKindResponse:
		if event.Response != nil && event.Request == nil {
			return interfaces.FactoryEventTypeInferenceResponse, *event.Response
		}
	}
	return "", nil
}

func splitPlaceID(placeID string) (string, string) {
	before, after, ok := strings.Cut(placeID, ":")
	if !ok {
		return placeID, ""
	}
	return before, after
}

func dispatchConsumedWorkRefsFromTokens(tokens []factorytoken.Token) []interfaces.DispatchConsumedWorkRef {
	out := make([]interfaces.DispatchConsumedWorkRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		workID := token.Color.WorkID
		if workID == "" {
			workID = token.ID
		}
		if workID == "" {
			continue
		}
		out = append(out, interfaces.DispatchConsumedWorkRef{WorkID: workID})
	}
	return out
}

func dispatchRequestEventMetadataPtr(replayKey string, selection workerexecution.ResolvedRunnerSelection) *interfaces.DispatchRequestEventMetadata {
	if replayKey == "" && selection.RunnerID == "" && selection.Source == "" {
		return nil
	}
	runnerID := stringPtrIfNotEmpty(selection.RunnerID)
	var source *workerexecution.RunnerSelectionSource
	if selection.Source != "" {
		value := selection.Source
		source = &value
	}
	return &interfaces.DispatchRequestEventMetadata{
		ReplayKey:             stringPtrIfNotEmpty(replayKey),
		RunnerID:              runnerID,
		RunnerSelectionSource: source,
	}
}

func eventRelations(relations []work.FactoryRelation) []work.WorkRequestEventRelation {
	out := make([]work.WorkRequestEventRelation, 0, len(relations))
	for _, relation := range relations {
		out = append(out, eventRelation(relation))
	}
	return out
}

func eventRelation(relation work.FactoryRelation) work.WorkRequestEventRelation {
	targetName := relation.TargetWorkName
	if targetName == "" {
		targetName = relation.TargetWorkID
	}
	return work.WorkRequestEventRelation{
		Type:           work.WorkRelationType(relation.Type),
		SourceWorkName: relation.SourceWorkName,
		TargetWorkName: targetName,
		TargetWorkID:   relation.TargetWorkID,
		RequiredState:  relation.RequiredState,
	}
}

func (h *FactoryEventHistory) dispatchResourcesPtr(tokens []factorytoken.Token) *[]interfaces.DispatchResourceRef {
	resources := make([]interfaces.DispatchResourceRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		resources = append(resources, h.dispatchResource(token.Color.WorkTypeID))
	}
	return slicePtr(resources)
}

func (h *FactoryEventHistory) dispatchOutputResourcesPtr(mutations []interfaces.TokenMutationRecord) *[]workerexecution.DispatchResourceEventRef {
	resources := make([]workerexecution.DispatchResourceEventRef, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.Token == nil || mutation.Token.Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		resource := h.dispatchResource(mutation.Token.Color.WorkTypeID)
		resources = append(resources, workerexecution.DispatchResourceEventRef(resource))
	}
	return &resources
}

func (h *FactoryEventHistory) dispatchResource(resourceID string) interfaces.DispatchResourceRef {
	resource := interfaces.DispatchResourceRef{Name: resourceID}
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

func eventWorks(items []work.FactoryWorkItem) []work.WorkRequestEventWork {
	out := make([]work.WorkRequestEventWork, 0, len(items))
	for _, item := range items {
		name := item.DisplayName
		if name == "" {
			name = item.ID
		}
		currentChainingTraceID := item.CurrentChainingTraceID
		if currentChainingTraceID == "" {
			currentChainingTraceID = item.TraceID
		}
		var state *work.WorkEventState
		if item.State != "" {
			state = &work.WorkEventState{Name: item.State, Type: inferredWorkEventStateType(item.State)}
		}
		out = append(out, work.WorkRequestEventWork{
			Name:                     name,
			WorkID:                   item.ID,
			WorkTypeID:               item.WorkTypeID,
			State:                    state,
			ChainingTraceDepth:       item.ChainingTraceDepth,
			CurrentChainingTraceID:   currentChainingTraceID,
			PreviousChainingTraceIDs: append([]string(nil), item.PreviousChainingTraceIDs...),
			TraceID:                  item.TraceID,
			Content:                  work.CloneWorkContentParts(item.Content),
			Tags:                     cloneStringMap(item.Tags),
		})
	}
	return out
}

func requestEventWorks(items []work.FactoryWorkItem) []work.WorkRequestEventWork {
	out := eventWorks(items)
	for index := range out {
		out[index].Content = requestEventContent(out[index].Content)
	}
	return out
}

func inferredWorkEventStateType(name string) string {
	switch name {
	case "init":
		return "INITIAL"
	case "complete", "done":
		return "TERMINAL"
	case "failed":
		return "FAILED"
	default:
		return "PROCESSING"
	}
}

func requestEventContent(parts []work.WorkContentPart) []work.WorkContentPart {
	out := make([]work.WorkContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type.Normalized() {
		case work.WorkContentPartTypeText, work.WorkContentPartTypeImage,
			work.WorkContentPartTypeAudio, work.WorkContentPartTypeBinary:
			out = append(out, part)
		case work.WorkContentPartTypeJSON:
			if len(part.JSON) == 0 || json.Valid(part.JSON) {
				out = append(out, part)
			}
		}
	}
	return work.CloneWorkContentParts(out)
}

func eventWorksPtr(items []work.FactoryWorkItem) *[]work.WorkRequestEventWork {
	out := eventWorks(items)
	return &out
}
