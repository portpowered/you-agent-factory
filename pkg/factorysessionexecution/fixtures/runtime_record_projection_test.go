package fixtures_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestJavaScriptRuntimeService_ProgressPrimitives_ProjectsArtifactsPhaseAndProgress(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "progress-primitives.workflow.js", "progress-primitives")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-progress-primitives",
		Source: fse.Source{
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

func TestJavaScriptRuntimeService_AgentRunFakeChild_ProjectsDispatchAndChildArtifact(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-fake-child",
		Source: fse.Source{
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
	projection := fse.ProjectRuntimeExecutionRecords(
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

func newJavaScriptRuntimeServiceWithFixture(t *testing.T, fixtureName, workflowName string) fse.Service {
	t.Helper()
	projectRoot := setupRuntimeWorkflowFixture(t, fixtureName, workflowName)
	return fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
}

func presetWorkerSettings() workflowruntime.WorkerSettingsConfig {
	return workflowruntime.WorkerSettingsConfig{Presets: map[string]workflowruntime.WorkerPreset{
		"careful-review": {ModelProvider: "codex", Model: "gpt-test", ReasoningEffort: "medium"},
	}}
}

func setupRuntimeWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	source := readRuntimeFixture(t, fixtureName)
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), []byte(source), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func readRuntimeFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "orchestrators", "javascript", "runtime", "testdata", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(raw)
}

func assertProgressPrimitivesSessionRead(t *testing.T, read fse.SessionReadResult) {
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

func assertProgressPrimitivesArtifacts(t *testing.T, service fse.Service, sessionID string) {
	t.Helper()
	artifact := assertListedProgressPrimitiveArtifact(t, service, sessionID)
	assertProgressPrimitiveArtifactDetail(t, service, sessionID, artifact)
	assertProgressPrimitiveResultArtifactIDs(t, service, sessionID)
}

func assertListedProgressPrimitiveArtifact(
	t *testing.T,
	service fse.Service,
	sessionID string,
) fse.ArtifactSummary {
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
	service fse.Service,
	sessionID string,
	artifact fse.ArtifactSummary,
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

func assertProgressPrimitiveResultArtifactIDs(t *testing.T, service fse.Service, sessionID string) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
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
	read fse.SessionReadResult,
) fse.DispatchSummary {
	t.Helper()
	if read.Progress == nil || read.Progress.TotalDispatches != 1 || read.Progress.CompletedDispatches != 1 {
		t.Fatalf("progress = %#v, want one completed dispatch", read.Progress)
	}
	return fse.DispatchSummary{
		ID:                "dispatch-1",
		Label:             "summarize-findings",
		Model:             "gpt-test",
		Provider:          "fake",
		OutputArtifactIDs: []string{"child-artifact-1"},
	}
}

func assertAgentRunFakeChildDispatch(
	t *testing.T,
	service fse.Service,
	sessionID string,
	want fse.DispatchSummary,
) {
	t.Helper()
	dispatch := assertListedAgentRunFakeChildDispatch(t, service, sessionID, want)
	assertAgentRunFakeChildDispatchDetail(t, service, sessionID, dispatch)
}

func assertListedAgentRunFakeChildDispatch(
	t *testing.T,
	service fse.Service,
	sessionID string,
	want fse.DispatchSummary,
) fse.DispatchSummary {
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
	if dispatch.Status != fse.DispatchStatusCompleted {
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
	service fse.Service,
	sessionID string,
	dispatch fse.DispatchSummary,
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

func assertAgentRunFakeChildArtifact(t *testing.T, service fse.Service, sessionID string) {
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

func newFakeExecutionServiceFromContractFixtures(t *testing.T) fse.Service {
	t.Helper()
	scenarios, err := fse.LoadFakeScenariosFromContractFixtures(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("LoadFakeScenariosFromContractFixtures: %v", err)
	}
	service, err := fse.NewExecutionService(
		fse.ExecutionProviderFake,
		fse.ServiceConfig{
			FakeOptions: []fse.FakeServiceOption{
				fse.WithFakeScenarios(scenarios...),
			},
		},
	)
	if err != nil {
		t.Fatalf("NewExecutionService(fake): %v", err)
	}
	return service
}

func TestNewExecutionService_FakeProvider_PublishedScenarios_StillDeterministic(t *testing.T) {
	service := newFakeExecutionServiceFromContractFixtures(t)

	successRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	terminal, err := service.StartSync(context.Background(), startRequestForPublished(successRow))
	if err != nil {
		t.Fatalf("StartSync success: %v", err)
	}
	if terminal.SyncOutcome != fse.SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", terminal.SyncOutcome)
	}
	if terminal.SessionID != successRow.SessionID {
		t.Fatalf("sessionId = %q, want %q", terminal.SessionID, successRow.SessionID)
	}
	terminalHash, err := fixtures.SyncStartResultHash(terminal)
	if err != nil {
		t.Fatalf("SyncStartResultHash: %v", err)
	}
	if terminalHash != "sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05" {
		t.Fatalf("sync success hash = %q, want sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05", terminalHash)
	}

	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(runningRow)); err != nil {
		t.Fatalf("StartAsync running: %v", err)
	}
	session, err := service.GetSession(context.Background(), runningRow.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != fse.LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", session.Status)
	}

	result, err := service.GetResult(context.Background(), runningRow.SessionID, fse.ResultRequest{
		Mode: fse.ResultModePartial,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusPartial {
		t.Fatalf("resultStatus = %q, want PARTIAL", result.ResultStatus)
	}
	resultHash, err := fixtures.ProjectedResultReadHash(result)
	if err != nil {
		t.Fatalf("ProjectedResultReadHash: %v", err)
	}
	if resultHash != "sha256:f4830cd3534f5de6491b04dd4c05b2b1e01cf73844877ad922ea7d6547ae07f6" {
		t.Fatalf("result hash = %q, want sha256:f4830cd3534f5de6491b04dd4c05b2b1e01cf73844877ad922ea7d6547ae07f6", resultHash)
	}
}

func TestJavaScriptRuntimeService_UsesExistingFactorySessionReadSurfaces(t *testing.T) {
	service := newJavaScriptRuntimeService(t)
	req := inlineWorkflowStartRequest(
		"req-runtime-session-surfaces-001",
		simpleFinalWorkflowSource,
		map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
		nil,
	)

	started, err := service.StartSync(context.Background(), req)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.SessionID == "" || started.SyncOutcome != fse.SyncOutcomeCompleted {
		t.Fatalf("sync start = %#v, want completed FactorySession execution response", started)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.SessionID != started.SessionID {
		t.Fatalf("sessionId = %q, want %q", session.SessionID, started.SessionID)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.SessionID != started.SessionID || result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("result = %#v, want final FactorySession result read", result)
	}
}
