package response_events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	codexPartialStreamGoldenCase = "success"

	functionalResponseEventCompletedRetentionWindow = time.Millisecond
	functionalResponseEventMissingSessionID         = "session-missing-response-events"

	sessionExpiryChildSessionID      = "cursor-js-session-expiry"
	sessionExpiryChildWorkflowFile   = "session-expiry-child-progress.workflow.js"
	sessionExpiryChildWorkflowSource = `return (async function () {
  const child = await agent.run({
    prompt: "summarize session expiry",
    label: "session-expiry-child",
    modelProvider: "cursor",
    model: "cursor-test-model",
  });
  return { label: "session-expiry", child: child };
})();`
)

const functionalResponseEventRetentionByteLimit = 16 * 1024 * 1024

// TestAPIResponseEventStreamClosesAtDocumentedBoundary proves the public
// Response Event SSE API closes after retained catch-up when a completed durable
// Factory Session reaches the documented response-event session boundary, rather
// than leaving the SSE connection open indefinitely past that terminal boundary.
func TestAPIResponseEventStreamClosesAtDocumentedBoundary(t *testing.T) {
	start := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := scaffoldSessionExpiryWorkflow(t)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: sessionExpiryChildProgressStream(sessionExpiryChildSessionID, "Child summary COMPLETE"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		ResponseEventRetentionLimits: &factorysessions.ResponseEventRetentionLimits{
			MaxEvents:                10_000,
			MaxBytes:                 functionalResponseEventRetentionByteLimit,
			CompletedRetentionWindow: functionalResponseEventCompletedRetentionWindow,
		},
		Edges: serviceedges.Edges{
			Clock:                 fakeClock,
			ProviderCommandRunner: runner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startSessionExpiryWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("completed durable session id is empty")
	}

	retained := retainedFactoryResponseEventsWithoutGaps(
		support.GetFactoryResponseEventsAt(t, server.URL(), sessionID),
	)
	if len(retained) == 0 {
		t.Fatal("completed session retained zero Response Events before stream open")
	}
	assertResponseEventsAscendingSequence(t, retained)

	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(server.URL(), sessionID),
	)
	drained := collectResponseEventStreamUntilRetainedCount(
		t,
		stream,
		len(retained),
		10*time.Second,
	)
	assertResponseEventFramesMatchRetainedCatchUp(t, retained, drained)
	stream.WaitClosed(5 * time.Second)
	if frame, ok := stream.TryNextFrame(500 * time.Millisecond); ok {
		t.Fatalf(
			"stream delivered unexpected frame after documented boundary close: event %q sequence %d",
			frame.Event.EventId,
			frame.Event.Sequence,
		)
	}
	stream.Close()

	lateStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(server.URL(), sessionID),
	)
	lateDrained := collectResponseEventStreamUntilRetainedCount(
		t,
		lateStream,
		len(retained),
		10*time.Second,
	)
	assertResponseEventFramesMatchRetainedCatchUp(t, retained, lateDrained)
	lateStream.WaitClosed(5 * time.Second)
	lateStream.Close()
}

// TestAPIResponseEventSessionExpiryReturnsTypedGone proves the public Response
// Event SSE API returns typed Gone (410 / RESPONSE_EVENT_STREAM_EXPIRED) before
// the stream opens when retained response-event history has expired for the
// targeted session, without falling back to the default session or conflating
// the outcome with unknown-session or invalid-cursor errors.
func TestAPIResponseEventSessionExpiryReturnsTypedGone(t *testing.T) {
	start := time.Date(2026, 7, 28, 14, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := scaffoldSessionExpiryWorkflow(t)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: sessionExpiryChildProgressStream(sessionExpiryChildSessionID, "Child summary COMPLETE"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		ResponseEventRetentionLimits: &factorysessions.ResponseEventRetentionLimits{
			MaxEvents:                10_000,
			MaxBytes:                 functionalResponseEventRetentionByteLimit,
			CompletedRetentionWindow: functionalResponseEventCompletedRetentionWindow,
		},
		Edges: serviceedges.Edges{
			Clock:                 fakeClock,
			ProviderCommandRunner: runner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startSessionExpiryWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("completed durable session id is empty")
	}
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("session id = %q, want explicit durable session id", sessionID)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1 child invocation", runner.CallCount())
	}

	beforeExpiry := support.GetFactoryResponseEventsAt(t, server.URL(), sessionID)
	if len(beforeExpiry) == 0 {
		t.Fatal("completed session retained zero Response Events before expiry window elapsed")
	}

	fakeClock.Advance(functionalResponseEventCompletedRetentionWindow)

	expired := support.GetFactoryResponseEventStreamHTTPErrorAt(
		t,
		support.SessionResponseEventsURL(server.URL(), sessionID),
	)
	assertResponseEventStreamExpiredError(t, expired)
	if strings.Contains(expired.Body, factorysessions.DefaultSessionID) {
		t.Fatalf(
			"expired stream error body references default session %q, want explicit session only: %s",
			factorysessions.DefaultSessionID,
			expired.Body,
		)
	}

	missing := support.GetFactoryResponseEventStreamHTTPErrorAt(
		t,
		support.SessionResponseEventsURL(server.URL(), functionalResponseEventMissingSessionID),
	)
	assertResponseEventSessionNotFoundError(t, missing)

	invalidCursor := support.GetFactoryResponseEventStreamHTTPErrorAt(
		t,
		support.SessionResponseEventsURL(server.URL(), factorysessions.DefaultSessionID)+
			"?after_sequence=-1",
	)
	assertResponseEventInvalidCursorError(t, invalidCursor)
}

// TestAPIResponseEventCursorGapEmitsStreamGap proves the public Response Event
// SSE API emits STREAM_GAP with explicit lost-range bounds when after_sequence
// predates the currently retained response-event window instead of silently
// skipping unavailable sequences.
func TestAPIResponseEventCursorGapEmitsStreamGap(t *testing.T) {
	loaded := loadCodexPartialStreamGoldenCase(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(
		t,
		dir,
		"worker",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model),
	)

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	providerResult := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	}
	runner := testutil.NewProviderCommandRunner(providerResult, providerResult)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		ResponseEventRetentionLimits: &factorysessions.ResponseEventRetentionLimits{
			MaxEvents: 2,
			MaxBytes:  functionalResponseEventRetentionByteLimit,
		},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	firstWorkName := "cursor-gap-first-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &firstWorkName,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "seed retained response history",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 20*time.Second)

	secondWorkName := "cursor-gap-second-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &secondWorkName,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "evict earlier retained response history",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 20*time.Second)

	sessionID := factorysessions.DefaultSessionID
	retained := retainedFactoryResponseEventsWithoutGaps(
		support.GetFactoryResponseEventsAt(t, server.URL(), sessionID),
	)
	if len(retained) < 2 {
		t.Fatalf(
			"retained Response Event count = %d, want at least 2 after tight retention",
			len(retained),
		)
	}
	firstAvailable := retained[0].Sequence
	if firstAvailable <= 1 {
		t.Fatalf(
			"first retained sequence = %d, want eviction to leave a later firstAvailableSequence",
			firstAvailable,
		)
	}
	staleCursor := int64(1)
	if firstAvailable <= 2 {
		staleCursor = 0
	}

	gapStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURLWithAfterSequence(server.URL(), sessionID, staleCursor),
	)
	gapFrame := gapStream.NextFrame(10 * time.Second)
	assertResponseEventStreamGapFrame(t, gapFrame, staleCursor, firstAvailable)

	retainedFromGapStream := collectResponseEventStreamUntilCount(
		t,
		gapStream,
		len(retained),
		10*time.Second,
	)
	assertResponseEventFramesMatchRetainedCatchUp(t, retained, retainedFromGapStream)
	gapStream.Close()

	resumeStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(server.URL(), sessionID),
	)
	resumeFromStream := collectResponseEventStreamUntilRetainedCount(
		t,
		resumeStream,
		len(retained),
		10*time.Second,
	)
	assertResponseEventFramesMatchRetainedCatchUp(t, retained, resumeFromStream)
	resumeStream.Close()
}

// TestAPIResponseEventSSEStreamsRetainedThenLiveEvents proves the public
// Response Event SSE API first delivers retained matching records in ascending
// FactoryResponseEvent.sequence and then continues with later live matching
// records on the same connection without reordering retained catch-up history.
func TestAPIResponseEventSSEStreamsRetainedThenLiveEvents(t *testing.T) {
	loaded := loadCodexPartialStreamGoldenCase(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(
		t,
		dir,
		"worker",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model),
	)

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	providerResult := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	}
	runner := testutil.NewProviderCommandRunner(providerResult, providerResult)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	firstWorkName := "retained-then-live-first-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &firstWorkName,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "first invocation for retained history",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 20*time.Second)

	sessionID := factorysessions.DefaultSessionID
	retained := support.GetFactoryResponseEventsAt(t, server.URL(), sessionID)
	if len(retained) < 2 {
		t.Fatalf(
			"retained Response Event count = %d, want at least 2 after first invocation",
			len(retained),
		)
	}
	assertResponseEventsAscendingSequence(t, retained)

	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(server.URL(), sessionID),
	)

	retainedFromStream := collectResponseEventStreamUntilCount(
		t,
		stream,
		len(retained),
		10*time.Second,
	)
	assertResponseEventFramesMatchRetainedCatchUp(t, retained, retainedFromStream)

	secondWorkName := "retained-then-live-second-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &secondWorkName,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "second invocation for live continuation",
		},
	})

	maxRetainedSequence := retained[len(retained)-1].Sequence
	liveFromStream := collectResponseEventStreamUntilQuietAfterSequence(
		t,
		stream,
		maxRetainedSequence,
		20*time.Second,
	)
	support.WaitForTerminalStatus(t, server.URL(), 20*time.Second)
	stream.Close()

	if len(liveFromStream) == 0 {
		t.Fatal("live continuation delivered zero Response Events on the same SSE connection")
	}
	for _, frame := range liveFromStream {
		if frame.Event.Sequence <= maxRetainedSequence {
			t.Fatalf(
				"live continuation event sequence %d is not after retained max %d",
				frame.Event.Sequence,
				maxRetainedSequence,
			)
		}
	}
	assertResponseEventFramesAscendingSequence(t, liveFromStream)
	assertResponseEventFramesSSEIDMatchesSequence(
		t,
		append(retainedFromStream, liveFromStream...),
	)

	if runner.CallCount() != 2 {
		t.Fatalf("provider command runner calls = %d, want 2 invocations", runner.CallCount())
	}
}

func loadCodexPartialStreamGoldenCase(t *testing.T) support.ProviderSessionCase {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath(
			string(modelprovider.ProviderCodex),
			codexPartialStreamGoldenCase,
		)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q): %v", codexPartialStreamGoldenCase, err)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityPartialStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityPartialStream,
		)
	}
	return loaded
}

func collectResponseEventStreamUntilRetainedCount(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	wantRetainedCount int,
	timeout time.Duration,
) []support.FactoryResponseEventFrame {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var collected []support.FactoryResponseEventFrame
	for len(collected) < wantRetainedCount {
		select {
		case <-deadline.C:
			t.Fatalf(
				"timed out collecting %d retained Response Event frames from stream; got %d within %s",
				wantRetainedCount,
				len(collected),
				timeout,
			)
		default:
			frame, ok := stream.TryNextFrame(50 * time.Millisecond)
			if !ok {
				continue
			}
			if frame.Event.Kind == factoryapi.FactoryResponseEventKindStreamGap {
				continue
			}
			collected = append(collected, frame)
		}
	}
	return collected
}

func collectResponseEventStreamUntilCount(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	wantCount int,
	timeout time.Duration,
) []support.FactoryResponseEventFrame {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var collected []support.FactoryResponseEventFrame
	for len(collected) < wantCount {
		select {
		case <-deadline.C:
			t.Fatalf(
				"timed out collecting %d Response Event frames from stream; got %d within %s",
				wantCount,
				len(collected),
				timeout,
			)
		default:
			frame, ok := stream.TryNextFrame(50 * time.Millisecond)
			if !ok {
				continue
			}
			collected = append(collected, frame)
		}
	}
	return collected
}

func collectResponseEventStreamUntilQuietAfterSequence(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	afterSequence int64,
	timeout time.Duration,
) []support.FactoryResponseEventFrame {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var collected []support.FactoryResponseEventFrame
	var quiet *time.Timer
	var quietC <-chan time.Time
	for {
		select {
		case <-deadline.C:
			return collected
		case <-quietC:
			return collected
		default:
			frame, ok := stream.TryNextFrame(50 * time.Millisecond)
			if !ok {
				continue
			}
			if frame.Event.Sequence <= afterSequence {
				t.Fatalf(
					"live continuation frame sequence %d is not after acknowledged sequence %d",
					frame.Event.Sequence,
					afterSequence,
				)
			}
			collected = append(collected, frame)
			if quiet == nil {
				quiet = time.NewTimer(250 * time.Millisecond)
			} else {
				if !quiet.Stop() {
					select {
					case <-quiet.C:
					default:
					}
				}
				quiet.Reset(250 * time.Millisecond)
			}
			quietC = quiet.C
		}
	}
}

func assertResponseEventsAscendingSequence(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	previousSequence := int64(-1)
	for index, event := range events {
		if event.Sequence <= previousSequence {
			t.Fatalf(
				"Response Event %d (%s) sequence %d precedes previous sequence %d",
				index,
				event.EventId,
				event.Sequence,
				previousSequence,
			)
		}
		previousSequence = event.Sequence
	}
}

func assertResponseEventFramesAscendingSequence(
	t *testing.T,
	frames []support.FactoryResponseEventFrame,
) {
	t.Helper()

	previousSequence := int64(-1)
	for index, frame := range frames {
		if frame.Event.Sequence <= previousSequence {
			t.Fatalf(
				"Response Event frame %d (%s) sequence %d precedes previous sequence %d",
				index,
				frame.Event.EventId,
				frame.Event.Sequence,
				previousSequence,
			)
		}
		previousSequence = frame.Event.Sequence
	}
}

func assertResponseEventFramesMatchRetainedCatchUp(
	t *testing.T,
	wantRetained []factoryapi.FactoryResponseEvent,
	gotFrames []support.FactoryResponseEventFrame,
) {
	t.Helper()

	if len(gotFrames) != len(wantRetained) {
		t.Fatalf(
			"retained catch-up frame count = %d, want %d",
			len(gotFrames),
			len(wantRetained),
		)
	}
	for index := range wantRetained {
		want := wantRetained[index]
		got := gotFrames[index].Event
		if got.EventId != want.EventId {
			t.Fatalf(
				"retained catch-up event at index %d = %q, want %q",
				index,
				got.EventId,
				want.EventId,
			)
		}
		if got.Sequence != want.Sequence {
			t.Fatalf(
				"retained catch-up sequence at index %d for event %q = %d, want %d",
				index,
				got.EventId,
				got.Sequence,
				want.Sequence,
			)
		}
	}
}

func retainedFactoryResponseEventsWithoutGaps(
	events []factoryapi.FactoryResponseEvent,
) []factoryapi.FactoryResponseEvent {
	filtered := make([]factoryapi.FactoryResponseEvent, 0, len(events))
	for _, event := range events {
		if event.Kind == factoryapi.FactoryResponseEventKindStreamGap {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func assertResponseEventStreamGapFrame(
	t *testing.T,
	frame support.FactoryResponseEventFrame,
	afterSequence int64,
	wantFirstAvailable int64,
) {
	t.Helper()

	if frame.Event.Kind != factoryapi.FactoryResponseEventKindStreamGap {
		t.Fatalf(
			"stale cursor first event kind = %q, want STREAM_GAP",
			frame.Event.Kind,
		)
	}
	if frame.Event.Phase != factoryapi.FactoryResponseEventPhaseUpdated {
		t.Fatalf(
			"STREAM_GAP phase = %q, want UPDATED",
			frame.Event.Phase,
		)
	}
	gapUnion, err := frame.Event.Payload.AsFactoryResponseEventStreamGapPayload()
	if err != nil {
		t.Fatalf("decode STREAM_GAP union payload: %v", err)
	}
	gapPayload, err := gapUnion.AsFactoryResponseEventStreamGapPayload0()
	if err != nil {
		t.Fatalf("decode STREAM_GAP payload: %v", err)
	}
	if gapPayload.FromSequence <= afterSequence {
		t.Fatalf(
			"STREAM_GAP fromSequence = %d, want greater than stale cursor %d",
			gapPayload.FromSequence,
			afterSequence,
		)
	}
	if gapPayload.ToSequence < gapPayload.FromSequence {
		t.Fatalf(
			"STREAM_GAP bounds = %d..%d, want non-empty unavailable range",
			gapPayload.FromSequence,
			gapPayload.ToSequence,
		)
	}
	if gapPayload.FirstAvailableSequence != wantFirstAvailable {
		t.Fatalf(
			"STREAM_GAP firstAvailableSequence = %d, want %d",
			gapPayload.FirstAvailableSequence,
			wantFirstAvailable,
		)
	}
	if gapPayload.Reason == nil || *gapPayload.Reason != "retention_window" {
		t.Fatalf("STREAM_GAP reason = %#v, want retention_window", gapPayload.Reason)
	}
}

func assertResponseEventFramesSSEIDMatchesSequence(
	t *testing.T,
	frames []support.FactoryResponseEventFrame,
) {
	t.Helper()

	for _, frame := range frames {
		wantID := strconv.FormatInt(frame.Event.Sequence, 10)
		if frame.SSEID != wantID {
			t.Fatalf(
				"Response Event SSE id = %q for event %q sequence %d, want decimal sequence %q",
				frame.SSEID,
				frame.Event.EventId,
				frame.Event.Sequence,
				wantID,
			)
		}
		if frame.Event.Kind == factoryapi.FactoryResponseEventKindStreamGap && frame.Event.Sequence == 0 {
			continue
		}
		if frame.SSEID != fmt.Sprint(frame.Event.Sequence) {
			t.Fatalf(
				"Response Event SSE id %q does not match event sequence %d",
				frame.SSEID,
				frame.Event.Sequence,
			)
		}
	}
}

func scaffoldSessionExpiryWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "response-events-session-expiry"})
	if err := os.WriteFile(
		filepath.Join(dir, sessionExpiryChildWorkflowFile),
		[]byte(sessionExpiryChildWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write session expiry child workflow: %v", err)
	}
	return dir
}

func startSessionExpiryWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, sessionExpiryChildWorkflowFile)
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "response-events-session-expiry",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal session expiry workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build session expiry workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start session expiry workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start session expiry workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode session expiry workflow response: %v", err)
	}
	return started
}

func sessionExpiryChildProgressStream(sessionID, result string) []byte {
	records := []string{
		`{"type":"system","subtype":"init","session_id":"` + sessionID + `"}`,
		`{"type":"assistant","timestamp_ms":1,"message":{"role":"assistant","content":[{"type":"text","text":"working"}]},"session_id":"` + sessionID + `"}`,
		`{"type":"tool_call","subtype":"started","call_id":"call-1","tool_call":{"readToolCall":{"args":{"path":"README.md"}}},"session_id":"` + sessionID + `"}`,
		`{"type":"tool_call","subtype":"completed","call_id":"call-1","tool_call":{"readToolCall":{"result":{"success":{}}}},"session_id":"` + sessionID + `"}`,
		string(sessionExpiryChildTerminalRecord(sessionID, result)),
	}
	return []byte(strings.Join(records, "\n"))
}

func sessionExpiryChildTerminalRecord(sessionID, result string) []byte {
	encoded, err := json.Marshal(result)
	if err != nil {
		panic(err)
	}
	return []byte(
		`{"type":"result","subtype":"success","is_error":false,"result":` +
			string(encoded) + `,"session_id":` + strconv.Quote(sessionID) + `}`,
	)
}

func assertResponseEventStreamExpiredError(
	t *testing.T,
	got support.FactoryResponseEventStreamHTTPError,
) {
	t.Helper()

	if got.StatusCode != http.StatusGone {
		t.Fatalf("expired stream status = %d, want 410 Gone", got.StatusCode)
	}
	if got.Response.Family != factoryapi.ErrorFamilyGone {
		t.Fatalf("expired stream family = %q, want GONE", got.Response.Family)
	}
	if got.Response.Code != factoryapi.ErrorResponseCodeRESPONSEEVENTSTREAMEXPIRED {
		t.Fatalf(
			"expired stream code = %q, want RESPONSE_EVENT_STREAM_EXPIRED",
			got.Response.Code,
		)
	}
}

func assertResponseEventSessionNotFoundError(
	t *testing.T,
	got support.FactoryResponseEventStreamHTTPError,
) {
	t.Helper()

	if got.StatusCode != http.StatusNotFound {
		t.Fatalf("missing session stream status = %d, want 404 Not Found", got.StatusCode)
	}
	if got.Response.Code != factoryapi.ErrorResponseCodeRESPONSEEVENTSESSIONNOTFOUND {
		t.Fatalf(
			"missing session stream code = %q, want RESPONSE_EVENT_SESSION_NOT_FOUND",
			got.Response.Code,
		)
	}
}

func assertResponseEventInvalidCursorError(
	t *testing.T,
	got support.FactoryResponseEventStreamHTTPError,
) {
	t.Helper()

	if got.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid cursor stream status = %d, want 400 Bad Request", got.StatusCode)
	}
	if got.Response.Code != factoryapi.ErrorResponseCodeINVALIDRESPONSEEVENTCURSOR {
		t.Fatalf(
			"invalid cursor stream code = %q, want INVALID_RESPONSE_EVENT_CURSOR",
			got.Response.Code,
		)
	}
}
