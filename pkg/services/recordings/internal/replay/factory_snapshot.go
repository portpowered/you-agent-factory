package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Retained within the replay implementation for package-local compatibility
// while public metadata comparison is owned by the Recordings root.
const (
	metadataFactoryHash       = "factory_hash"
	metadataWorkersHash       = "workers_hash"
	metadataWorkstationsHash  = "workstations_hash"
	metadataRuntimeConfigHash = "runtime_config_hash"
)

func validatedFactorySnapshotFromJSON(
	data []byte,
	decode interfaces.FactorySnapshotJSONDecoder,
) (*interfaces.FactorySnapshot, error) {
	if decode == nil {
		return nil, fmt.Errorf("Factory snapshot decoder is required")
	}
	return decode(data)
}

func factorySnapshotFromJSON(data []byte) (*interfaces.FactorySnapshot, error) {
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	snapshot, err := interfaces.NewFactorySnapshot(object)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func runtimeWorkersByName(factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) map[string]interfaces.FactoryWorkerConfig {
	workers := make(map[string]interfaces.FactoryWorkerConfig)
	for _, workerCfg := range factoryCfg.Workers {
		def, ok := runtimeCfg.Worker(workerCfg.Name)
		if ok && def != nil {
			workers[workerCfg.Name] = interfaces.CloneWorkerConfig(*def)
		}
	}
	return workers
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringSliceMap(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func sortedWorkerNames(workers map[string]interfaces.FactoryWorkerConfig) []string {
	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func replayV2EventStructuralFailure(
	event interfaces.FactoryEvent,
	sequence int,
) (recordings.ReplayArtifactDiagnosticCode, error) {
	switch {
	case event.Context.Sequence != sequence:
		return recordings.ReplayArtifactDiagnosticInvalidOrder, fmt.Errorf(
			"replay v2 event sequence is %d, want %d",
			event.Context.Sequence,
			sequence,
		)
	case event.Id == "":
		return recordings.ReplayArtifactDiagnosticInvalidIdentity, fmt.Errorf("replay v2 event id is required")
	case event.SchemaVersion != interfaces.FactoryEventSchemaVersionV1:
		return recordings.ReplayArtifactDiagnosticMalformed, fmt.Errorf("replay v2 event schema is unsupported")
	case event.Type == "":
		return recordings.ReplayArtifactDiagnosticMalformed, fmt.Errorf("replay v2 event type is required")
	case event.Context.EventTime.IsZero():
		return recordings.ReplayArtifactDiagnosticMalformed, fmt.Errorf("replay v2 event eventTime is required")
	case !json.Valid(event.Payload):
		return recordings.ReplayArtifactDiagnosticMalformed, fmt.Errorf("replay v2 event payload is invalid JSON")
	default:
		return "", nil
	}
}

func replayEventStructuralError(
	code recordings.ReplayArtifactDiagnosticCode,
	index int,
	eventID string,
	cause error,
) *recordings.ReplayArtifactError {
	if index < 0 {
		index = 0
	}
	safeID := safeReplayEventID(eventID)
	message := fmt.Sprintf("event %q is malformed", safeID)
	switch code {
	case recordings.ReplayArtifactDiagnosticInvalidIdentity:
		message = fmt.Sprintf("event %q has invalid identity", safeID)
	case recordings.ReplayArtifactDiagnosticInvalidOrder:
		message = fmt.Sprintf("event %q has invalid order", safeID)
	case recordings.ReplayArtifactDiagnosticMissingReference:
		message = fmt.Sprintf("event %q has a missing reference", safeID)
	case recordings.ReplayArtifactDiagnosticForeignReference:
		message = fmt.Sprintf("event %q has a foreign reference", safeID)
	}
	return &recordings.ReplayArtifactError{
		Kind: replayArtifactErrorKind(code),
		Diagnostic: recordings.ReplayArtifactDiagnostic{
			Code:    code,
			Area:    "events",
			Path:    fmt.Sprintf("events[%d]", index),
			Message: message,
			Action:  recordings.ReplayArtifactStructuralRepairAction,
		},
		Cause: errors.Join(recordings.ErrCorruptReplayInput, cause),
	}
}

func replayArtifactErrorKind(code recordings.ReplayArtifactDiagnosticCode) recordings.ReplayArtifactErrorKind {
	switch code {
	case recordings.ReplayArtifactDiagnosticInvalidOrder:
		return recordings.ReplayArtifactErrorInvalidOrder
	case recordings.ReplayArtifactDiagnosticForeignReference:
		return recordings.ReplayArtifactErrorForeign
	default:
		return recordings.ReplayArtifactErrorCorruptInput
	}
}

func safeReplayEventID(eventID string) string {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" || len(eventID) > 256 {
		return "unknown"
	}
	for _, character := range eventID {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("-_.:/", character) {
			continue
		}
		return "unknown"
	}
	return eventID
}

func validateReplayEventReferences(artifact *interfaces.ReplayArtifact) error {
	if artifact == nil {
		return nil
	}
	workIDs := replayRecordedWorkIDs(artifact.Events)
	dispatchIDs := replayRecordedDispatchIDs(artifact.Events)
	for index, event := range artifact.Events {
		switch event.Type {
		case interfaces.FactoryEventTypeWorkRequest:
			if err := validateReplayWorkRequestReferences(index, event, workIDs); err != nil {
				return err
			}
		case interfaces.FactoryEventTypeRelationshipChangeRequest:
			if err := validateReplayRelationshipReferences(index, event, workIDs); err != nil {
				return err
			}
		case interfaces.FactoryEventTypeDispatchRequest:
			if err := validateReplayDispatchReferences(index, event, workIDs); err != nil {
				return err
			}
		case interfaces.FactoryEventTypeWorkStateChange:
			if err := validateReplayWorkStateReference(index, event, workIDs); err != nil {
				return err
			}
		case interfaces.FactoryEventTypeDispatchResponse,
			interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
			if len(dispatchIDs) > 0 {
				if err := validateReplayDispatchIDReference(index, event, dispatchIDs); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func replayRecordedWorkIDs(events []interfaces.FactoryEvent) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, event := range events {
		if event.Type == interfaces.FactoryEventTypeWorkRequest {
			for _, workID := range stringSliceValue(event.Context.WorkIDs) {
				if workID != "" {
					ids[workID] = struct{}{}
				}
			}
		}
		var payload struct {
			Works []struct {
				WorkID string `json:"workId"`
			} `json:"works"`
			OutputWork []struct {
				WorkID string `json:"workId"`
			} `json:"outputWork"`
		}
		if event.Type != interfaces.FactoryEventTypeWorkRequest &&
			event.Type != interfaces.FactoryEventTypeDispatchResponse {
			continue
		}
		if json.Unmarshal(event.Payload, &payload) != nil {
			continue
		}
		for _, item := range append(payload.Works, payload.OutputWork...) {
			if item.WorkID != "" {
				ids[item.WorkID] = struct{}{}
			}
		}
	}
	return ids
}

func replayRecordedDispatchIDs(events []interfaces.FactoryEvent) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchRequest {
			continue
		}
		if dispatchID := stringValue(event.Context.DispatchID); dispatchID != "" {
			ids[dispatchID] = struct{}{}
		}
	}
	return ids
}

func validateReplayWorkRequestReferences(
	index int,
	event interfaces.FactoryEvent,
	workIDs map[string]struct{},
) error {
	var payload work.WorkRequestEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticMalformed,
			index,
			event.Id,
			fmt.Errorf("decode work request references: %w", err),
		)
	}
	contextWorkIDs := stringSliceValue(event.Context.WorkIDs)
	for workIndex, item := range payload.Works {
		workID := strings.TrimSpace(item.WorkID)
		if workID == "" && workIndex < len(contextWorkIDs) {
			workID = strings.TrimSpace(contextWorkIDs[workIndex])
		}
		if workID == "" && len(contextWorkIDs) > 0 {
			return replayEventStructuralError(
				recordings.ReplayArtifactDiagnosticMissingReference,
				index,
				event.Id,
				fmt.Errorf("work request work[%d] has no work ID", workIndex),
			)
		}
	}
	for relationIndex, relation := range payload.Relations {
		if strings.TrimSpace(relation.SourceWorkName) == "" {
			return replayEventStructuralError(
				recordings.ReplayArtifactDiagnosticMissingReference,
				index,
				event.Id,
				fmt.Errorf("work request relation[%d] source is required", relationIndex),
			)
		}
		if strings.TrimSpace(relation.TargetWorkID) == "" && strings.TrimSpace(relation.TargetWorkName) == "" {
			return replayEventStructuralError(
				recordings.ReplayArtifactDiagnosticMissingReference,
				index,
				event.Id,
				fmt.Errorf("work request relation[%d] target is required", relationIndex),
			)
		}
		if targetID := strings.TrimSpace(relation.TargetWorkID); targetID != "" {
			if _, ok := workIDs[targetID]; !ok {
				return replayEventStructuralError(
					recordings.ReplayArtifactDiagnosticForeignReference,
					index,
					event.Id,
					fmt.Errorf("work request relation[%d] target is unknown", relationIndex),
				)
			}
		}
	}
	return nil
}

func validateReplayDispatchReferences(
	index int,
	event interfaces.FactoryEvent,
	workIDs map[string]struct{},
) error {
	if strings.TrimSpace(stringValue(event.Context.DispatchID)) == "" {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticMissingReference,
			index,
			event.Id,
			fmt.Errorf("dispatch ID is required"),
		)
	}
	var payload interfaces.DispatchRequestEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticMalformed,
			index,
			event.Id,
			fmt.Errorf("decode dispatch references: %w", err),
		)
	}
	contextWorkIDs := stringSliceValue(event.Context.WorkIDs)
	for inputIndex, input := range payload.Inputs {
		workID := strings.TrimSpace(input.WorkID)
		if workID == "" && inputIndex < len(contextWorkIDs) {
			workID = strings.TrimSpace(contextWorkIDs[inputIndex])
		}
		if workID == "" {
			return replayEventStructuralError(
				recordings.ReplayArtifactDiagnosticMissingReference,
				index,
				event.Id,
				fmt.Errorf("dispatch input[%d] work ID is required", inputIndex),
			)
		}
		if len(workIDs) > 0 {
			if _, ok := workIDs[workID]; !ok {
				return replayEventStructuralError(
					recordings.ReplayArtifactDiagnosticForeignReference,
					index,
					event.Id,
					fmt.Errorf("dispatch input[%d] work ID is unknown", inputIndex),
				)
			}
		}
	}
	return nil
}

func validateReplayRelationshipReferences(
	index int,
	event interfaces.FactoryEvent,
	workIDs map[string]struct{},
) error {
	var payload struct {
		Relation work.WorkRequestEventRelation `json:"relation"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticMalformed,
			index,
			event.Id,
			fmt.Errorf("decode relationship references: %w", err),
		)
	}
	contextWorkIDs := stringSliceValue(event.Context.WorkIDs)
	source := strings.TrimSpace(payload.Relation.SourceWorkName)
	if source == "" && len(contextWorkIDs) > 0 {
		source = strings.TrimSpace(contextWorkIDs[0])
	}
	if source == "" {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticMissingReference,
			index,
			event.Id,
			fmt.Errorf("relationship source is required"),
		)
	}
	targetID := strings.TrimSpace(payload.Relation.TargetWorkID)
	targetName := strings.TrimSpace(payload.Relation.TargetWorkName)
	if targetID == "" && targetName == "" && len(contextWorkIDs) > 1 {
		targetID = strings.TrimSpace(contextWorkIDs[1])
	}
	if targetID == "" && targetName == "" {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticMissingReference,
			index,
			event.Id,
			fmt.Errorf("relationship target is required"),
		)
	}
	if targetID != "" && len(workIDs) > 0 {
		if _, ok := workIDs[targetID]; !ok {
			return replayEventStructuralError(
				recordings.ReplayArtifactDiagnosticForeignReference,
				index,
				event.Id,
				fmt.Errorf("relationship target work ID is unknown"),
			)
		}
	}
	return nil
}

func validateReplayWorkStateReference(
	index int,
	event interfaces.FactoryEvent,
	workIDs map[string]struct{},
) error {
	var payload interfaces.WorkStateChangeEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticMalformed,
			index,
			event.Id,
			fmt.Errorf("decode work state reference: %w", err),
		)
	}
	if payload.Source != work.WorkStateChangeSourceAPI && payload.Source != work.WorkStateChangeSourceCLI {
		return nil
	}
	workID := strings.TrimSpace(payload.WorkID)
	if workID == "" {
		workID = strings.TrimSpace(firstString(event.Context.WorkIDs))
	}
	if workID == "" {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticMissingReference,
			index,
			event.Id,
			fmt.Errorf("work state change work ID is required"),
		)
	}
	if len(workIDs) > 0 {
		if _, ok := workIDs[workID]; !ok {
			return replayEventStructuralError(
				recordings.ReplayArtifactDiagnosticForeignReference,
				index,
				event.Id,
				fmt.Errorf("work state change work ID is unknown"),
			)
		}
	}
	if triggerWorkID := strings.TrimSpace(stringValue(payload.TriggerWorkID)); triggerWorkID != "" && len(workIDs) > 0 {
		if _, ok := workIDs[triggerWorkID]; !ok {
			return replayEventStructuralError(
				recordings.ReplayArtifactDiagnosticForeignReference,
				index,
				event.Id,
				fmt.Errorf("work state change trigger work ID is unknown"),
			)
		}
	}
	return nil
}

func validateReplayDispatchIDReference(
	index int,
	event interfaces.FactoryEvent,
	dispatchIDs map[string]struct{},
) error {
	dispatchID := strings.TrimSpace(stringValue(event.Context.DispatchID))
	if dispatchID == "" {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticMissingReference,
			index,
			event.Id,
			fmt.Errorf("dispatch ID is required"),
		)
	}
	if _, ok := dispatchIDs[dispatchID]; !ok {
		return replayEventStructuralError(
			recordings.ReplayArtifactDiagnosticForeignReference,
			index,
			event.Id,
			fmt.Errorf("dispatch ID is unknown"),
		)
	}
	return nil
}
