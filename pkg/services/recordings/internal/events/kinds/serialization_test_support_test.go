package factoryeventkinds

import (
	"encoding/json"
	"fmt"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryEventSerializationError names the FactoryEvent kind under test when
// envelope serialization or payload discrimination fails.
type FactoryEventSerializationError struct {
	Kind  recordings.FactoryEventType
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
	kind := recordings.FactoryEventType(event.Type)
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

	if err := decodeFactoryEventPayloadForKind(recordings.FactoryEventType(roundTripped.Type), roundTripped.Payload); err != nil {
		return FactoryEventSerializationError{
			Kind:  kind,
			Phase: "round-trip-decode-payload",
			Cause: err,
		}
	}

	return nil
}

func decodeFactoryEventPayloadForKind(
	kind recordings.FactoryEventType,
	payload factoryapi.FactoryEvent_Payload,
) error {
	decode, ok := publicFactoryEventPayloadDecoders[kind]
	if !ok {
		return fmt.Errorf("no payload decoder registered for kind %q", kind)
	}
	return decode(payload)
}

var publicFactoryEventPayloadDecoders = map[recordings.FactoryEventType]func(factoryapi.FactoryEvent_Payload) error{
	recordings.FactoryEventTypeRunRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRunRequestEventPayload()
		return err
	},
	recordings.FactoryEventTypeInitialStructureRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInitialStructureRequestEventPayload()
		return err
	},
	recordings.FactoryEventTypeFactoryChange: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsFactoryChangeEventPayload()
		return err
	},
	recordings.FactoryEventTypeWorkRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsWorkRequestEventPayload()
		return err
	},
	recordings.FactoryEventTypeRelationshipChangeRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRelationshipChangeRequestEventPayload()
		return err
	},
	recordings.FactoryEventTypeDispatchRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchRequestEventPayload()
		return err
	},
	recordings.FactoryEventTypeDispatchResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchResponseEventPayload()
		return err
	},
	recordings.FactoryEventTypeFactoryStateResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsFactoryStateResponseEventPayload()
		return err
	},
	recordings.FactoryEventTypeRunResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRunResponseEventPayload()
		return err
	},
	recordings.FactoryEventTypeWorkStateChange: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsWorkStateChangeEventPayload()
		return err
	},
	recordings.FactoryEventTypeInferenceRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInferenceRequestEventPayload()
		return err
	},
	recordings.FactoryEventTypeInferenceResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInferenceResponseEventPayload()
		return err
	},
	recordings.FactoryEventTypeModelRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsModelRequestEventPayload()
		return err
	},
	recordings.FactoryEventTypeModelResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsModelResponseEventPayload()
		return err
	},
	recordings.FactoryEventTypeScriptRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsScriptRequestEventPayload()
		return err
	},
	recordings.FactoryEventTypeScriptResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsScriptResponseEventPayload()
		return err
	},
	recordings.FactoryEventTypeAgentRunResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsAgentRunResponseEventPayload()
		return err
	},
	recordings.FactoryEventTypeSessionStarted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionStartedEventPayload()
		return err
	},
	recordings.FactoryEventTypeSessionPaused: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionPausedEventPayload()
		return err
	},
	recordings.FactoryEventTypeSessionResumed: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionResumedEventPayload()
		return err
	},
	recordings.FactoryEventTypeSessionResultUpdated: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionResultUpdatedEventPayload()
		return err
	},
	recordings.FactoryEventTypeSessionCompleted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionCompletedEventPayload()
		return err
	},
	recordings.FactoryEventTypeSessionLifecycleControl: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionLifecycleControlEventPayload()
		return err
	},
	recordings.FactoryEventTypeOrchestratorPhaseChanged: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsOrchestratorPhaseChangedEventPayload()
		return err
	},
	recordings.FactoryEventTypeOrchestratorCheckpointWritten: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsOrchestratorCheckpointWrittenEventPayload()
		return err
	},
	recordings.FactoryEventTypeDispatchQueued: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchQueuedEventPayload()
		return err
	},
	recordings.FactoryEventTypeDispatchInterrupted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchInterruptedEventPayload()
		return err
	},
	recordings.FactoryEventTypeDispatchReconciled: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchReconciledEventPayload()
		return err
	},
	recordings.FactoryEventTypeArtifactCreated: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsArtifactCreatedEventPayload()
		return err
	},
}
