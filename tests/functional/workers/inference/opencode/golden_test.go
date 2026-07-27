package opencode_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	openCodeStructuredSnapshotGoldenCase = "structured-snapshot-success"
	openCodeFinalOnlyFallbackGoldenCase  = "final-only-fallback"
	openCodeStructuredFailureGoldenCase  = "structured-failure"
	openCodeTimeoutGoldenCase            = "timeout"
)

// TestOpenCodeGoldenStructuredSnapshotSuccess replays a sanitized OpenCode
// structured-snapshot transcript through the customer process boundary and
// proves successful structured snapshot outcomes with matching public metadata.
// golden: docs/temp/functional/provider-sessions/opencode/structured-snapshot-success/manifest.json
func TestOpenCodeGoldenStructuredSnapshotSuccess(t *testing.T) {
	loaded, request := loadOpenCodeGoldenCase(
		t,
		openCodeStructuredSnapshotGoldenCase,
		"opencode-structured-snapshot-success",
		support.ProviderSessionFidelitySnapshotOnly,
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderOpenCode, request.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"opencode golden structured snapshot success"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	executablePath := writeOpenCodeFixtureExecutable(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("1.2.3\n")},
		platformprocess.CommandResult{ExitCode: 0},
		platformprocess.CommandResult{
			Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
			Stderr:   []byte(loaded.Stderr),
			ExitCode: exitCode,
		},
	)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			ProviderCommandRunner:    runner,
			WorkersExecutableLocator: fixedExecutableLocator{path: executablePath},
			WorkersResolveSymlinks:   identityResolveSymlinks,
		},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 3 {
		t.Fatalf("provider command runner calls = %d, want discovery probes plus one invocation", runner.CallCount())
	}

	inferencePayload, dispatchOutput := openCodeGoldenInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("inference outcome = %q, want SUCCEEDED", inferencePayload.Outcome)
	}
	if inferencePayload.ProviderSession == nil || inferencePayload.ProviderSession.Id == nil {
		t.Fatal("inference response missing provider session identity")
	}
	if got := support.StringPointerValue(inferencePayload.ProviderSession.Id); got != request.SessionID {
		t.Fatalf("provider session id = %q, want golden session %q", got, request.SessionID)
	}
	if inferencePayload.Response == nil || *inferencePayload.Response != dispatchOutput {
		t.Fatalf("inference response text = %#v, want dispatch output %q", inferencePayload.Response, dispatchOutput)
	}
	if dispatchOutput == "" || !strings.Contains(dispatchOutput, "COMPLETE") {
		t.Fatalf("dispatch output = %q, want terminal COMPLETE-bearing success text", dispatchOutput)
	}

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeOpenCodeProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeOpenCodeResponseEventGoldens(responseEvents),
		InvocationResult: observeOpenCodeInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestOpenCodeGoldenFinalOnlyFallback replays a sanitized OpenCode final-only
// transcript through the customer process boundary and proves authoritative
// terminal success without fabricated streaming deltas or structured snapshot
// lifecycle events.
// golden: docs/temp/functional/provider-sessions/opencode/final-only-fallback/manifest.json
func TestOpenCodeGoldenFinalOnlyFallback(t *testing.T) {
	loaded, request := loadOpenCodeGoldenCase(
		t,
		openCodeFinalOnlyFallbackGoldenCase,
		"opencode-final-only-fallback",
		support.ProviderSessionFidelityFinalOnly,
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderOpenCode, request.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"opencode golden final-only fallback"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	executablePath := writeOpenCodeFixtureExecutable(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("1.2.3\n")},
		platformprocess.CommandResult{
			Stderr:   []byte("unknown option --format\n"),
			ExitCode: 2,
		},
		platformprocess.CommandResult{
			Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
			Stderr:   []byte(loaded.Stderr),
			ExitCode: exitCode,
		},
	)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			ProviderCommandRunner:    runner,
			WorkersExecutableLocator: fixedExecutableLocator{path: executablePath},
			WorkersResolveSymlinks:   identityResolveSymlinks,
		},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 3 {
		t.Fatalf("provider command runner calls = %d, want discovery probes plus one invocation", runner.CallCount())
	}

	inferencePayload, dispatchOutput := openCodeGoldenInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("inference outcome = %q, want SUCCEEDED", inferencePayload.Outcome)
	}
	if inferencePayload.Response == nil || *inferencePayload.Response != dispatchOutput {
		t.Fatalf("inference response text = %#v, want dispatch output %q", inferencePayload.Response, dispatchOutput)
	}
	if dispatchOutput == "" || !strings.Contains(dispatchOutput, "COMPLETE") {
		t.Fatalf("dispatch output = %q, want terminal COMPLETE-bearing success text", dispatchOutput)
	}

	assertOpenCodeFinalOnlyPublicResponseEvents(t, responseEvents)

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeOpenCodeProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeOpenCodeResponseEventGoldens(responseEvents),
		InvocationResult: observeOpenCodeInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestOpenCodeGoldenStructuredFailureAndTimeout replays sanitized OpenCode
// structured-failure and timeout transcripts through the customer process
// boundary and proves those public failure classes remain distinct.
// golden: docs/temp/functional/provider-sessions/opencode/structured-failure/manifest.json
// golden: docs/temp/functional/provider-sessions/opencode/timeout/manifest.json
func TestOpenCodeGoldenStructuredFailureAndTimeout(t *testing.T) {
	t.Run("structured-failure", func(t *testing.T) {
		runOpenCodeFailureGoldenCase(
			t,
			openCodeStructuredFailureGoldenCase,
			"opencode-structured-failure",
			support.ProviderSessionFidelitySnapshotOnly,
			factoryapi.WorkFailureTypeAuthFailure,
			factoryapi.WorkFailureTypeTimeout,
		)
	})
	t.Run("timeout", func(t *testing.T) {
		runOpenCodeFailureGoldenCase(
			t,
			openCodeTimeoutGoldenCase,
			"opencode-timeout",
			support.ProviderSessionFidelitySnapshotOnly,
			factoryapi.WorkFailureTypeTimeout,
			factoryapi.WorkFailureTypeAuthFailure,
		)
	})
}

func runOpenCodeFailureGoldenCase(
	t *testing.T,
	caseName string,
	manifestID string,
	fidelityClass string,
	wantReason factoryapi.WorkFailureType,
	notReason factoryapi.WorkFailureType,
) {
	t.Helper()

	loaded, request := loadOpenCodeGoldenCase(
		t,
		caseName,
		manifestID,
		fidelityClass,
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderOpenCode, request.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"opencode golden `+caseName+`"}`))

	exitCode := 1
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	executablePath := writeOpenCodeFixtureExecutable(t)
	runner := newOpenCodeGoldenReplayRunner(loaded, exitCode)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			ProviderCommandRunner:    runner,
			WorkersExecutableLocator: fixedExecutableLocator{path: executablePath},
			WorkersResolveSymlinks:   identityResolveSymlinks,
		},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if wantReason != factoryapi.WorkFailureTypeTimeout {
		if runner.CallCount() != 3 {
			t.Fatalf("provider command runner calls = %d, want discovery probes plus one invocation", runner.CallCount())
		}
	} else if runner.CallCount() < 4 {
		t.Fatalf("provider command runner calls = %d, want discovery probes plus retryable timeout attempts", runner.CallCount())
	}

	inferencePayload := openCodeGoldenFailedInferenceObservation(t, events)
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if got := inferencePayload.FailureDetail.Reason; got != wantReason {
		t.Fatalf("failure reason = %q, want %q", got, wantReason)
	}
	if notReason != "" && inferencePayload.FailureDetail.Reason == notReason {
		t.Fatalf("failure reason = %q, must remain distinct from %q", inferencePayload.FailureDetail.Reason, notReason)
	}
	if inferencePayload.ProviderSession == nil || inferencePayload.ProviderSession.Id == nil {
		t.Fatal("inference response missing provider session identity")
	}
	if got := support.StringPointerValue(inferencePayload.ProviderSession.Id); got != request.SessionID {
		t.Fatalf("provider session id = %q, want golden session %q", got, request.SessionID)
	}
	assertOpenCodeFailureDoesNotLeakSensitiveOutput(t, events, responseEvents)

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeOpenCodeProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeOpenCodeResponseEventGoldens(responseEvents),
		InvocationResult: observeOpenCodeFailedInvocationResultGolden(inferencePayload),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

type openCodeGoldenRequest struct {
	Model     string `json:"model"`
	SessionID string `json:"session_id"`
}

func loadOpenCodeGoldenCase(
	t *testing.T,
	caseName string,
	manifestID string,
	fidelityClass string,
) (support.ProviderSessionCase, openCodeGoldenRequest) {
	t.Helper()
	caseDir := filepath.Join(
		testutil.MustRepoRoot(t),
		filepath.FromSlash(support.ProviderSessionFixturePath("opencode", caseName)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != manifestID {
		t.Fatalf("manifest.ID = %q, want %s", loaded.Manifest.ID, manifestID)
	}
	if loaded.Manifest.FidelityClass != fidelityClass {
		t.Fatalf("manifest.fidelityClass = %q, want %q", loaded.Manifest.FidelityClass, fidelityClass)
	}
	var request openCodeGoldenRequest
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" || request.SessionID == "" {
		t.Fatalf("request.json = %#v, want model and session_id", request)
	}
	return loaded, request
}

func assertOpenCodeFailureDoesNotLeakSensitiveOutput(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	forbidden := []string{
		"sk-opencode-secret",
		"private prompt",
		"private provider body",
		"responseBody",
	}
	encoded, err := json.Marshal(struct {
		Events         []factoryapi.FactoryEvent
		ResponseEvents []factoryapi.FactoryResponseEvent
	}{events, responseEvents})
	if err != nil {
		t.Fatalf("marshal observed events: %v", err)
	}
	payload := string(encoded)
	for _, needle := range forbidden {
		if strings.Contains(payload, needle) {
			t.Fatalf("public observation leaked sensitive OpenCode output containing %q", needle)
		}
	}
}

func assertOpenCodeFinalOnlyPublicResponseEvents(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()

	var completedMessages int
	for _, event := range events {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase == factoryapi.FactoryResponseEventPhaseDelta {
				t.Fatalf("final-only replay fabricated message delta: %#v", event)
			}
			if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
				completedMessages++
			}
		case factoryapi.FactoryResponseEventKindTool:
			t.Fatalf("final-only replay fabricated tool lifecycle: %#v", event)
		case factoryapi.FactoryResponseEventKindUsage:
			t.Fatalf("final-only replay fabricated usage lifecycle: %#v", event)
		}
	}
	if completedMessages == 0 {
		t.Fatal("final-only replay missing authoritative completed message")
	}
}

type fixedExecutableLocator struct {
	path string
}

func (l fixedExecutableLocator) LookPath(file string) (string, error) {
	if file == "opencode" {
		return l.path, nil
	}
	return "", fmt.Errorf("executable %q not found", file)
}

func identityResolveSymlinks(path string) (string, error) {
	return path, nil
}

func writeOpenCodeFixtureExecutable(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(path, []byte("opencode-fixture-executable\n"), 0o755); err != nil {
		t.Fatalf("write opencode fixture executable: %v", err)
	}
	return path
}

func newOpenCodeGoldenReplayRunner(
	loaded support.ProviderSessionCase,
	exitCode int,
) *openCodeGoldenReplayRunner {
	return &openCodeGoldenReplayRunner{
		transcript: platformprocess.CommandResult{
			Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
			Stderr:   []byte(loaded.Stderr),
			ExitCode: exitCode,
		},
	}
}

type openCodeGoldenReplayRunner struct {
	mu         sync.Mutex
	calls      int
	transcript platformprocess.CommandResult
}

func (r *openCodeGoldenReplayRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++
	switch r.calls {
	case 1:
		return platformprocess.CommandResult{Stdout: []byte("1.2.3\n")}, nil
	case 2:
		return platformprocess.CommandResult{ExitCode: 0}, nil
	default:
		return r.transcript, nil
	}
}

func (r *openCodeGoldenReplayRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func openCodeGoldenInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) (factoryapi.InferenceResponseEventPayload, string) {
	t.Helper()

	var (
		inferencePayload factoryapi.InferenceResponseEventPayload
		foundInference   bool
		dispatchOutput   string
	)
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeInferenceResponse:
			payload, err := event.Payload.AsInferenceResponseEventPayload()
			if err != nil {
				t.Fatalf("decode inference response: %v", err)
			}
			if payload.Outcome != factoryapi.InferenceOutcomeSucceeded {
				continue
			}
			inferencePayload = payload
			foundInference = true
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode dispatch response: %v", err)
			}
			if payload.Output != nil && *payload.Output != "" {
				dispatchOutput = *payload.Output
			}
		}
	}
	if !foundInference {
		t.Fatal("missing succeeded INFERENCE_RESPONSE in factory events")
	}
	if dispatchOutput == "" {
		t.Fatal("missing dispatch output in factory events")
	}
	return inferencePayload, dispatchOutput
}

func openCodeGoldenFailedInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	var (
		inferencePayload factoryapi.InferenceResponseEventPayload
		foundInference   bool
	)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeFailed {
			continue
		}
		inferencePayload = payload
		foundInference = true
	}
	if !foundInference {
		t.Fatal("missing failed INFERENCE_RESPONSE in factory events")
	}
	return inferencePayload
}

func observeOpenCodeProviderSessionGolden(
	payload factoryapi.InferenceResponseEventPayload,
	manifest support.ProviderSessionGoldenManifest,
) json.RawMessage {
	status := "failed"
	if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
		status = "completed"
	}
	provider := string(modelprovider.ProviderOpenCode)
	sessionID := ""
	if payload.ProviderSession != nil {
		if payload.ProviderSession.Provider != nil {
			provider = support.StringPointerValue(payload.ProviderSession.Provider)
		}
		if payload.ProviderSession.Id != nil {
			sessionID = support.StringPointerValue(payload.ProviderSession.Id)
		}
	}
	record := map[string]string{
		"provider":          provider,
		"providerSessionId": sessionID,
		"fidelityClass":     manifest.FidelityClass,
		"status":            status,
	}
	return mustMarshalJSON(record)
}

func observeOpenCodeResponseEventGoldens(events []factoryapi.FactoryResponseEvent) []json.RawMessage {
	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindError:
			if event.Phase != factoryapi.FactoryResponseEventPhaseFailed {
				continue
			}
			errorPayload, err := event.Payload.AsFactoryResponseEventErrorPayload()
			if err != nil {
				continue
			}
			record := map[string]any{
				"type":             "error.failed",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           openCodeGoldenItemID(event),
				"code":             errorPayload.Code,
				"message":          errorPayload.Message,
			}
			if errorPayload.Retryable != nil {
				record["retryable"] = *errorPayload.Retryable
			}
			records = append(records, mustMarshalJSON(record))
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
				continue
			}
			message, err := event.Payload.AsFactoryResponseEventMessagePayload()
			if err != nil {
				continue
			}
			text := openCodeGoldenMessageText(message)
			if text == "" {
				continue
			}
			record := map[string]any{
				"type":             "message.completed",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           openCodeGoldenItemID(event),
				"text":             text,
				"finishReason":     "stop",
			}
			records = append(records, mustMarshalJSON(record))
		case factoryapi.FactoryResponseEventKindTool:
			if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
				continue
			}
			tool, err := event.Payload.AsFactoryResponseEventToolPayload()
			if err != nil {
				continue
			}
			record := map[string]any{
				"type":             "tool.completed",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           openCodeGoldenItemID(event),
				"toolName":         tool.ToolName,
			}
			records = append(records, mustMarshalJSON(record))
		case factoryapi.FactoryResponseEventKindUsage:
			if event.Phase != factoryapi.FactoryResponseEventPhaseUpdated {
				continue
			}
			usage, err := event.Payload.AsFactoryResponseEventUsagePayload()
			if err != nil {
				continue
			}
			record := map[string]any{
				"type":             "usage.updated",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"inputTokens":      usage.InputTokens,
				"outputTokens":     usage.OutputTokens,
			}
			records = append(records, mustMarshalJSON(record))
		}
	}
	return records
}

func observeOpenCodeInvocationResultGolden(
	payload factoryapi.InferenceResponseEventPayload,
	dispatchOutput string,
) json.RawMessage {
	ok := payload.Outcome == factoryapi.InferenceOutcomeSucceeded
	content := dispatchOutput
	if payload.Response != nil && *payload.Response != "" {
		content = *payload.Response
	}
	record := map[string]any{
		"ok":           ok,
		"content":      content,
		"finishReason": "stop",
	}
	return mustMarshalJSON(record)
}

func observeOpenCodeFailedInvocationResultGolden(
	payload factoryapi.InferenceResponseEventPayload,
) json.RawMessage {
	record := map[string]any{
		"ok": false,
	}
	if payload.FailureDetail != nil {
		record["failureReason"] = payload.FailureDetail.Reason
		record["message"] = payload.FailureDetail.Message
	}
	return mustMarshalJSON(record)
}

func openCodeGoldenItemID(event factoryapi.FactoryResponseEvent) string {
	if event.ItemId != nil && *event.ItemId != "" {
		return *event.ItemId
	}
	return ""
}

func openCodeGoldenMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
	for _, block := range message.ContentBlocks {
		text, err := block.AsFactoryResponseEventTextContentBlock()
		if err != nil {
			continue
		}
		if text.Text != "" {
			return text.Text
		}
	}
	return ""
}

func mustMarshalJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
