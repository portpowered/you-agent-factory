package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestRun_RedirectedHumanResponseStreamConsumesOnlyCanonicalTypedEvents(t *testing.T) {
	preserveRunGlobals(t)

	const canary = "SECRET_PROVIDER_PAYLOAD_7f8a"
	const answer = "authoritative answer"
	var output strings.Builder
	stub := &stubInvocationService{
		run: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
		events: canonicalJavaScriptFactoryEvents(),
	}
	stub.invoke = func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		return apisurface.FactoryInvocationResult{
			Status:        interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: answer}},
		}, nil
	}
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stub, nil
	}

	text := "prompt"
	err := Run(context.Background(), RunConfig{
		FactoryConfigPath: "/tmp/factory.json", InvocationPositionalText: &text,
		InvocationOutputMode: InvocationOutputResponseStream, StdinIsTTY: func() bool { return true },
		OutputIsTTY: false, Output: &output,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "[1] Factory Session started\n" +
		"[2] workflow phase synthesize: ACTIVE\n" +
		"[3] workflow checkpoint written: draft-ready (RESUMABLE)\n\n" +
		responseStreamPrimaryResultHeader + "\n" + answer
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if strings.Contains(output.String(), canary) {
		t.Fatalf("human output used unsafe provider data: %q", output.String())
	}
}

func TestRun_TerminalOnlyModesIgnoreFactoryEventsForLiveAndReplay(t *testing.T) {
	preserveRunGlobals(t)

	const providerCanary = "SECRET_PROVIDER_CHUNK_3c19"
	const finalResult = "authoritative terminal result"
	providerResponse := providerCanary
	events := append(canonicalJavaScriptFactoryEvents(), canonicalFactoryEventWithPayload(
		4,
		interfaces.FactoryEventTypeInferenceResponse,
		workers.InferenceResponseEventPayload{Response: &providerResponse},
	))

	for _, source := range []struct {
		name       string
		replayPath string
	}{
		{name: "live"},
		{name: "replay", replayPath: "/tmp/terminal-only-replay.json"},
	} {
		for _, mode := range []struct {
			name       string
			jsonOutput bool
		}{
			{name: "quiet"},
			{name: "single JSON", jsonOutput: true},
		} {
			t.Run(source.name+"/"+mode.name, func(t *testing.T) {
				var output strings.Builder
				var openedReplayPath string
				stub := &stubInvocationService{
					run: func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					},
					events: events,
					invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
						return apisurface.FactoryInvocationResult{
							RequestID: "request-terminal-only",
							Status:    interfaces.InvocationTerminalStatusCompleted,
							PrimaryResult: []work.WorkContentPart{{
								Type: work.WorkContentPartTypeText,
								Text: finalResult,
							}},
						}, nil
					},
				}
				openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
					openedReplayPath = cfg.ReplayPath
					return stub, nil
				}

				prompt := "terminal-only prompt"
				err := Run(context.Background(), RunConfig{
					FactoryConfigPath:        "/tmp/factory.json",
					InvocationPositionalText: &prompt,
					StdinIsTTY:               func() bool { return true },
					ReplayPath:               source.replayPath,
					TerminalPolicy: terminalpolicy.Resolve(terminalpolicy.Options{
						Quiet: true,
					}),
					JSONOutput: mode.jsonOutput,
					Output:     &output,
				})
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
				if openedReplayPath != source.replayPath {
					t.Fatalf("opened replay path = %q, want %q", openedReplayPath, source.replayPath)
				}
				if strings.Contains(output.String(), providerCanary) || strings.Contains(output.String(), "factory_event") {
					t.Fatalf("terminal-only output exposed lifecycle data: %q", output.String())
				}

				if !mode.jsonOutput {
					if got := output.String(); got != finalResult {
						t.Fatalf("quiet stdout = %q, want raw result %q", got, finalResult)
					}
					return
				}

				var response factoryapi.InvocationResponse
				if err := json.Unmarshal([]byte(output.String()), &response); err != nil {
					t.Fatalf("decode single JSON stdout: %v\n%s", err, output.String())
				}
				if response.Status != factoryapi.InvocationTerminalStatusCompleted ||
					response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
					t.Fatalf("single JSON response = %#v, want one completed InvocationResponse", response)
				}
			})
		}
	}
}

func TestHumanResponseStreamRenderer_CanonicalNonToolGolden(t *testing.T) {
	attempt, delay, percent := 3, int64(12), 42.5
	tests := []struct {
		name  string
		event factorysessions.FactoryResponseEvent
		want  string
	}{
		{"reasoning started", humanResponseEvent(factorysessions.ResponseEventKindReasoning, factorysessions.ResponseEventPhaseStarted, factorysessions.ResponseEventReasoning{}), "reasoning: started\n"},
		{"reasoning delta", humanResponseEvent(factorysessions.ResponseEventKindReasoning, factorysessions.ResponseEventPhaseDelta, factorysessions.ResponseEventReasoning{SummaryDelta: "compare\noptions"}), "reasoning: compare options\n"},
		{"reasoning completed", humanResponseEvent(factorysessions.ResponseEventKindReasoning, factorysessions.ResponseEventPhaseCompleted, factorysessions.ResponseEventReasoning{Summary: "selected path"}), "reasoning: selected path\n"},
		{"reasoning completed empty", humanResponseEvent(factorysessions.ResponseEventKindReasoning, factorysessions.ResponseEventPhaseCompleted, factorysessions.ResponseEventReasoning{}), "reasoning: completed\n"},
		{"retry minimal", humanResponseEvent(factorysessions.ResponseEventKindError, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventErrorPayload{Code: "busy", Message: "hidden", Retryable: true}), "retry: code=busy\n"},
		{"retry full", humanResponseEvent(factorysessions.ResponseEventKindError, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventErrorPayload{Code: "rate_limited", Message: "hidden", RetryAttempt: &attempt, RetryAfterSeconds: &delay}), "retry: code=rate_limited attempt=3 retry-in=12s\n"},
		{"throttle", humanResponseEvent(factorysessions.ResponseEventKindError, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventErrorPayload{Code: "throttled", Message: "hidden"}), "retry: code=throttled\n"},
		{"progress minimal", humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "planning"}), "progress: planning\n"},
		{"progress full", humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "review", Message: "checking\r\nresults", PercentComplete: &percent}), "progress: review — checking results (42.5%)\n"},
		{"stream gap", humanResponseEvent(factorysessions.ResponseEventKindStreamGap, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventStreamGap{FromSequence: 8, ToSequence: 14, FirstAvailableSequence: 15, Reason: "retention\nwindow"}), "stream gap: sequences 8-14 unavailable (reason=retention window)\n"},
		{"item stream gap", humanResponseEvent(factorysessions.ResponseEventKindStreamGap, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventStreamGap{AffectedItemID: "cursor-tool/call-1", ToolCallID: "call-1", Reason: "provider_reconnect"}), "stream gap: item cursor-tool/call-1 lifecycle is incomplete (reason=provider_reconnect)\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var output strings.Builder
			renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
			renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{tc.event})
			renderer.stopProgressRendering()
			if got := output.String(); got != tc.want {
				t.Fatalf("output = %q, want %q", got, tc.want)
			}
		})
	}
}

func humanResponseEvent(kind factorysessions.ResponseEventKind, phase factorysessions.ResponseEventPhase, payload any) factorysessions.FactoryResponseEvent {
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return factorysessions.FactoryResponseEvent{Kind: kind, Phase: phase, Payload: encoded}
}

func TestHumanResponseStreamRenderer_CanonicalMessagesDoNotDuplicatePrimaryResult(t *testing.T) {
	t.Parallel()

	messageEvents := []factorysessions.FactoryResponseEvent{
		humanResponseEvent(factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseDelta, factorysessions.ResponseEventMessageDelta{
			ContentBlockIndex: 0, ContentBlockKind: factorysessions.ResponseEventContentBlockText, TextDelta: "final ",
		}),
		humanResponseEvent(factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseCompleted, factorysessions.ResponseEventMessage{
			Role: "assistant", ContentBlocks: []factorysessions.ResponseEventContentBlock{{Kind: factorysessions.ResponseEventContentBlockText, Text: "final answer"}},
		}),
	}
	result := apisurface.FactoryInvocationResult{
		Status: interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "final answer"},
		},
	}

	t.Run("suppressed messages preserve plain output", func(t *testing.T) {
		var output strings.Builder
		renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
		renderer.PresentResponseEvents(messageEvents)
		if err := renderer.writeFinalInvocationResult(result); err != nil {
			t.Fatalf("writeFinalInvocationResult: %v", err)
		}
		if got := output.String(); got != "final answer" {
			t.Fatalf("output = %q, want unchanged primary result", got)
		}
	})

	t.Run("progress precedes one authoritative answer", func(t *testing.T) {
		var output strings.Builder
		renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
		events := append(messageEvents, humanResponseEvent(
			factorysessions.ResponseEventKindProgress,
			factorysessions.ResponseEventPhaseUpdated,
			factorysessions.ResponseEventProgress{Label: "checking result"},
		))
		renderer.PresentResponseEvents(events)
		if err := renderer.writeFinalInvocationResult(result); err != nil {
			t.Fatalf("writeFinalInvocationResult: %v", err)
		}
		want := "progress: checking result\n\n--- primary result ---\nfinal answer"
		if got := output.String(); got != want || strings.Count(got, "final answer") != 1 {
			t.Fatalf("output = %q, want %q with one answer", got, want)
		}
	})
}

func TestHumanResponseStreamRenderer_TerminalBlockIsWrittenOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result apisurface.FactoryInvocationResult
		want   string
	}{
		{
			name: "success",
			result: apisurface.FactoryInvocationResult{Status: interfaces.InvocationTerminalStatusCompleted,
				PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "answer"}}},
			want: "answer",
		},
		{
			name:   "failure",
			result: apisurface.FactoryInvocationResult{Status: interfaces.InvocationTerminalStatusFailed, ErrorCode: "FAILED_SAFE"},
			want:   "--- invocation outcome ---\nstatus: FAILED\nerror: FAILED_SAFE\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var output strings.Builder
			renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for range 8 {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- renderer.writeFinalInvocationResult(tc.result) }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatalf("writeFinalInvocationResult: %v", err)
				}
			}
			if got := output.String(); got != tc.want {
				t.Fatalf("output = %q, want one terminal block %q", got, tc.want)
			}
		})
	}
}

func TestHumanResponseStreamRenderer_CanonicalToolLifecycleGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		phase  factorysessions.ResponseEventPhase
		status string
		want   string
	}{
		{name: "started", phase: factorysessions.ResponseEventPhaseStarted, status: "provider-pending", want: "started"},
		{name: "completed", phase: factorysessions.ResponseEventPhaseCompleted, status: "provider-done", want: "completed"},
		{name: "failed", phase: factorysessions.ResponseEventPhaseFailed, status: "provider-crashed", want: "failed"},
		{name: "canceled", phase: factorysessions.ResponseEventPhaseCanceled, status: "provider-aborted", want: "canceled"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			event := humanResponseEvent(factorysessions.ResponseEventKindTool, tc.phase, factorysessions.ResponseEventTool{
				ToolCallID: "call\r\n42", ToolName: "read\nfile", Status: tc.status,
			})
			var output strings.Builder
			renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
			renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{event})
			renderer.stopProgressRendering()
			want := "tool: name=read file call=call 42 status=" + tc.want + "\n"
			if got := output.String(); got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestHumanResponseStreamRenderer_ToolLifecyclePreservesCorrelationOrder(t *testing.T) {
	t.Parallel()

	events := []factorysessions.FactoryResponseEvent{
		humanResponseEvent(factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseStarted, factorysessions.ResponseEventTool{ToolCallID: "call-a", ToolName: "search"}),
		humanResponseEvent(factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseStarted, factorysessions.ResponseEventTool{ToolCallID: "call-b", ToolName: "read"}),
		humanResponseEvent(factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseCompleted, factorysessions.ResponseEventTool{ToolCallID: "call-b", ToolName: "read"}),
		humanResponseEvent(factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseFailed, factorysessions.ResponseEventTool{ToolCallID: "call-a", ToolName: "search"}),
	}
	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	renderer.PresentResponseEvents(events)
	renderer.stopProgressRendering()
	want := "tool: name=search call=call-a status=started\n" +
		"tool: name=read call=call-b status=started\n" +
		"tool: name=read call=call-b status=completed\n" +
		"tool: name=search call=call-a status=failed\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want ordered lifecycle %q", got, want)
	}
}

func TestHumanResponseStreamRenderer_ToolDataNeverLeaks(t *testing.T) {
	t.Parallel()

	canaries := []string{
		"SECRET_ARGUMENT", "SECRET_RESULT", "SECRET_DELTA", "SECRET_STATUS",
		"SECRET_RAW_PAYLOAD", "SECRET_PROMPT", "SECRET_CREDENTIAL",
		"SECRET_ENVIRONMENT", "SECRET_PROVENANCE",
	}
	lifecycle := humanResponseEvent(factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseCompleted, map[string]any{
		"toolCallId": "call-safe", "toolName": "safe-tool", "status": canaries[3],
		"argumentsSummary":   map[string]string{"argument": canaries[0], "prompt": canaries[5], "credential": canaries[6]},
		"resultSummary":      map[string]string{"result": canaries[1], "environment": canaries[7]},
		"rawProviderPayload": canaries[4],
	})
	lifecycle.Provenance = factorysessions.ResponseEventProvenance{
		Provider: canaries[8], NativeEventType: canaries[8],
		Delivery: factorysessions.ResponseEventDeliveryNativeStream, Representation: factorysessions.ResponseEventRepresentationNotification,
		Fidelity: factorysessions.ResponseEventFidelityLifecycleOnly,
	}
	delta := humanResponseEvent(factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseDelta, factorysessions.ResponseEventToolDelta{
		ToolCallID: "call-safe", OutputDelta: canaries[2],
	})

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{lifecycle, delta})
	renderer.stopProgressRendering()
	got := output.String()
	if want := "tool: name=safe-tool call=call-safe status=completed\n"; got != want {
		t.Fatalf("output = %q, want lifecycle-only %q", got, want)
	}
	for _, canary := range canaries {
		if strings.Contains(got, canary) {
			t.Fatalf("human output leaked %q: %q", canary, got)
		}
	}
}

func TestHumanResponseStreamRenderer_ToolIdentityIsUTF8SafeAndBounded(t *testing.T) {
	t.Parallel()

	event := humanResponseEvent(factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseStarted, factorysessions.ResponseEventTool{
		ToolCallID: "call\x00id", ToolName: strings.Repeat("界", maxHumanProgressLineBytes),
		ArgumentsSummary: json.RawMessage(`{"secret":"must-not-render"}`),
	})
	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{event})
	renderer.stopProgressRendering()
	got := strings.TrimSuffix(output.String(), "\n")
	if !utf8.ValidString(got) || len([]byte(got)) > maxHumanProgressLineBytes {
		t.Fatalf("bounded tool output is invalid: bytes=%d output=%q", len([]byte(got)), got)
	}
	if strings.ContainsRune(got, '\x00') || strings.Contains(got, "must-not-render") || !strings.HasSuffix(got, "...") {
		t.Fatalf("tool output was not safely normalized and redacted: %q", got)
	}
}

func TestHumanResponseStreamRenderer_CanonicalOutputIsUTF8SafeAndBounded(t *testing.T) {
	t.Parallel()

	event := humanResponseEvent(factorysessions.ResponseEventKindReasoning, factorysessions.ResponseEventPhaseCompleted, factorysessions.ResponseEventReasoning{
		Summary: strings.Repeat("界", maxHumanProgressLineBytes),
	})
	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{event})
	renderer.stopProgressRendering()

	got := strings.TrimSuffix(output.String(), "\n")
	if !utf8.ValidString(got) {
		t.Fatalf("output is not valid UTF-8: %q", got)
	}
	if len([]byte(got)) > maxHumanProgressLineBytes {
		t.Fatalf("output is %d bytes, want at most %d", len([]byte(got)), maxHumanProgressLineBytes)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("output = %q, want stable omission marker", got)
	}
}

func TestHumanResponseStreamRenderer_CanonicalEventsPreserveSessionOrderAndSkipDuplicateSequences(t *testing.T) {
	t.Parallel()

	first := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "first"})
	first.Sequence = 1
	duplicate := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "duplicate"})
	duplicate.Sequence = 1
	second := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "second"})
	second.Sequence = 2

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{first, duplicate, second})
	renderer.stopProgressRendering()
	if got, want := output.String(), "progress: first\nprogress: second\n"; got != want {
		t.Fatalf("output = %q, want ordered unique output %q", got, want)
	}
}

func TestHumanResponseStreamRenderer_CanonicalInvalidEventsDoNotLeakPayload(t *testing.T) {
	t.Parallel()

	const canary = "RAW_UNKNOWN_PROVIDER_EVENT_CANARY"
	const answer = "authoritative answer"
	var output strings.Builder
	validationCalls := 0
	reject := factorysessions.ResponseEventValidator(func(factorysessions.FactoryResponseEvent) error {
		validationCalls++
		return errors.New("programmed owner rejection")
	})
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), reject)
	unknownKind := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: canary})
	unknownKind.Kind = factorysessions.ResponseEventKind("PROVIDER_NATIVE_UNKNOWN")
	invalidPhase := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: canary})
	invalidPhase.Phase = factorysessions.ResponseEventPhaseCompleted
	invalidPayload := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: canary})
	invalidPayload.Payload = json.RawMessage(`{"label":"` + canary + `"`)

	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{unknownKind, invalidPhase, invalidPayload})
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:        interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: answer}},
	}); err != nil {
		t.Fatalf("write final invocation result: %v", err)
	}
	if got := output.String(); got != answer {
		t.Fatalf("invalid canonical event leaked through human stdout: %q", got)
	}
	if validationCalls != 3 {
		t.Fatalf("owner validation calls = %d, want 3", validationCalls)
	}
}

func TestRun_BootstrapErrorSkipsServiceStart(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	builderCalled := false
	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Bootstrap: true,
		ResolveCurrentFactoryDir: func(string) (string, error) {
			return "", interfaces.ErrFactoryLayoutNotFound
		},
		FactoryScaffoldInitializer: func(interfaces.ScaffoldConfig) error {
			return errors.New("bootstrap failed")
		},
	})
	if err == nil {
		t.Fatal("expected bootstrap failure")
	}
	if !strings.Contains(err.Error(), "bootstrap failed") {
		t.Fatalf("error = %q, want bootstrap failure", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not run when bootstrap fails")
	}
}

func TestRun_VerbosePassedToServiceConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedVerbose bool
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedVerbose = cfg.Verbose
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Verbose: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !capturedVerbose {
		t.Fatal("verbose = false, want true")
	}
}

func TestRun_DoesNotDiscoverMissingExecutionBaseDir(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedBaseDir string
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory", DisableDefaultRecording: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedBaseDir != "" {
		t.Fatalf("execution base dir = %q, want the missing Process input to remain missing", capturedBaseDir)
	}
}

func TestRun_PreservesExplicitExecutionBaseDir(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	overrideDir := t.TempDir()

	var capturedBaseDir string
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory", ExecutionBaseDir: overrideDir, DisableDefaultRecording: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if testutil.CanonicalPath(capturedBaseDir) != testutil.CanonicalPath(overrideDir) {
		t.Fatalf("execution base dir = %q, want %q", capturedBaseDir, overrideDir)
	}
}

func TestRun_RuntimeLogConfigPassedToServiceConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	runtimeLogConfig := logging.RuntimeLogConfig{
		MaxSize:    12,
		MaxBackups: 6,
		MaxAge:     21,
		Compress:   true,
	}
	err := Run(context.Background(), RunConfig{
		RuntimeLogDir:    "runtime-logs",
		RuntimeLogConfig: runtimeLogConfig,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service to be built")
	}
	if capturedConfig.RuntimeLogDir != "runtime-logs" {
		t.Fatalf("runtime log dir = %q, want runtime-logs", capturedConfig.RuntimeLogDir)
	}
	if capturedConfig.RuntimeLogConfig != runtimeLogConfig {
		t.Fatalf("runtime log config = %#v, want %#v", capturedConfig.RuntimeLogConfig, runtimeLogConfig)
	}
}

func TestRun_RuntimeMetricsConfigPassedToServiceConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	runtimeMetricsConfig := platformmetrics.RuntimeMetricsConfig{
		MaxSize:    14,
		MaxBackups: 7,
		MaxAge:     28,
		Compress:   true,
	}
	err := Run(context.Background(), RunConfig{
		RuntimeMetricsDir:    "runtime-metrics",
		RuntimeMetricsConfig: runtimeMetricsConfig,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service to be built")
	}
	if capturedConfig.RuntimeMetricsDir != "runtime-metrics" {
		t.Fatalf("runtime metrics dir = %q, want runtime-metrics", capturedConfig.RuntimeMetricsDir)
	}
	if capturedConfig.RuntimeMetricsConfig != runtimeMetricsConfig {
		t.Fatalf("runtime metrics config = %#v, want %#v", capturedConfig.RuntimeMetricsConfig, runtimeMetricsConfig)
	}
}

func TestRun_ModelCacheDirPassedToServiceConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{ModelCacheDir: "managed-model-cache"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service to be built")
	}
	if capturedConfig.ModelCacheDir != "managed-model-cache" {
		t.Fatalf("model cache dir = %q, want managed-model-cache", capturedConfig.ModelCacheDir)
	}
}

func TestRun_WithMockWorkersWithoutPathPassesDefaultConfigToService(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{MockWorkersEnabled: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.MockWorkersConfig == nil {
		t.Fatal("expected default mock workers config to be passed to service")
	}
	if len(capturedConfig.MockWorkersConfig.MockWorkers) != 0 {
		t.Fatalf("mock worker count = %d, want empty default accept config", len(capturedConfig.MockWorkersConfig.MockWorkers))
	}
}

func TestRun_WithMockWorkersConfigPathLoadsConfigBeforeServiceStart(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	mockWorkersPath := filepath.Join(t.TempDir(), "mock-workers.json")
	exitCode := 42
	wantConfig := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		ID: "reviewer-rejects", WorkerName: "reviewer", RunType: workers.MockWorkerRunTypeReject,
		RejectConfig: &workers.MockWorkerRejectConfig{Stderr: "needs changes", ExitCode: &exitCode},
	}}}
	var loadedPath string
	load := workers.MockWorkersConfigLoader(func(path string) (*workers.MockWorkersConfig, error) {
		loadedPath = path
		return wantConfig, nil
	})

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := runWithMockWorkersConfigLoader(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	}, load)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.MockWorkersConfig == nil {
		t.Fatal("expected loaded mock workers config to be passed to service")
	}
	if loadedPath != mockWorkersPath {
		t.Fatalf("loader path = %q, want exact CLI path %q", loadedPath, mockWorkersPath)
	}
	got := capturedConfig.MockWorkersConfig.MockWorkers
	if len(got) != 1 {
		t.Fatalf("mock worker count = %d, want 1", len(got))
	}
	if got[0].ID != "reviewer-rejects" || got[0].WorkerName != "reviewer" {
		t.Fatalf("loaded mock worker = %#v, want reviewer target", got[0])
	}
	if got[0].RejectConfig == nil || got[0].RejectConfig.ExitCode == nil || *got[0].RejectConfig.ExitCode != 42 {
		t.Fatalf("reject config = %#v, want exit code 42", got[0].RejectConfig)
	}
}

func TestRun_WithMockWorkersInvalidPathFailsBeforeServiceStart(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	builderCalled := false
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	wantReadErr := errors.New("read mock workers config: missing")
	err := runWithMockWorkersConfigLoader(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	}, func(string) (*workers.MockWorkersConfig, error) { return nil, wantReadErr })
	if err == nil {
		t.Fatal("expected missing mock workers config path to fail")
	}
	if !strings.Contains(err.Error(), "read mock workers config") {
		t.Fatalf("error = %q, want read mock workers config context", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not be called when mock config loading fails")
	}
}

func TestRun_WithMockWorkersInvalidJSONFailsBeforeServiceStart(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	dir := t.TempDir()
	mockWorkersPath := filepath.Join(dir, "mock-workers.json")
	writeFile(t, mockWorkersPath, `{"mockWorkers":[{"runType":"bogus"}]}`)

	builderCalled := false
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	wantParseErr := errors.New("runType must be one of accept, script, or reject")
	err := runWithMockWorkersConfigLoader(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	}, func(string) (*workers.MockWorkersConfig, error) { return nil, wantParseErr })
	if err == nil {
		t.Fatal("expected invalid mock workers config to fail")
	}
	if !strings.Contains(err.Error(), "runType must be one of") {
		t.Fatalf("error = %q, want runType validation context", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not be called when mock config validation fails")
	}
}

func TestRun_WithSkipPermissionsPassesInvocationOverrideToService(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	override := true
	err := Run(context.Background(), RunConfig{
		InvocationSkipPermissionsOverride: &override,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.InvocationSkipPermissionsOverride == nil {
		t.Fatal("expected invocation skip-permissions override to be passed to service")
	}
	if !*capturedConfig.InvocationSkipPermissionsOverride {
		t.Fatal("expected invocation skip-permissions override to be true")
	}
}

func TestRun_WithoutSkipPermissionsOmitsInvocationOverrideFromService(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedConfig *testRuntimeSelections
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service config to be captured")
	}
	if capturedConfig.InvocationSkipPermissionsOverride != nil {
		t.Fatalf("invocation skip-permissions override = %#v, want nil when flag omitted", capturedConfig.InvocationSkipPermissionsOverride)
	}
}

func TestRun_WithSkipPermissionsDoesNotMutatePersistedFactoryWorkerConfig(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	dir := t.TempDir()
	factoryJSON := filepath.Join(dir, "factory.json")
	writeFile(t, factoryJSON, `{
  "factory": {
    "workers": [
      {
        "name": "agent",
        "type": "MODEL_WORKER",
        "modelProvider": "CLAUDE",
        "skipPermissions": false
      }
    ]
  }
}`)

	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	override := true
	err := Run(context.Background(), RunConfig{
		Dir:                               dir,
		InvocationSkipPermissionsOverride: &override,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := os.ReadFile(factoryJSON)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	if !strings.Contains(string(got), `"skipPermissions": false`) {
		t.Fatalf("factory.json = %s, want persisted skipPermissions:false unchanged", string(got))
	}
	if strings.Contains(string(got), `"skipPermissions": true`) {
		t.Fatalf("factory.json = %s, want skipPermissions not persisted as true", string(got))
	}
}
