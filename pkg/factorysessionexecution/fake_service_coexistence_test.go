package factorysessionexecution_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func contractFixturesPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "api", "testdata", "durable-session-contract-fixtures.json")
}

func TestFakeAndRuntimeService_ImplementSharedContract(t *testing.T) {
	t.Helper()
	var fakeService factorysessionexecution.Service
	var runtimeService factorysessionexecution.Service

	contractFake, err := factorysessionexecution.NewFakeServiceFromContractFixtures(contractFixturesPath(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	fakeService = contractFake

	projectRoot, runtime := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")
	runtimeService = runtime
	_ = projectRoot

	if fakeService == nil || runtimeService == nil {
		t.Fatal("expected non-nil fake and runtime service implementations")
	}
}

func TestFakeService_ConstructorAPIsRemainAvailable(t *testing.T) {
	builtin := factorysessionexecution.BuiltinInterruptedRecoverableScenario()
	if builtin.ListSummary == nil {
		t.Fatal("builtin scenario missing list summary")
	}

	service := factorysessionexecution.NewFakeService(
		factorysessionexecution.WithFakeScenarios(builtin),
		factorysessionexecution.WithPersistedSessionSeeds(*builtin.ListSummary),
	)

	scenarios, err := factorysessionexecution.LoadFakeScenariosFromContractFixtures(contractFixturesPath(t))
	if err != nil {
		t.Fatalf("LoadFakeScenariosFromContractFixtures: %v", err)
	}
	if len(scenarios) == 0 {
		t.Fatal("expected contract fixture scenarios")
	}

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: builtin.RequestID,
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "long-running-audit",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync builtin scenario: %v", err)
	}
	if started.SessionID != builtin.Session.SessionID {
		t.Fatalf("sessionId = %q, want %q", started.SessionID, builtin.Session.SessionID)
	}

	persisted, err := service.ListSessions(context.Background(), factorysessionexecution.ListSessionsRequest{
		Scope: factorysessionexecution.SessionListScopePersisted,
	})
	if err != nil {
		t.Fatalf("ListSessions persisted: %v", err)
	}
	foundSeed := false
	for _, summary := range persisted.DurableSessions {
		if summary.SessionID == builtin.ListSummary.SessionID {
			foundSeed = true
			break
		}
	}
	if !foundSeed {
		t.Fatalf("persisted seeds = %#v, want builtin list summary row", persisted.DurableSessions)
	}
}

func TestFakeService_EventReplay_MatchesLiveFixtureProjection(t *testing.T) {
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(contractFixturesPath(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}

	const requestID = "req-petri-success-001"
	const sessionID = "dur-sess-petri-success-001"

	_, err = service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: requestID,
		Source:    factorysessionexecution.Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	liveSession, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	liveResult, err := service.GetResult(context.Background(), sessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	events, err := service.ReadEvents(context.Background(), sessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) == 0 {
		t.Fatal("fixture events missing")
	}

	replayedSession, replayedResult, err := factorysessionexecution.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if replayedSession.SessionID != liveSession.SessionID {
		t.Fatalf("sessionId = %q, want %q", replayedSession.SessionID, liveSession.SessionID)
	}
	if replayedSession.Status != liveSession.Status {
		t.Fatalf("status = %q, want %q", replayedSession.Status, liveSession.Status)
	}
	if replayedSession.SourceHash != liveSession.SourceHash {
		t.Fatalf("sourceHash = %q, want %q", replayedSession.SourceHash, liveSession.SourceHash)
	}
	if replayedSession.Policy.EffectiveHash != liveSession.Policy.EffectiveHash {
		t.Fatalf("policyHash = %q, want %q", replayedSession.Policy.EffectiveHash, liveSession.Policy.EffectiveHash)
	}
	if liveSession.ResultSummary != nil {
		if replayedSession.ResultSummary == nil {
			t.Fatal("replayed resultSummary missing")
		}
		if replayedSession.ResultSummary.ResultStatus != liveSession.ResultSummary.ResultStatus {
			t.Fatalf("resultSummary.status = %q, want %q", replayedSession.ResultSummary.ResultStatus, liveSession.ResultSummary.ResultStatus)
		}
	}
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)

	if err := factorysessionexecution.ValidateResultMatchesEventProjection(replayedResult, events.Events); err != nil {
		t.Fatalf("ValidateResultMatchesEventProjection: %v", err)
	}
	if err := factorysessionexecution.ValidateResultMatchesSessionRead(replayedSession, replayedResult); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}
}

func TestFakeService_IdempotencyRemainsCompatibleWithRuntimeBackedPath(t *testing.T) {
	fakeService, err := factorysessionexecution.NewFakeServiceFromContractFixtures(contractFixturesPath(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	_, runtimeService := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	fakeReq := factorysessionexecution.StartRequest{
		RequestID: "req-idempotent-replay-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/idempotent.yaml",
		},
		Args: map[string]any{"task": "replay"},
		RequestedPolicy: map[string]any{
			"policyHash": "req-policy-idempotent",
		},
	}
	fakeFirst, err := fakeService.StartAsync(context.Background(), fakeReq)
	if err != nil {
		t.Fatalf("fake StartAsync: %v", err)
	}
	fakeSecond, err := fakeService.StartAsync(context.Background(), fakeReq)
	if err != nil {
		t.Fatalf("fake StartAsync replay: %v", err)
	}
	if fakeSecond.SessionID != fakeFirst.SessionID {
		t.Fatalf("fake replay sessionId = %q, want %q", fakeSecond.SessionID, fakeFirst.SessionID)
	}

	runtimeReq := factorysessionexecution.StartRequest{
		RequestID: "req-runtime-idempotency-coexist",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
	}
	runtimeFirst, err := runtimeService.StartAsync(context.Background(), runtimeReq)
	if err != nil {
		t.Fatalf("runtime StartAsync: %v", err)
	}
	runtimeSecond, err := runtimeService.StartAsync(context.Background(), runtimeReq)
	if err != nil {
		t.Fatalf("runtime StartAsync replay: %v", err)
	}
	if runtimeSecond.SessionID != runtimeFirst.SessionID {
		t.Fatalf("runtime replay sessionId = %q, want %q", runtimeSecond.SessionID, runtimeFirst.SessionID)
	}
}
