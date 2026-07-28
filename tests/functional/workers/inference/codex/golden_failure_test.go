package codex

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
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
	codexGoldenStructuredFailureCase = "structured-failure"
	codexGoldenTimeoutCase           = "timeout"
)

// TestCodexGoldenStructuredFailure replays a sanitized Codex structured-failure
// transcript through the customer process boundary and proves public surfaces
// expose normalized structured non-zero failure metadata rather than success or
// timeout-classified outcomes.
//golden: docs/temp/functional/provider-sessions/codex/structured-failure/manifest.json
func TestCodexGoldenStructuredFailure(t *testing.T) {
	replay := runCodexGoldenStructuredFailureReplay(t)

	if got := support.CountWorkAtCustomerState(replay.Listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(replay.Listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, replay.Listed)
	}
	if replay.Runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", replay.Runner.CallCount())
	}

	inference := failedInferenceResponse(t, replay.FactoryEvents)
	if inference.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inference.Outcome)
	}
	if inference.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inference.FailureDetail.Reason == factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want structured failure not timeout", inference.FailureDetail.Reason)
	}
	if inference.FailureDetail.Reason != factoryapi.WorkFailureTypeAuthFailure {
		t.Fatalf("failure reason = %q, want auth_failure", inference.FailureDetail.Reason)
	}
	if inference.FailureDetail.Message != "Codex authentication failed." {
		t.Fatalf("failure message = %q, want Codex authentication failed.", inference.FailureDetail.Message)
	}

	observed := observeCodexStructuredFailureGoldens(t, replay, inference)
	if err := support.CompareOrUpdateProviderSessionGoldens(replay.Loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("golden files rewritten under %s: %v", support.ProviderSessionUpdateFunctionalGoldensEnv, err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestCodexGoldenTimeoutHasNoFalseTerminalMessage replays a sanitized Codex timeout
// transcript through the customer process boundary and proves public surfaces do not
// emit a false terminal success message or invent a successful invocation outcome.
//golden: docs/temp/functional/provider-sessions/codex/timeout/manifest.json
func TestCodexGoldenTimeoutHasNoFalseTerminalMessage(t *testing.T) {
	replay := runCodexGoldenTimeoutReplay(t)

	if got := support.CountWorkAtCustomerState(replay.Listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0; listed=%#v", got, replay.Listed)
	}
	if got := support.CountWorkAtCustomerState(replay.Listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, replay.Listed)
	}
	if replay.Runner.CallCount() < 1 {
		t.Fatalf("provider command runner calls = %d, want at least 1", replay.Runner.CallCount())
	}

	inference := failedInferenceResponseWithReason(t, replay.FactoryEvents, factoryapi.WorkFailureTypeTimeout)
	if inference.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inference.Outcome)
	}
	if inference.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inference.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want TIMEOUT", inference.FailureDetail.Reason)
	}
	if inference.FailureDetail.Message != "provider invocation timed out" {
		t.Fatalf("failure message = %q, want provider invocation timed out", inference.FailureDetail.Message)
	}
	assertCodexGoldenTimeoutHasNoFalseTerminalSuccess(t, replay, inference)

	observed := observeCodexTimeoutGoldens(t, replay, inference)
	if err := support.CompareOrUpdateProviderSessionGoldens(replay.Loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("golden files rewritten under %s: %v", support.ProviderSessionUpdateFunctionalGoldensEnv, err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

func runCodexGoldenStructuredFailureReplay(t *testing.T) codexGoldenReplayResult {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("codex", codexGoldenStructuredFailureCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "codex-structured-failure" {
		t.Fatalf("manifest.ID = %q, want codex-structured-failure", loaded.Manifest.ID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityPartialStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityPartialStream,
		)
	}

	var request struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" {
		t.Fatal("request.model must be non-empty")
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"codex golden structured failure"}`))

	exitCode := 1
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	return codexGoldenReplayResult{
		Loaded:         loaded,
		Listed:         listed,
		FactoryEvents:  events,
		ResponseEvents: responseEvents,
		Runner:         runner,
	}
}

func runCodexGoldenTimeoutReplay(t *testing.T) codexGoldenReplayResult {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("codex", codexGoldenTimeoutCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "codex-timeout" {
		t.Fatalf("manifest.ID = %q, want codex-timeout", loaded.Manifest.ID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityPartialStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityPartialStream,
		)
	}

	var request struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" {
		t.Fatal("request.model must be non-empty")
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"codex golden timeout"}`))

	exitCode := 1
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	timeoutResult := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	}
	timeoutResults := make([]platformprocess.CommandResult, 12)
	for i := range timeoutResults {
		timeoutResults[i] = timeoutResult
	}
	runner := testutil.NewProviderCommandRunner(timeoutResults...)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		30*time.Second,
	)

	return codexGoldenReplayResult{
		Loaded:         loaded,
		Listed:         listed,
		FactoryEvents:  events,
		ResponseEvents: responseEvents,
		Runner:         runner,
	}
}

func observeCodexStructuredFailureGoldens(
	t *testing.T,
	replay codexGoldenReplayResult,
	inference factoryapi.InferenceResponseEventPayload,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	providerSession := projectCodexFailureProviderSessionGolden(replay.Loaded, inference)
	responseEvents := projectCodexFailureResponseEventGoldens(t, replay.ResponseEvents)
	invocationResult := projectCodexFailureInvocationResultGolden(t, inference)

	sessionBytes, err := json.Marshal(providerSession)
	if err != nil {
		t.Fatalf("marshal observed provider session: %v", err)
	}
	resultBytes, err := json.Marshal(invocationResult)
	if err != nil {
		t.Fatalf("marshal observed invocation result: %v", err)
	}

	return support.ProviderSessionObservedGoldens{
		ProviderSession:  sessionBytes,
		ResponseEvents:   responseEvents,
		InvocationResult: resultBytes,
	}
}

func observeCodexTimeoutGoldens(
	t *testing.T,
	replay codexGoldenReplayResult,
	inference factoryapi.InferenceResponseEventPayload,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	providerSession := projectCodexFailureProviderSessionGolden(replay.Loaded, inference)
	responseEvents := projectCodexFailureResponseEventGoldens(t, replay.ResponseEvents)
	invocationResult := projectCodexFailureInvocationResultGolden(t, inference)

	sessionBytes, err := json.Marshal(providerSession)
	if err != nil {
		t.Fatalf("marshal observed provider session: %v", err)
	}
	resultBytes, err := json.Marshal(invocationResult)
	if err != nil {
		t.Fatalf("marshal observed invocation result: %v", err)
	}

	return support.ProviderSessionObservedGoldens{
		ProviderSession:  sessionBytes,
		ResponseEvents:   responseEvents,
		InvocationResult: resultBytes,
	}
}

func failedInferenceResponse(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	var terminal factoryapi.InferenceResponseEventPayload
	found := false
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response event: %v", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeFailed {
			continue
		}
		terminal = payload
		found = true
	}
	if !found {
		t.Fatal("missing failed inference response")
	}
	return terminal
}

func failedInferenceResponseWithReason(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantReason factoryapi.WorkFailureType,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	inference := failedInferenceResponse(t, events)
	if inference.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inference.FailureDetail.Reason != wantReason {
		t.Fatalf("failure reason = %q, want %q", inference.FailureDetail.Reason, wantReason)
	}
	return inference
}

func assertCodexGoldenTimeoutHasNoFalseTerminalSuccess(
	t *testing.T,
	replay codexGoldenReplayResult,
	inference factoryapi.InferenceResponseEventPayload,
) {
	t.Helper()

	var lastInference factoryapi.InferenceResponseEventPayload
	foundInference := false
	for _, event := range replay.FactoryEvents {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response event: %v", err)
		}
		lastInference = payload
		foundInference = true
	}
	if !foundInference {
		t.Fatal("missing public inference observations")
	}
	if lastInference.Outcome == factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf(
			"terminal inference outcome = %q, must not invent successful terminal inference on timeout",
			lastInference.Outcome,
		)
	}
	if inference.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("terminal failure inference outcome = %q, want FAILED", inference.Outcome)
	}

	for _, event := range replay.ResponseEvents {
		if event.Kind != factoryapi.FactoryResponseEventKindMessage {
			continue
		}
		if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
			continue
		}
		payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
		if err != nil {
			t.Fatalf("decode message response event: %v", err)
		}
		for _, block := range payload.ContentBlocks {
			text, err := block.AsFactoryResponseEventTextContentBlock()
			if err != nil {
				continue
			}
			if strings.Contains(text.Text, "COMPLETE") {
				t.Fatalf(
					"message.completed text = %q, must not treat COMPLETE-bearing partial stdout as terminal success on timeout",
					text.Text,
				)
			}
		}
	}

	if inference.Response != nil && strings.Contains(*inference.Response, "COMPLETE") {
		t.Fatalf(
			"timeout inference response = %q, must not treat COMPLETE-bearing stdout as success",
			*inference.Response,
		)
	}

	dispatches := support.ObserveDispatchEvents(t, replay.FactoryEvents)
	if len(dispatches) == 0 {
		t.Fatal("missing public dispatch observations")
	}
	response := dispatches[len(dispatches)-1].Response
	if response != nil && response.Outcome == factoryapi.WorkOutcomeAccepted {
		t.Fatalf("terminal dispatch outcome = %#v, must not invent ACCEPTED work outcome on timeout", response)
	}
}

func projectCodexFailureProviderSessionGolden(
	loaded support.ProviderSessionCase,
	inference factoryapi.InferenceResponseEventPayload,
) map[string]any {
	session := map[string]any{
		"provider":      loaded.Manifest.Provider,
		"fidelityClass": loaded.Manifest.FidelityClass,
		"status":        "failed",
	}
	if inference.Outcome == factoryapi.InferenceOutcomeSucceeded {
		session["status"] = "completed"
	}

	sessionID := ""
	if inference.ProviderSession != nil && inference.ProviderSession.Id != nil {
		sessionID = strings.TrimSpace(*inference.ProviderSession.Id)
	}
	if sessionID == "" && inference.Response != nil {
		sessionID = codexThreadIDFromTranscript(*inference.Response)
	}
	session["providerSessionId"] = sessionID
	return session
}

func projectCodexFailureResponseEventGoldens(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) []json.RawMessage {
	t.Helper()

	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		record := projectCodexFailureResponseEventGolden(event)
		if len(record) == 0 {
			continue
		}
		records = append(records, record)
	}
	return records
}

func projectCodexFailureResponseEventGolden(event factoryapi.FactoryResponseEvent) json.RawMessage {
	switch event.Kind {
	case factoryapi.FactoryResponseEventKindError:
		if event.Phase != factoryapi.FactoryResponseEventPhaseFailed {
			return nil
		}
		payload, err := event.Payload.AsFactoryResponseEventErrorPayload()
		if err != nil {
			return nil
		}
		record := map[string]any{
			"type":    "error.failed",
			"eventId": event.EventId,
			"code":    payload.Code,
			"message": payload.Message,
		}
		if event.FactorySessionId != "" {
			record["factorySessionId"] = event.FactorySessionId
		}
		if event.RunId != "" {
			record["runId"] = event.RunId
		}
		if event.ItemId != nil && strings.TrimSpace(*event.ItemId) != "" {
			record["itemId"] = strings.TrimSpace(*event.ItemId)
		}
		if payload.Retryable != nil {
			record["retryable"] = *payload.Retryable
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return nil
		}
		return encoded
	case factoryapi.FactoryResponseEventKindRun:
		payload, err := event.Payload.AsFactoryResponseEventRunPayload()
		if err == nil && payload.Status != nil && strings.TrimSpace(*payload.Status) != "" {
			record := map[string]any{
				"type":   strings.ToLower(string(event.Kind) + "." + strings.ToLower(string(event.Phase))),
				"status": strings.ToLower(strings.TrimSpace(*payload.Status)),
			}
			if nativeType := strings.TrimSpace(event.Provenance.NativeEventType); nativeType != "" {
				record["type"] = strings.ToLower(nativeType)
			}
			encoded, err := json.Marshal(record)
			if err != nil {
				return nil
			}
			return encoded
		}
	}
	return projectCodexResponseEventGolden(event)
}

func projectCodexFailureInvocationResultGolden(
	t *testing.T,
	inference factoryapi.InferenceResponseEventPayload,
) map[string]any {
	t.Helper()

	result := map[string]any{
		"ok": false,
	}
	if inference.FailureDetail != nil {
		result["failureReason"] = inference.FailureDetail.Reason
		result["message"] = inference.FailureDetail.Message
	}
	return result
}
