// Package factoryeventkinds publishes the runtime-facing inventory of public
// FactoryEvent kinds emitted on the canonical factory event history path.
package factoryeventkinds

import (
	"sort"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// PublicEmittableKind names one public FactoryEvent kind the runtime can append
// to canonical factory event history today.
type PublicEmittableKind struct {
	Kind             recordings.FactoryEventType
	EmissionEvidence string
}

// PublicEmittableFactoryEventKinds returns every public FactoryEvent kind
// currently emitted through the factory event history and worker boundary
// recorders. The inventory does not invent kinds; it only names kinds with an
// existing runtime emission path.
func PublicEmittableFactoryEventKinds() []PublicEmittableKind {
	kinds := []PublicEmittableKind{
		{Kind: recordings.FactoryEventTypeRunRequest, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordRunRequest"},
		{Kind: recordings.FactoryEventTypeInitialStructureRequest, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordInitialStructure"},
		{Kind: recordings.FactoryEventTypeFactoryChange, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordFactoryChange"},
		{Kind: recordings.FactoryEventTypeFactoryChangeRequest, EmissionEvidence: "pkg/services/factory_sessions/internal/livechange/service.go appendRequest"},
		{Kind: recordings.FactoryEventTypeFactoryChangeFailed, EmissionEvidence: "pkg/services/factory_sessions/internal/livechange/service.go closeFailure"},
		{Kind: recordings.FactoryEventTypeWorkRequest, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordWorkRequest"},
		{Kind: recordings.FactoryEventTypeRelationshipChangeRequest, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordRelationshipChange"},
		{Kind: recordings.FactoryEventTypeDispatchRequest, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordWorkstationRequest"},
		{Kind: recordings.FactoryEventTypeHumanApprovalRequested, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordHumanApprovalRequested"},
		{Kind: recordings.FactoryEventTypeDispatchWorkerSessionAssoc, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordDispatchWorkerSessionAssociation"},
		{Kind: recordings.FactoryEventTypeDispatchResponse, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordWorkstationResponse"},
		{Kind: recordings.FactoryEventTypeFactoryStateResponse, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordFactoryStateChange"},
		{Kind: recordings.FactoryEventTypeRunResponse, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordRunResponse"},
		{Kind: recordings.FactoryEventTypeWorkStateChange, EmissionEvidence: "pkg/services/recordings/internal/events/event_history.go RecordWorkStateChange"},
		{Kind: recordings.FactoryEventTypeInferenceRequest, EmissionEvidence: "pkg/services/workers/internal/execution/recording/provider.go and pkg/services/recordings/internal/events/event_history.go RecordInferenceEvent"},
		{Kind: recordings.FactoryEventTypeInferenceResponse, EmissionEvidence: "pkg/services/workers/internal/execution/recording/provider.go and pkg/services/recordings/internal/events/event_history.go RecordInferenceEvent"},
		{Kind: recordings.FactoryEventTypeModelRequest, EmissionEvidence: "pkg/services/workers/internal/execution/recording/model.go and pkg/services/recordings/internal/events/event_history.go RecordModelEvent"},
		{Kind: recordings.FactoryEventTypeModelResponse, EmissionEvidence: "pkg/services/workers/internal/execution/recording/model.go and pkg/services/recordings/internal/events/event_history.go RecordModelEvent"},
		{Kind: recordings.FactoryEventTypeScriptRequest, EmissionEvidence: "pkg/services/workers/internal/services/workstations/executor/script.go and pkg/services/recordings/internal/events/event_history.go RecordScriptEvent"},
		{Kind: recordings.FactoryEventTypeScriptResponse, EmissionEvidence: "pkg/services/workers/internal/services/workstations/executor/script.go and pkg/services/recordings/internal/events/event_history.go RecordScriptEvent"},
		{Kind: recordings.FactoryEventTypeAgentRunResponse, EmissionEvidence: "pkg/services/workers/internal/services/workstations/executor/agentrun/events.go and pkg/services/recordings/internal/events/event_history.go RecordAgentRunEvent"},
		{Kind: recordings.FactoryEventTypeSessionStarted, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_session_lifecycle.go RecordSessionStarted"},
		{Kind: recordings.FactoryEventTypeSessionPaused, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_session_lifecycle.go RecordSessionPaused"},
		{Kind: recordings.FactoryEventTypeSessionResumed, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_session_lifecycle.go RecordSessionResumed"},
		{Kind: recordings.FactoryEventTypeSessionResultUpdated, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_session_lifecycle.go RecordSessionResultUpdated"},
		{Kind: recordings.FactoryEventTypeSessionCompleted, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_session_lifecycle.go RecordSessionCompleted"},
		{Kind: recordings.FactoryEventTypeSessionLifecycleControl, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_session_lifecycle.go RecordSessionLifecycleControl"},
		{Kind: recordings.FactoryEventTypeOrchestratorPhaseChanged, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_orchestrator_progress.go RecordOrchestratorPhaseChanged"},
		{Kind: recordings.FactoryEventTypeOrchestratorCheckpointWritten, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_orchestrator_progress.go RecordOrchestratorCheckpointWritten"},
		{Kind: recordings.FactoryEventTypeDispatchQueued, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_dispatch_lifecycle.go RecordDispatchQueued"},
		{Kind: recordings.FactoryEventTypeDispatchInterrupted, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_dispatch_lifecycle.go RecordDispatchInterrupted"},
		{Kind: recordings.FactoryEventTypeDispatchReconciled, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_dispatch_lifecycle.go RecordDispatchReconciled"},
		{Kind: recordings.FactoryEventTypeDispatchResultIgnored, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_dispatch_lifecycle.go RecordDispatchResultIgnored"},
		{Kind: recordings.FactoryEventTypeArtifactCreated, EmissionEvidence: "pkg/services/recordings/internal/events/event_history_dispatch_lifecycle.go RecordArtifactCreated"},
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i].Kind < kinds[j].Kind
	})
	return kinds
}
