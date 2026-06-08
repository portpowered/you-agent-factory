package factorysessions

import (
	"encoding/json"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestProjectRuntime_LegacyPetriSessionIncludesMarkingAndEnabledTransitions(t *testing.T) {
	now := time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC)
	token := &interfaces.Token{
		ID:      "tok-init",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			WorkID:     "work-1",
			WorkTypeID: "task",
			TraceID:    "trace-1",
		},
		CreatedAt: now,
		EnteredAt: now,
	}
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusIdle,
		FactoryState:  "RUNNING",
		InFlightCount: 0,
		Uptime:        2 * time.Minute,
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{"tok-init": token},
		},
		Topology: &state.Net{
			Places: map[string]*petri.Place{
				"task:init": {ID: "task:init", TypeID: "task", State: "init"},
			},
			WorkTypes: map[string]*state.WorkType{
				"task": {
					ID: "task",
					States: []state.StateDefinition{
						{Value: "init", Category: state.StateCategoryInitial},
					},
				},
			},
		},
	}
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{ID: "~default", IsDefault: true},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "legacy-petri",
		},
		Snapshot: snapshot,
		Enabled: []interfaces.EnabledTransition{{
			TransitionID: "tr-process",
			WorkerType:   "worker-a",
		}},
		Now: now,
	})
	if runtime.OrchestratorKind != factoryapi.PETRI {
		t.Fatalf("orchestrator kind = %q, want PETRI", runtime.OrchestratorKind)
	}
	if runtime.Petri == nil || len(runtime.Petri.Marking) != 1 {
		t.Fatalf("petri projection = %#v, want one marking token", runtime.Petri)
	}
	if len(runtime.Petri.EnabledTransitions) != 1 || runtime.Petri.EnabledTransitions[0].TransitionId != "tr-process" {
		t.Fatalf("enabled transitions = %#v, want tr-process", runtime.Petri.EnabledTransitions)
	}
	if runtime.Status != factoryapi.FactorySessionStatusIDLE {
		t.Fatalf("status = %q, want IDLE", runtime.Status)
	}
	if runtime.Progress.TotalTokens != 1 || runtime.Progress.FactoryState != "RUNNING" {
		t.Fatalf("progress = %#v, want one token and RUNNING factory state", runtime.Progress)
	}
	if runtime.Javascript != nil {
		t.Fatalf("javascript projection = %#v, want nil for Petri session", runtime.Javascript)
	}
}

func TestProjectRuntime_JavaScriptWorkflowSessionIncludesPhaseAndCheckpointRefs(t *testing.T) {
	now := time.Date(2026, 6, 8, 14, 5, 0, 0, time.UTC)
	argsSchema := json.RawMessage(`{"type":"object","properties":{"topic":{"type":"string"}}}`)
	defaultPolicy := json.RawMessage(`{"maxAgents":3}`)
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{ID: "session-js", Project: "dynamic-workflow"},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "dynamic-workflow",
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
				JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
					Dialect:       "workflow-v1",
					SourceRef:     "factory/workflows/review.js",
					SourceHash:    "sha256:abc123",
					ArgsSchema:    argsSchema,
					DefaultPolicy: defaultPolicy,
				},
			},
		},
		JavaScript: &interfaces.FactorySessionJavaScriptRuntimeState{
			Phase:      "review",
			Phases:     []string{"plan", "review"},
			ArgsDigest: "sha256:args-digest",
			Checkpoints: []interfaces.FactorySessionJavaScriptCheckpointRef{{
				ID:        "ckpt-1",
				Label:     "after-plan",
				Summary:   "Completed planning phase",
				Timestamp: now,
			}},
			ScriptStatus:        "RUNNING",
			QueuedDispatches:    1,
			RunningDispatches:   2,
			CompletedDispatches: 4,
		},
		Now: now,
	})
	if runtime.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("orchestrator kind = %q, want JAVASCRIPT", runtime.OrchestratorKind)
	}
	if runtime.Dialect == nil || *runtime.Dialect != "workflow-v1" {
		t.Fatalf("dialect = %#v, want workflow-v1", runtime.Dialect)
	}
	if runtime.SourceRef == nil || *runtime.SourceRef != "factory/workflows/review.js" {
		t.Fatalf("source ref = %#v", runtime.SourceRef)
	}
	if runtime.PolicyHash == nil || *runtime.PolicyHash == "" {
		t.Fatalf("policy hash = %#v, want digest", runtime.PolicyHash)
	}
	if runtime.Petri != nil {
		t.Fatalf("petri projection = %#v, want nil for JavaScript session", runtime.Petri)
	}
	if runtime.Javascript == nil {
		t.Fatal("javascript projection is nil")
	}
	if runtime.Javascript.Phase == nil || *runtime.Javascript.Phase != "review" {
		t.Fatalf("phase = %#v, want review", runtime.Javascript.Phase)
	}
	if runtime.Javascript.ArgsDigest == nil || *runtime.Javascript.ArgsDigest != "sha256:args-digest" {
		t.Fatalf("args digest = %#v", runtime.Javascript.ArgsDigest)
	}
	if runtime.Javascript.Checkpoints == nil || len(*runtime.Javascript.Checkpoints) != 1 {
		t.Fatalf("checkpoints = %#v, want one checkpoint ref", runtime.Javascript.Checkpoints)
	}
	if runtime.Javascript.ScriptStatus != factoryapi.FactorySessionJavaScriptScriptStatusRUNNING {
		t.Fatalf("script status = %q, want RUNNING", runtime.Javascript.ScriptStatus)
	}
	if runtime.Javascript.ChildDispatchCounts.Queued != 1 || runtime.Javascript.ChildDispatchCounts.Running != 2 || runtime.Javascript.ChildDispatchCounts.Completed != 4 {
		t.Fatalf("child dispatch counts = %#v", runtime.Javascript.ChildDispatchCounts)
	}
	if runtime.Budgets == nil || runtime.Budgets.MaxAgents == nil || *runtime.Budgets.MaxAgents != 3 {
		t.Fatalf("budgets = %#v, want maxAgents=3", runtime.Budgets)
	}
}
