package factoryeventkinds

import (
	"encoding/json"
	"fmt"

	factorycontracts "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryEventSerializationError names the FactoryEvent kind under test when
// envelope serialization or payload discrimination fails.
type FactoryEventSerializationError struct {
	Kind  factorycontracts.FactoryEventType
	Phase string
	Cause error
}

func (e FactoryEventSerializationError) Error() string {
	return fmt.Sprintf(
		"factory event serialization for kind %s failed during %s: %v",
		e.Kind,
		e.Phase,
		e.Cause,
	)
}

func (e FactoryEventSerializationError) Unwrap() error {
	return e.Cause
}

// RoundTripFactoryEventEnvelope marshals and unmarshals a public FactoryEvent
// envelope and verifies the event type selects the expected generated payload
// shape through the discriminator-backed union decoder.
func RoundTripFactoryEventEnvelope(event factoryapi.FactoryEvent) error {
	kind := factorycontracts.FactoryEventType(event.Type)
	if err := decodeFactoryEventPayloadForKind(kind, event.Payload); err != nil {
		return FactoryEventSerializationError{
			Kind:  kind,
			Phase: "decode-payload",
			Cause: err,
		}
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		return FactoryEventSerializationError{
			Kind:  kind,
			Phase: "marshal",
			Cause: err,
		}
	}

	var roundTripped factoryapi.FactoryEvent
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		return FactoryEventSerializationError{
			Kind:  kind,
			Phase: "unmarshal",
			Cause: err,
		}
	}
	if roundTripped.Type != event.Type {
		return FactoryEventSerializationError{
			Kind:  kind,
			Phase: "type-discriminator",
			Cause: fmt.Errorf("round-tripped type = %q, want %q", roundTripped.Type, event.Type),
		}
	}

	if err := decodeFactoryEventPayloadForKind(factorycontracts.FactoryEventType(roundTripped.Type), roundTripped.Payload); err != nil {
		return FactoryEventSerializationError{
			Kind:  kind,
			Phase: "round-trip-decode-payload",
			Cause: err,
		}
	}

	return nil
}

func decodeFactoryEventPayloadForKind(
	kind factorycontracts.FactoryEventType,
	payload factoryapi.FactoryEvent_Payload,
) error {
	decode, ok := publicFactoryEventPayloadDecoders[kind]
	if !ok {
		return fmt.Errorf("no payload decoder registered for kind %q", kind)
	}
	return decode(payload)
}

var publicFactoryEventPayloadDecoders = map[factorycontracts.FactoryEventType]func(factoryapi.FactoryEvent_Payload) error{
	factorycontracts.FactoryEventTypeRunRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRunRequestEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeInitialStructureRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInitialStructureRequestEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeFactoryChange: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsFactoryChangeEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeWorkRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsWorkRequestEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeRelationshipChangeRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRelationshipChangeRequestEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeDispatchRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchRequestEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeDispatchResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchResponseEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeFactoryStateResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsFactoryStateResponseEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeRunResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRunResponseEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeWorkStateChange: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsWorkStateChangeEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeInferenceRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInferenceRequestEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeInferenceResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInferenceResponseEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeModelRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsModelRequestEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeModelResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsModelResponseEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeScriptRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsScriptRequestEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeScriptResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsScriptResponseEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeAgentRunResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsAgentRunResponseEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeSessionStarted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionStartedEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeSessionPaused: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionPausedEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeSessionResumed: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionResumedEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeSessionResultUpdated: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionResultUpdatedEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeSessionCompleted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionCompletedEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeSessionLifecycleControl: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionLifecycleControlEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeOrchestratorPhaseChanged: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsOrchestratorPhaseChangedEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeOrchestratorCheckpointWritten: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsOrchestratorCheckpointWrittenEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeDispatchQueued: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchQueuedEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeDispatchInterrupted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchInterruptedEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeDispatchReconciled: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchReconciledEventPayload()
		return err
	},
	factorycontracts.FactoryEventTypeArtifactCreated: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsArtifactCreatedEventPayload()
		return err
	},
}
