package service_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func withProjectionLogicalIdentity(ctx factorysessions.ProjectionContext, backendScopeID string) factorysessions.ProjectionContext {
	if ctx.Session == nil {
		return ctx
	}
	ref, err := logicaltarget.NormalizeTargetRefWithEffects(
		filepath.EvalSymlinks,
		os.UserHomeDir,
		backendScopeID,
		ctx.Session.FolderPath,
		ctx.Session.Target,
	)
	if err != nil {
		return ctx
	}
	ctx.LogicalSessionKeyID = logicaltarget.DeriveLogicalSessionKeyID(ref)
	target := logicaltarget.RuntimeLogicalTarget(ref)
	ctx.NormalizedTarget = &target
	return ctx
}

func projectRuntimeToAPI(ctx factorysessions.ProjectionContext) factoryapi.FactorySessionRuntime {
	return factorysessionmapping.RuntimeProjectionToAPI(
		factorysessions.ProjectRuntimeContract(ctx),
		ctx.NormalizedTarget,
	)
}

func TestProjectRuntime_JavaScriptWorkflowSessionStreamIdentityIncludesLogicalFields(t *testing.T) {
	now := time.Date(2026, 6, 8, 14, 5, 0, 0, time.UTC)
	startedAt := now.Add(-5 * time.Minute)
	runtime := projectRuntimeToAPI(withProjectionLogicalIdentity(factorysessions.ProjectionContext{
		Session: &factorysessions.LiveSession{
			ID:      "session-js",
			Project: "dynamic-workflow",
			SessionState: factorysessions.SessionState{
				FolderPath: t.TempDir(),
			},
			Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
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
		JavaScript: &interfaces.FactorySessionJavaScriptRuntimeState{
			Phase:        "review",
			Phases:       []string{"plan", "review"},
			ArgsDigest:   "sha256:args-digest",
			ScriptStatus: "RUNNING",
		},
		BackendScopeID:   "backend-scope-1",
		RuntimeStartedAt: startedAt,
		Now:              now,
	}, "backend-scope-1"))
	if runtime.StreamIdentity == nil {
		t.Fatal("stream identity = nil, want identity for javascript session")
	}
	if runtime.StreamIdentity.BackendScopeID != "backend-scope-1" ||
		runtime.StreamIdentity.FactorySessionID != "session-js" ||
		runtime.StreamIdentity.StreamGenerationID != startedAt.Format(time.RFC3339Nano) {
		t.Fatalf("stream identity = %#v, want stable backend/session/start tuple", runtime.StreamIdentity)
	}
	if runtime.StreamIdentity.LogicalSessionKeyID == "" {
		t.Fatal("stream identity logicalSessionKeyID is empty")
	}
	if runtime.StreamIdentity.NormalizedTarget == nil || runtime.StreamIdentity.NormalizedTarget.Kind != factoryapi.FactorySessionLogicalTargetKindDefault {
		t.Fatalf("normalized target = %#v, want default kind", runtime.StreamIdentity.NormalizedTarget)
	}
}

func TestProjectRuntime_JavaScriptWorkflowSessionPrefersSnapshotStreamGenerationIDWithLogicalFields(t *testing.T) {
	now := time.Date(2026, 6, 27, 7, 30, 0, 0, time.UTC)
	startedAt := now.Add(-10 * time.Minute)
	runtime := projectRuntimeToAPI(withProjectionLogicalIdentity(factorysessions.ProjectionContext{
		Session: &factorysessions.LiveSession{
			ID:      "session-js",
			Project: "dynamic-workflow",
			SessionState: factorysessions.SessionState{
				FolderPath: t.TempDir(),
			},
			Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
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
		BackendScopeID:   "backend-scope-1",
		RuntimeStartedAt: startedAt,
		Now:              now,
	}, "backend-scope-1"))
	if runtime.StreamIdentity == nil {
		t.Fatal("stream identity = nil, want identity for javascript session")
	}
	if runtime.StreamIdentity.StreamGenerationID != "stream-from-snapshot" {
		t.Fatalf("stream generation id = %q, want snapshot token", runtime.StreamIdentity.StreamGenerationID)
	}
	if runtime.StreamIdentity.LogicalSessionKeyID == "" || runtime.StreamIdentity.NormalizedTarget == nil {
		t.Fatalf("stream identity = %#v, want logical identity fields", runtime.StreamIdentity)
	}
}
