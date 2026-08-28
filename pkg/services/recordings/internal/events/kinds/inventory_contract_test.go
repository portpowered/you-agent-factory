package factoryeventkinds

import (
	"sort"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// ExcludedNonPublicKind names an event vocabulary intentionally outside the
// public FactoryEvent contract together with reviewable exclusion evidence.
// This inventory is contract-test data, not runtime admission state.
type ExcludedNonPublicKind struct {
	Name     string
	Category string
	Evidence string
}

// ContractOnlyKind names a public FactoryEventType documented in OpenAPI that
// does not yet have a canonical runtime emission path on factory event history.
// This inventory is contract-test data, not runtime admission state.
type ContractOnlyKind struct {
	Kind     recordings.FactoryEventType
	Evidence string
}

// ExcludedNonPublicFactoryEventKinds returns internal or non-public event
// vocabularies that must never be silently omitted from parity work.
func ExcludedNonPublicFactoryEventKinds() []ExcludedNonPublicKind {
	excluded := []ExcludedNonPublicKind{
		{
			Name:     "FactoryResponseEvent",
			Category: "response-stream",
			Evidence: "FactoryResponseEvent is an ephemeral Factory Session observation stream with its own OpenAPI schema family under api/components/schemas/response-events/. It is intentionally separate from canonical FactoryEvent replay state; see pkg/services/factory_sessions/internal/responseevents/types.go and pkg/services/factory_sessions/internal/responsestream/types.go.",
		},
		{
			Name:     "responsestream.EventKind",
			Category: "internal-response-stream",
			Evidence: "Session response-stream record kinds such as PROGRESS_FRAGMENT and RESPONSE_FRAGMENT are internal retention records and are explicitly not projected into canonical factory event history; see pkg/services/factory_sessions/internal/responsestream/types.go EventKind.",
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
			Kind:     recordings.FactoryEventTypeJavaScriptCheckpointRef,
			Evidence: "Authored OpenAPI and fixture vocabulary for JavaScript workflow checkpoint refs. Canonical durable runtime emission uses ORCHESTRATOR_CHECKPOINT_WRITTEN via pkg/factory/events/event_history_orchestrator_progress.go RecordOrchestratorCheckpointWritten; projection_consistency.go accepts JAVASCRIPT_CHECKPOINT_REF for replay compatibility only.",
		},
		{
			Kind:     recordings.FactoryEventTypeJavaScriptPhaseChange,
			Evidence: "Authored OpenAPI and fixture vocabulary for JavaScript workflow phase transitions. Canonical durable runtime emission uses ORCHESTRATOR_PHASE_CHANGED via pkg/factory/events/event_history_orchestrator_progress.go RecordOrchestratorPhaseChanged; projection_consistency.go accepts JAVASCRIPT_PHASE_CHANGE for replay compatibility only.",
		},
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i].Kind < kinds[j].Kind
	})
	return kinds
}
