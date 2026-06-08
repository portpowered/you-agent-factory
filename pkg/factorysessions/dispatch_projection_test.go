package factorysessions

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestProjectRuntime_PetriTransitionDispatchProjection(t *testing.T) {
	now := time.Date(2026, 6, 8, 16, 0, 0, 0, time.UTC)
	token := &interfaces.Token{
		ID:      "tok-1",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			WorkID:     "work-1",
			WorkTypeID: "task",
			TraceID:    "trace-1",
		},
	}
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  "RUNNING",
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch-petri-1": {
				DispatchID:      "dispatch-petri-1",
				TransitionID:    "tr-process",
				WorkstationName: "process",
				StartTime:       now,
				ConsumedTokens:  []interfaces.Token{*token},
			},
		},
		Topology: &state.Net{
			Transitions: map[string]*petri.Transition{
				"tr-process": {
					ID:         "tr-process",
					WorkerType: "worker-a",
				},
			},
		},
	}
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{ID: "~default"},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "legacy-petri",
		},
		Snapshot: snapshot,
		Now:      now,
	})
	if runtime.Dispatches == nil || len(*runtime.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one Petri transition dispatch", runtime.Dispatches)
	}
	dispatch := (*runtime.Dispatches)[0]
	if dispatch.DispatchKind != factoryapi.FactoryDispatchKindPETRITRANSITION {
		t.Fatalf("dispatch kind = %q, want PETRI_TRANSITION", dispatch.DispatchKind)
	}
	if dispatch.Status != factoryapi.FactoryDispatchStatusRUNNING {
		t.Fatalf("dispatch status = %q, want RUNNING", dispatch.Status)
	}
	if dispatch.Petri == nil || dispatch.Petri.TransitionId != "tr-process" {
		t.Fatalf("petri projection = %#v, want tr-process", dispatch.Petri)
	}
	if dispatch.Javascript != nil {
		t.Fatalf("javascript projection = %#v, want nil for Petri dispatch", dispatch.Javascript)
	}
	if dispatch.RelatedWorkIds == nil || len(*dispatch.RelatedWorkIds) != 1 || (*dispatch.RelatedWorkIds)[0] != "work-1" {
		t.Fatalf("related work ids = %#v, want work-1", dispatch.RelatedWorkIds)
	}
}

func TestProjectRuntime_JavaScriptChildAgentDispatchAndArtifactProjection(t *testing.T) {
	now := time.Date(2026, 6, 8, 16, 5, 0, 0, time.UTC)
	runtime := ProjectRuntime(ProjectionContext{
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
			Phase:      "review",
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
				ID:         "artifact-child-1",
				Kind:       string(factoryapi.FactoryArtifactKindCHILDRESULT),
				Visibility: string(factoryapi.FactoryArtifactVisibilityPUBLIC),
				Label:      "Child result",
				Summary:    "Agent output summary",
				AuditMode:  string(factoryapi.FactoryArtifactAuditModeREDACTED),
				ContentHash: "sha256:child-result",
				SizeBytes:  128,
				RedactionCounts: map[string]int{
					"secrets": 1,
				},
				CaptureMetadata: artifactCaptureMetadata(now, "dispatch-agent-1", "application/json"),
				CapturedAt:      now,
			}},
		},
		Now: now,
	})
	assertJavaScriptChildAgentDispatchProjection(t, runtime)
}

func assertJavaScriptChildAgentDispatchProjection(t *testing.T, runtime factoryapi.FactorySessionRuntime) {
	t.Helper()
	assertJavaScriptChildAgentDispatch(t, runtime.Dispatches)
	assertJavaScriptChildResultArtifact(t, runtime.Artifacts)
}

func assertJavaScriptChildAgentDispatch(t *testing.T, dispatches *[]factoryapi.FactoryDispatch) {
	t.Helper()
	if dispatches == nil || len(*dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one JavaScript child-agent dispatch", dispatches)
	}
	dispatch := (*dispatches)[0]
	if dispatch.DispatchKind != factoryapi.FactoryDispatchKindJAVASCRIPTAGENT {
		t.Fatalf("dispatch kind = %q, want JAVASCRIPT_AGENT", dispatch.DispatchKind)
	}
	if dispatch.Petri != nil {
		t.Fatalf("petri projection = %#v, want nil for JavaScript dispatch", dispatch.Petri)
	}
	if dispatch.Javascript == nil || dispatch.Javascript.TaskKind != factoryapi.FactoryDispatchJavaScriptTaskKindAGENT {
		t.Fatalf("javascript projection = %#v, want AGENT task kind", dispatch.Javascript)
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
