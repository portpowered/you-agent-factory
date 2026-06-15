package factorysessionexecution_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestRuntimeService_ProgressPrimitives_ProjectsArtifactsPhaseAndProgress(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "progress-primitives.workflow.js", "progress-primitives")

	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-progress-primitives",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "progress-primitives",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	assertProgressPrimitivesSessionRead(t, read)
	assertProgressPrimitivesArtifacts(t, service, completed.SessionID)
}

func TestRuntimeService_AgentRunFakeChild_ProjectsDispatchAndChildArtifact(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")

	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-agent-run-fake-child",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	dispatch := assertAgentRunFakeChildSessionRead(t, read)
	assertAgentRunFakeChildDispatch(t, service, completed.SessionID, dispatch)
	assertAgentRunFakeChildArtifact(t, service, completed.SessionID)
}

func TestProjectRuntimeExecutionRecords_ProgressPrimitivesFixture(t *testing.T) {
	source := readRuntimeFixture(t, "progress-primitives.workflow.js")
	args, err := json.Marshal(map[string]any{})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
		Source:    source,
		SourceRef: "progress-primitives.workflow.js",
		SessionID: "session-progress-primitives",
		Args:      args,
		Metadata: map[string]string{
			"name": "progress-primitives",
		},
	}, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run failure = %#v", outcome.Failure)
	}

	observedAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	projection := factorysessionexecution.ProjectRuntimeExecutionRecords(
		"session-progress-primitives",
		outcome.Records,
		observedAt,
	)
	if projection.Phase != "execute" || projection.PhaseCount != 2 {
		t.Fatalf("phase projection = %#v", projection)
	}
	if len(projection.Artifacts) != 1 || projection.Artifacts[0].ID != "artifact-1" {
		t.Fatalf("artifacts = %#v", projection.Artifacts)
	}
	if projection.Progress.TotalDispatches != 0 || projection.Progress.PhaseCount != 2 {
		t.Fatalf("progress = %#v", projection.Progress)
	}
}

func assertProgressPrimitivesSessionRead(t *testing.T, read factorysessionexecution.SessionReadResult) {
	t.Helper()
	if read.Phase != "execute" {
		t.Fatalf("phase = %q, want execute", read.Phase)
	}
	if read.Progress == nil || read.Progress.PhaseCount != 2 {
		t.Fatalf("progress = %#v, want phaseCount=2", read.Progress)
	}
	if read.Progress.TotalDispatches != 0 {
		t.Fatalf("totalDispatches = %d, want 0", read.Progress.TotalDispatches)
	}
	if read.ArtifactCount != 1 {
		t.Fatalf("artifactCount = %d, want 1", read.ArtifactCount)
	}
}

func assertProgressPrimitivesArtifacts(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
) {
	t.Helper()
	artifact := assertListedProgressPrimitiveArtifact(t, service, sessionID)
	assertProgressPrimitiveArtifactDetail(t, service, sessionID, artifact)
	assertProgressPrimitiveResultArtifactIDs(t, service, sessionID)
}

func assertListedProgressPrimitiveArtifact(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
) factorysessionexecution.ArtifactSummary {
	t.Helper()
	artifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one artifact", artifacts.Artifacts)
	}
	artifact := artifacts.Artifacts[0]
	if artifact.ID != "artifact-1" || artifact.Kind != "log" || artifact.Label != "step-output" {
		t.Fatalf("artifact = %#v", artifact)
	}
	if artifact.ContentHash == "" || artifact.SizeBytes <= 0 {
		t.Fatalf("artifact content metadata = %#v", artifact)
	}
	wantHref := "/factory-sessions/" + sessionID + "/artifacts/artifact-1"
	if artifact.RetrievalRef == nil || artifact.RetrievalRef.Href != wantHref {
		t.Fatalf("retrieval ref = %#v, want %q", artifact.RetrievalRef, wantHref)
	}
	return artifact
}

func assertProgressPrimitiveArtifactDetail(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
	artifact factorysessionexecution.ArtifactSummary,
) {
	t.Helper()
	detail, err := service.GetArtifact(context.Background(), sessionID, "artifact-1")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if detail.ID != artifact.ID || detail.SessionID != sessionID {
		t.Fatalf("artifact detail = %#v", detail)
	}
}

func assertProgressPrimitiveResultArtifactIDs(t *testing.T, service factorysessionexecution.Service, sessionID string) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if len(result.ArtifactIDs) != 1 || result.ArtifactIDs[0] != "artifact-1" {
		t.Fatalf("artifactIds = %#v, want [artifact-1]", result.ArtifactIDs)
	}
}

func assertAgentRunFakeChildSessionRead(
	t *testing.T,
	read factorysessionexecution.SessionReadResult,
) factorysessionexecution.DispatchSummary {
	t.Helper()
	if read.Progress == nil || read.Progress.TotalDispatches != 1 || read.Progress.CompletedDispatches != 1 {
		t.Fatalf("progress = %#v, want one completed dispatch", read.Progress)
	}
	return factorysessionexecution.DispatchSummary{
		ID:           "dispatch-1",
		Label:        "summarize-findings",
		Model:        "gpt-test",
		Provider:     "fake",
		OutputArtifactIDs: []string{"child-artifact-1"},
	}
}

func assertAgentRunFakeChildDispatch(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
	want factorysessionexecution.DispatchSummary,
) {
	t.Helper()
	dispatch := assertListedAgentRunFakeChildDispatch(t, service, sessionID, want)
	assertAgentRunFakeChildDispatchDetail(t, service, sessionID, dispatch)
}

func assertListedAgentRunFakeChildDispatch(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
	want factorysessionexecution.DispatchSummary,
) factorysessionexecution.DispatchSummary {
	t.Helper()
	dispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one dispatch", dispatches.Dispatches)
	}
	dispatch := dispatches.Dispatches[0]
	if dispatch.ID != want.ID {
		t.Fatalf("dispatch id = %q, want %q", dispatch.ID, want.ID)
	}
	if dispatch.Status != factorysessionexecution.DispatchStatusCompleted {
		t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
	}
	if dispatch.DispatchKind != "JAVASCRIPT_AGENT" || dispatch.Label != want.Label {
		t.Fatalf("dispatch metadata = %#v", dispatch)
	}
	if dispatch.Model != want.Model || dispatch.Provider != want.Provider {
		t.Fatalf("dispatch model/provider = %#v", dispatch)
	}
	if len(dispatch.ProviderSessionRefs) != 1 || dispatch.ProviderSessionRefs[0].ID != "fake-provider-session-1" {
		t.Fatalf("providerSessionRefs = %#v", dispatch.ProviderSessionRefs)
	}
	if len(dispatch.OutputArtifactIDs) != 1 || dispatch.OutputArtifactIDs[0] != want.OutputArtifactIDs[0] {
		t.Fatalf("outputArtifactIds = %#v, want %v", dispatch.OutputArtifactIDs, want.OutputArtifactIDs)
	}
	return dispatch
}

func assertAgentRunFakeChildDispatchDetail(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
	dispatch factorysessionexecution.DispatchSummary,
) {
	t.Helper()
	dispatchDetail, err := service.GetDispatch(context.Background(), sessionID, dispatch.ID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.OrchestratorKind != "JAVASCRIPT" || dispatchDetail.Label != dispatch.Label {
		t.Fatalf("dispatch detail = %#v", dispatchDetail)
	}
}

func assertAgentRunFakeChildArtifact(t *testing.T, service factorysessionexecution.Service, sessionID string) {
	t.Helper()
	artifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one child artifact", artifacts.Artifacts)
	}
	childArtifact := artifacts.Artifacts[0]
	if childArtifact.ID != "child-artifact-1" || childArtifact.DispatchID != "dispatch-1" {
		t.Fatalf("child artifact = %#v", childArtifact)
	}
	wantURI := workflowresult.FormatArtifactURI(sessionID, "child-artifact-1")
	if childArtifact.RetrievalRef == nil || childArtifact.RetrievalRef.Href != "/factory-sessions/"+sessionID+"/artifacts/child-artifact-1" {
		t.Fatalf("child artifact retrieval = %#v, uri %q", childArtifact.RetrievalRef, wantURI)
	}
}
