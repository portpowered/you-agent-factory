package stream_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/factorysessions/stream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	codexpkg "github.com/portpowered/infinite-you/pkg/workers/provider/codex"
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

type codexTerminalOutcomeRunner struct {
	publisher workerprovider.InferenceProgressPublisher
	result    workerprovider.CommandResult
	err       error
}

func (r *codexTerminalOutcomeRunner) Run(ctx context.Context, req workerprovider.CommandRequest) (workerprovider.CommandResult, error) {
	normalizer := codexpkg.NewCommandOutputNormalizer(req, r.publisher)
	normalizer.Observe(workerprocess.OutputStreamStdout, r.result.Stdout)
	normalizer.Flush(ctx, r.result, r.err)
	return r.result, r.err
}

func (*codexTerminalOutcomeRunner) SupportsResponseStreaming() bool { return true }

func (*codexTerminalOutcomeRunner) PublishesCanonicalCodexJSONL() bool { return true }

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

func TestManager_PublishesProviderCanonicalDraftWithoutLegacyRemapping(t *testing.T) {
	t.Parallel()
	session := factorysessions.NewLiveSession("sess-adapter", "/factory", "/workspace", "/workspace", factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory")
	manager := stream.NewManager(&streamTestHost{session: session})
	payload, _ := json.Marshal(responseevents.MessagePayload{Role: "assistant", ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: "snapshot"}}})
	draft := responseevents.Draft{DispatchID: "dispatch-adapter", TurnID: "turn-1", ItemID: "native-message-1", ProviderSessionRef: "thread-1",
		Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted, Payload: payload,
		Provenance: responseevents.Provenance{Provider: "codex", NativeEventType: "item.completed", Delivery: responseevents.DeliveryNativeStream, Representation: responseevents.RepresentationSnapshot, Fidelity: responseevents.FidelityNormalized}}
	fragment := workerprovider.ResponseFragment("dispatch-adapter", &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "thread-1"}, "snapshot")
	fragment.CanonicalDraft = &draft
	manager.InferenceProgressPublisherFactory(nil)(session.ID)(fragment)

	events := session.ResponseEvents.Events()
	if len(events) != 1 || events[0].Kind != responseevents.KindMessage || events[0].Phase != responseevents.PhaseCompleted || events[0].ItemID != "native-message-1" {
		t.Fatalf("canonical events = %#v", events)
	}
}

func TestManager_DoesNotDuplicateTypedTerminalErrorWithLegacyMarker(t *testing.T) {
	t.Parallel()
	session := factorysessions.NewLiveSession("sess-terminal-dedup", "/factory", "/workspace", "/workspace", factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory")
	manager := stream.NewManager(&streamTestHost{session: session})
	publish := manager.InferenceProgressPublisherFactory(nil)(session.ID)
	payload, _ := json.Marshal(responseevents.ErrorPayload{Code: "codex_turn_failed", Message: "Codex request timed out.", Retryable: true})
	draft := responseevents.Draft{DispatchID: "dispatch-terminal", ProviderSessionRef: "thread-terminal", Kind: responseevents.KindError, Phase: responseevents.PhaseFailed, Payload: payload,
		Provenance: responseevents.Provenance{Provider: "codex", NativeEventType: "turn.failed", Delivery: responseevents.DeliveryNativeStream, Representation: responseevents.RepresentationNotification, Fidelity: responseevents.FidelityNormalized}}
	native := workerprovider.ProgressFragment("dispatch-terminal", &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "thread-terminal"}, "ERROR")
	native.CanonicalDraft = &draft
	publish(native)
	terminal := workerprovider.FailedFragment("dispatch-terminal", native.ProviderSessionRef, "Codex request timed out.")
	terminal.CanonicalEventAlreadyPublished = true
	publish(terminal)

	events := session.ResponseEvents.Events()
	if len(events) != 1 || events[0].Kind != responseevents.KindError || events[0].Provenance.NativeEventType != "turn.failed" {
		t.Fatalf("canonical events = %#v, want one native terminal error", events)
	}
}

func TestCodexTerminalReconciliationPublishesOnlyWinningProcessFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result workerprovider.CommandResult
		err    error
	}{
		{name: "cancellation", err: context.DeadlineExceeded},
		{name: "timeout exit", result: workerprovider.CommandResult{ExitCode: 124}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := factorysessions.NewLiveSession("sess-terminal-reconcile", "/factory", "/workspace", "/workspace", factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory")
			manager := stream.NewManager(&streamTestHost{session: session})
			publish := manager.InferenceProgressPublisherFactory(nil)(session.ID)
			tc.result.Stdout = []byte("{\"type\":\"thread.started\",\"thread_id\":\"thread-terminal\"}\n" +
				"{\"type\":\"turn.failed\",\"error\":{\"message\":\"unexpected status 429\"}}\n")
			runner := &codexTerminalOutcomeRunner{publisher: publish, result: tc.result, err: tc.err}
			provider := workerprovider.NewScriptWrapProvider(
				workerprovider.WithProviderCommandRunner(runner),
				workerprovider.WithInferenceProgressPublisher(publish),
				workerprovider.WithCodexJSONLFinalParser(func(output []byte) (interfaces.InferenceResponse, error) {
					parsed, err := codexpkg.ParseFinalOutput(output)
					return interfaces.InferenceResponse{Content: parsed.Content, ProviderSession: parsed.ProviderSession}, err
				}),
				workerprovider.WithCodexJSONLTerminalFailureParser(func(output []byte) (workerprovider.CodexJSONLTerminalFailure, bool) {
					failure, ok := codexpkg.ParseTerminalFailure(output)
					return workerprovider.CodexJSONLTerminalFailure{Type: failure.Type, Message: failure.Message, ProviderSession: failure.ProviderSession}, ok
				}),
			)

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				Dispatch: interfaces.WorkDispatch{DispatchID: "dispatch-terminal"}, ModelProvider: string(interfaces.ModelProviderCodex), UserMessage: "private prompt",
			})
			providerErr, ok := err.(*workerprovider.ProviderError)
			if !ok || providerErr.Type != interfaces.WorkFailureTypeTimeout {
				t.Fatalf("error = %#v, want timeout ProviderError", err)
			}
			var terminal []responseevents.FactoryResponseEvent
			for _, event := range session.ResponseEvents.Events() {
				if event.Kind == responseevents.KindError {
					terminal = append(terminal, event)
				}
			}
			if len(terminal) != 1 {
				t.Fatalf("terminal events = %#v, want exactly one", terminal)
			}
			var payload responseevents.ErrorPayload
			if err := json.Unmarshal(terminal[0].Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Code == "codex_turn_failed" || payload.Message != providerErr.Message {
				t.Fatalf("terminal payload = %#v, want winning process timeout %#v", payload, providerErr)
			}
		})
	}
}

func TestCodexTerminalReconciliationKeepsRecognizedTypedFailure(t *testing.T) {
	session := factorysessions.NewLiveSession("sess-terminal-selection", "/factory", "/workspace", "/workspace", factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, false, "factory")
	manager := stream.NewManager(&streamTestHost{session: session})
	publish := manager.InferenceProgressPublisherFactory(nil)(session.ID)
	result := workerprovider.CommandResult{Stdout: []byte(
		"{\"type\":\"thread.started\",\"thread_id\":\"thread-terminal\"}\n" +
			"{\"type\":\"turn.failed\",\"error\":{\"message\":\"unexpected status 429\"}}\n" +
			"{\"type\":\"error\",\"message\":\"cleanup detail that must not override\"}\n")}
	runner := &codexTerminalOutcomeRunner{publisher: publish, result: result}
	provider := workerprovider.NewScriptWrapProvider(
		workerprovider.WithProviderCommandRunner(runner),
		workerprovider.WithInferenceProgressPublisher(publish),
		workerprovider.WithCodexJSONLFinalParser(func(output []byte) (interfaces.InferenceResponse, error) {
			parsed, err := codexpkg.ParseFinalOutput(output)
			return interfaces.InferenceResponse{Content: parsed.Content, ProviderSession: parsed.ProviderSession}, err
		}),
		workerprovider.WithCodexJSONLTerminalFailureParser(func(output []byte) (workerprovider.CodexJSONLTerminalFailure, bool) {
			failure, ok := codexpkg.ParseTerminalFailure(output)
			return workerprovider.CodexJSONLTerminalFailure{Type: failure.Type, Message: failure.Message, ProviderSession: failure.ProviderSession}, ok
		}),
	)

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch: interfaces.WorkDispatch{DispatchID: "dispatch-terminal"}, ModelProvider: string(interfaces.ModelProviderCodex), UserMessage: "private prompt",
	})
	providerErr, ok := err.(*workerprovider.ProviderError)
	if !ok || providerErr.Type != interfaces.WorkFailureTypeThrottled {
		t.Fatalf("error = %#v, want throttled ProviderError", err)
	}
	var terminal []responseevents.FactoryResponseEvent
	for _, event := range session.ResponseEvents.Events() {
		if event.Kind == responseevents.KindError {
			terminal = append(terminal, event)
		}
	}
	if len(terminal) != 1 || terminal[0].Provenance.NativeEventType != "turn.failed" {
		t.Fatalf("terminal events = %#v, want one recognized turn.failed", terminal)
	}
	var payload responseevents.ErrorPayload
	if err := json.Unmarshal(terminal[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Retryable || payload.Message != providerErr.Message {
		t.Fatalf("terminal payload = %#v, want authoritative failure %#v", payload, providerErr)
	}
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
