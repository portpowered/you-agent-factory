package replay_contracts

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
)

func TestJavaScriptFactorySessionRuntimeProjectionExposesReplayIdentityAndProgress(t *testing.T) {
	now := time.Date(2026, 7, 18, 9, 30, 0, 0, time.UTC)
	startedAt := now.Add(-5 * time.Minute)
	runtime := factorysessions.ProjectRuntimeContract(factorysessions.ProjectionContext{
		Session: &factorysessions.LiveSession{ID: "session-replay-projection"},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "replay-projection",
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
				JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
					Dialect:       "workflow-v1",
					SourceRef:     "factory/workflows/replay.js",
					SourceHash:    "sha256:replay-source",
					ArgsSchema:    json.RawMessage(`{"type":"object"}`),
					DefaultPolicy: json.RawMessage(`{"maxAgents":3}`),
				},
			},
		},
		JavaScript: &interfaces.FactorySessionJavaScriptRuntimeState{
			Phase:               "project",
			Phases:              []string{"load", "project"},
			ArgsDigest:          "sha256:replay-args",
			ScriptStatus:        "RUNNING",
			QueuedDispatches:    1,
			RunningDispatches:   2,
			CompletedDispatches: 4,
			Checkpoints: []interfaces.FactorySessionJavaScriptCheckpointRef{{
				ID:        "checkpoint-project",
				Label:     "projected",
				Summary:   "Replay projection is available",
				Timestamp: now,
			}},
		},
		BackendScopeID:      "backend-replay",
		LogicalSessionKeyID: "logical-replay",
		RuntimeStartedAt:    startedAt,
		Now:                 now,
	})

	if runtime.OrchestratorKind != interfaces.OrchestratorKindJavaScript || runtime.JavaScript == nil {
		t.Fatalf("runtime = %#v, want JavaScript runtime projection", runtime)
	}
	if runtime.StreamIdentity == nil ||
		runtime.StreamIdentity.FactorySessionID != "session-replay-projection" ||
		runtime.StreamIdentity.StreamGenerationID != startedAt.Format(time.RFC3339Nano) {
		t.Fatalf("stream identity = %#v, want stable session/start identity", runtime.StreamIdentity)
	}
	if runtime.JavaScript.Phase == nil || *runtime.JavaScript.Phase != "project" ||
		runtime.JavaScript.ScriptStatus != interfaces.FactorySessionJavaScriptScriptStatusRunning ||
		runtime.JavaScript.ChildDispatchCounts.Queued != 1 ||
		runtime.JavaScript.ChildDispatchCounts.Running != 2 ||
		runtime.JavaScript.ChildDispatchCounts.Completed != 4 {
		t.Fatalf("javascript projection = %#v, want selected replay progress", runtime.JavaScript)
	}
	if runtime.JavaScript.Checkpoints == nil || len(*runtime.JavaScript.Checkpoints) != 1 ||
		(*runtime.JavaScript.Checkpoints)[0].ID != "checkpoint-project" {
		t.Fatalf("checkpoints = %#v, want checkpoint-project", runtime.JavaScript.Checkpoints)
	}
	if runtime.Budgets == nil || runtime.Budgets.MaxAgents == nil || *runtime.Budgets.MaxAgents != 3 {
		t.Fatalf("budgets = %#v, want maxAgents=3", runtime.Budgets)
	}
}
