package replayfixtures

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/definitionmapping"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings/events/snapshot"
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
	mapper, err := definitionmapping.New(func() string { return "fixture-id" })
	if err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("construct topology fixture mapper: %w", err)
	}
	net, err := mapper.Map(context.Background(), factory)
	if err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("map topology fixture: %w", err)
	}
	lookup := runtimefixtures.RuntimeDefinitionLookupFixture{
		Factory:      factory,
		Workers:      mapWorkers(factory.Workers),
		Workstations: mapWorkstations(factory.Workstations),
	}
	factorySnapshot := snapshot.FromInitialStructure(state.ProjectInitialStructure(net, lookup))
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
		Resources: []factoryresource.Config{{ID: "gpu-stable", Name: resourceName, Capacity: 2}},
		Workers: []workerconfig.Config{{
			ID: "writer-stable", Name: workerName, Type: interfaces.WorkerTypeScript,
			Resources: []factoryresource.Config{{Name: resourceName, Capacity: 1}},
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
			Resources:      []factoryresource.Config{{Name: resourceName, Capacity: 1}},
		}},
	}
}

func mapWorkers(workers []workerconfig.Config) map[string]*workerconfig.Config {
	result := make(map[string]*workerconfig.Config, len(workers))
	for i := range workers {
		result[workers[i].Name] = &workers[i]
	}
	return result
}

func mapWorkstations(workstations []interfaces.FactoryWorkstationConfig) map[string]*interfaces.FactoryWorkstationConfig {
	result := make(map[string]*interfaces.FactoryWorkstationConfig, len(workstations))
	for i := range workstations {
		result[workstations[i].Name] = &workstations[i]
	}
	return result
}
