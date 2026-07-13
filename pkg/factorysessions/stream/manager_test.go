package stream_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/factorysessions/stream"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

type streamTestHost struct {
	session     *factorysessions.LiveSession
	streams     *factorysessions.SessionResponseStreamSet
	published   int
	compactions int
	degraded    int
	checkpoint  *factorysessions.JavaScriptCheckpointStore
}

func (h *streamTestHost) RequireSession(_ string) (*factorysessions.LiveSession, error) {
	if h.session == nil {
		return nil, errMissingSession
	}
	return h.session, nil
}

func (h *streamTestHost) GetLiveSession(_ string) *factorysessions.LiveSession {
	return h.session
}

func (h *streamTestHost) ResponseStreams(_ *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if h.streams == nil {
		h.streams = factorysessions.NewSessionResponseStreamSetWithFactory(factorysessions.NewSessionResponseStream)
	}
	return h.streams
}

func (h *streamTestHost) NewResponseStream() *factorysessions.SessionResponseStream {
	return factorysessions.NewSessionResponseStream()
}

func (h *streamTestHost) CloseResponseStreams(_ *factorysessions.LiveSession) {
	if h.streams != nil {
		h.streams.Close()
	}
}

func (h *streamTestHost) CloseResponseStreamDispatch(_ *factorysessions.LiveSession, dispatchID string) bool {
	if h.streams == nil {
		return false
	}
	return h.streams.CloseDispatch(dispatchID)
}

func (h *streamTestHost) JavaScriptCheckpointStore(_ *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	if h.checkpoint == nil {
		h.checkpoint = factorysessions.NewJavaScriptCheckpointStore()
	}
	return h.checkpoint
}

func (h *streamTestHost) ObserveResponseStreamPublished(_ *factorysessions.LiveSession, _ string, _ responsestream.Event) {
	h.published++
}

func (h *streamTestHost) ObserveResponseStreamCompaction(
	_ *factorysessions.LiveSession,
	_ string,
	_ string,
	_ responsestream.CompactionSummary,
) {
	h.compactions++
}

func (h *streamTestHost) ObserveResponseStreamDegraded(
	_ *factorysessions.LiveSession,
	_ string,
	_ string,
	_ string,
	_ *zap.Logger,
	_ error,
) {
	h.degraded++
}

var errMissingSession = errors.New("missing session")

func TestManager_SubscribeAndPublishInferenceProgress(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-stream"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-stream")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))
	publisher(workerprovider.ProgressFragment("dispatch-1", nil, "beta"))

	dispatchIDs, err := manager.DispatchIDs("sess-stream")
	if err != nil {
		t.Fatalf("DispatchIDs: %v", err)
	}
	if len(dispatchIDs) != 1 || dispatchIDs[0] != "dispatch-1" {
		t.Fatalf("dispatch IDs = %#v, want [dispatch-1]", dispatchIDs)
	}

	subscription, err := manager.Subscribe("sess-stream", "dispatch-1", 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(initial.Events) != 2 {
		t.Fatalf("event count = %d, want 2", len(initial.Events))
	}
	if host.published != 2 {
		t.Fatalf("published observations = %d, want 2", host.published)
	}
}

func TestManager_PublishesCanonicalResponseEventsToSessionStore(t *testing.T) {
	t.Parallel()

	session := factorysessions.NewLiveSession(
		"sess-canonical",
		"/factory",
		"/workspace",
		"/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		nil,
		false,
		"factory",
	)
	host := &streamTestHost{session: session}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)(session.ID)
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))

	events := session.ResponseEvents.Events()
	if len(events) != 1 {
		t.Fatalf("canonical event count = %d, want 1", len(events))
	}
	if events[0].FactorySessionID != factorysessions.CanonicalFactorySessionID(session) {
		t.Fatalf("factorySessionId = %q, want %q", events[0].FactorySessionID, factorysessions.CanonicalFactorySessionID(session))
	}
	if events[0].DispatchID != "dispatch-1" {
		t.Fatalf("dispatchId = %q, want dispatch-1", events[0].DispatchID)
	}
}

func TestManager_CursorInvocationPublishesStructuredSnapshotAndUnresolvedToolGap(t *testing.T) {
	session := factorysessions.NewLiveSession(
		"sess-cursor-structured", "/factory", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory",
	)
	host := &streamTestHost{session: session}
	publisher := stream.NewManager(host).InferenceProgressPublisherFactory(nil)(session.ID)
	stdout := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"cursor-session-live"}`,
		`{"type":"assistant","timestamp_ms":1,"message":{"role":"assistant","content":[{"type":"text","text":"draft answer"}]},"session_id":"cursor-session-live"}`,
		`{"type":"tool_call","subtype":"started","call_id":"call-live-1","tool_call":{"readToolCall":{"args":{"path":"README.md"}}},"session_id":"cursor-session-live"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"authoritative answer","session_id":"cursor-session-live"}`,
	}, "\n") + "\n"
	command, args := writeCursorStreamFixture(t, []byte(stdout))

	runner := workerprovider.NewInferenceProgressPublishingCommandRunner(publisher, nil)
	result, err := runner.Run(context.Background(), workerprovider.CommandRequest{
		Command: command, Args: args, DispatchID: "dispatch-cursor-live",
	})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Run() result = %#v, error = %v", result, err)
	}

	events := session.ResponseEvents.Events()
	if len(events) != 5 {
		t.Fatalf("response events = %#v, want session, delta, tool, gap, and snapshot", events)
	}
	assertCursorStructuredInvocationEvents(t, events)
	if host.published != 0 {
		t.Fatalf("legacy fragment publications = %d, want structured Cursor path to bypass compatibility fragments", host.published)
	}
}

func assertCursorStructuredInvocationEvents(t *testing.T, events []responseevents.FactoryResponseEvent) {
	t.Helper()
	want := []struct {
		kind  responseevents.Kind
		phase responseevents.Phase
	}{
		{responseevents.KindSession, responseevents.PhaseStarted},
		{responseevents.KindMessage, responseevents.PhaseDelta},
		{responseevents.KindTool, responseevents.PhaseStarted},
		{responseevents.KindStreamGap, responseevents.PhaseUpdated},
		{responseevents.KindMessage, responseevents.PhaseCompleted},
	}
	for index, expected := range want {
		event := events[index]
		if event.Kind != expected.kind || event.Phase != expected.phase {
			t.Fatalf("events[%d] = %s/%s, want %s/%s", index, event.Kind, event.Phase, expected.kind, expected.phase)
		}
		if event.DispatchID != "dispatch-cursor-live" || event.Provenance.Provider != "cursor" {
			t.Fatalf("events[%d] correlation = %#v, want live Cursor dispatch", index, event)
		}
	}
	assertCursorUnresolvedToolGap(t, events[3])
	assertCursorAuthoritativeSnapshot(t, events[4])
}

func assertCursorUnresolvedToolGap(t *testing.T, event responseevents.FactoryResponseEvent) {
	t.Helper()
	var gap responseevents.StreamGapPayload
	if err := json.Unmarshal(event.Payload, &gap); err != nil {
		t.Fatalf("decode gap: %v", err)
	}
	if gap.AffectedItemID != "cursor-tool/call-live-1" || gap.ToolCallID != "call-live-1" {
		t.Fatalf("gap = %#v, want unresolved call-live-1", gap)
	}
}

func assertCursorAuthoritativeSnapshot(t *testing.T, event responseevents.FactoryResponseEvent) {
	t.Helper()
	var snapshot responseevents.MessagePayload
	if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snapshot.ContentBlocks) != 1 || snapshot.ContentBlocks[0].Text != "authoritative answer" {
		t.Fatalf("snapshot = %#v, want authoritative terminal result", snapshot)
	}
}

func writeCursorStreamFixture(t *testing.T, stdout []byte) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "cursor.stdout")
	if err := os.WriteFile(stdoutPath, stdout, 0o600); err != nil {
		t.Fatalf("write Cursor stdout fixture: %v", err)
	}
	if runtime.GOOS == "windows" {
		script := filepath.Join(dir, "agent.cmd")
		body := fmt.Sprintf("@echo off\r\ntype %q\r\n", stdoutPath)
		if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
			t.Fatalf("write Cursor command fixture: %v", err)
		}
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		return "agent", nil
	}
	script := filepath.Join(dir, "agent")
	body := fmt.Sprintf("#!/bin/sh\ncat %q\n", stdoutPath)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write Cursor command fixture: %v", err)
	}
	return "agent", nil
}

func TestManager_CloseAllPreventsSubscription(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-close-all"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-close-all")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))
	manager.CloseAll(host.session)

	_, err := manager.Subscribe("sess-close-all", "dispatch-1", 0)
	if err != responsestream.ErrSubscriptionClosed {
		t.Fatalf("Subscribe after close all = %v, want %v", err, responsestream.ErrSubscriptionClosed)
	}
}

func TestManager_JavaScriptCheckpointStoreIsSessionOwned(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-checkpoint"},
	}
	manager := stream.NewManager(host)

	first := manager.JavaScriptCheckpointStore(host.session)
	second := manager.JavaScriptCheckpointStore(host.session)
	if first == nil || second == nil || first != second {
		t.Fatalf("checkpoint store = (%p, %p), want same session-owned instance", first, second)
	}
}

func TestManager_CloseDispatch_ReleasesOneDispatchStream(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-close-dispatch"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-close-dispatch")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))

	if !manager.CloseDispatch(host.session, "dispatch-1") {
		t.Fatal("CloseDispatch = false, want true")
	}
}

func TestManager_DispatchCompletionObserverFactory_ClosesDispatchOnCompletion(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-observer"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-observer")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "alpha"))

	observer := manager.DispatchCompletionObserverFactory()("sess-observer")
	observer("dispatch-1")

	subscription, err := manager.Subscribe("sess-observer", "dispatch-1", 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(initial.Events) != 1 {
		t.Fatalf("events = %d, want one buffered event after dispatch close", len(initial.Events))
	}
}

func TestManager_InferenceProgressPublisher_NormalizesFragmentKinds(t *testing.T) {
	t.Parallel()

	host := &streamTestHost{
		session: &factorysessions.LiveSession{ID: "sess-fragments"},
	}
	manager := stream.NewManager(host)
	publisher := manager.InferenceProgressPublisherFactory(nil)("sess-fragments")
	publisher(workerprovider.ResponseFragment("dispatch-1", nil, "response"))
	publisher(workerprovider.CompletedFragment("dispatch-1", nil))
	publisher(workerprovider.FailedFragment("dispatch-1", nil, "failed"))

	subscription, err := manager.Subscribe("sess-fragments", "dispatch-1", 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(initial.Events) != 3 {
		t.Fatalf("event count = %d, want 3", len(initial.Events))
	}
}
