package kiro

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
	kiroGoldenTextSuccessCase       = "text-success"
	kiroGoldenAuthFailureCase       = "auth-failure"
	kiroGoldenStructuredFailureCase = "structured-failure"
	kiroGoldenTimeoutCase           = "timeout"
)

// TestKiroGoldenTextSuccess replays a sanitized Kiro text-success transcript through
// the customer process boundary and proves successful text output with matching
// public Provider Session, response-event, and invocation-result metadata.
// golden: docs/temp/functional/provider-sessions/kiro/text-success/manifest.json
func TestKiroGoldenTextSuccess(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("kiro", kiroGoldenTextSuccessCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "kiro-text-success" {
		t.Fatalf("manifest.ID = %q, want kiro-text-success", loaded.Manifest.ID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityFinalOnly {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityFinalOnly,
		)
	}

	var request struct {
		Model     string `json:"model"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" || request.SessionID == "" {
		t.Fatalf("request.json = %#v, want model and session_id", request)
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderKiro, request.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"kiro golden text success"}`))

	exitCode := 0
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
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	inferencePayload, dispatchOutput := kiroGoldenInferenceObservation(t, events)
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
		ProviderSession:  observeKiroProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeKiroResponseEventGoldens(responseEvents),
		InvocationResult: observeKiroInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestKiroGoldenAuthAndStructuredFailure replays sanitized Kiro auth-failure
// and structured-failure transcripts through the customer process boundary and
// proves those public failure classes remain distinct without leaking private detail.
// golden: docs/temp/functional/provider-sessions/kiro/auth-failure/manifest.json
// golden: docs/temp/functional/provider-sessions/kiro/structured-failure/manifest.json
func TestKiroGoldenAuthAndStructuredFailure(t *testing.T) {
	t.Run("auth-failure", func(t *testing.T) {
		runKiroFailureGoldenCase(
			t,
			kiroGoldenAuthFailureCase,
			"kiro-auth-failure",
			support.ProviderSessionFidelityFinalOnly,
			factoryapi.WorkFailureTypeAuthFailure,
			factoryapi.WorkFailureTypeInternalServerError,
			[]string{
				`C:\private\kiro-token.txt`,
				"kiro-token",
			},
		)
	})
	t.Run("structured-failure", func(t *testing.T) {
		runKiroFailureGoldenCase(
			t,
			kiroGoldenStructuredFailureCase,
			"kiro-structured-failure",
			support.ProviderSessionFidelityFinalOnly,
			factoryapi.WorkFailureTypePermanentBadRequest,
			factoryapi.WorkFailureTypeAuthFailure,
			[]string{
				"private customer request detail",
			},
		)
	})
}

// TestKiroGoldenTimeout replays a sanitized Kiro timeout transcript through the
// customer process boundary and proves a public timeout outcome distinct from
// successful text completion, with matching Provider Session, response-event,
// and invocation-result metadata.
// golden: docs/temp/functional/provider-sessions/kiro/timeout/manifest.json
func TestKiroGoldenTimeout(t *testing.T) {
	loaded, request := loadKiroGoldenCase(
		t,
		kiroGoldenTimeoutCase,
		"kiro-timeout",
		support.ProviderSessionFidelityFinalOnly,
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderKiro, request.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"kiro golden timeout"}`))

	exitCode := 124
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	timeoutResult := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	}
	runner := testutil.NewProviderCommandRunner(
		timeoutResult,
		timeoutResult,
		timeoutResult,
		timeoutResult,
	)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() < 1 {
		t.Fatalf("provider command runner calls = %d, want at least 1", runner.CallCount())
	}

	inferencePayload := kiroGoldenFailedInferenceObservationWithReason(t, events, factoryapi.WorkFailureTypeTimeout)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inferencePayload.Outcome)
	}
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inferencePayload.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want TIMEOUT (runner calls=%d)", inferencePayload.FailureDetail.Reason, runner.CallCount())
	}
	if inferencePayload.FailureDetail.Message != "provider invocation timed out" {
		t.Fatalf("failure message = %q, want provider invocation timed out", inferencePayload.FailureDetail.Message)
	}
	if inferencePayload.Response != nil && strings.Contains(*inferencePayload.Response, "COMPLETE") {
		t.Fatalf("timeout inference response = %q, must not treat COMPLETE-bearing stdout as success", *inferencePayload.Response)
	}
	assertKiroGoldenDispatchDoesNotTreatTimeoutStdoutAsSuccess(t, events)

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeKiroProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeKiroResponseEventGoldens(responseEvents),
		InvocationResult: observeKiroFailedInvocationResultGolden(inferencePayload),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

func assertKiroGoldenDispatchDoesNotTreatTimeoutStdoutAsSuccess(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output != nil && strings.Contains(*payload.Output, "COMPLETE") {
			t.Fatalf("dispatch output = %q, must not treat COMPLETE-bearing stdout as success on timeout", *payload.Output)
		}
	}
}

func runKiroFailureGoldenCase(
	t *testing.T,
	caseName string,
	manifestID string,
	fidelityClass string,
	wantReason factoryapi.WorkFailureType,
	notReason factoryapi.WorkFailureType,
	forbiddenNeedles []string,
) {
	t.Helper()

	loaded, request := loadKiroGoldenCase(t, caseName, manifestID, fidelityClass)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderKiro, request.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"kiro golden `+caseName+`"}`))

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
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	inferencePayload := kiroGoldenFailedInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inferencePayload.Outcome)
	}
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if got := inferencePayload.FailureDetail.Reason; got != wantReason {
		t.Fatalf("failure reason = %q, want %q", got, wantReason)
	}
	if notReason != "" && inferencePayload.FailureDetail.Reason == notReason {
		t.Fatalf("failure reason = %q, must remain distinct from %q", inferencePayload.FailureDetail.Reason, notReason)
	}
	if request.SessionID != "" {
		if inferencePayload.ProviderSession == nil || inferencePayload.ProviderSession.Id == nil {
			t.Fatal("inference response missing provider session identity")
		}
		if got := support.StringPointerValue(inferencePayload.ProviderSession.Id); got != request.SessionID {
			t.Fatalf("provider session id = %q, want golden session %q", got, request.SessionID)
		}
	}
	assertKiroFailureDoesNotLeakSensitiveOutput(t, events, responseEvents, forbiddenNeedles)

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeKiroProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeKiroResponseEventGoldens(responseEvents),
		InvocationResult: observeKiroFailedInvocationResultGolden(inferencePayload),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

type kiroGoldenRequest struct {
	Model     string `json:"model"`
	SessionID string `json:"session_id"`
}

func loadKiroGoldenCase(
	t *testing.T,
	caseName string,
	manifestID string,
	fidelityClass string,
) (support.ProviderSessionCase, kiroGoldenRequest) {
	t.Helper()
	caseDir := filepath.Join(
		testutil.MustRepoRoot(t),
		filepath.FromSlash(support.ProviderSessionFixturePath("kiro", caseName)),
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
	var request kiroGoldenRequest
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" {
		t.Fatalf("request.json = %#v, want model", request)
	}
	return loaded, request
}

func assertKiroFailureDoesNotLeakSensitiveOutput(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
	forbidden []string,
) {
	t.Helper()

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
			t.Fatalf("public observation leaked sensitive Kiro output containing %q", needle)
		}
	}
}

func kiroGoldenFailedInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	return kiroGoldenFailedInferenceObservationWithReason(t, events, "")
}

func kiroGoldenFailedInferenceObservationWithReason(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantReason factoryapi.WorkFailureType,
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
		if wantReason != "" && payload.FailureDetail != nil && payload.FailureDetail.Reason != wantReason {
			continue
		}
		inferencePayload = payload
		foundInference = true
	}
	if !foundInference {
		if wantReason != "" {
			t.Fatalf("missing failed INFERENCE_RESPONSE with reason %q in factory events", wantReason)
		}
		t.Fatal("missing failed INFERENCE_RESPONSE in factory events")
	}
	return inferencePayload
}

func kiroGoldenInferenceObservation(
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

func observeKiroProviderSessionGolden(
	payload factoryapi.InferenceResponseEventPayload,
	manifest support.ProviderSessionGoldenManifest,
) json.RawMessage {
	status := "failed"
	if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
		status = "completed"
	}
	provider := "kiro"
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

func observeKiroFailedInvocationResultGolden(
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

func observeKiroResponseEventGoldens(events []factoryapi.FactoryResponseEvent) []json.RawMessage {
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
				"itemId":           kiroGoldenItemID(event),
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
			text := kiroGoldenMessageText(message)
			if text == "" {
				continue
			}
			record := map[string]any{
				"type":             "message.completed",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           kiroGoldenItemID(event),
				"text":             text,
				"finishReason":     "stop",
			}
			records = append(records, mustMarshalJSON(record))
		}
	}
	return records
}

func observeKiroInvocationResultGolden(
	payload factoryapi.InferenceResponseEventPayload,
	dispatchOutput string,
) json.RawMessage {
	ok := payload.Outcome == factoryapi.InferenceOutcomeSucceeded
	content := dispatchOutput
	if payload.Response != nil && *payload.Response != "" {
		content = *payload.Response
	}
	finishReason := "stop"
	if !ok {
		finishReason = "error"
	}
	record := map[string]any{
		"ok":           ok,
		"content":      content,
		"finishReason": finishReason,
	}
	return mustMarshalJSON(record)
}

func kiroGoldenItemID(event factoryapi.FactoryResponseEvent) string {
	if event.ItemId != nil && *event.ItemId != "" {
		return *event.ItemId
	}
	return ""
}

func kiroGoldenMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
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
