package factorysessionexecution

import (
	"encoding/json"
	"time"

	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

// BuiltinInterruptedRecoverableScenario is a deterministic JavaScript session that
// was interrupted with a stale lease and remains recoverable for persisted listing.
func BuiltinInterruptedRecoverableScenario() FakeScenario {
	startedAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	interruptedAt := time.Date(2026, 6, 8, 10, 5, 0, 0, time.UTC)
	sessionID := "dur-sess-js-interrupted-001"
	links := InspectionLinksForSession(sessionID, true)
	session := SessionReadResult{
		SessionID:        sessionID,
		Status:           LifecycleStatusInterrupted,
		OrchestratorKind: "JAVASCRIPT",
		Dialect:          "you-workflow-v1",
		ResolvedSource: ResolvedSource{
			Kind:       workflowsource.KindWorkflowName,
			SourceRef:  "workflow/recoverable-audit",
			SourceHash: "sha256:js-workflow-recoverable-audit",
			Dialect:    "you-workflow-v1",
		},
		SourceHash: "sha256:js-workflow-recoverable-audit",
		Phase:      "audit",
		Progress: &ProgressCounts{
			TotalDispatches:     2,
			CompletedDispatches: 1,
			FailedDispatches:    0,
			InFlightDispatches:  0,
		},
		ResultSummary: &ResultSummary{
			ResultStatus: string(ResultStatusPartial),
			Summary:      "Interrupted after partial audit progress.",
		},
		StaleLease: true,
		Lifecycle: &LifecycleTimestamps{
			StartedAt:     &startedAt,
			InterruptedAt: &interruptedAt,
			UpdatedAt:     &interruptedAt,
		},
		Links: links,
	}
	result := ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  ResultStatusPartial,
		SessionStatus: LifecycleStatusInterrupted,
		Mode:          ResultModePartial,
		PrimaryResult: json.RawMessage(`[{"type":"text","text":"Partial audit notes before interruption."}]`),
	}
	dispatches := []DispatchSummary{
		{
			ID:           "disp-js-interrupted-001",
			Status:       DispatchStatusCompleted,
			DispatchKind: "JAVASCRIPT_AGENT",
			Phase:        "plan",
			Label:        "plan-audit",
			Attempt:      1,
		},
		{
			ID:           "disp-js-interrupted-002",
			Status:       DispatchStatusCanceled,
			DispatchKind: "JAVASCRIPT_AGENT",
			Phase:        "audit",
			Label:        "audit",
			Attempt:      1,
		},
	}
	listSummary := DurableListSummaryFromSessionRead(session)
	return FakeScenario{
		ID:        "javascript-interrupted-recoverable",
		RequestID: "req-js-interrupted-001",
		Session:   session,
		Dispatches: dispatches,
		DispatchDetails: map[string]DispatchDetail{
			"disp-js-interrupted-002": {
				DispatchSummary:  dispatches[1],
				SessionID:        sessionID,
				OrchestratorKind: "JAVASCRIPT",
				JavaScript: &DispatchJavaScriptProjection{
					TaskKind:  "AGENT",
					TaskLabel: "audit",
				},
			},
		},
		Result:      result,
		ListSummary: &listSummary,
		AsyncStart: &AsyncStartResult{
			SessionID:        sessionID,
			Status:           string(LifecycleStatusInterrupted),
			OrchestratorKind: "JAVASCRIPT",
			Dialect:          "you-workflow-v1",
			ResolvedSource:   session.ResolvedSource,
			SourceHash:       session.SourceHash,
			Links:            links,
		},
	}
}
