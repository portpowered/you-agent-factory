package factorysessionexecution_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestRuntimeService_StartAsync_ReturnsRunningSessionWithSourceAndPolicy(t *testing.T) {
	projectRoot, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-async-simple",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
		RequestedPolicy: map[string]any{
			"mode": "READ_ONLY",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.SessionID == "" {
		t.Fatal("expected session id")
	}
	if !strings.HasPrefix(started.SessionID, "dur-sess-") {
		t.Fatalf("sessionId = %q, want dur-sess- prefix", started.SessionID)
	}
	if started.Status != string(factorysessionexecution.LifecycleStatusRunning) {
		t.Fatalf("status = %q, want RUNNING", started.Status)
	}
	if started.OrchestratorKind != "JAVASCRIPT" {
		t.Fatalf("orchestratorKind = %q, want JAVASCRIPT", started.OrchestratorKind)
	}
	if started.SourceHash == "" {
		t.Fatal("expected source hash")
	}
	if started.Policy.EffectiveHash == "" {
		t.Fatal("expected policy hash")
	}
	if started.Links.Session != "/factory-sessions/"+started.SessionID {
		t.Fatalf("session link = %q", started.Links.Session)
	}
	if started.Links.Results != "/factory-sessions/"+started.SessionID+"/results" {
		t.Fatalf("results link = %q", started.Links.Results)
	}
	if started.ResolvedSource.SourceRef != workflowsource.ProjectClaudeWorkflowsDir+"/simple-final.js" {
		t.Fatalf("sourceRef = %q", started.ResolvedSource.SourceRef)
	}
	_ = projectRoot
}

func TestRuntimeService_GetSession_ReportsRunningStateWithProgressAndLinks(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-async-busy",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", read.Status)
	}
	if read.OrchestratorKind != "JAVASCRIPT" {
		t.Fatalf("orchestratorKind = %q, want JAVASCRIPT", read.OrchestratorKind)
	}
	if read.SourceHash == "" || read.Policy.EffectiveHash == "" {
		t.Fatal("expected source and policy hashes on session read")
	}
	if read.Progress == nil {
		t.Fatal("expected progress projection")
	}
	if read.Progress.TotalDispatches != 0 {
		t.Fatalf("totalDispatches = %d, want 0 for simple runtime session", read.Progress.TotalDispatches)
	}
	if read.ResultSummary == nil || read.ResultSummary.ResultStatus != string(factorysessionexecution.ResultStatusNotReady) {
		t.Fatalf("resultSummary = %#v, want NOT_READY", read.ResultSummary)
	}
	if read.Links.Session != started.Links.Session {
		t.Fatalf("session link = %q, want %q", read.Links.Session, started.Links.Session)
	}

	dispatches, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("dispatch stubs = %d, want 0", len(dispatches.Dispatches))
	}

	artifacts, err := service.ListArtifacts(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 0 {
		t.Fatalf("artifact stubs = %d, want 0", len(artifacts.Artifacts))
	}

	result, err := service.GetResult(context.Background(), started.SessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != factorysessionexecution.ResultStatusNotReady {
		t.Fatalf("resultStatus = %q, want NOT_READY", result.ResultStatus)
	}
}

func TestRuntimeService_ReadEvents_ModelsSessionStartedForAsyncSession(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	projectRoot := setupRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewRuntimeService(
		factorysessionexecution.StartPrepareContext{
			StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
		},
		factorysessionexecution.WithRuntimeClock(func() time.Time { return fixedNow }),
	)

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-async-events",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	events, err := service.ReadEvents(context.Background(), started.SessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) < 1 {
		t.Fatalf("events = %d, want at least SESSION_STARTED", len(events.Events))
	}
	assertRuntimeCanonicalEvent(t, events.Events[0], "SESSION_STARTED", "session-started/"+started.SessionID)

	var payload struct {
		SourceHash string `json:"sourceHash"`
		PolicyHash string `json:"policyHash"`
	}
	var envelope struct {
		Context struct {
			Source string `json:"source"`
		} `json:"context"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(events.Events[0], &envelope); err != nil {
		t.Fatalf("Unmarshal event: %v", err)
	}
	if envelope.Context.Source != "runtime-service" {
		t.Fatalf("event source = %q, want runtime-service", envelope.Context.Source)
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	if payload.SourceHash == "" {
		t.Fatal("expected sourceHash on SESSION_STARTED payload")
	}
}

func TestRuntimeService_StartAsync_CompletesSimpleFinalInBackground(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-async-complete",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		read, err := service.GetSession(context.Background(), started.SessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if read.Status == factorysessionexecution.LifecycleStatusSucceeded {
			if read.ResultSummary == nil || read.ResultSummary.ResultStatus != string(factorysessionexecution.ResultStatusFinal) {
				t.Fatalf("resultSummary = %#v, want FINAL", read.ResultSummary)
			}
			events, err := service.ReadEvents(context.Background(), started.SessionID, factorysessionexecution.EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			if len(events.Events) < 3 {
				t.Fatalf("events = %d, want start/result-updated/completed", len(events.Events))
			}
			assertRuntimeCanonicalEvent(t, events.Events[2], "SESSION_COMPLETED", "session-completed/"+started.SessionID)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session still %q after wait, want SUCCEEDED", read.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func newRuntimeServiceWithFixture(t *testing.T, fixtureName, workflowName string) (string, *factorysessionexecution.RuntimeService) {
	t.Helper()
	projectRoot := setupRuntimeWorkflowFixture(t, fixtureName, workflowName)
	service := factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	return projectRoot, service
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

func assertRuntimeCanonicalEvent(t *testing.T, raw json.RawMessage, eventType, id string) {
	t.Helper()
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		ID            string `json:"id"`
		Type          string `json:"type"`
		Context       struct {
			Sequence int `json:"sequence"`
		} `json:"context"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal event: %v", err)
	}
	if envelope.ID != id {
		t.Fatalf("id = %q, want %q", envelope.ID, id)
	}
	if envelope.Type != eventType {
		t.Fatalf("type = %q, want %q", envelope.Type, eventType)
	}
	if envelope.Context.Sequence <= 0 {
		t.Fatalf("sequence = %d, want positive", envelope.Context.Sequence)
	}
	if len(envelope.Payload) == 0 {
		t.Fatal("payload missing")
	}
}
