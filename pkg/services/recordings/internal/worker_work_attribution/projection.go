package workerworkattribution

import (
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type association struct {
	dispatch string
	workIDs  []string
}

type nameProjection struct {
	names           map[string]string
	associations    map[string]association
	reportedDefault bool
}

func projectNames(history recordings.HistoricalRecordingQueryResult, factory string) (nameProjection, error) {
	projection := nameProjection{names: make(map[string]string), associations: make(map[string]association)}
	if history.Recording.Scope.FactorySessionID != factory {
		return projection, recordings.ErrInvalidProjectionScope
	}
	dispatchWork := make(map[string][]string)
	for _, event := range history.Events {
		if event.Scope.FactorySessionID != factory {
			return projection, recordings.ErrInvalidProjectionScope
		}
		var context factorydefinitions.FactoryEventContext
		if json.Unmarshal([]byte(event.SourceContext), &context) != nil {
			return projection, recordings.ErrInvalidProjectionInput
		}
		if context.SessionID != nil && *context.SessionID != factory {
			return projection, recordings.ErrInvalidProjectionScope
		}
		switch event.Kind {
		case recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest):
			if err := applyWorkNames(projection.names, event, context); err != nil {
				return projection, err
			}
		case recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchRequest),
			recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchQueued):
			if err := rememberDispatchWork(dispatchWork, event, context); err != nil {
				return projection, err
			}
		case recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc):
			if err := applyAssociation(projection.associations, dispatchWork, event, context); err != nil {
				return projection, err
			}
		}
	}
	return projection, nil
}

// Only authored Name/DisplayName values qualify. Work read-model fallback names
// and provider transcripts never enter this projection.
func applyWorkNames(names map[string]string, event recordings.CanonicalEvent, context factorydefinitions.FactoryEventContext) error {
	var current work.WorkRequestEventPayload
	var legacy factorydefinitions.WorkRequestPayload
	if json.Unmarshal([]byte(event.Payload), &current) != nil || json.Unmarshal([]byte(event.Payload), &legacy) != nil {
		return recordings.ErrInvalidProjectionInput
	}
	eventNames := make(map[string]string)
	for index, item := range current.Works {
		id := item.WorkID
		if context.WorkIDs != nil && index < len(*context.WorkIDs) {
			contextID := (*context.WorkIDs)[index]
			if id != "" && contextID != "" && id != contextID {
				return recordings.ErrInvalidProjectionInput
			}
			if id == "" {
				id = contextID
			}
		}
		if err := recordExplicitName(eventNames, id, item.Name); err != nil {
			return err
		}
	}
	for _, item := range legacy.WorkItems {
		if err := recordExplicitName(eventNames, item.ID, item.DisplayName); err != nil {
			return err
		}
	}
	for id, name := range eventNames {
		names[id] = name
	}
	return nil
}

func recordExplicitName(names map[string]string, id, name string) error {
	if id == "" || name == "" {
		return nil
	}
	if existing, exists := names[id]; exists && existing != name {
		return recordings.ErrInvalidProjectionInput
	}
	names[id] = name
	return nil
}

func rememberDispatchWork(dispatches map[string][]string, event recordings.CanonicalEvent, context factorydefinitions.FactoryEventContext) error {
	if context.DispatchID == nil || *context.DispatchID == "" {
		return recordings.ErrInvalidProjectionInput
	}
	if context.WorkIDs != nil {
		dispatches[*context.DispatchID] = append([]string(nil), (*context.WorkIDs)...)
		return nil
	}
	if event.Kind == recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchRequest) {
		var payload factorydefinitions.DispatchRequestEventPayload
		if json.Unmarshal([]byte(event.Payload), &payload) != nil {
			return recordings.ErrInvalidProjectionInput
		}
		ids := make([]string, 0, len(payload.Inputs))
		for _, input := range payload.Inputs {
			ids = append(ids, input.WorkID)
		}
		dispatches[*context.DispatchID] = ids
	}
	return nil
}

func applyAssociation(associations map[string]association, dispatches map[string][]string, event recordings.CanonicalEvent, context factorydefinitions.FactoryEventContext) error {
	var payload factorydefinitions.DispatchWorkerSessionAssociationEventPayload
	if json.Unmarshal([]byte(event.Payload), &payload) != nil || payload.WorkerSessionID == "" ||
		context.DispatchID == nil || *context.DispatchID == "" {
		return recordings.ErrInvalidProjectionInput
	}
	if previous, exists := associations[payload.WorkerSessionID]; exists && previous.dispatch != *context.DispatchID {
		return recordings.ErrInvalidProjectionInput
	}
	ids, exists := dispatches[*context.DispatchID]
	if !exists {
		return recordings.ErrInvalidProjectionInput
	}
	associations[payload.WorkerSessionID] = association{*context.DispatchID, ids}
	return nil
}
