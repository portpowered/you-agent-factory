// Package factoryeventkinds publishes the runtime-facing inventory of public
// FactoryEvent kinds emitted on the canonical factory event history path, plus
// explicit exclusions for internal or non-public event vocabularies.
package factoryeventkinds

import (
	"sort"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// PublicEmittableKind names one public FactoryEvent kind the runtime can append
// to canonical factory event history today.
type PublicEmittableKind struct {
	Kind            factoryapi.FactoryEventType
	EmissionEvidence string
}

// ExcludedNonPublicKind names an event vocabulary intentionally outside the
// public FactoryEvent contract together with reviewable exclusion evidence.
type ExcludedNonPublicKind struct {
	Name     string
	Category string
	Evidence string
}

// ContractOnlyKind names a public FactoryEventType documented in OpenAPI that
// does not yet have a canonical runtime emission path on factory event history.
type ContractOnlyKind struct {
	Kind     factoryapi.FactoryEventType
	Evidence string
}

// PublicEmittableFactoryEventKinds returns every public FactoryEvent kind
// currently emitted through the factory event history and worker boundary
// recorders. The inventory does not invent kinds; it only names kinds with an
// existing runtime emission path.
func PublicEmittableFactoryEventKinds() []PublicEmittableKind {
	kinds := []PublicEmittableKind{
		{Kind: factoryapi.FactoryEventTypeRunRequest, EmissionEvidence: "pkg/factory/events/event_history.go RecordRunRequest"},
		{Kind: factoryapi.FactoryEventTypeInitialStructureRequest, EmissionEvidence: "pkg/factory/events/event_history.go RecordInitialStructure"},
		{Kind: factoryapi.FactoryEventTypeFactoryChange, EmissionEvidence: "pkg/factory/events/event_history.go RecordFactoryChange"},
		{Kind: factoryapi.FactoryEventTypeWorkRequest, EmissionEvidence: "pkg/factory/events/event_history.go RecordWorkRequest"},
		{Kind: factoryapi.FactoryEventTypeRelationshipChangeRequest, EmissionEvidence: "pkg/factory/events/event_history.go RecordRelationshipChange"},
		{Kind: factoryapi.FactoryEventTypeDispatchRequest, EmissionEvidence: "pkg/factory/events/event_history.go RecordWorkstationRequest"},
		{Kind: factoryapi.FactoryEventTypeDispatchResponse, EmissionEvidence: "pkg/factory/events/event_history.go RecordWorkstationResponse"},
		{Kind: factoryapi.FactoryEventTypeFactoryStateResponse, EmissionEvidence: "pkg/factory/events/event_history.go RecordFactoryStateChange"},
		{Kind: factoryapi.FactoryEventTypeRunResponse, EmissionEvidence: "pkg/factory/events/event_history.go RecordRunResponse"},
		{Kind: factoryapi.FactoryEventTypeWorkStateChange, EmissionEvidence: "pkg/factory/events/event_history.go RecordWorkStateChange"},
		{Kind: factoryapi.FactoryEventTypeInferenceRequest, EmissionEvidence: "pkg/workers/provider/recording_provider.go and pkg/factory/events/event_history.go RecordInferenceEvent"},
		{Kind: factoryapi.FactoryEventTypeInferenceResponse, EmissionEvidence: "pkg/workers/provider/recording_provider.go and pkg/factory/events/event_history.go RecordInferenceEvent"},
		{Kind: factoryapi.FactoryEventTypeModelRequest, EmissionEvidence: "pkg/service/factory_runtime_state.go and pkg/factory/events/event_history.go RecordModelEvent"},
		{Kind: factoryapi.FactoryEventTypeModelResponse, EmissionEvidence: "pkg/service/factory_runtime_state.go and pkg/factory/events/event_history.go RecordModelEvent"},
		{Kind: factoryapi.FactoryEventTypeScriptRequest, EmissionEvidence: "pkg/workers/executor/script.go and pkg/factory/events/event_history.go RecordScriptEvent"},
		{Kind: factoryapi.FactoryEventTypeScriptResponse, EmissionEvidence: "pkg/workers/executor/script.go and pkg/factory/events/event_history.go RecordScriptEvent"},
		{Kind: factoryapi.FactoryEventTypeAgentRunResponse, EmissionEvidence: "pkg/workers/executor/agentrun/events.go and pkg/factory/events/event_history.go RecordAgentRunEvent"},
		{Kind: factoryapi.FactoryEventTypeSessionStarted, EmissionEvidence: "pkg/factory/events/event_history_session_lifecycle.go RecordSessionStarted"},
		{Kind: factoryapi.FactoryEventTypeSessionPaused, EmissionEvidence: "pkg/factory/events/event_history_session_lifecycle.go RecordSessionPaused"},
		{Kind: factoryapi.FactoryEventTypeSessionResumed, EmissionEvidence: "pkg/factory/events/event_history_session_lifecycle.go RecordSessionResumed"},
		{Kind: factoryapi.FactoryEventTypeSessionResultUpdated, EmissionEvidence: "pkg/factory/events/event_history_session_lifecycle.go RecordSessionResultUpdated"},
		{Kind: factoryapi.FactoryEventTypeSessionCompleted, EmissionEvidence: "pkg/factory/events/event_history_session_lifecycle.go RecordSessionCompleted"},
		{Kind: factoryapi.FactoryEventTypeSessionLifecycleControl, EmissionEvidence: "pkg/factory/events/event_history_session_lifecycle.go RecordSessionLifecycleControl"},
		{Kind: factoryapi.FactoryEventTypeOrchestratorPhaseChanged, EmissionEvidence: "pkg/factory/events/event_history_orchestrator_progress.go RecordOrchestratorPhaseChanged"},
		{Kind: factoryapi.FactoryEventTypeOrchestratorCheckpointWritten, EmissionEvidence: "pkg/factory/events/event_history_orchestrator_progress.go RecordOrchestratorCheckpointWritten"},
		{Kind: factoryapi.FactoryEventTypeDispatchQueued, EmissionEvidence: "pkg/factory/events/event_history_dispatch_lifecycle.go RecordDispatchQueued"},
		{Kind: factoryapi.FactoryEventTypeDispatchInterrupted, EmissionEvidence: "pkg/factory/events/event_history_dispatch_lifecycle.go RecordDispatchInterrupted"},
		{Kind: factoryapi.FactoryEventTypeDispatchReconciled, EmissionEvidence: "pkg/factory/events/event_history_dispatch_lifecycle.go RecordDispatchReconciled"},
		{Kind: factoryapi.FactoryEventTypeArtifactCreated, EmissionEvidence: "pkg/factory/events/event_history_dispatch_lifecycle.go RecordArtifactCreated"},
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i].Kind < kinds[j].Kind
	})
	return kinds
}

// ExcludedNonPublicFactoryEventKinds returns internal or non-public event
// vocabularies that must never be silently omitted from parity work.
func ExcludedNonPublicFactoryEventKinds() []ExcludedNonPublicKind {
	excluded := []ExcludedNonPublicKind{
		{
			Name:     "FactoryResponseEvent",
			Category: "response-stream",
			Evidence: "FactoryResponseEvent is an ephemeral Factory Session observation stream with its own OpenAPI schema family under api/components/schemas/response-events/. It is intentionally separate from canonical FactoryEvent replay state; see pkg/interfaces/responseevents/types.go and pkg/factory/sessions/responsestream/types.go.",
		},
		{
			Name:     "responsestream.EventKind",
			Category: "internal-response-stream",
			Evidence: "Session response-stream record kinds such as PROGRESS_FRAGMENT and RESPONSE_FRAGMENT are internal retention records and are explicitly not projected into canonical factory event history; see pkg/factory/sessions/responsestream/types.go EventKind.",
		},
		{
			Name:     "responsestream.EventType",
			Category: "internal-response-stream",
			Evidence: "Provider-neutral response-stream semantic shapes (STARTED, TEXT_DELTA, FAILED, etc.) are adapter-facing transport records and are not FactoryEventType values; see pkg/factory/sessions/responsestream/types.go EventType.",
		},
		{
			Name:     "RUN_STARTED",
			Category: "retired-factory-event-vocabulary",
			Evidence: "Retired public FactoryEvent alias replaced by RUN_REQUEST. Runtime emission uses RUN_REQUEST only; retired values remain documented only for replay migration guards in pkg/transports/http/contracttests/openapi_contract_common_test.go.",
		},
		{
			Name:     "INITIAL_STRUCTURE",
			Category: "retired-factory-event-vocabulary",
			Evidence: "Retired public FactoryEvent alias replaced by INITIAL_STRUCTURE_REQUEST. Runtime emission uses INITIAL_STRUCTURE_REQUEST only.",
		},
		{
			Name:     "RELATIONSHIP_CHANGE",
			Category: "retired-factory-event-vocabulary",
			Evidence: "Retired public FactoryEvent alias replaced by RELATIONSHIP_CHANGE_REQUEST. Runtime emission uses RELATIONSHIP_CHANGE_REQUEST only.",
		},
		{
			Name:     "DISPATCH_CREATED",
			Category: "retired-factory-event-vocabulary",
			Evidence: "Retired public FactoryEvent alias replaced by DISPATCH_REQUEST. Runtime emission uses DISPATCH_REQUEST only.",
		},
		{
			Name:     "DISPATCH_COMPLETED",
			Category: "retired-factory-event-vocabulary",
			Evidence: "Retired public FactoryEvent alias replaced by DISPATCH_RESPONSE. Runtime emission uses DISPATCH_RESPONSE only.",
		},
		{
			Name:     "FACTORY_STATE_CHANGE",
			Category: "retired-factory-event-vocabulary",
			Evidence: "Retired public FactoryEvent alias replaced by FACTORY_STATE_RESPONSE. Runtime emission uses FACTORY_STATE_RESPONSE only.",
		},
		{
			Name:     "RUN_FINISHED",
			Category: "retired-factory-event-vocabulary",
			Evidence: "Retired public FactoryEvent alias replaced by RUN_RESPONSE. Runtime emission uses RUN_RESPONSE only.",
		},
	}
	sort.Slice(excluded, func(i, j int) bool {
		if excluded[i].Category == excluded[j].Category {
			return excluded[i].Name < excluded[j].Name
		}
		return excluded[i].Category < excluded[j].Category
	})
	return excluded
}

// ContractOnlyFactoryEventKinds returns public FactoryEventType values that are
// authored in OpenAPI but intentionally absent from the runtime-emittable public
// inventory because no canonical Record* emission path exists yet.
func ContractOnlyFactoryEventKinds() []ContractOnlyKind {
	kinds := []ContractOnlyKind{
		{
			Kind: factoryapi.FactoryEventTypeJavaScriptCheckpointRef,
			Evidence: "Authored OpenAPI and fixture vocabulary for JavaScript workflow checkpoint refs. Canonical durable runtime emission uses ORCHESTRATOR_CHECKPOINT_WRITTEN via pkg/factory/events/event_history_orchestrator_progress.go RecordOrchestratorCheckpointWritten; projection_consistency.go accepts JAVASCRIPT_CHECKPOINT_REF for replay compatibility only.",
		},
		{
			Kind: factoryapi.FactoryEventTypeJavaScriptPhaseChange,
			Evidence: "Authored OpenAPI and fixture vocabulary for JavaScript workflow phase transitions. Canonical durable runtime emission uses ORCHESTRATOR_PHASE_CHANGED via pkg/factory/events/event_history_orchestrator_progress.go RecordOrchestratorPhaseChanged; projection_consistency.go accepts JAVASCRIPT_PHASE_CHANGE for replay compatibility only.",
		},
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i].Kind < kinds[j].Kind
	})
	return kinds
}
