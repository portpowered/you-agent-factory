package replayfixtures

import (
	"encoding/json"
	"fmt"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

// CanonicalTopologyReplacementEvents returns backend-produced topology events
// whose public entities keep durable IDs while their authored names change.
func CanonicalTopologyReplacementEvents() ([]interfaces.FactoryEvent, error) {
	initial, err := canonicalTopologyEvent("initial-topology", interfaces.FactoryEventTypeInitialStructureRequest, 0, 1, topologyFactory("gpu", "writer", "task", "queued", "review"))
	if err != nil {
		return nil, err
	}
	replacement, err := canonicalTopologyEvent("replacement-topology", interfaces.FactoryEventTypeFactoryChange, 3, 2, topologyFactory("accelerator", "author", "job", "waiting", "approval"))
	if err != nil {
		return nil, err
	}
	return []interfaces.FactoryEvent{initial, replacement}, nil
}

func canonicalTopologyEvent(id string, eventType interfaces.FactoryEventType, tick, sequence int, factory *interfaces.FactoryConfig) (interfaces.FactoryEvent, error) {
	publicFactory, err := factorysnapshot.ObjectFromFactoryConfig(factory)
	if err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("map topology fixture to public Factory: %w", err)
	}
	factorySnapshot, err := interfaces.NewFactorySnapshot(publicFactory)
	if err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("snapshot topology fixture: %w", err)
	}
	var payload []byte
	if eventType == interfaces.FactoryEventTypeFactoryChange {
		payload, err = json.Marshal(interfaces.FactoryChangeEventPayload{Factory: factorySnapshot})
	} else {
		payload, err = json.Marshal(interfaces.InitialStructureRequestEventPayload{Factory: factorySnapshot})
	}
	if err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("marshal topology fixture: %w", err)
	}
	return interfaces.FactoryEvent{
		Id: id, SchemaVersion: interfaces.FactoryEventSchemaVersionV1, Type: eventType,
		Context: interfaces.FactoryEventContext{
			Tick: tick, Sequence: sequence,
			EventTime: time.Date(2026, 7, 18, 11, int(tick), 0, 0, time.UTC),
		},
		Payload: payload,
	}, nil
}

func topologyFactory(resourceName, workerName, workTypeName, initialStateName, workstationName string) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Name:      "canonical-topology",
		Resources: []interfaces.ResourceConfig{{ID: "gpu-stable", Name: resourceName, Capacity: 2}},
		Workers: []interfaces.FactoryWorkerConfig{{
			ID: "writer-stable", Name: workerName, Type: interfaces.WorkerTypeScript,
			Resources: []interfaces.ResourceConfig{{Name: resourceName, Capacity: 1}},
		}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			ID: "task-stable", Name: workTypeName,
			States: []interfaces.StateConfig{
				{ID: "queued-stable", Name: initialStateName, Type: interfaces.StateTypeInitial},
				{ID: "done-stable", Name: "done", Type: interfaces.StateTypeTerminal},
				{ID: "failed-stable", Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			ID: "review-stable", Name: workstationName, Type: interfaces.WorkstationTypeScript,
			WorkerTypeName: workerName,
			Inputs:         []interfaces.IOConfig{{WorkTypeName: workTypeName, StateName: initialStateName}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: workTypeName, StateName: "done"}},
			OnFailure:      []interfaces.IOConfig{{WorkTypeName: workTypeName, StateName: "failed"}},
			Resources:      []interfaces.ResourceConfig{{Name: resourceName, Capacity: 1}},
		}},
	}
}
