package factorysessions

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestProjectCheckpointRef_OmitsRawCheckpointBodyFromPublicProjection(t *testing.T) {
	now := time.Date(2026, 6, 8, 16, 0, 0, 0, time.UTC)
	store := NewJavaScriptCheckpointStore()
	store.Put(interfaces.JavaScriptCheckpointRecord{
		ID:          "ckpt-1",
		Label:       "after-plan",
		Summary:     "Completed planning phase",
		Timestamp:   now,
		ArtifactID:  "artifact-ckpt-1",
		ContentHash: "sha256:checkpoint-body",
		SizeBytes:   128,
		RawBody:     json.RawMessage(`{"vmState":"raw javascript checkpoint body must stay private","hostPath":"/tmp/checkpoints/ckpt-1.json"}`),
		StoragePath: "/tmp/checkpoints/ckpt-1.json",
	})

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
	store := NewJavaScriptCheckpointStore()
	store.Put(interfaces.JavaScriptCheckpointRecord{
		ID:          "ckpt-1",
		Label:       "after-plan",
		Summary:     "Completed planning phase",
		Timestamp:   now,
		ArtifactID:  "artifact-ckpt-1",
		ContentHash: "sha256:checkpoint-body",
		SizeBytes:   128,
		RawBody:     json.RawMessage(`{"vmState":"raw javascript checkpoint body must stay private"}`),
		StoragePath: "/tmp/checkpoints/ckpt-1.json",
	})
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

	result := ProjectSessionResult("session-js", ctx, store)
	if result.ResultArtifactRef == nil || result.ResultArtifactRef.Id != "artifact-ckpt-1" {
		t.Fatalf("result artifact ref = %#v", result.ResultArtifactRef)
	}
	partial := ProjectSessionPartialResult("session-js", ctx, store)
	if partial.PartialResultArtifactRef == nil || partial.PartialResultArtifactRef.Id != "artifact-ckpt-1" {
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
