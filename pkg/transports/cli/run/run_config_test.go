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
	"time"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
)

type canonicalResponseEventRunStub struct {
	stubResponseStreamInvocationService
	store      *factorysessions.SessionResponseEventStore
	subscribed chan struct{}
	once       sync.Once
}

func (s *canonicalResponseEventRunStub) SubscribeSessionResponseEventsFromLatest(
	_ string,
) (*responseeventstore.Subscription, error) {
	subscription, err := s.store.Subscribe(s.store.LatestSequence())
	if err == nil {
		s.once.Do(func() { close(s.subscribed) })
	}
	return subscription, err
}

func TestRun_HumanResponseStreamConsumesOnlyCanonicalTypedEvents(t *testing.T) {
	preserveRunGlobals(t)

	const canary = "SECRET_PROVIDER_PAYLOAD_7f8a"
	const answer = "authoritative answer"
	var output strings.Builder
	legacy := newRecordingResponseStreamAttachable()
	legacy.ensureDispatch("dispatch-1")
	store := factorysessions.NewSessionResponseEventStore(factorysessions.DefaultSessionID)
	stub := &canonicalResponseEventRunStub{
		stubResponseStreamInvocationService: stubResponseStreamInvocationService{
			stubInvocationService: stubInvocationService{run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}},
			attachable: legacy,
		},
		store: store, subscribed: make(chan struct{}),
	}
	stub.invoke = func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		select {
		case <-stub.subscribed:
		case <-time.After(2 * time.Second):
			t.Fatal("canonical response-event subscription was not established")
		}
		legacy.stream("dispatch-1").Append(responsestream.Event{
			Kind: responsestream.EventKindProgressFragment, Type: responsestream.EventTypeProgress,
			Payload: canary,
		})
		attempt := 2
		events := []responseevents.FactoryResponseEvent{
			humanResponseEvent(responseevents.KindReasoning, responseevents.PhaseCompleted, responseevents.ReasoningPayload{Summary: "selected safe path"}),
			humanResponseEvent(responseevents.KindTool, responseevents.PhaseStarted, responseevents.ToolPayload{
				ToolCallID: "call-1", ToolName: "search", ArgumentsSummary: json.RawMessage(`{"secret":"` + canary + `"}`),
			}),
			humanResponseEvent(responseevents.KindTool, responseevents.PhaseDelta, responseevents.ToolDeltaPayload{ToolCallID: "call-1", OutputDelta: canary}),
			humanResponseEvent(responseevents.KindError, responseevents.PhaseUpdated, responseevents.ErrorPayload{
				Code: "rate_limited", Message: canary, Retryable: true, RetryAttempt: &attempt,
			}),
			humanResponseEvent(responseevents.KindProgress, responseevents.PhaseUpdated, responseevents.ProgressPayload{Label: "planning", Message: "next step"}),
			humanResponseEvent(responseevents.KindMessage, responseevents.PhaseDelta, responseevents.MessageDeltaPayload{
				ContentBlockIndex: 0, ContentBlockKind: responseevents.ContentBlockText, TextDelta: answer,
			}),
			humanResponseEvent(responseevents.KindUsage, responseevents.PhaseUpdated, responseevents.UsagePayload{InputTokens: 99, Model: canary}),
			humanResponseEvent(responseevents.KindStreamGap, responseevents.PhaseUpdated, responseevents.StreamGapPayload{
				FromSequence: 40, ToSequence: 44, FirstAvailableSequence: 45, Reason: "retention window",
			}),
		}
		for _, event := range events {
			if _, err := store.Publish(event); err != nil {
				t.Fatalf("publish canonical response event: %v", err)
			}
		}
		store.Complete()
		return apisurface.FactoryInvocationResult{
			Status:        interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: answer}},
		}, nil
	}
	buildInvocationBootstrap = func(context.Context, *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stub, nil
	}

	text := "prompt"
	err := Run(context.Background(), RunConfig{
		FactoryConfigPath: "/tmp/factory.json", InvocationPositionalText: &text,
		InvocationOutputMode: InvocationOutputResponseStream, StdinIsTTY: func() bool { return true }, Output: &output,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "reasoning: selected safe path\n" +
		"tool: name=search call=call-1 status=started\n" +
		"retry: code=rate_limited attempt=2\n" +
		"progress: planning — next step\n" +
		"stream gap: sequences 40-44 unavailable (reason=retention window)\n\n" +
		responseStreamPrimaryResultHeader + "\n" + answer
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if strings.Contains(output.String(), canary) || len(legacy.subscribeCalls) != 0 {
		t.Fatalf("human output used unsafe legacy/provider data: %q", output.String())
	}
}

func TestHumanResponseStreamRenderer_CanonicalNonToolGolden(t *testing.T) {
	attempt, delay, percent := 3, int64(12), 42.5
	tests := []struct {
		name  string
		event responseevents.FactoryResponseEvent
		want  string
	}{
		{"reasoning started", humanResponseEvent(responseevents.KindReasoning, responseevents.PhaseStarted, responseevents.ReasoningPayload{}), "reasoning: started\n"},
		{"reasoning delta", humanResponseEvent(responseevents.KindReasoning, responseevents.PhaseDelta, responseevents.ReasoningPayload{SummaryDelta: "compare\noptions"}), "reasoning: compare options\n"},
		{"reasoning completed", humanResponseEvent(responseevents.KindReasoning, responseevents.PhaseCompleted, responseevents.ReasoningPayload{Summary: "selected path"}), "reasoning: selected path\n"},
		{"reasoning completed empty", humanResponseEvent(responseevents.KindReasoning, responseevents.PhaseCompleted, responseevents.ReasoningPayload{}), "reasoning: completed\n"},
		{"retry minimal", humanResponseEvent(responseevents.KindError, responseevents.PhaseUpdated, responseevents.ErrorPayload{Code: "busy", Message: "hidden", Retryable: true}), "retry: code=busy\n"},
		{"retry full", humanResponseEvent(responseevents.KindError, responseevents.PhaseUpdated, responseevents.ErrorPayload{Code: "rate_limited", Message: "hidden", RetryAttempt: &attempt, RetryAfterSeconds: &delay}), "retry: code=rate_limited attempt=3 retry-in=12s\n"},
		{"throttle", humanResponseEvent(responseevents.KindError, responseevents.PhaseUpdated, responseevents.ErrorPayload{Code: "throttled", Message: "hidden"}), "retry: code=throttled\n"},
		{"progress minimal", humanResponseEvent(responseevents.KindProgress, responseevents.PhaseUpdated, responseevents.ProgressPayload{Label: "planning"}), "progress: planning\n"},
		{"progress full", humanResponseEvent(responseevents.KindProgress, responseevents.PhaseUpdated, responseevents.ProgressPayload{Label: "review", Message: "checking\r\nresults", PercentComplete: &percent}), "progress: review — checking results (42.5%)\n"},
		{"stream gap", humanResponseEvent(responseevents.KindStreamGap, responseevents.PhaseUpdated, responseevents.StreamGapPayload{FromSequence: 8, ToSequence: 14, FirstAvailableSequence: 15, Reason: "retention\nwindow"}), "stream gap: sequences 8-14 unavailable (reason=retention window)\n"},
		{"item stream gap", humanResponseEvent(responseevents.KindStreamGap, responseevents.PhaseUpdated, responseevents.StreamGapPayload{AffectedItemID: "cursor-tool/call-1", ToolCallID: "call-1", Reason: "provider_reconnect"}), "stream gap: item cursor-tool/call-1 lifecycle is incomplete (reason=provider_reconnect)\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var output strings.Builder
			renderer := newHumanResponseStreamRenderer(&output)
			renderer.onResponseEvents([]responseevents.FactoryResponseEvent{tc.event})
			renderer.stopProgressRendering()
			if got := output.String(); got != tc.want {
				t.Fatalf("output = %q, want %q", got, tc.want)
			}
		})
	}
}

func humanResponseEvent(kind responseevents.Kind, phase responseevents.Phase, payload any) responseevents.FactoryResponseEvent {
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return responseevents.FactoryResponseEvent{Kind: kind, Phase: phase, Payload: encoded}
}

func TestHumanResponseStreamRenderer_CanonicalMessagesDoNotDuplicatePrimaryResult(t *testing.T) {
	t.Parallel()

	messageEvents := []responseevents.FactoryResponseEvent{
		humanResponseEvent(responseevents.KindMessage, responseevents.PhaseDelta, responseevents.MessageDeltaPayload{
			ContentBlockIndex: 0, ContentBlockKind: responseevents.ContentBlockText, TextDelta: "final ",
		}),
		humanResponseEvent(responseevents.KindMessage, responseevents.PhaseCompleted, responseevents.MessagePayload{
			Role: "assistant", ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: "final answer"}},
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
		renderer := newHumanResponseStreamRenderer(&output)
		renderer.onResponseEvents(messageEvents)
		if err := renderer.writeFinalInvocationResult(result); err != nil {
			t.Fatalf("writeFinalInvocationResult: %v", err)
		}
		if got := output.String(); got != "final answer" {
			t.Fatalf("output = %q, want unchanged primary result", got)
		}
	})

	t.Run("progress precedes one authoritative answer", func(t *testing.T) {
		var output strings.Builder
		renderer := newHumanResponseStreamRenderer(&output)
		events := append(messageEvents, humanResponseEvent(
			responseevents.KindProgress,
			responseevents.PhaseUpdated,
			responseevents.ProgressPayload{Label: "checking result"},
		))
		renderer.onResponseEvents(events)
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
			renderer := newHumanResponseStreamRenderer(&output)
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
		phase  responseevents.Phase
		status string
		want   string
	}{
		{name: "started", phase: responseevents.PhaseStarted, status: "provider-pending", want: "started"},
		{name: "completed", phase: responseevents.PhaseCompleted, status: "provider-done", want: "completed"},
		{name: "failed", phase: responseevents.PhaseFailed, status: "provider-crashed", want: "failed"},
		{name: "canceled", phase: responseevents.PhaseCanceled, status: "provider-aborted", want: "canceled"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			event := humanResponseEvent(responseevents.KindTool, tc.phase, responseevents.ToolPayload{
				ToolCallID: "call\r\n42", ToolName: "read\nfile", Status: tc.status,
			})
			var output strings.Builder
			renderer := newHumanResponseStreamRenderer(&output)
			renderer.onResponseEvents([]responseevents.FactoryResponseEvent{event})
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

	events := []responseevents.FactoryResponseEvent{
		humanResponseEvent(responseevents.KindTool, responseevents.PhaseStarted, responseevents.ToolPayload{ToolCallID: "call-a", ToolName: "search"}),
		humanResponseEvent(responseevents.KindTool, responseevents.PhaseStarted, responseevents.ToolPayload{ToolCallID: "call-b", ToolName: "read"}),
		humanResponseEvent(responseevents.KindTool, responseevents.PhaseCompleted, responseevents.ToolPayload{ToolCallID: "call-b", ToolName: "read"}),
		humanResponseEvent(responseevents.KindTool, responseevents.PhaseFailed, responseevents.ToolPayload{ToolCallID: "call-a", ToolName: "search"}),
	}
	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onResponseEvents(events)
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
	lifecycle := humanResponseEvent(responseevents.KindTool, responseevents.PhaseCompleted, map[string]any{
		"toolCallId": "call-safe", "toolName": "safe-tool", "status": canaries[3],
		"argumentsSummary":   map[string]string{"argument": canaries[0], "prompt": canaries[5], "credential": canaries[6]},
		"resultSummary":      map[string]string{"result": canaries[1], "environment": canaries[7]},
		"rawProviderPayload": canaries[4],
	})
	lifecycle.Provenance = responseevents.Provenance{
		Provider: canaries[8], NativeEventType: canaries[8],
		Delivery: responseevents.DeliveryNativeStream, Representation: responseevents.RepresentationNotification,
		Fidelity: responseevents.FidelityLifecycleOnly,
	}
	delta := humanResponseEvent(responseevents.KindTool, responseevents.PhaseDelta, responseevents.ToolDeltaPayload{
		ToolCallID: "call-safe", OutputDelta: canaries[2],
	})

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onResponseEvents([]responseevents.FactoryResponseEvent{lifecycle, delta})
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

	event := humanResponseEvent(responseevents.KindTool, responseevents.PhaseStarted, responseevents.ToolPayload{
		ToolCallID: "call\x00id", ToolName: strings.Repeat("界", maxHumanProgressLineBytes),
		ArgumentsSummary: json.RawMessage(`{"secret":"must-not-render"}`),
	})
	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onResponseEvents([]responseevents.FactoryResponseEvent{event})
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

	event := humanResponseEvent(responseevents.KindReasoning, responseevents.PhaseCompleted, responseevents.ReasoningPayload{
		Summary: strings.Repeat("界", maxHumanProgressLineBytes),
	})
	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onResponseEvents([]responseevents.FactoryResponseEvent{event})
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

	first := humanResponseEvent(responseevents.KindProgress, responseevents.PhaseUpdated, responseevents.ProgressPayload{Label: "first"})
	first.Sequence = 1
	duplicate := humanResponseEvent(responseevents.KindProgress, responseevents.PhaseUpdated, responseevents.ProgressPayload{Label: "duplicate"})
	duplicate.Sequence = 1
	second := humanResponseEvent(responseevents.KindProgress, responseevents.PhaseUpdated, responseevents.ProgressPayload{Label: "second"})
	second.Sequence = 2

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onResponseEvents([]responseevents.FactoryResponseEvent{first, duplicate, second})
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
	renderer := newHumanResponseStreamRenderer(&output)
	unknownKind := humanResponseEvent(responseevents.KindProgress, responseevents.PhaseUpdated, responseevents.ProgressPayload{Label: canary})
	unknownKind.Kind = responseevents.Kind("PROVIDER_NATIVE_UNKNOWN")
	invalidPhase := humanResponseEvent(responseevents.KindProgress, responseevents.PhaseUpdated, responseevents.ProgressPayload{Label: canary})
	invalidPhase.Phase = responseevents.PhaseCompleted
	invalidPayload := humanResponseEvent(responseevents.KindProgress, responseevents.PhaseUpdated, responseevents.ProgressPayload{Label: canary})
	invalidPayload.Payload = json.RawMessage(`{"label":"` + canary + `"`)

	renderer.onResponseEvents([]responseevents.FactoryResponseEvent{unknownKind, invalidPhase, invalidPayload})
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:        interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: answer}},
	}); err != nil {
		t.Fatalf("write final invocation result: %v", err)
	}
	if got := output.String(); got != answer {
		t.Fatalf("invalid canonical event leaked through human stdout: %q", got)
	}
}

func TestRun_BootstrapErrorSkipsServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	originalBootstrap := bootstrapFactory
	defer func() {
		buildFactoryService = originalBuilder
		bootstrapFactory = originalBootstrap
	}()

	bootstrapFactory = func(_ string) error {
		return errors.New("bootstrap failed")
	}

	builderCalled := false
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{Bootstrap: true})
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedVerbose bool
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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

func TestRun_DefaultsExecutionBaseDirToCurrentWorkingDirectory(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	workingDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	var capturedBaseDir string
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if testutil.CanonicalPath(capturedBaseDir) != testutil.CanonicalPath(workingDirectory) {
		t.Fatalf("execution base dir = %q, want %q", capturedBaseDir, workingDirectory)
	}
}

func TestRun_ExplicitExecutionBaseDirOverridesCurrentWorkingDirectory(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	workingDirectory := t.TempDir()
	overrideDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	var capturedBaseDir string
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory", ExecutionBaseDir: overrideDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if testutil.CanonicalPath(capturedBaseDir) != testutil.CanonicalPath(overrideDir) {
		t.Fatalf("execution base dir = %q, want %q", capturedBaseDir, overrideDir)
	}
}

func TestRun_RuntimeLogConfigPassedToServiceConfig(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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

func TestRun_WithMockWorkersWithoutPathPassesDefaultConfigToService(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	dir := t.TempDir()
	mockWorkersPath := filepath.Join(dir, "mock-workers.json")
	writeFile(t, mockWorkersPath, `{
  "mockWorkers": [
    {
      "id": "reviewer-rejects",
      "workerName": "reviewer",
      "runType": "reject",
      "rejectConfig": {
        "stderr": "needs changes",
        "exitCode": 42
      }
    }
  ]
}
`)

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.MockWorkersConfig == nil {
		t.Fatal("expected loaded mock workers config to be passed to service")
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	builderCalled := false
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	})
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	dir := t.TempDir()
	mockWorkersPath := filepath.Join(dir, "mock-workers.json")
	writeFile(t, mockWorkersPath, `{"mockWorkers":[{"runType":"bogus"}]}`)

	builderCalled := false
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	})
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
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

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
