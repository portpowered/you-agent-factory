package response_events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	concurrentIsolationTimeout = 30 * time.Second

	concurrentIsolationPromptFirst  = "isolated-response-events-first-prompt"
	concurrentIsolationPromptSecond = "isolated-response-events-second-prompt"
	concurrentIsolationResultFirst  = "first session ordered response COMPLETE"
	concurrentIsolationResultSecond = "second session ordered response COMPLETE"
)

// TestConcurrentFactorySessionResponseEventStreamsStayIsolatedAndResumeFromCursor
// runs two Factory Sessions concurrently against one public HTTP server, each
// with its own open Response Event SSE stream, and proves three observable
// contracts at the public boundary:
//
//   - typed ordered payloads: each stream decodes its own MESSAGE payload union
//     before its terminal RUN payload union, in emission order;
//   - concurrent-session isolation: neither stream carries the other session's
//     identity, event identities, dispatch identity, or message text;
//   - reconnect/replay: reopening one session's stream from an acknowledged
//     cursor replays exactly the retained suffix, in order, with no duplicate.
func TestConcurrentFactorySessionResponseEventStreamsStayIsolatedAndResumeFromCursor(t *testing.T) {
	t.Parallel()

	runner := newPromptKeyedCodexRunner(map[string]string{
		concurrentIsolationPromptFirst:  concurrentIsolationResultFirst,
		concurrentIsolationPromptSecond: concurrentIsolationResultSecond,
	})
	firstDir := scaffoldConcurrentIsolationFactory(t, concurrentIsolationPromptFirst)
	secondDir := scaffoldConcurrentIsolationFactory(t, concurrentIsolationPromptSecond)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                firstDir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })
	baseURL := server.URL()

	// The default session is addressed through its public alias while its events
	// carry the canonical identity, so the alias route is also under test here.
	firstSessionID := factorysessions.DefaultSessionID
	firstCanonicalID := support.GetDefaultSession(t, baseURL).Id
	if strings.TrimSpace(firstCanonicalID) == "" {
		t.Fatal("default Factory Session reported no canonical identity")
	}
	opened := support.OpenFactorySessionAt(t, baseURL, secondDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("opened Factory Session = %#v, want a second session identity", opened)
	}
	secondSessionID := opened.Session.Id
	if secondSessionID == firstCanonicalID {
		t.Fatalf("second Factory Session id = %q, want an identity distinct from the default session", secondSessionID)
	}
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, secondSessionID) })

	firstStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(baseURL, firstSessionID),
	)
	t.Cleanup(firstStream.Close)
	secondStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(baseURL, secondSessionID),
	)
	t.Cleanup(secondStream.Close)

	invocations := startConcurrentIsolationInvocations(t, baseURL, map[string]string{
		firstSessionID:  concurrentIsolationPromptFirst,
		secondSessionID: concurrentIsolationPromptSecond,
	})

	firstFrames := collectConcurrentIsolationFrames(
		t,
		firstStream,
		concurrentIsolationResultFirst,
		concurrentIsolationTimeout,
	)
	secondFrames := collectConcurrentIsolationFrames(
		t,
		secondStream,
		concurrentIsolationResultSecond,
		concurrentIsolationTimeout,
	)

	assertConcurrentIsolationInvocationCompleted(
		t,
		awaitConcurrentIsolationInvocation(t, invocations[firstSessionID]),
		concurrentIsolationResultFirst,
	)
	assertConcurrentIsolationInvocationCompleted(
		t,
		awaitConcurrentIsolationInvocation(t, invocations[secondSessionID]),
		concurrentIsolationResultSecond,
	)

	assertSessionScopedOrderedTypedPayloads(
		t,
		firstCanonicalID,
		responseEventsFromFrames(firstFrames),
		concurrentIsolationResultFirst,
		concurrentIsolationResultSecond,
	)
	assertSessionScopedOrderedTypedPayloads(
		t,
		secondSessionID,
		responseEventsFromFrames(secondFrames),
		concurrentIsolationResultSecond,
		concurrentIsolationResultFirst,
	)
	assertDisjointResponseEventCorrelation(t, firstFrames, secondFrames)

	if calls := runner.callCount(); calls != 2 {
		t.Fatalf("provider dispatches = %d, want exactly one per concurrent Factory Session", calls)
	}

	assertResponseEventStreamResumesFromCursor(t, baseURL, firstSessionID, firstFrames)
}

// assertResponseEventStreamResumesFromCursor reconnects one session's stream
// from a mid-stream acknowledged cursor and requires the retained suffix to
// arrive in order with no duplicate of the acknowledged prefix.
func assertResponseEventStreamResumesFromCursor(
	t *testing.T,
	baseURL string,
	sessionID string,
	observed []support.FactoryResponseEventFrame,
) {
	t.Helper()

	if len(observed) < 2 {
		t.Fatalf("observed Response Event frames = %d, want at least 2 to resume mid-stream", len(observed))
	}
	cursor := observed[0].Event.Sequence
	retained := retainedFactoryResponseEventsWithoutGaps(
		support.GetFactoryResponseEventsAt(t, baseURL, sessionID),
	)
	wantSuffix := make([]factoryapi.FactoryResponseEvent, 0, len(retained))
	for _, event := range retained {
		if event.Sequence > cursor {
			wantSuffix = append(wantSuffix, event)
		}
	}
	if len(wantSuffix) == 0 {
		t.Fatalf("retained Response Events after cursor %d = 0, want a replayable suffix", cursor)
	}
	if len(wantSuffix) >= len(retained) {
		t.Fatalf(
			"cursor %d excludes no retained Response Event (%d of %d), so the resume would not be observable",
			cursor,
			len(wantSuffix),
			len(retained),
		)
	}

	resumed := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURLWithAfterSequence(baseURL, sessionID, cursor),
	)
	t.Cleanup(resumed.Close)
	frames := collectResponseEventStreamUntilCount(t, resumed, len(wantSuffix), concurrentIsolationTimeout)
	for _, frame := range frames {
		if frame.Event.Sequence <= cursor {
			t.Fatalf(
				"resumed stream replayed acknowledged event %q at sequence %d, want only sequences after %d",
				frame.Event.EventId,
				frame.Event.Sequence,
				cursor,
			)
		}
	}
	assertResponseEventFramesMatchRetainedCatchUp(t, wantSuffix, frames)
	assertResponseEventFramesAscendingSequence(t, frames)
}

// collectConcurrentIsolationFrames reads one session's stream until the
// assistant MESSAGE carrying that session's text arrives, which is the last
// published event of the dispatch. Collection is content-driven so it neither
// hardcodes an event count nor waits out an idle interval.
func collectConcurrentIsolationFrames(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	wantText string,
	timeout time.Duration,
) []support.FactoryResponseEventFrame {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var collected []support.FactoryResponseEventFrame
	for {
		select {
		case <-deadline.C:
			t.Fatalf(
				"timed out waiting for the MESSAGE Response Event containing %q; got %d frames within %s",
				wantText,
				len(collected),
				timeout,
			)
			return collected
		default:
			frame, ok := stream.TryNextFrame(50 * time.Millisecond)
			if !ok {
				continue
			}
			collected = append(collected, frame)
			if responseEventTerminalOutcomeFor(frame.Event) == responseEventFailed {
				t.Fatalf(
					"session stream reported a failed %s Response Event in phase %q",
					frame.Event.Kind,
					frame.Event.Phase,
				)
			}
			if strings.Contains(concurrentIsolationMessageText(frame.Event), wantText) {
				return collected
			}
		}
	}
}

// assertSessionScopedOrderedTypedPayloads decodes the discriminated payload
// unions off one session's stream and requires the RUN lifecycle payloads and
// the assistant MESSAGE payload to arrive in their published order, carrying
// only this session's identity and text.
func assertSessionScopedOrderedTypedPayloads(
	t *testing.T,
	sessionID string,
	events []factoryapi.FactoryResponseEvent,
	wantText string,
	foreignText string,
) {
	t.Helper()

	if len(events) == 0 {
		t.Fatalf("session %q delivered zero Response Events", sessionID)
	}
	assertResponseEventsAscendingSequence(t, events)

	messageIndex, runStartedIndex, runCompletedIndex := -1, -1, -1
	for index, event := range events {
		if event.FactorySessionId != sessionID {
			t.Fatalf(
				"Response Event %q on session %q stream reports factorySessionId %q",
				event.EventId,
				sessionID,
				event.FactorySessionId,
			)
		}
		text := concurrentIsolationMessageText(event)
		if text != "" && strings.Contains(text, foreignText) {
			t.Fatalf(
				"session %q Response Event %q carried the other session's message text %q",
				sessionID,
				event.EventId,
				text,
			)
		}
		if messageIndex < 0 && text != "" && strings.Contains(text, wantText) {
			messageIndex = index
		}
		if event.Kind != factoryapi.FactoryResponseEventKindRun {
			continue
		}
		payload, err := event.Payload.AsFactoryResponseEventRunPayload()
		if err != nil {
			t.Fatalf("session %q RUN payload at index %d does not decode: %v", sessionID, index, err)
		}
		if payload.Status == nil || strings.TrimSpace(*payload.Status) == "" {
			t.Fatalf("session %q RUN payload at index %d = %#v, want a status", sessionID, index, payload)
		}
		switch event.Phase {
		case factoryapi.FactoryResponseEventPhaseStarted:
			if runStartedIndex < 0 {
				runStartedIndex = index
			}
		case factoryapi.FactoryResponseEventPhaseCompleted:
			if runCompletedIndex < 0 {
				runCompletedIndex = index
			}
		}
	}
	if messageIndex < 0 {
		t.Fatalf("session %q delivered no MESSAGE payload containing %q", sessionID, wantText)
	}
	if runStartedIndex < 0 || runCompletedIndex < 0 {
		t.Fatalf(
			"session %q RUN lifecycle indexes = started %d, completed %d, want both published",
			sessionID,
			runStartedIndex,
			runCompletedIndex,
		)
	}
	if runStartedIndex >= runCompletedIndex {
		t.Fatalf(
			"session %q RUN/STARTED at index %d does not precede RUN/COMPLETED at index %d",
			sessionID,
			runStartedIndex,
			runCompletedIndex,
		)
	}
	if runStartedIndex >= messageIndex {
		t.Fatalf(
			"session %q RUN/STARTED at index %d does not precede the assistant MESSAGE at index %d",
			sessionID,
			runStartedIndex,
			messageIndex,
		)
	}
}

// concurrentIsolationMessageText returns the decoded assistant text for MESSAGE
// events and the empty string for every other discriminated payload variant.
func concurrentIsolationMessageText(event factoryapi.FactoryResponseEvent) string {
	if event.Kind != factoryapi.FactoryResponseEventKindMessage {
		return ""
	}
	if event.Phase == factoryapi.FactoryResponseEventPhaseDelta {
		payload, err := event.Payload.AsFactoryResponseEventMessageDeltaPayload()
		if err != nil || payload.TextDelta == nil {
			return ""
		}
		return *payload.TextDelta
	}
	payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
	if err != nil {
		return ""
	}
	for _, block := range payload.ContentBlocks {
		text, err := block.AsFactoryResponseEventTextContentBlock()
		if err == nil && text.Text != "" {
			return text.Text
		}
	}
	return ""
}

func assertDisjointResponseEventCorrelation(
	t *testing.T,
	first []support.FactoryResponseEventFrame,
	second []support.FactoryResponseEventFrame,
) {
	t.Helper()

	eventIDs := make(map[string]struct{}, len(first))
	dispatchIDs := make(map[string]struct{}, len(first))
	for _, frame := range first {
		eventIDs[frame.Event.EventId] = struct{}{}
		if frame.Event.DispatchId != nil && strings.TrimSpace(*frame.Event.DispatchId) != "" {
			dispatchIDs[*frame.Event.DispatchId] = struct{}{}
		}
	}
	if len(dispatchIDs) == 0 {
		t.Fatal("first session Response Events carried no dispatch correlation")
	}
	for _, frame := range second {
		if _, shared := eventIDs[frame.Event.EventId]; shared {
			t.Fatalf("Response Event id %q was delivered on both concurrent session streams", frame.Event.EventId)
		}
		if frame.Event.DispatchId == nil {
			continue
		}
		if _, shared := dispatchIDs[*frame.Event.DispatchId]; shared {
			t.Fatalf(
				"dispatch %q appeared on both concurrent session streams",
				*frame.Event.DispatchId,
			)
		}
	}
}

type concurrentIsolationInvocation struct {
	response factoryapi.InvocationResponse
	err      error
}

func startConcurrentIsolationInvocations(
	t *testing.T,
	baseURL string,
	promptsBySession map[string]string,
) map[string]chan concurrentIsolationInvocation {
	t.Helper()

	results := make(map[string]chan concurrentIsolationInvocation, len(promptsBySession))
	for sessionID, prompt := range promptsBySession {
		done := make(chan concurrentIsolationInvocation, 1)
		results[sessionID] = done
		go func(sessionID, prompt string, done chan concurrentIsolationInvocation) {
			response, err := postConcurrentIsolationInvocation(t.Context(), baseURL, sessionID, prompt)
			done <- concurrentIsolationInvocation{response: response, err: err}
		}(sessionID, prompt, done)
	}
	return results
}

func awaitConcurrentIsolationInvocation(
	t *testing.T,
	results <-chan concurrentIsolationInvocation,
) concurrentIsolationInvocation {
	t.Helper()

	select {
	case result := <-results:
		return result
	case <-time.After(concurrentIsolationTimeout):
		t.Fatal("timed out waiting for a concurrent Factory Session invocation")
		return concurrentIsolationInvocation{}
	}
}

func assertConcurrentIsolationInvocationCompleted(
	t *testing.T,
	result concurrentIsolationInvocation,
	wantText string,
) {
	t.Helper()

	if result.err != nil {
		t.Fatalf("concurrent Factory Session invocation error = %v", result.err)
	}
	if result.response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("concurrent invocation response = %#v, want COMPLETED", result.response)
	}
	if result.response.PrimaryResult == nil || len(*result.response.PrimaryResult) == 0 {
		t.Fatalf("concurrent invocation primary result = %#v, want session-specific content", result.response.PrimaryResult)
	}
	part, err := (*result.response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("concurrent invocation primary result part does not decode as text: %v", err)
	}
	if !strings.Contains(part.Text, wantText) {
		t.Fatalf("concurrent invocation primary result text = %q, want it to contain %q", part.Text, wantText)
	}
}

func postConcurrentIsolationInvocation(
	ctx context.Context,
	baseURL string,
	sessionID string,
	prompt string,
) (factoryapi.InvocationResponse, error) {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: prompt,
	}); err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	payload, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	})
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return factoryapi.InvocationResponse{}, fmt.Errorf(
			"POST %s status = %d: %s",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	return decoded, nil
}

// scaffoldConcurrentIsolationFactory authors one Factory whose workstation
// prompt carries a session-specific marker, so the shared provider effect can
// answer each concurrent dispatch with that session's own output.
func scaffoldConcurrentIsolationFactory(t *testing.T, promptMarker string) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, concurrentIsolationFactoryConfig())
	support.WriteWorkstationConfig(
		t,
		dir,
		"process",
		"---\ntype: MODEL_WORKSTATION\n---\n"+promptMarker+"\n",
	)
	support.WriteAgentConfig(
		t,
		dir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	return dir
}

func concurrentIsolationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "concurrent-response-event-isolation",
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

// promptKeyedCodexRunner answers each concurrent provider dispatch from the
// prompt it actually received, so a stream that carried another session's text
// would have to come from a real cross-session leak rather than from shared
// fixture output.
type promptKeyedCodexRunner struct {
	mu        sync.Mutex
	responses map[string]string
	calls     int
}

func newPromptKeyedCodexRunner(responses map[string]string) *promptKeyedCodexRunner {
	return &promptKeyedCodexRunner{responses: responses}
}

func (r *promptKeyedCodexRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++
	observed := string(request.Stdin) + "\n" + strings.Join(request.Args, "\n")
	for prompt, result := range r.responses {
		if strings.Contains(observed, prompt) {
			return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(result)}, nil
		}
	}
	return platformprocess.CommandResult{}, fmt.Errorf(
		"provider dispatch carried no known session prompt: %q",
		observed,
	)
}

func (r *promptKeyedCodexRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.calls
}
