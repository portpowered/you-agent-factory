package fixtures_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"
)

func TestJavaScriptRuntimeService_LiveAndReplayEventsRemainIdenticalAcrossPhaseCheckpointPhase(t *testing.T) {
	records := []factory.JavaScriptRuntimeRecord{
		{Sequence: 1, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "plan"}},
		{Sequence: 2, Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-plan", Label: "plan-ready"}},
		{Sequence: 3, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "execute"}},
	}
	workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{RunFunc: func(
		_ context.Context,
		_ factory.JavaScriptRuntimeRequest,
		hooks factory.JavaScriptRuntimeHooks,
	) (factory.JavaScriptRuntimeOutcome, error) {
		for _, record := range records {
			hooks.OnRecord(record)
		}
		value, err := json.Marshal(map[string]any{"status": "complete"})
		return factory.JavaScriptRuntimeOutcome{OK: true, Value: factory.TypedValue{JSON: value}, Records: records}, err
	}}
	service := newJavaScriptRuntimeService(t, workflows)
	request := simpleFinalSyncStartRequest()
	request.RequestID = "req-runtime-phase-checkpoint-phase-live-replay"
	var live []interfaces.FactoryEvent
	request.EventConsumer = func(events []interfaces.FactoryEvent) {
		for _, event := range events {
			live = append(live, event.Clone())
		}
	}

	completed, err := service.StartSync(context.Background(), request)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	replayed, err := service.ReadEvents(context.Background(), completed.SessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	replay := decodeCanonicalFactoryEvents(t, replayed.Events)
	assertCanonicalEventStreamsEqual(t, live, replay)
	assertStrictlyIncreasingFactoryEventSequences(t, live)
	assertPhaseCheckpointPhaseTransitions(t, live)
}

func decodeCanonicalFactoryEvents(t *testing.T, rawEvents []json.RawMessage) []interfaces.FactoryEvent {
	t.Helper()
	events := make([]interfaces.FactoryEvent, len(rawEvents))
	for index, raw := range rawEvents {
		if err := json.Unmarshal(raw, &events[index]); err != nil {
			t.Fatalf("decode canonical Factory Event %d: %v", index, err)
		}
	}
	return events
}

func assertCanonicalEventStreamsEqual(t *testing.T, live, replay []interfaces.FactoryEvent) {
	t.Helper()
	if len(live) != len(replay) {
		t.Fatalf("live events = %d, replay events = %d", len(live), len(replay))
	}
	for index := range live {
		liveJSON, _ := json.Marshal(live[index])
		replayJSON, _ := json.Marshal(replay[index])
		if string(liveJSON) != string(replayJSON) {
			t.Fatalf("event %d differs:\nlive=%s\nreplay=%s", index, liveJSON, replayJSON)
		}
	}
}

func assertStrictlyIncreasingFactoryEventSequences(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	previousSequence := 0
	previousSessionSequence := -1
	for index, event := range events {
		if event.Context.Sequence <= previousSequence {
			t.Fatalf("event %d sequence = %d after %d", index, event.Context.Sequence, previousSequence)
		}
		if event.Context.SessionSequence == nil || *event.Context.SessionSequence <= previousSessionSequence {
			t.Fatalf("event %d sessionSequence = %#v after %d", index, event.Context.SessionSequence, previousSessionSequence)
		}
		previousSequence = event.Context.Sequence
		previousSessionSequence = *event.Context.SessionSequence
	}
}

func assertPhaseCheckpointPhaseTransitions(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	var transitions []interfaces.FactoryEvent
	for _, event := range events {
		if event.Type == interfaces.FactoryEventTypeOrchestratorPhaseChanged ||
			event.Type == interfaces.FactoryEventTypeOrchestratorCheckpointWritten {
			transitions = append(transitions, event)
		}
	}
	wantTypes := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorCheckpointWritten,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
	}
	if len(transitions) != len(wantTypes) {
		t.Fatalf("orchestrator transitions = %d, want %d: %#v", len(transitions), len(wantTypes), transitions)
	}
	for index, wantType := range wantTypes {
		if transitions[index].Type != wantType {
			t.Fatalf("orchestrator transition %d type = %s, want %s", index, transitions[index].Type, wantType)
		}
	}
	var terminalPhase struct {
		PhaseStatus string `json:"phaseStatus"`
	}
	if err := json.Unmarshal(transitions[len(transitions)-1].Payload, &terminalPhase); err != nil {
		t.Fatalf("decode terminal phase payload: %v", err)
	}
	if terminalPhase.PhaseStatus != "COMPLETED" {
		t.Fatalf("terminal phase status = %q, want COMPLETED", terminalPhase.PhaseStatus)
	}
}

func TestJavaScriptRuntimeService_ProgressPrimitives_ProjectsArtifactsPhaseAndProgress(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "progress-primitives.workflow.js", "progress-primitives",
		scriptedRecordOutcome([]factory.JavaScriptRuntimeRecord{
			{Sequence: 1, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "setup"}},
			{Sequence: 2, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "execute"}},
			{Sequence: 3, Kind: factory.JavaScriptRecordKindArtifact, Artifact: &factory.JavaScriptArtifactRecord{ID: "artifact-1", Kind: "log", Label: "step-output", Visibility: "PUBLIC", ContentHash: "sha256:scripted", SizeBytes: 12}},
			{Sequence: 4, Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-1", Label: "after-artifact"}},
		}))

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-progress-primitives",
		Source: fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
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
	service := newJavaScriptRuntimeServiceWithFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child",
		scriptedSingleChildWorkflows(factory.JavaScriptChildExecutionRequest{
			Label: "summarize-findings", Model: "gpt-test",
		}))

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-fake-child",
		Source: fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
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
	records := []factory.JavaScriptRuntimeRecord{
		{
			Sequence: 1,
			Kind:     factory.JavaScriptRecordKindPhase,
			Phase:    &factory.JavaScriptPhaseRecord{Name: "setup"},
		},
		{
			Sequence: 2,
			Kind:     factory.JavaScriptRecordKindPhase,
			Phase:    &factory.JavaScriptPhaseRecord{Name: "execute"},
		},
		{
			Sequence: 3,
			Kind:     factory.JavaScriptRecordKindArtifact,
			Artifact: &factory.JavaScriptArtifactRecord{ID: "artifact-1", Kind: "log", Label: "step-output"},
		},
	}

	observedAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	projection := fse.ProjectRuntimeExecutionRecords(
		"session-progress-primitives",
		records,
		observedAt,
	)
	if projection.Phase != "execute" || projection.PhaseCount != 2 {
		t.Fatalf("phase projection = %#v", projection)
	}
	if len(projection.PhaseSummaries) != 2 || projection.PhaseSummaries[0].Phase != "setup" || projection.PhaseSummaries[1].Phase != "execute" {
		t.Fatalf("phase summaries = %#v, want ordered setup, execute", projection.PhaseSummaries)
	}
	if len(projection.Artifacts) != 1 || projection.Artifacts[0].ID != "artifact-1" {
		t.Fatalf("artifacts = %#v", projection.Artifacts)
	}
	if projection.Progress.TotalDispatches != 0 || projection.Progress.PhaseCount != 2 {
		t.Fatalf("progress = %#v", projection.Progress)
	}
}

func newJavaScriptRuntimeServiceWithFixture(t *testing.T, fixtureName, workflowName string, workflows ...factory.JavaScriptWorkflows) fse.Service {
	t.Helper()
	projectRoot := setupRuntimeWorkflowFixture(t, fixtureName, workflowName)
	config := runtimeServiceConfig{ProjectRoot: projectRoot}
	if len(workflows) > 0 {
		config.Workflows = workflows[0]
	}
	return newConfiguredJavaScriptRuntimeService(config)
}

func scriptedRecordOutcome(records []factory.JavaScriptRuntimeRecord) factory.JavaScriptWorkflows {
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		RunFunc: func(context.Context, factory.JavaScriptRuntimeRequest, factory.JavaScriptRuntimeHooks) (factory.JavaScriptRuntimeOutcome, error) {
			value, err := json.Marshal(map[string]any{"status": "scripted"})
			return factory.JavaScriptRuntimeOutcome{OK: true, Value: factory.TypedValue{JSON: value}, Records: records}, err
		},
	}
}

func presetWorkerSettings() factory.JavaScriptWorkerSettings {
	return factory.JavaScriptWorkerSettings{Presets: map[string]factory.JavaScriptWorkerPreset{
		"careful-review": {ModelProvider: "codex", Model: "gpt-test", ReasoningEffort: "medium"},
	}}
}

func setupRuntimeWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
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
	path := filepath.Join("..", "..", "..", "..", "..", "tests", "fixtures", "javascript_runtime", name)
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
	if len(read.PhaseSummaries) != 2 || read.PhaseSummaries[0].Phase != "setup" || read.PhaseSummaries[1].Phase != "execute" {
		t.Fatalf("phase summaries = %#v, want ordered setup, execute", read.PhaseSummaries)
	}
	if read.LatestCheckpoint == nil || read.LatestCheckpoint.ID == "" || read.LatestCheckpoint.Label != "after-artifact" || read.LatestCheckpoint.Phase != "execute" {
		t.Fatalf("latest checkpoint = %#v, want after-artifact in execute", read.LatestCheckpoint)
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
	wantURI := factory.FormatArtifactURI(sessionID, "child-artifact-1")
	if childArtifact.RetrievalRef == nil || childArtifact.RetrievalRef.Href != "/factory-sessions/"+sessionID+"/artifacts/child-artifact-1" {
		t.Fatalf("child artifact retrieval = %#v, uri %q", childArtifact.RetrievalRef, wantURI)
	}
}

func newFakeExecutionServiceFromContractFixtures(t *testing.T) fse.Service {
	t.Helper()
	scenarios, err := fse.LoadFakeScenariosFromContractFixtures(
		contractFixtureCatalogPath(t),
		fileeffects.ContractFixtureReader(os.ReadFile),
	)
	if err != nil {
		t.Fatalf("LoadFakeScenariosFromContractFixtures: %v", err)
	}
	service, err := newExecutionService(
		fse.ExecutionProviderFake,
		executionServiceConfig{FakeScenarios: scenarios},
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
