package apisurface_test

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestNormalizeWorkflowSourceRequest_ResolvesWorkflowNameThroughOrchestratorSource(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := workflowsource.Request{
		Kind:  workflowsource.KindWorkflowName,
		Value: "review",
	}

	viaSurface := apisurface.NormalizeWorkflowSourceRequest(req, ctx)
	direct := workflowsource.Resolve(req, ctx)
	if viaSurface.Found != direct.Found ||
		viaSurface.SourceRef != direct.SourceRef ||
		viaSurface.SourceHash != direct.SourceHash {
		t.Fatalf("surface = %#v, orchestrator = %#v, want equivalent source resolution", viaSurface, direct)
	}
}

func TestNormalizeWorkflowSourceRequest_MissingWorkflowReportsNotFoundDiagnostic(t *testing.T) {
	projectRoot := t.TempDir()
	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	resolution := apisurface.NormalizeWorkflowSourceRequest(workflowsource.Request{
		Kind:  workflowsource.KindWorkflowName,
		Value: "missing",
	}, ctx)
	if resolution.Found {
		t.Fatalf("resolution = %#v, want not found", resolution)
	}
	if len(resolution.Diagnostics) == 0 {
		t.Fatal("expected source resolution diagnostics")
	}
	if resolution.Diagnostics[0].Code != workflowsource.CodeSourceNotFound {
		t.Fatalf("diagnostic code = %q, want %q", resolution.Diagnostics[0].Code, workflowsource.CodeSourceNotFound)
	}
}

func TestWorkflowSessionResultMappingPreservesDomainProjection(t *testing.T) {
	timestamp := time.Date(2026, 7, 16, 3, 15, 0, 0, time.UTC)
	input := workflowresult.SessionResultInput{
		SessionID:    "session-js-1",
		Status:       interfaces.RuntimeStatusFinished,
		PrimaryValue: workflowresult.TypedValue{JSON: json.RawMessage(`{"ok":true}`)},
		CheckpointRefs: []interfaces.FactorySessionJavaScriptCheckpointEventRef{{
			ID: "checkpoint-1", Timestamp: &timestamp,
			ArtifactRef: &interfaces.FactoryArtifactRef{
				ID: "artifact-checkpoint-1", Kind: "CHECKPOINT", Visibility: "INTERNAL_CHECKPOINT",
			},
		}},
		ResultArtifact: &interfaces.FactoryArtifactRef{
			ID: "artifact-result-1", Kind: "FINAL_RESULT", Visibility: "PUBLIC",
		},
	}

	live := apisurface.BuildWorkflowSessionLiveResult(input)
	assertLiveWorkflowResult(t, live, timestamp)
	assertDurableWorkflowResult(t, input)
}

func assertLiveWorkflowResult(t *testing.T, live factoryapi.FactorySessionLiveResult, timestamp time.Time) {
	t.Helper()
	if live.Status != factoryapi.FactorySessionStatusFINISHED || live.CheckpointRefs == nil || len(*live.CheckpointRefs) != 1 {
		t.Fatalf("live result = %#v", live)
	}
	if got := (*live.CheckpointRefs)[0]; got.ArtifactRef == nil || got.ArtifactRef.Id != "artifact-checkpoint-1" || got.Timestamp == nil || !got.Timestamp.Equal(timestamp) {
		t.Fatalf("checkpoint ref = %#v", got)
	}
	if live.ResultArtifactRef == nil || live.ResultArtifactRef.Id != "artifact-result-1" {
		t.Fatalf("result artifact = %#v", live.ResultArtifactRef)
	}
}

func assertDurableWorkflowResult(t *testing.T, input workflowresult.SessionResultInput) {
	t.Helper()
	durable := apisurface.BuildWorkflowSessionResult(input)
	if durable.ResultStatus != factoryapi.FactorySessionResultStatusFinal || durable.PrimaryResult == nil || len(*durable.PrimaryResult) != 1 {
		t.Fatalf("durable result = %#v", durable)
	}
	payload := apisurface.BuildWorkflowSessionResultUpdatedPayload(input)
	if payload.ResultStatus != factoryapi.FactoryEventSessionResultStatusFinal || payload.ResultSummary == nil || len(*payload.ResultSummary) != 1 {
		t.Fatalf("event payload = %#v", payload)
	}
}
