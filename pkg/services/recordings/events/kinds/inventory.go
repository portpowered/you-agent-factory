// Package factoryeventkinds publishes the Recordings-owned compile-time
// inventory of public FactoryEvent kinds emitted on the canonical factory event
// history path, plus explicit exclusions for internal or non-public event
// vocabularies and classified contract-only OpenAPI kinds. Ownership/parity
// consumers in this packet must use PublicEmittableFactoryEventKinds,
// ExcludedNonPublicFactoryEventKinds, and ContractOnlyFactoryEventKinds as the
// sole public Factory Event kind inventory API surface.
package factoryeventkinds

import (
	"fmt"
	"sort"
	"strings"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// FactoryEventKindInventory is the Recordings-owned compile-time vocabulary
// surface for public emittable, excluded non-public, and contract-only kinds.
type FactoryEventKindInventory struct {
	PublicEmittable []PublicEmittableKind
	Excluded        []ExcludedNonPublicKind
	ContractOnly    []ContractOnlyKind
}

// PublicEmittableKind names one public FactoryEvent kind the runtime can append
// to canonical factory event history today.
type PublicEmittableKind struct {
	Kind             factorycontracts.FactoryEventType
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
	Kind     factorycontracts.FactoryEventType
	Evidence string
}

// PublicEmittableFactoryEventKinds returns every public FactoryEvent kind
// currently emitted through the factory event history and worker boundary
// recorders. The inventory does not invent kinds; it only names kinds with an
// existing runtime emission path.
func PublicEmittableFactoryEventKinds() []PublicEmittableKind {
	kinds := []PublicEmittableKind{
		{Kind: factorycontracts.FactoryEventTypeRunRequest, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordRunRequest"},
		{Kind: factorycontracts.FactoryEventTypeInitialStructureRequest, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordInitialStructure"},
		{Kind: factorycontracts.FactoryEventTypeFactoryChange, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordFactoryChange"},
		{Kind: factorycontracts.FactoryEventTypeWorkRequest, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordWorkRequest"},
		{Kind: factorycontracts.FactoryEventTypeRelationshipChangeRequest, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordRelationshipChange"},
		{Kind: factorycontracts.FactoryEventTypeDispatchRequest, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordWorkstationRequest"},
		{Kind: factorycontracts.FactoryEventTypeDispatchResponse, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordWorkstationResponse"},
		{Kind: factorycontracts.FactoryEventTypeFactoryStateResponse, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordFactoryStateChange"},
		{Kind: factorycontracts.FactoryEventTypeRunResponse, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordRunResponse"},
		{Kind: factorycontracts.FactoryEventTypeWorkStateChange, EmissionEvidence: "pkg/services/recordings/events/event_history.go RecordWorkStateChange"},
		{Kind: factorycontracts.FactoryEventTypeInferenceRequest, EmissionEvidence: "pkg/services/workers/provider/recording_provider.go and pkg/services/recordings/events/event_history.go RecordInferenceEvent"},
		{Kind: factorycontracts.FactoryEventTypeInferenceResponse, EmissionEvidence: "pkg/services/workers/provider/recording_provider.go and pkg/services/recordings/events/event_history.go RecordInferenceEvent"},
		{Kind: factorycontracts.FactoryEventTypeModelRequest, EmissionEvidence: "pkg/services/workers/execution/recording/model.go and pkg/services/recordings/events/event_history.go RecordModelEvent"},
		{Kind: factorycontracts.FactoryEventTypeModelResponse, EmissionEvidence: "pkg/services/workers/execution/recording/model.go and pkg/services/recordings/events/event_history.go RecordModelEvent"},
		{Kind: factorycontracts.FactoryEventTypeScriptRequest, EmissionEvidence: "pkg/services/workers/executor/script.go and pkg/services/recordings/events/event_history.go RecordScriptEvent"},
		{Kind: factorycontracts.FactoryEventTypeScriptResponse, EmissionEvidence: "pkg/services/workers/executor/script.go and pkg/services/recordings/events/event_history.go RecordScriptEvent"},
		{Kind: factorycontracts.FactoryEventTypeAgentRunResponse, EmissionEvidence: "pkg/services/workers/executor/agentrun/events.go and pkg/services/recordings/events/event_history.go RecordAgentRunEvent"},
		{Kind: factorycontracts.FactoryEventTypeSessionStarted, EmissionEvidence: "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionStarted"},
		{Kind: factorycontracts.FactoryEventTypeSessionPaused, EmissionEvidence: "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionPaused"},
		{Kind: factorycontracts.FactoryEventTypeSessionResumed, EmissionEvidence: "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionResumed"},
		{Kind: factorycontracts.FactoryEventTypeSessionResultUpdated, EmissionEvidence: "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionResultUpdated"},
		{Kind: factorycontracts.FactoryEventTypeSessionCompleted, EmissionEvidence: "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionCompleted"},
		{Kind: factorycontracts.FactoryEventTypeSessionLifecycleControl, EmissionEvidence: "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionLifecycleControl"},
		{Kind: factorycontracts.FactoryEventTypeOrchestratorPhaseChanged, EmissionEvidence: "pkg/services/recordings/events/event_history_orchestrator_progress.go RecordOrchestratorPhaseChanged"},
		{Kind: factorycontracts.FactoryEventTypeOrchestratorCheckpointWritten, EmissionEvidence: "pkg/services/recordings/events/event_history_orchestrator_progress.go RecordOrchestratorCheckpointWritten"},
		{Kind: factorycontracts.FactoryEventTypeDispatchQueued, EmissionEvidence: "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordDispatchQueued"},
		{Kind: factorycontracts.FactoryEventTypeDispatchInterrupted, EmissionEvidence: "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordDispatchInterrupted"},
		{Kind: factorycontracts.FactoryEventTypeDispatchReconciled, EmissionEvidence: "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordDispatchReconciled"},
		{Kind: factorycontracts.FactoryEventTypeArtifactCreated, EmissionEvidence: "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordArtifactCreated"},
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
			Kind:     factorycontracts.FactoryEventTypeJavaScriptCheckpointRef,
			Evidence: "Authored OpenAPI and fixture vocabulary for JavaScript workflow checkpoint refs. Canonical durable runtime emission uses ORCHESTRATOR_CHECKPOINT_WRITTEN via pkg/services/recordings/events/event_history_orchestrator_progress.go RecordOrchestratorCheckpointWritten; projection_consistency.go accepts JAVASCRIPT_CHECKPOINT_REF for replay compatibility only.",
		},
		{
			Kind:     factorycontracts.FactoryEventTypeJavaScriptPhaseChange,
			Evidence: "Authored OpenAPI and fixture vocabulary for JavaScript workflow phase transitions. Canonical durable runtime emission uses ORCHESTRATOR_PHASE_CHANGED via pkg/services/recordings/events/event_history_orchestrator_progress.go RecordOrchestratorPhaseChanged; projection_consistency.go accepts JAVASCRIPT_PHASE_CHANGE for replay compatibility only.",
		},
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i].Kind < kinds[j].Kind
	})
	return kinds
}

// RequiredExcludedNonPublicKinds returns the exclusion identities that must
// never be silently omitted from the Recordings-owned inventory: Factory Session
// response-stream vocabulary and retired public FactoryEvent aliases.
func RequiredExcludedNonPublicKinds() []ExcludedNonPublicKind {
	required := []ExcludedNonPublicKind{
		{Name: "FactoryResponseEvent", Category: "response-stream"},
		{Name: "responsestream.EventKind", Category: "internal-response-stream"},
		{Name: "responsestream.EventType", Category: "internal-response-stream"},
		{Name: "RUN_STARTED", Category: "retired-factory-event-vocabulary"},
		{Name: "INITIAL_STRUCTURE", Category: "retired-factory-event-vocabulary"},
		{Name: "RELATIONSHIP_CHANGE", Category: "retired-factory-event-vocabulary"},
		{Name: "DISPATCH_CREATED", Category: "retired-factory-event-vocabulary"},
		{Name: "DISPATCH_COMPLETED", Category: "retired-factory-event-vocabulary"},
		{Name: "FACTORY_STATE_CHANGE", Category: "retired-factory-event-vocabulary"},
		{Name: "RUN_FINISHED", Category: "retired-factory-event-vocabulary"},
	}
	sort.Slice(required, func(i, j int) bool {
		if required[i].Category == required[j].Category {
			return required[i].Name < required[j].Name
		}
		return required[i].Category < required[j].Category
	})
	return required
}

// ValidateFactoryEventKindInventory fails closed when the Recordings-owned
// vocabulary inventory is missing evidence, contains duplicates, is unsorted,
// silently omits a required exclusion, or overlaps public and contract-only kinds.
func ValidateFactoryEventKindInventory(inventory FactoryEventKindInventory) error {
	publicSeen := make(map[factorycontracts.FactoryEventType]struct{}, len(inventory.PublicEmittable))
	for i, entry := range inventory.PublicEmittable {
		if strings.TrimSpace(string(entry.Kind)) == "" {
			return fmt.Errorf("public emittable inventory entry %d has an empty kind", i)
		}
		if strings.TrimSpace(entry.EmissionEvidence) == "" {
			return fmt.Errorf("public emittable kind %q missing emission evidence", entry.Kind)
		}
		if _, ok := publicSeen[entry.Kind]; ok {
			return fmt.Errorf("public emittable inventory contains duplicate kind %q", entry.Kind)
		}
		publicSeen[entry.Kind] = struct{}{}
		if i > 0 && inventory.PublicEmittable[i-1].Kind >= entry.Kind {
			return fmt.Errorf(
				"public emittable inventory is not strictly sorted by kind: %q then %q",
				inventory.PublicEmittable[i-1].Kind,
				entry.Kind,
			)
		}
	}

	excludedSeen := make(map[string]struct{}, len(inventory.Excluded))
	for i, entry := range inventory.Excluded {
		if strings.TrimSpace(entry.Name) == "" {
			return fmt.Errorf("excluded inventory entry %d has an empty name", i)
		}
		if strings.TrimSpace(entry.Category) == "" {
			return fmt.Errorf("excluded kind %q missing category", entry.Name)
		}
		if strings.TrimSpace(entry.Evidence) == "" {
			return fmt.Errorf("excluded kind %q missing exclusion evidence", entry.Name)
		}
		key := entry.Category + "\x00" + entry.Name
		if _, ok := excludedSeen[key]; ok {
			return fmt.Errorf("excluded inventory contains duplicate entry %q in category %q", entry.Name, entry.Category)
		}
		excludedSeen[key] = struct{}{}
		if i > 0 {
			prev := inventory.Excluded[i-1]
			if prev.Category > entry.Category ||
				(prev.Category == entry.Category && prev.Name >= entry.Name) {
				return fmt.Errorf(
					"excluded inventory is not strictly sorted by category then name: %q/%q then %q/%q",
					prev.Category,
					prev.Name,
					entry.Category,
					entry.Name,
				)
			}
		}
	}

	for _, required := range RequiredExcludedNonPublicKinds() {
		key := required.Category + "\x00" + required.Name
		if _, ok := excludedSeen[key]; !ok {
			return fmt.Errorf(
				"required exclusion %q in category %q silently omitted from excluded inventory",
				required.Name,
				required.Category,
			)
		}
	}

	contractSeen := make(map[factorycontracts.FactoryEventType]struct{}, len(inventory.ContractOnly))
	for i, entry := range inventory.ContractOnly {
		if strings.TrimSpace(string(entry.Kind)) == "" {
			return fmt.Errorf("contract-only inventory entry %d has an empty kind", i)
		}
		if strings.TrimSpace(entry.Evidence) == "" {
			return fmt.Errorf("contract-only kind %q missing evidence", entry.Kind)
		}
		if _, ok := contractSeen[entry.Kind]; ok {
			return fmt.Errorf("contract-only inventory contains duplicate kind %q", entry.Kind)
		}
		if _, ok := publicSeen[entry.Kind]; ok {
			return fmt.Errorf("contract-only kind %q also appears in the public emittable inventory", entry.Kind)
		}
		contractSeen[entry.Kind] = struct{}{}
		if i > 0 && inventory.ContractOnly[i-1].Kind >= entry.Kind {
			return fmt.Errorf(
				"contract-only inventory is not strictly sorted by kind: %q then %q",
				inventory.ContractOnly[i-1].Kind,
				entry.Kind,
			)
		}
	}

	return nil
}
