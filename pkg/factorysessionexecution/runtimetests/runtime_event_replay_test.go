package factorysessionexecution_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestRuntimeService_EventReplay_ReconstructsCompletedSessionProjection(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-event-replay-complete",
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
		t.Fatalf("StartSync: %v", err)
	}

	liveSession, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	liveResult, err := service.GetResult(context.Background(), completed.SessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	events, err := service.ReadEvents(context.Background(), completed.SessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) < 3 {
		t.Fatalf("events = %d, want start/result-updated/completed", len(events.Events))
	}

	replayedSession, replayedResult, err := factorysessionexecution.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)

	if err := factorysessionexecution.ValidateResultMatchesEventProjection(replayedResult, events.Events); err != nil {
		t.Fatalf("ValidateResultMatchesEventProjection: %v", err)
	}
	if err := factorysessionexecution.ValidateResultMatchesSessionRead(replayedSession, replayedResult); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}

	mappedSession := factorysession.SessionReadResponseToAPI(replayedSession)
	if mappedSession.Status != factorysession.SessionReadResponseToAPI(liveSession).Status {
		t.Fatalf("mapped replayed status = %q, want %q", mappedSession.Status, factorysession.SessionReadResponseToAPI(liveSession).Status)
	}
}

func TestRuntimeService_EventReplay_ReconstructsRunningSessionProjection(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-event-replay-running",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	liveSession, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	events, err := service.ReadEvents(context.Background(), started.SessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) < 2 {
		t.Fatalf("events = %d, want start and result-updated", len(events.Events))
	}

	replayedSession, replayedResult, err := factorysessionexecution.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	if replayedResult.ResultStatus != factorysessionexecution.ResultStatusNotReady {
		t.Fatalf("replayed resultStatus = %q, want NOT_READY", replayedResult.ResultStatus)
	}
}

func TestRuntimeService_EventReplay_ReconstructsSyncTimeoutProjection(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")
	timeoutMillis := int64(25)

	timedOut, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-event-replay-timeout",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
		Wait: &factorysessionexecution.WaitOptions{
			TimeoutMillis: &timeoutMillis,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	liveSession, err := service.GetSession(context.Background(), timedOut.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	events, err := service.ReadEvents(context.Background(), timedOut.SessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}

	replayedSession, replayedResult, err := factorysessionexecution.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	if replayedResult.ResultStatus != factorysessionexecution.ResultStatusNotReady {
		t.Fatalf("replayed resultStatus = %q, want NOT_READY", replayedResult.ResultStatus)
	}
}

func TestRuntimeService_EventReplay_IsIdempotent(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-event-replay-idempotent",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	events, err := service.ReadEvents(context.Background(), completed.SessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}

	firstSession, firstResult, err := factorysessionexecution.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection first: %v", err)
	}
	secondSession, secondResult, err := factorysessionexecution.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection second: %v", err)
	}

	if firstSession.Status != secondSession.Status {
		t.Fatalf("status drift = %q vs %q", firstSession.Status, secondSession.Status)
	}
	if firstSession.SourceHash != secondSession.SourceHash {
		t.Fatalf("sourceHash drift = %q vs %q", firstSession.SourceHash, secondSession.SourceHash)
	}
	if firstSession.Policy.EffectiveHash != secondSession.Policy.EffectiveHash {
		t.Fatalf("policyHash drift = %q vs %q", firstSession.Policy.EffectiveHash, secondSession.Policy.EffectiveHash)
	}
	if firstSession.Links.Session != secondSession.Links.Session {
		t.Fatalf("links drift = %q vs %q", firstSession.Links.Session, secondSession.Links.Session)
	}
	if len(firstSession.ArtifactRefs) != len(secondSession.ArtifactRefs) {
		t.Fatalf("artifact stub drift = %d vs %d", len(firstSession.ArtifactRefs), len(secondSession.ArtifactRefs))
	}
	if firstResult.ResultStatus != secondResult.ResultStatus {
		t.Fatalf("resultStatus drift = %q vs %q", firstResult.ResultStatus, secondResult.ResultStatus)
	}
}

func TestRuntimeService_EventReplay_ReconstructsAsyncCompletedSession(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-event-replay-async-complete",
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
		liveSession, err := service.GetSession(context.Background(), started.SessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if liveSession.Status == factorysessionexecution.LifecycleStatusSucceeded {
			events, err := service.ReadEvents(context.Background(), started.SessionID, factorysessionexecution.EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			replayedSession, replayedResult, err := factorysessionexecution.ReplaySessionProjection(events.Events)
			if err != nil {
				t.Fatalf("ReplaySessionProjection: %v", err)
			}
			assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
			if replayedResult.ResultStatus != factorysessionexecution.ResultStatusFinal {
				t.Fatalf("replayed resultStatus = %q, want FINAL", replayedResult.ResultStatus)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session still %q after wait, want SUCCEEDED", liveSession.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertReplayedSessionMatchesLive(t *testing.T, live, replayed factorysessionexecution.SessionReadResult) {
	t.Helper()
	if replayed.SessionID != live.SessionID {
		t.Fatalf("sessionId = %q, want %q", replayed.SessionID, live.SessionID)
	}
	if replayed.Status != live.Status {
		t.Fatalf("status = %q, want %q", replayed.Status, live.Status)
	}
	if replayed.SourceHash != live.SourceHash {
		t.Fatalf("sourceHash = %q, want %q", replayed.SourceHash, live.SourceHash)
	}
	if replayed.Policy.EffectiveHash != live.Policy.EffectiveHash {
		t.Fatalf("policyHash = %q, want %q", replayed.Policy.EffectiveHash, live.Policy.EffectiveHash)
	}
	if replayed.Links.Session != live.Links.Session {
		t.Fatalf("session link = %q, want %q", replayed.Links.Session, live.Links.Session)
	}
	if replayed.Links.Results != live.Links.Results {
		t.Fatalf("results link = %q, want %q", replayed.Links.Results, live.Links.Results)
	}
	if replayed.Links.Events != live.Links.Events {
		t.Fatalf("events link = %q, want %q", replayed.Links.Events, live.Links.Events)
	}
	if live.ResultSummary != nil {
		if replayed.ResultSummary == nil {
			t.Fatal("replayed resultSummary missing")
		}
		if replayed.ResultSummary.ResultStatus != live.ResultSummary.ResultStatus {
			t.Fatalf("resultSummary.status = %q, want %q", replayed.ResultSummary.ResultStatus, live.ResultSummary.ResultStatus)
		}
	}
}

func assertReplayedResultStatusMatchesLive(t *testing.T, live, replayed factorysessionexecution.ResultReadResult) {
	t.Helper()
	if replayed.ResultStatus != live.ResultStatus {
		t.Fatalf("resultStatus = %q, want %q", replayed.ResultStatus, live.ResultStatus)
	}
	if replayed.SessionStatus != live.SessionStatus {
		t.Fatalf("sessionStatus = %q, want %q", replayed.SessionStatus, live.SessionStatus)
	}
}
