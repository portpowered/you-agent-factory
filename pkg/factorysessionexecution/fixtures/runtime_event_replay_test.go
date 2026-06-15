package fixtures_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const runtimeEventSource = "runtime-service"

func TestJavaScriptRuntimeService_EventReplay_ReconstructsCompletedSessionProjection(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-complete",
		Source: fse.Source{
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

	liveSession, liveResult, events := readRuntimeSessionEvents(t, service, completed.SessionID)
	assertRuntimeEventSource(t, events.Events)

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)
	assertReplayedResultMatchesEventProjection(t, replayedResult, events.Events)
	assertReplayedResultMatchesSessionRead(t, replayedSession, replayedResult)

	mappedSession := factorysession.SessionReadResponseToAPI(replayedSession)
	if mappedSession.Status != factorysession.SessionReadResponseToAPI(liveSession).Status {
		t.Fatalf("mapped replayed status = %q, want %q", mappedSession.Status, factorysession.SessionReadResponseToAPI(liveSession).Status)
	}
}

func TestJavaScriptRuntimeService_EventReplay_ReconstructsRunningSessionProjection(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-running",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	liveSession, _, events := readRuntimeSessionEvents(t, service, started.SessionID)
	assertRuntimeEventSource(t, events.Events)
	if len(events.Events) < 2 {
		t.Fatalf("events = %d, want start and result-updated", len(events.Events))
	}

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	if replayedResult.ResultStatus != fse.ResultStatusNotReady {
		t.Fatalf("replayed resultStatus = %q, want NOT_READY", replayedResult.ResultStatus)
	}
}

func TestJavaScriptRuntimeService_EventReplay_ReconstructsSyncTimeoutProjection(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")
	timeoutMillis := int64(25)

	timedOut, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-timeout",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
		Wait: &fse.WaitOptions{
			TimeoutMillis: &timeoutMillis,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	liveSession, liveResult, events := readRuntimeSessionEvents(t, service, timedOut.SessionID)
	assertRuntimeEventSource(t, events.Events)

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)
	if replayedResult.Availability == nil || replayedResult.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("replayed availability = %#v, want SYNC_WAIT_TIMED_OUT", replayedResult.Availability)
	}
}

func TestJavaScriptRuntimeService_EventReplay_IsIdempotent(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-idempotent",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	events, err := service.ReadEvents(context.Background(), completed.SessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	assertRuntimeEventSource(t, events.Events)

	firstSession, firstResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection first: %v", err)
	}
	secondSession, secondResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection second: %v", err)
	}
	assertReplayProjectionStable(t, firstSession, secondSession, firstResult, secondResult)
}

func TestJavaScriptRuntimeService_EventReplay_ReconstructsAsyncCompletedSession(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-async-complete",
		Source: fse.Source{
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
		if liveSession.Status == fse.LifecycleStatusSucceeded {
			events, err := service.ReadEvents(context.Background(), started.SessionID, fse.EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			assertRuntimeEventSource(t, events.Events)
			replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
			if err != nil {
				t.Fatalf("ReplaySessionProjection: %v", err)
			}
			assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
			if replayedResult.ResultStatus != fse.ResultStatusFinal {
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

func readRuntimeSessionEvents(
	t *testing.T,
	service fse.Service,
	sessionID string,
) (fse.SessionReadResult, fse.ResultReadResult, fse.EventReadResult) {
	t.Helper()
	liveSession, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	liveResult, err := service.GetResult(context.Background(), sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	events, err := service.ReadEvents(context.Background(), sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) < 2 {
		t.Fatalf("events = %d, want at least start and result-updated", len(events.Events))
	}
	return liveSession, liveResult, events
}

func assertRuntimeEventSource(t *testing.T, events []json.RawMessage) {
	t.Helper()
	for index, raw := range events {
		var envelope struct {
			Context struct {
				Source *string `json:"source"`
			} `json:"context"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("Unmarshal event[%d]: %v", index, err)
		}
		if envelope.Context.Source == nil || *envelope.Context.Source != runtimeEventSource {
			got := "<nil>"
			if envelope.Context.Source != nil {
				got = *envelope.Context.Source
			}
			t.Fatalf("event[%d].context.source = %q, want %q", index, got, runtimeEventSource)
		}
	}
}

func assertReplayedSessionMatchesLive(t *testing.T, live, replayed fse.SessionReadResult) {
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

func assertReplayedResultStatusMatchesLive(t *testing.T, live, replayed fse.ResultReadResult) {
	t.Helper()
	if replayed.ResultStatus != live.ResultStatus {
		t.Fatalf("resultStatus = %q, want %q", replayed.ResultStatus, live.ResultStatus)
	}
	if replayed.SessionStatus != live.SessionStatus {
		t.Fatalf("sessionStatus = %q, want %q", replayed.SessionStatus, live.SessionStatus)
	}
	if live.Availability == nil {
		if replayed.Availability != nil {
			t.Fatalf("replayed availability = %#v, want nil", replayed.Availability)
		}
		return
	}
	if replayed.Availability == nil {
		t.Fatalf("replayed availability missing, want %#v", live.Availability)
	}
	if replayed.Availability.Reason != live.Availability.Reason {
		t.Fatalf("availability.reason = %q, want %q", replayed.Availability.Reason, live.Availability.Reason)
	}
	if replayed.Availability.Message != live.Availability.Message {
		t.Fatalf("availability.message = %q, want %q", replayed.Availability.Message, live.Availability.Message)
	}
	if replayed.Availability.Retryable != live.Availability.Retryable {
		t.Fatalf("availability.retryable = %v, want %v", replayed.Availability.Retryable, live.Availability.Retryable)
	}
}

func assertReplayedResultMatchesEventProjection(t *testing.T, replayed fse.ResultReadResult, events []json.RawMessage) {
	t.Helper()
	if err := fse.ValidateResultMatchesEventProjection(replayed, events); err != nil {
		t.Fatalf("ValidateResultMatchesEventProjection: %v", err)
	}
}

func assertReplayedResultMatchesSessionRead(t *testing.T, session fse.SessionReadResult, result fse.ResultReadResult) {
	t.Helper()
	if err := fse.ValidateResultMatchesSessionRead(session, result); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}
}

func assertReplayProjectionStable(
	t *testing.T,
	firstSession, secondSession fse.SessionReadResult,
	firstResult, secondResult fse.ResultReadResult,
) {
	t.Helper()
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
