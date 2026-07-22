package factorysessions_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	. "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// projectionCheckpointStore is a static Factory Runtime root-contract script.
// Checkpoint persistence, replacement, and ordering remain covered by the
// Factory Runtime-owned checkpointstore suite.
type projectionCheckpointStore struct {
	records []interfaces.JavaScriptCheckpointRecord
}

func (projectionCheckpointStore) Put(interfaces.JavaScriptCheckpointRecord) {}

func (store projectionCheckpointStore) List() []interfaces.JavaScriptCheckpointRecord {
	return append([]interfaces.JavaScriptCheckpointRecord(nil), store.records...)
}

func (store projectionCheckpointStore) Get(id string) (interfaces.JavaScriptCheckpointRecord, bool) {
	for _, record := range store.records {
		if record.ID == id {
			return record, true
		}
	}
	return interfaces.JavaScriptCheckpointRecord{}, false
}

var _ factoryruntime.JavaScriptCheckpointStore = projectionCheckpointStore{}

type sessionResultProjectionRole struct{}

func (sessionResultProjectionRole) ProjectSessionResults(
	input factoryruntime.SessionResultInput,
) factoryruntime.SessionResultProjection {
	result := factoryruntime.LiveSessionResult{
		SessionID:      strings.TrimSpace(input.SessionID),
		Status:         input.Status,
		CheckpointRefs: append([]interfaces.FactorySessionJavaScriptCheckpointEventRef(nil), input.CheckpointRefs...),
	}
	if input.ResultArtifact != nil {
		ref := *input.ResultArtifact
		result.ResultArtifactRef = &ref
	}
	return factoryruntime.SessionResultProjection{Live: result}
}

func ProjectRuntime(ctx ProjectionContext) factoryapi.FactorySessionRuntime {
	return factorysessionmapping.RuntimeProjectionToAPI(ProjectRuntimeContract(ctx), ctx.NormalizedTarget)
}

func SessionResponse(ctx ProjectionContext) factoryapi.FactorySession {
	return factorysessionmapping.SessionResponseToAPI(SessionProjection{
		Context: ctx,
		Runtime: ProjectRuntimeContract(ctx),
	})
}

func TestBuildProjectionContextRequiresExplicitProjectionTime(t *testing.T) {
	if _, err := BuildProjectionContext(ProjectionBuildInput{}); err == nil ||
		!strings.Contains(err.Error(), "projection time is required") {
		t.Fatalf("BuildProjectionContext() error = %v, want required projection time", err)
	}

	now := time.Date(2026, 7, 20, 16, 45, 0, 0, time.UTC)
	ctx, err := BuildProjectionContext(ProjectionBuildInput{Now: now})
	if err != nil {
		t.Fatalf("BuildProjectionContext() error = %v", err)
	}
	if !ctx.Now.Equal(now) {
		t.Fatalf("projection time = %v, want %v", ctx.Now, now)
	}
	projection := ProjectRuntimeContract(ctx)
	if !projection.Lifecycle.UpdatedAt.Equal(now) {
		t.Fatalf("projection updated at = %v, want %v", projection.Lifecycle.UpdatedAt, now)
	}
}

func TestProjectRuntime_LegacyPetriSessionIncludesMarkingAndEnabledTransitions(t *testing.T) {
	now := time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC)
	token := &factoryruntime.RuntimeToken{
		ID:      "tok-init",
		PlaceID: "task:init",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID:     "work-1",
			WorkTypeID: "task",
			TraceID:    "trace-1",
		},
		CreatedAt: now,
		EnteredAt: now,
		History: factoryruntime.RuntimeTokenHistory{
			ConsecutiveFailures: map[string]int{"tr-process": 1},
			LastError:           "provider unavailable",
			PlaceVisits:         map[string]int{"task:init": 2},
			TotalVisits:         map[string]int{"tr-process": 3},
		},
	}
	snapshot := &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
		RuntimeStatus: interfaces.RuntimeStatusIdle,
		FactoryState:  "RUNNING",
		InFlightCount: 0,
		Uptime:        2 * time.Minute,
		Marking: factoryruntime.PetriMarkingSnapshot{
			Tokens: map[string]*factoryruntime.RuntimeToken{"tok-init": token},
		},
		Topology: &factoryruntime.Net{
			Places: map[string]*factoryruntime.PetriPlace{
				"task:init": {ID: "task:init", TypeID: "task", State: "init"},
			},
			WorkTypes: map[string]*factoryruntime.WorkType{
				"task": {
					ID: "task",
					States: []factoryruntime.StateDefinition{
						{Value: "init", Category: factoryruntime.StateCategoryInitial},
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
	history := runtime.Petri.Marking[0].History
	if history == nil || history.LastError == nil || *history.LastError != "provider unavailable" {
		t.Fatalf("token history = %#v, want provider unavailable", history)
	}
	if history.ConsecutiveFailures == nil || (*history.ConsecutiveFailures)["tr-process"] != 1 {
		t.Fatalf("consecutive failures = %#v, want tr-process=1", history.ConsecutiveFailures)
	}
	if history.PlaceVisits == nil || (*history.PlaceVisits)["task:init"] != 2 {
		t.Fatalf("place visits = %#v, want task:init=2", history.PlaceVisits)
	}
	if history.TotalVisits == nil || (*history.TotalVisits)["tr-process"] != 3 {
		t.Fatalf("total visits = %#v, want tr-process=3", history.TotalVisits)
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

func TestProjectRuntimeContract_DetachesTokenHistoryAcrossPublicMapping(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	source := &factoryruntime.RuntimeToken{
		ID:      "tok-history",
		PlaceID: "task:failed",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID:     "work-history",
			WorkTypeID: "task",
		},
		CreatedAt: now,
		EnteredAt: now,
		History: factoryruntime.RuntimeTokenHistory{
			ConsecutiveFailures: map[string]int{"process": 2},
			LastError:           "original error",
			PlaceVisits:         map[string]int{"task:failed": 1},
			TotalVisits:         map[string]int{"process": 4},
		},
	}
	domain := ProjectRuntimeContract(ProjectionContext{
		Session: &LiveSession{ID: "session-history"},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "history",
		},
		Snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
			Marking: factoryruntime.PetriMarkingSnapshot{
				Tokens: map[string]*factoryruntime.RuntimeToken{source.ID: source},
			},
			Topology: &factoryruntime.Net{},
		},
		Now: now,
	})
	source.History.ConsecutiveFailures["process"] = 99
	source.History.PlaceVisits["task:failed"] = 99
	source.History.TotalVisits["process"] = 99

	if domain.Petri == nil || len(domain.Petri.Marking) != 1 || domain.Petri.Marking[0].History == nil {
		t.Fatalf("domain token history = %#v, want one populated history", domain.Petri)
	}
	domainHistory := domain.Petri.Marking[0].History
	if domainHistory.ConsecutiveFailures["process"] != 2 ||
		domainHistory.PlaceVisits["task:failed"] != 1 ||
		domainHistory.TotalVisits["process"] != 4 {
		t.Fatalf("domain token history aliased source = %#v", domainHistory)
	}

	public := factorysessionmapping.RuntimeProjectionToAPI(domain, nil)
	domainHistory.TotalVisits["process"] = 100
	publicHistory := public.Petri.Marking[0].History
	if publicHistory == nil || publicHistory.TotalVisits == nil || (*publicHistory.TotalVisits)["process"] != 4 {
		t.Fatalf("public token history aliased domain = %#v", publicHistory)
	}
}

func TestProjectRuntimeContract_OwnsDetachedJavaScriptStatusAndPublicMapping(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 16, 3, 30, 0, 0, time.UTC)
	state := &interfaces.FactorySessionJavaScriptRuntimeState{
		Phase:               "review",
		Phases:              []string{"plan", "review"},
		ScriptStatus:        "RUNNING",
		CompletedDispatches: 2,
	}
	ctx := ProjectionContext{
		Session: &LiveSession{ID: "session-domain-projection"},
		FactoryCfg: &interfaces.FactoryConfig{Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind:       interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{},
		}},
		JavaScript: state,
		Now:        now,
	}

	domain := ProjectRuntimeContract(ctx)
	state.Phases[0] = "mutated"
	if domain.Status != string(interfaces.RuntimeStatusIdle) || domain.JavaScript == nil {
		t.Fatalf("domain runtime = %#v, want idle JavaScript projection", domain)
	}
	if domain.JavaScript.ScriptStatus != interfaces.FactorySessionJavaScriptScriptStatusRunning ||
		domain.JavaScript.Phases[0] != "plan" || domain.JavaScript.ChildDispatchCounts.Completed != 2 {
		t.Fatalf("domain JavaScript projection = %#v, want detached owner-defined values", domain.JavaScript)
	}

	public := ProjectRuntime(ctx)
	if public.Status != factoryapi.FactorySessionStatusIDLE || public.Javascript == nil ||
		public.Javascript.ScriptStatus != factoryapi.FactorySessionJavaScriptScriptStatusRUNNING ||
		public.Javascript.ChildDispatchCounts.Completed != 2 {
		t.Fatalf("public runtime = %#v, want compatible mapped values", public)
	}
}

func TestProjectRuntime_JavaScriptWorkflowSessionIncludesPhaseAndCheckpointRefs(t *testing.T) {
	now := time.Date(2026, 6, 8, 14, 5, 0, 0, time.UTC)
	startedAt := now.Add(-5 * time.Minute)
	argsSchema := json.RawMessage(`{"type":"object","properties":{"topic":{"type":"string"}}}`)
	defaultPolicy := json.RawMessage(`{"maxAgents":3}`)
	folderPath := t.TempDir()
	logicalTarget := RuntimeLogicalTarget{
		Kind:       "default",
		FolderPath: folderPath,
	}
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{
			ID:      "session-js",
			Project: "dynamic-workflow",
			SessionState: SessionState{
				FolderPath: folderPath,
			},
			Target: TargetRef{Kind: TargetKindDefault},
		},
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
		BackendScopeID:      "backend-scope-1",
		LogicalSessionKeyID: "lsk-0123456789abcdef0123456789abcdef",
		NormalizedTarget:    &logicalTarget,
		RuntimeStartedAt:    startedAt,
		Now:                 now,
	})
	assertJavaScriptWorkflowSessionProjection(t, runtime)
	if runtime.Budgets == nil || runtime.Budgets.MaxAgents == nil || *runtime.Budgets.MaxAgents != 3 {
		t.Fatalf("budgets = %#v, want maxAgents=3", runtime.Budgets)
	}
	if runtime.StreamIdentity == nil {
		t.Fatal("stream identity = nil, want identity for javascript session")
	}
	if runtime.StreamIdentity.BackendScopeID != "backend-scope-1" ||
		runtime.StreamIdentity.LogicalSessionKeyID != "lsk-0123456789abcdef0123456789abcdef" ||
		runtime.StreamIdentity.FactorySessionID != "session-js" ||
		runtime.StreamIdentity.StreamGenerationID != startedAt.Format(time.RFC3339Nano) {
		t.Fatalf("stream identity = %#v, want stable backend/logical/session/start tuple", runtime.StreamIdentity)
	}
	if runtime.StreamIdentity.LogicalSessionKeyID == "" || runtime.StreamIdentity.NormalizedTarget == nil {
		t.Fatalf("stream identity = %#v, want logical identity fields", runtime.StreamIdentity)
	}
}

func TestProjectRuntime_JavaScriptWorkflowSessionPrefersSnapshotStreamGenerationID(t *testing.T) {
	now := time.Date(2026, 6, 27, 7, 30, 0, 0, time.UTC)
	startedAt := now.Add(-10 * time.Minute)
	folderPath := t.TempDir()
	logicalTarget := RuntimeLogicalTarget{
		Kind:       "default",
		FolderPath: folderPath,
	}
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{
			ID:      "session-js",
			Project: "dynamic-workflow",
			SessionState: SessionState{
				FolderPath: folderPath,
			},
			Target: TargetRef{Kind: TargetKindDefault},
		},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "dynamic-workflow",
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
				JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
					Dialect:   "workflow-v1",
					SourceRef: "factory/workflows/review.js",
				},
			},
		},
		Snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
			StreamGenerationID: "stream-from-snapshot",
		},
		JavaScript: &interfaces.FactorySessionJavaScriptRuntimeState{
			Phase:        "review",
			Phases:       []string{"plan", "review"},
			ArgsDigest:   "sha256:args-digest",
			ScriptStatus: "RUNNING",
		},
		BackendScopeID:      "backend-scope-1",
		LogicalSessionKeyID: "lsk-0123456789abcdef0123456789abcdef",
		NormalizedTarget:    &logicalTarget,
		RuntimeStartedAt:    startedAt,
		Now:                 now,
	})
	if runtime.StreamIdentity == nil {
		t.Fatal("stream identity = nil, want identity for javascript session")
	}
	if runtime.StreamIdentity.StreamGenerationID != "stream-from-snapshot" {
		t.Fatalf("stream generation id = %q, want snapshot token", runtime.StreamIdentity.StreamGenerationID)
	}
}

func TestProjectRuntime_PausedSessionIncludesStopSummary(t *testing.T) {
	now := time.Date(2026, 6, 27, 8, 15, 0, 0, time.UTC)
	token := &factoryruntime.RuntimeToken{
		ID:      "tok-goal-review",
		PlaceID: "goal:review",
		Color: factoryruntime.RuntimeTokenColor{
			Name:       "Resume draft",
			WorkID:     "work-goal-1",
			WorkTypeID: "goal",
			TraceID:    "trace-goal-1",
		},
		CreatedAt: now.Add(-2 * time.Minute),
		EnteredAt: now.Add(-1 * time.Minute),
	}
	runtime := ProjectRuntimeContract(ProjectionContext{
		Session: &LiveSession{ID: "session-paused"},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "goal",
		},
		Snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
			RuntimeStatus:          interfaces.RuntimeStatusIdle,
			FactoryState:           "PAUSED",
			LifecycleControlStatus: string(factoryapi.FactorySessionDurableLifecycleStatusPaused),
			Marking:                factoryruntime.PetriMarkingSnapshot{Tokens: map[string]*factoryruntime.RuntimeToken{"tok-goal-review": token}},
			Topology:               &factoryruntime.Net{Places: map[string]*factoryruntime.PetriPlace{"goal:review": {ID: "goal:review", TypeID: "goal", State: "review"}}},
		},
		Now: now,
	})
	if runtime.StopSummary == nil {
		t.Fatal("stopSummary = nil, want paused summary")
	}
	if runtime.StopSummary.StopKind != StopKindPaused {
		t.Fatalf("stop kind = %q, want PAUSED", runtime.StopSummary.StopKind)
	}
	if runtime.StopSummary.WorkID == nil || *runtime.StopSummary.WorkID != "work-goal-1" {
		t.Fatalf("stopSummary.workId = %#v, want work-goal-1", runtime.StopSummary.WorkID)
	}
	if runtime.StopSummary.WorkState == nil || *runtime.StopSummary.WorkState != "goal:review" {
		t.Fatalf("stopSummary.workState = %#v, want goal:review", runtime.StopSummary.WorkState)
	}
	if runtime.StopSummary.SessionLifecycleStatus == nil || *runtime.StopSummary.SessionLifecycleStatus != "PAUSED" {
		t.Fatalf("stopSummary.lifecycle = %#v, want PAUSED", runtime.StopSummary.SessionLifecycleStatus)
	}
}

func TestProjectRuntime_BlockedAndNeedsHumanSessionsIncludeStopSummary(t *testing.T) {
	now := time.Date(2026, 6, 27, 8, 30, 0, 0, time.UTC)
	testCases := []struct {
		name               string
		placeID            string
		stateName          string
		wantStop           StopKind
		wantSummary        string
		wantDispatchStatus StopDispatchStatus
		lastError          string
	}{
		{name: "blocked", placeID: "goal:blocked", stateName: "blocked", wantStop: StopKindBlocked, wantSummary: "provider timeout", wantDispatchStatus: StopDispatchStatusFailed},
		{name: "needs-human", placeID: "goal:needs-human", stateName: "needs-human", wantStop: StopKindNeedsHuman, wantSummary: "awaiting operator approval", wantDispatchStatus: StopDispatchStatusFailed},
		{name: "interrupted", placeID: "goal:interrupted", stateName: "interrupted", wantStop: StopKindInterrupted, wantSummary: "Operator interrupted review after partial output was available.", wantDispatchStatus: StopDispatchStatusInterrupted, lastError: "Operator interrupted review after partial output was available."},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token := &factoryruntime.RuntimeToken{
				ID:      "tok-goal-stop",
				PlaceID: tc.placeID,
				Color: factoryruntime.RuntimeTokenColor{
					Name:       "Recover goal",
					WorkID:     "work-goal-stop",
					WorkTypeID: "goal",
					TraceID:    "trace-goal-stop",
				},
				CreatedAt: now.Add(-2 * time.Minute),
				EnteredAt: now.Add(-1 * time.Minute),
				History: factoryruntime.RuntimeTokenHistory{
					LastError: tc.lastError,
				},
			}
			runtime := ProjectRuntimeContract(ProjectionContext{
				Session: &LiveSession{ID: "session-stop"},
				FactoryCfg: &interfaces.FactoryConfig{
					Name: "goal",
				},
				Snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
					RuntimeStatus: interfaces.RuntimeStatusIdle,
					FactoryState:  "RUNNING",
					Marking:       factoryruntime.PetriMarkingSnapshot{Tokens: map[string]*factoryruntime.RuntimeToken{"tok-goal-stop": token}},
					Topology:      &factoryruntime.Net{Places: map[string]*factoryruntime.PetriPlace{tc.placeID: {ID: tc.placeID, TypeID: "goal", State: tc.stateName}}},
					DispatchHistory: []interfaces.CompletedDispatch{{
						DispatchID:      "dispatch-stop-1",
						TransitionID:    "execute-goal",
						WorkstationName: "execute-goal",
						Outcome:         workerexecution.OutcomeFailed,
						Reason:          tc.wantSummary,
						EndTime:         now,
						ConsumedTokens:  []factoryruntime.RuntimeToken{{Color: factoryruntime.RuntimeTokenColor{WorkID: "work-goal-stop"}}},
					}},
				},
				Now: now,
			})
			if runtime.StopSummary == nil {
				t.Fatal("stopSummary = nil, want work-level stop summary")
			}
			assertWorkStateStopSummary(t, runtime.StopSummary, tc.placeID, tc.wantStop, tc.wantDispatchStatus, tc.wantSummary)
		})
	}
}

func assertWorkStateStopSummary(
	t *testing.T,
	summary *StopSummary,
	wantPlaceID string,
	wantStop StopKind,
	wantDispatchStatus StopDispatchStatus,
	wantSummary string,
) {
	t.Helper()

	if summary.StopKind != wantStop {
		t.Fatalf("stop kind = %q, want %q", summary.StopKind, wantStop)
	}
	if summary.WorkState == nil || *summary.WorkState != wantPlaceID {
		t.Fatalf("stopSummary.workState = %#v, want %s", summary.WorkState, wantPlaceID)
	}
	if summary.LatestDispatch == nil || summary.LatestDispatch.DispatchID != "dispatch-stop-1" {
		t.Fatalf("latestDispatch = %#v, want dispatch-stop-1", summary.LatestDispatch)
	}
	if summary.LatestDispatch.Status != wantDispatchStatus {
		t.Fatalf("latestDispatch.status = %q, want %q", summary.LatestDispatch.Status, wantDispatchStatus)
	}
	if summary.LatestResultSummary == nil || *summary.LatestResultSummary != wantSummary {
		t.Fatalf("latestResultSummary = %#v, want %q", summary.LatestResultSummary, wantSummary)
	}
	if summary.SuggestedRecoverySurface == nil || strings.TrimSpace(*summary.SuggestedRecoverySurface) == "" {
		t.Fatalf("suggestedRecoverySurface = %#v, want operator recovery guidance", summary.SuggestedRecoverySurface)
	}
	if summary.SuggestedRecoveryAction == nil || strings.TrimSpace(*summary.SuggestedRecoveryAction) == "" {
		t.Fatalf("suggestedRecoveryAction = %#v, want operator next step", summary.SuggestedRecoveryAction)
	}
}

func TestProjectRuntime_InterruptedSessionIncludesStopSummary(t *testing.T) {
	now := time.Date(2026, 6, 27, 8, 45, 0, 0, time.UTC)
	token := &factoryruntime.RuntimeToken{
		ID:      "tok-goal-interrupted",
		PlaceID: "goal:review",
		Color: factoryruntime.RuntimeTokenColor{
			Name:       "Interrupted goal",
			WorkID:     "work-goal-interrupted",
			WorkTypeID: "goal",
		},
		CreatedAt: now.Add(-2 * time.Minute),
		EnteredAt: now.Add(-1 * time.Minute),
	}
	runtime := ProjectRuntimeContract(ProjectionContext{
		Session: &LiveSession{ID: "session-interrupted"},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "goal",
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
				JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
					Dialect:   "workflow-v1",
					SourceRef: "factory/workflows/goal.js",
				},
			},
		},
		Snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
			Marking:  factoryruntime.PetriMarkingSnapshot{Tokens: map[string]*factoryruntime.RuntimeToken{"tok-goal-interrupted": token}},
			Topology: &factoryruntime.Net{Places: map[string]*factoryruntime.PetriPlace{"goal:review": {ID: "goal:review", TypeID: "goal", State: "review"}}},
		},
		JavaScript: &interfaces.FactorySessionJavaScriptRuntimeState{
			ScriptStatus: "INTERRUPTED",
			Dispatches: []interfaces.FactorySessionDispatchState{{
				ID:             "dispatch-js-1",
				Status:         "INTERRUPTED",
				DispatchKind:   string(factoryapi.FactoryDispatchKindJAVASCRIPTAGENT),
				Label:          "review child",
				RelatedWorkIDs: []string{"work-goal-interrupted"},
				FailureDetail: &interfaces.FactorySessionDispatchFailureDetail{
					Reason:  "operator_interrupt",
					Message: "Operator interrupted the dispatch",
				},
			}},
		},
		Now: now,
	})
	if runtime.StopSummary == nil {
		t.Fatal("stopSummary = nil, want interrupted summary")
	}
	if runtime.StopSummary.StopKind != StopKindInterrupted {
		t.Fatalf("stop kind = %q, want INTERRUPTED", runtime.StopSummary.StopKind)
	}
	if runtime.StopSummary.LatestDispatch == nil || runtime.StopSummary.LatestDispatch.Status != StopDispatchStatusInterrupted {
		t.Fatalf("latestDispatch = %#v, want interrupted dispatch", runtime.StopSummary.LatestDispatch)
	}
	if runtime.StopSummary.WorkID == nil || *runtime.StopSummary.WorkID != "work-goal-interrupted" {
		t.Fatalf("stopSummary.workId = %#v, want work-goal-interrupted", runtime.StopSummary.WorkID)
	}
}

func TestProjectRuntime_InterruptedSessionWithoutMatchingRelatedWorkLeavesWorkContextEmpty(t *testing.T) {
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	token := &factoryruntime.RuntimeToken{
		ID:      "tok-goal-review",
		PlaceID: "goal:review",
		Color: factoryruntime.RuntimeTokenColor{
			Name:       "Nearby goal",
			WorkID:     "work-goal-review",
			WorkTypeID: "goal",
		},
		CreatedAt: now.Add(-2 * time.Minute),
		EnteredAt: now.Add(-1 * time.Minute),
	}
	runtime := ProjectRuntimeContract(ProjectionContext{
		Session: &LiveSession{ID: "session-interrupted-unmatched"},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "goal",
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
				JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
					Dialect:   "workflow-v1",
					SourceRef: "factory/workflows/goal.js",
				},
			},
		},
		Snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
			Marking:  factoryruntime.PetriMarkingSnapshot{Tokens: map[string]*factoryruntime.RuntimeToken{"tok-goal-review": token}},
			Topology: &factoryruntime.Net{Places: map[string]*factoryruntime.PetriPlace{"goal:review": {ID: "goal:review", TypeID: "goal", State: "review"}}},
		},
		JavaScript: &interfaces.FactorySessionJavaScriptRuntimeState{
			ScriptStatus: "INTERRUPTED",
			Dispatches: []interfaces.FactorySessionDispatchState{{
				ID:             "dispatch-js-unmatched",
				Status:         "INTERRUPTED",
				DispatchKind:   string(factoryapi.FactoryDispatchKindJAVASCRIPTAGENT),
				Label:          "review child",
				RelatedWorkIDs: []string{"work-missing"},
				FailureDetail: &interfaces.FactorySessionDispatchFailureDetail{
					Reason:  "operator_interrupt",
					Message: "Operator interrupted the dispatch",
				},
			}},
		},
		Now: now,
	})
	if runtime.StopSummary == nil {
		t.Fatal("stopSummary = nil, want interrupted summary")
	}
	if runtime.StopSummary.StopKind != StopKindInterrupted {
		t.Fatalf("stop kind = %q, want INTERRUPTED", runtime.StopSummary.StopKind)
	}
	if runtime.StopSummary.WorkID != nil {
		t.Fatalf("stopSummary.workId = %#v, want nil when related work cannot be matched", runtime.StopSummary.WorkID)
	}
	if runtime.StopSummary.WorkName != nil {
		t.Fatalf("stopSummary.workName = %#v, want nil when related work cannot be matched", runtime.StopSummary.WorkName)
	}
	if runtime.StopSummary.WorkState != nil {
		t.Fatalf("stopSummary.workState = %#v, want nil when related work cannot be matched", runtime.StopSummary.WorkState)
	}
}

func assertJavaScriptWorkflowSessionProjection(t *testing.T, runtime factoryapi.FactorySessionRuntime) {
	t.Helper()
	assertJavaScriptSessionIdentity(t, runtime)
	if runtime.Javascript == nil {
		t.Fatal("javascript projection is nil")
	}
	assertJavaScriptSessionRuntimeFields(t, runtime.Javascript)
}

func assertJavaScriptSessionIdentity(t *testing.T, runtime factoryapi.FactorySessionRuntime) {
	t.Helper()
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
}

func assertJavaScriptSessionRuntimeFields(t *testing.T, javascript *factoryapi.FactorySessionJavaScriptProjection) {
	t.Helper()
	if javascript.Phase == nil || *javascript.Phase != "review" {
		t.Fatalf("phase = %#v, want review", javascript.Phase)
	}
	if javascript.ArgsDigest == nil || *javascript.ArgsDigest != "sha256:args-digest" {
		t.Fatalf("args digest = %#v", javascript.ArgsDigest)
	}
	if javascript.Checkpoints == nil || len(*javascript.Checkpoints) != 1 {
		t.Fatalf("checkpoints = %#v, want one checkpoint ref", javascript.Checkpoints)
	}
	if javascript.ScriptStatus != factoryapi.FactorySessionJavaScriptScriptStatusRUNNING {
		t.Fatalf("script status = %q, want RUNNING", javascript.ScriptStatus)
	}
	if javascript.ChildDispatchCounts.Queued != 1 || javascript.ChildDispatchCounts.Running != 2 || javascript.ChildDispatchCounts.Completed != 4 {
		t.Fatalf("child dispatch counts = %#v", javascript.ChildDispatchCounts)
	}
}
func TestSessionResponse_PetriRuntimeOmitsDispatchesWhenCanonicalStateExists(t *testing.T) {
	now := time.Date(2026, 6, 8, 16, 0, 0, 0, time.UTC)
	token := &factoryruntime.RuntimeToken{
		ID:      "tok-1",
		PlaceID: "task:init",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID:     "work-1",
			WorkTypeID: "task",
			TraceID:    "trace-1",
		},
	}
	snapshot := &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  "RUNNING",
		InFlightCount: 1,
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch-petri-1": {
				DispatchID:      "dispatch-petri-1",
				TransitionID:    "tr-process",
				WorkstationName: "process",
				StartTime:       now,
				ConsumedTokens:  []factoryruntime.RuntimeToken{*token},
			},
		},
		Topology: &factoryruntime.Net{
			Transitions: map[string]*factoryruntime.PetriTransition{
				"tr-process": {
					ID:         "tr-process",
					WorkerType: "worker-a",
				},
			},
		},
	}
	session := SessionResponse(ProjectionContext{
		Session: &LiveSession{ID: "~default"},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "legacy-petri",
		},
		Snapshot: snapshot,
		Now:      now,
	})
	assertRuntimeJSONOmitsDispatches(t, session.Runtime)
	if session.Runtime.Progress.InFlightCount != 1 {
		t.Fatalf("in-flight count = %d, want 1", session.Runtime.Progress.InFlightCount)
	}
}

func TestSessionResponse_JavaScriptRuntimeOmitsDispatchesAndPreservesArtifacts(t *testing.T) {
	now := time.Date(2026, 6, 8, 16, 5, 0, 0, time.UTC)
	session := SessionResponse(ProjectionContext{
		Session: &LiveSession{ID: "session-js"},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "dynamic-workflow",
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
				JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
					Dialect: "workflow-v1",
				},
			},
		},
		JavaScript: &interfaces.FactorySessionJavaScriptRuntimeState{
			Phase:        "review",
			ScriptStatus: "RUNNING",
			Dispatches: []interfaces.FactorySessionDispatchState{{
				ID:           "dispatch-agent-1",
				Status:       string(factoryapi.FactoryDispatchStatusCOMPLETED),
				Phase:        "review",
				Label:        "Summarize findings",
				RunnerID:     "runner-fast",
				Model:        "gpt-test",
				Provider:     "openai",
				PromptDigest: "sha256:prompt",
				SchemaDigest: "sha256:schema",
				ArtifactIDs:  []string{"artifact-child-1"},
				JavaScript: &interfaces.FactorySessionDispatchJavaScriptState{
					TaskKind:  "AGENT",
					TaskLabel: "Summarize findings",
				},
			}},
			Artifacts: []interfaces.FactorySessionArtifactState{{
				ID:          "artifact-child-1",
				Kind:        string(factoryapi.FactoryArtifactKindCHILDRESULT),
				Visibility:  string(factoryapi.FactoryArtifactVisibilityPUBLIC),
				Label:       "Child result",
				Summary:     "Agent output summary",
				AuditMode:   string(factoryapi.FactoryArtifactAuditModeREDACTED),
				ContentHash: "sha256:child-result",
				SizeBytes:   128,
				RedactionCounts: map[string]int{
					"secrets": 1,
				},
				CaptureMetadata: map[string]string{
					"capturedAt": now.UTC().Format(time.RFC3339), "sourceDispatchId": "dispatch-agent-1",
					"mimeType": "application/json",
				},
				CapturedAt: now,
			}},
		},
		Now: now,
	})
	assertRuntimeJSONOmitsDispatches(t, session.Runtime)
	assertJavaScriptChildResultArtifact(t, session.Runtime.Artifacts)
}

func assertRuntimeJSONOmitsDispatches(t *testing.T, runtime factoryapi.FactorySessionRuntime) {
	t.Helper()
	encoded, err := json.Marshal(runtime)
	if err != nil {
		t.Fatalf("marshal runtime: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal runtime: %v", err)
	}
	if _, ok := payload["dispatches"]; ok {
		t.Fatalf("runtime JSON contains dispatches: %s", encoded)
	}
}

func assertJavaScriptChildResultArtifact(t *testing.T, artifacts *[]factoryapi.FactoryArtifact) {
	t.Helper()
	if artifacts == nil || len(*artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one child result artifact", artifacts)
	}
	artifact := (*artifacts)[0]
	if artifact.Kind != factoryapi.FactoryArtifactKindCHILDRESULT {
		t.Fatalf("artifact kind = %q, want CHILD_RESULT", artifact.Kind)
	}
	if artifact.RedactionCounts == nil || artifact.RedactionCounts.Secrets == nil || *artifact.RedactionCounts.Secrets != 1 {
		t.Fatalf("artifact redaction counts = %#v, want one secret redaction", artifact.RedactionCounts)
	}
	if artifact.CaptureMetadata == nil || artifact.CaptureMetadata.SourceDispatchId == nil || *artifact.CaptureMetadata.SourceDispatchId != "dispatch-agent-1" {
		t.Fatalf("artifact capture metadata = %#v", artifact.CaptureMetadata)
	}
}
func TestProjectCheckpointRef_OmitsRawCheckpointBodyFromPublicProjection(t *testing.T) {
	now := time.Date(2026, 6, 8, 16, 0, 0, 0, time.UTC)
	store := projectionCheckpointStore{records: []interfaces.JavaScriptCheckpointRecord{{
		ID:          "ckpt-1",
		Label:       "after-plan",
		Summary:     "Completed planning phase",
		Timestamp:   now,
		ArtifactID:  "artifact-ckpt-1",
		ContentHash: "sha256:checkpoint-body",
		SizeBytes:   128,
		RawBody:     json.RawMessage(`{"vmState":"raw javascript checkpoint body must stay private","hostPath":"/tmp/checkpoints/ckpt-1.json"}`),
		StoragePath: "/tmp/checkpoints/ckpt-1.json",
	}}}

	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{ID: "session-js"},
		FactoryCfg: &interfaces.FactoryConfig{
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
			},
		},
		JavaScript: JavaScriptRuntimeStateFromCheckpoints(store, &interfaces.FactorySessionJavaScriptRuntimeState{
			Phase:        "review",
			Phases:       []string{"plan", "review"},
			ScriptStatus: "RUNNING",
		}),
		Now: now,
	})
	if runtime.Javascript == nil || runtime.Javascript.Checkpoints == nil || len(*runtime.Javascript.Checkpoints) != 1 {
		t.Fatalf("javascript checkpoints = %#v, want one checkpoint ref", runtime.Javascript)
	}
	checkpoint := (*runtime.Javascript.Checkpoints)[0]
	if checkpoint.ArtifactRef == nil || checkpoint.ArtifactRef.Visibility != factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT {
		t.Fatalf("artifact ref = %#v, want INTERNAL_CHECKPOINT visibility", checkpoint.ArtifactRef)
	}

	encoded, err := json.Marshal(runtime)
	if err != nil {
		t.Fatalf("marshal runtime projection: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{
		"raw javascript checkpoint body must stay private",
		"/tmp/checkpoints/ckpt-1.json",
		"vmState",
		"hostPath",
		"rawBody",
		"storagePath",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("public runtime JSON leaked %q: %s", forbidden, body)
		}
	}
}

func TestProjectSessionResultAndPartialResult_ReferenceCheckpointArtifactsOnly(t *testing.T) {
	now := time.Date(2026, 6, 8, 16, 5, 0, 0, time.UTC)
	store := projectionCheckpointStore{records: []interfaces.JavaScriptCheckpointRecord{{
		ID:          "ckpt-1",
		Label:       "after-plan",
		Summary:     "Completed planning phase",
		Timestamp:   now,
		ArtifactID:  "artifact-ckpt-1",
		ContentHash: "sha256:checkpoint-body",
		SizeBytes:   128,
		RawBody:     json.RawMessage(`{"vmState":"raw javascript checkpoint body must stay private"}`),
		StoragePath: "/tmp/checkpoints/ckpt-1.json",
	}}}
	ctx := ProjectionContext{
		Session: &LiveSession{ID: "session-js"},
		FactoryCfg: &interfaces.FactoryConfig{
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
			},
		},
		JavaScript: JavaScriptRuntimeStateFromCheckpoints(store, &interfaces.FactorySessionJavaScriptRuntimeState{
			Phase:        "review",
			Phases:       []string{"plan", "review"},
			ScriptStatus: "RUNNING",
		}),
		Now: now,
	}

	result := ProjectSessionResult("session-js", ctx, store, sessionResultProjectionRole{})
	if result.ResultArtifactRef == nil || result.ResultArtifactRef.ID != "artifact-ckpt-1" {
		t.Fatalf("result artifact ref = %#v", result.ResultArtifactRef)
	}
	partial := ProjectSessionPartialResult("session-js", ctx, store)
	if partial.PartialResultArtifactRef == nil || partial.PartialResultArtifactRef.ID != "artifact-ckpt-1" {
		t.Fatalf("partial result artifact ref = %#v", partial.PartialResultArtifactRef)
	}
	if partial.Phase != "review" {
		t.Fatalf("partial phase = %q, want review", partial.Phase)
	}

	for _, payload := range []any{result, partial} {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal result payload: %v", err)
		}
		if strings.Contains(string(encoded), "raw javascript checkpoint body must stay private") {
			t.Fatalf("result payload leaked raw checkpoint body: %s", encoded)
		}
	}
}
func TestProjectRuntime_LifecycleControlStatusReflectsCanonicalProjection(t *testing.T) {
	now := time.Date(2026, 6, 20, 15, 0, 0, 0, time.UTC)
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{ID: "~default", IsDefault: true},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "legacy-petri",
		},
		Snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
			RuntimeStatus:          interfaces.RuntimeStatusIdle,
			FactoryState:           "RUNNING",
			LifecycleControlStatus: string(factoryapi.FactorySessionDurableLifecycleStatusPaused),
		},
		Now: now,
	})
	if runtime.LifecycleControlStatus == nil {
		t.Fatal("lifecycleControlStatus = nil, want PAUSED")
	}
	if *runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("lifecycleControlStatus = %q, want PAUSED", *runtime.LifecycleControlStatus)
	}
	if runtime.Progress.FactoryState != "RUNNING" {
		t.Fatalf("progress.factoryState = %q, want raw engine snapshot RUNNING", runtime.Progress.FactoryState)
	}
}

func TestProjectRuntime_UntouchedIdleSessionPreservesRawFactoryState(t *testing.T) {
	now := time.Date(2026, 6, 20, 15, 10, 0, 0, time.UTC)
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{ID: "~default", IsDefault: true},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "legacy-petri",
		},
		Snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
			RuntimeStatus: interfaces.RuntimeStatusIdle,
			FactoryState:  "IDLE",
		},
		Now: now,
	})
	if runtime.LifecycleControlStatus != nil {
		t.Fatalf("lifecycleControlStatus = %#v, want unset for untouched idle session", runtime.LifecycleControlStatus)
	}
	if runtime.Progress.FactoryState != "IDLE" {
		t.Fatalf("progress.factoryState = %q, want raw engine snapshot IDLE", runtime.Progress.FactoryState)
	}
}

func TestProjectRuntime_LifecycleControlStatusUnchangedWithoutControlEvents(t *testing.T) {
	now := time.Date(2026, 6, 20, 15, 5, 0, 0, time.UTC)
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{ID: "~default", IsDefault: true},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "legacy-petri",
		},
		Snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
			RuntimeStatus:          interfaces.RuntimeStatusIdle,
			FactoryState:           "RUNNING",
			LifecycleControlStatus: string(factoryapi.FactorySessionDurableLifecycleStatusRunning),
		},
		Now: now,
	})
	if runtime.LifecycleControlStatus == nil || *runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("lifecycleControlStatus = %#v, want RUNNING", runtime.LifecycleControlStatus)
	}
}
