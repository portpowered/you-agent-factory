package projections_test

import factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

// pkgmaintcheck:ignore-cyclomatic-complexity this event fixture builder keeps the supported replay payload variants on one canonical helper.
func assignGeneratedProjectionPayload(event *factoryapi.FactoryEvent, payload any) {
	switch typed := payload.(type) {
	case factoryapi.RunRequestEventPayload:
		if err := event.Payload.FromRunRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.InitialStructureRequestEventPayload:
		if err := event.Payload.FromInitialStructureRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.FactoryChangeEventPayload:
		if err := event.Payload.FromFactoryChangeEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.WorkRequestEventPayload:
		if err := event.Payload.FromWorkRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.DispatchRequestEventPayload:
		if err := event.Payload.FromDispatchRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.InferenceRequestEventPayload:
		if err := event.Payload.FromInferenceRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.InferenceResponseEventPayload:
		if err := event.Payload.FromInferenceResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.ModelResponseEventPayload:
		if err := event.Payload.FromModelResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.ScriptRequestEventPayload:
		if err := event.Payload.FromScriptRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.ScriptResponseEventPayload:
		if err := event.Payload.FromScriptResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.AgentRunResponseEventPayload:
		if err := event.Payload.FromAgentRunResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.DispatchResponseEventPayload:
		if err := event.Payload.FromDispatchResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.RelationshipChangeRequestEventPayload:
		if err := event.Payload.FromRelationshipChangeRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.FactoryStateResponseEventPayload:
		if err := event.Payload.FromFactoryStateResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.WorkStateChangeEventPayload:
		if err := event.Payload.FromWorkStateChangeEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.RunResponseEventPayload:
		if err := event.Payload.FromRunResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.JavaScriptCheckpointRefEventPayload:
		if err := event.Payload.FromJavaScriptCheckpointRefEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.JavaScriptPhaseChangeEventPayload:
		if err := event.Payload.FromJavaScriptPhaseChangeEventPayload(typed); err != nil {
			panic(err)
		}
	default:
		assignGeneratedProjectionSessionLifecyclePayload(event, payload)
	}
}
