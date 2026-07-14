package factoryeventkinds

import (
	"encoding/json"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryEventSerializationError names the FactoryEvent kind under test when
// envelope serialization or payload discrimination fails.
type FactoryEventSerializationError struct {
	Kind  factoryapi.FactoryEventType
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
	if err := decodeFactoryEventPayloadForKind(event.Type, event.Payload); err != nil {
		return FactoryEventSerializationError{
			Kind:  event.Type,
			Phase: "decode-payload",
			Cause: err,
		}
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		return FactoryEventSerializationError{
			Kind:  event.Type,
			Phase: "marshal",
			Cause: err,
		}
	}

	var roundTripped factoryapi.FactoryEvent
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		return FactoryEventSerializationError{
			Kind:  event.Type,
			Phase: "unmarshal",
			Cause: err,
		}
	}
	if roundTripped.Type != event.Type {
		return FactoryEventSerializationError{
			Kind:  event.Type,
			Phase: "type-discriminator",
			Cause: fmt.Errorf("round-tripped type = %q, want %q", roundTripped.Type, event.Type),
		}
	}

	if err := decodeFactoryEventPayloadForKind(roundTripped.Type, roundTripped.Payload); err != nil {
		return FactoryEventSerializationError{
			Kind:  event.Type,
			Phase: "round-trip-decode-payload",
			Cause: err,
		}
	}

	return nil
}

func decodeFactoryEventPayloadForKind(
	kind factoryapi.FactoryEventType,
	payload factoryapi.FactoryEvent_Payload,
) error {
	decode, ok := publicFactoryEventPayloadDecoders[kind]
	if !ok {
		return fmt.Errorf("no payload decoder registered for kind %q", kind)
	}
	return decode(payload)
}

var publicFactoryEventPayloadDecoders = map[factoryapi.FactoryEventType]func(factoryapi.FactoryEvent_Payload) error{
	factoryapi.FactoryEventTypeRunRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRunRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeInitialStructureRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInitialStructureRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeFactoryChange: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsFactoryChangeEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeWorkRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsWorkRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeRelationshipChangeRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRelationshipChangeRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeFactoryStateResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsFactoryStateResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeRunResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRunResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeWorkStateChange: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsWorkStateChangeEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeInferenceRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInferenceRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeInferenceResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInferenceResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeModelRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsModelRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeModelResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsModelResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeScriptRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsScriptRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeScriptResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsScriptResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeAgentRunResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsAgentRunResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionStarted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionStartedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionPaused: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionPausedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionResumed: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionResumedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionResultUpdated: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionResultUpdatedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionCompleted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionCompletedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionLifecycleControl: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionLifecycleControlEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeOrchestratorPhaseChanged: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsOrchestratorPhaseChangedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeOrchestratorCheckpointWritten: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsOrchestratorCheckpointWrittenEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchQueued: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchQueuedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchInterrupted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchInterruptedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchReconciled: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchReconciledEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeArtifactCreated: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsArtifactCreatedEventPayload()
		return err
	},
}
