package pi

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
	piGoldenTextSuccessCase       = "text-success"
	piGoldenStructuredFailureCase = "structured-failure"
	piGoldenTimeoutCase           = "timeout"
)

// TestPiGoldenTextSuccess replays a sanitized Pi text-success transcript through
// the customer process boundary and proves successful text output with matching
// public Provider Session, response-event, and invocation-result metadata.
//golden: docs/temp/functional/provider-sessions/pi/text-success/manifest.json
func TestPiGoldenTextSuccess(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("pi", piGoldenTextSuccessCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "pi-text-success" {
		t.Fatalf("manifest.ID = %q, want pi-text-success", loaded.Manifest.ID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityPartialStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityPartialStream,
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
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderPi, request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"pi golden text success"}`))

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

	inferencePayload, dispatchOutput := piGoldenInferenceObservation(t, events)
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
		ProviderSession:   observePiProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observePiResponseEventGoldens(responseEvents),
		InvocationResult: observePiInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestPiGoldenStructuredFailure replays a sanitized Pi structured-failure transcript
// through the customer process boundary and proves a public structured failure outcome
// with matching Provider Session, response-event, and invocation-result metadata.
//golden: docs/temp/functional/provider-sessions/pi/structured-failure/manifest.json
func TestPiGoldenStructuredFailure(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("pi", piGoldenStructuredFailureCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "pi-structured-failure" {
		t.Fatalf("manifest.ID = %q, want pi-structured-failure", loaded.Manifest.ID)
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
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderPi, request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"pi golden structured failure"}`))

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
		t.Fatalf("completed work = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	inferencePayload := piGoldenFailedInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inferencePayload.Outcome)
	}
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inferencePayload.FailureDetail.Reason == factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want structured failure not timeout", inferencePayload.FailureDetail.Reason)
	}
	if inferencePayload.FailureDetail.Message != "rate limited" {
		t.Fatalf("failure message = %q, want rate limited", inferencePayload.FailureDetail.Message)
	}

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:   observePiProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observePiResponseEventGoldens(responseEvents),
		InvocationResult: observePiInvocationResultGolden(inferencePayload, ""),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestPiGoldenTimeout replays a sanitized Pi timeout transcript through the customer
// process boundary and proves a public timeout outcome distinct from structured failure,
// with matching Provider Session, response-event, and invocation-result metadata.
//golden: docs/temp/functional/provider-sessions/pi/timeout/manifest.json
func TestPiGoldenTimeout(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("pi", piGoldenTimeoutCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "pi-timeout" {
		t.Fatalf("manifest.ID = %q, want pi-timeout", loaded.Manifest.ID)
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
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderPi, request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"pi golden timeout"}`))

	exitCode := 124
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	timeoutResult := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	}
	runner := testutil.NewProviderCommandRunner(timeoutResult, timeoutResult, timeoutResult)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() < 1 {
		t.Fatalf("provider command runner calls = %d, want at least 1", runner.CallCount())
	}

	inferencePayload := piGoldenFailedInferenceObservationWithReason(t, events, factoryapi.WorkFailureTypeTimeout)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inferencePayload.Outcome)
	}
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inferencePayload.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want TIMEOUT (runner calls=%d)", inferencePayload.FailureDetail.Reason, runner.CallCount())
	}
	if inferencePayload.FailureDetail.Message != "Pi execution timed out." {
		t.Fatalf("failure message = %q, want Pi execution timed out.", inferencePayload.FailureDetail.Message)
	}

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:   observePiProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observePiResponseEventGoldens(responseEvents),
		InvocationResult: observePiInvocationResultGolden(inferencePayload, ""),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

func piGoldenInferenceObservation(
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

func piGoldenFailedInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()
	return piGoldenFailedInferenceObservationWithReason(t, events, "")
}

func piGoldenFailedInferenceObservationWithReason(
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
			t.Fatalf("missing failed INFERENCE_RESPONSE with reason %q", wantReason)
		}
		t.Fatal("missing failed INFERENCE_RESPONSE in factory events")
	}
	return inferencePayload
}

func observePiProviderSessionGolden(
	payload factoryapi.InferenceResponseEventPayload,
	manifest support.ProviderSessionGoldenManifest,
) json.RawMessage {
	status := "failed"
	if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
		status = "completed"
	}
	provider := string(modelprovider.ProviderPi)
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

func observePiResponseEventGoldens(events []factoryapi.FactoryResponseEvent) []json.RawMessage {
	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindSession:
			if event.Phase != factoryapi.FactoryResponseEventPhaseStarted {
				continue
			}
			session, err := event.Payload.AsFactoryResponseEventSessionPayload()
			if err != nil {
				continue
			}
			record := map[string]any{
				"type":             "session.started",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           piGoldenItemID(event),
			}
			if session.Status != nil {
				record["status"] = *session.Status
			}
			records = append(records, mustMarshalJSON(record))
		case factoryapi.FactoryResponseEventKindMessage:
			switch event.Phase {
			case factoryapi.FactoryResponseEventPhaseDelta:
				delta, err := event.Payload.AsFactoryResponseEventMessageDeltaPayload()
				if err != nil || delta.TextDelta == nil {
					continue
				}
				record := map[string]any{
					"type":             "message.delta",
					"eventId":          event.EventId,
					"factorySessionId": event.FactorySessionId,
					"runId":            event.RunId,
					"itemId":           piGoldenItemID(event),
					"text":             *delta.TextDelta,
				}
				records = append(records, mustMarshalJSON(record))
			case factoryapi.FactoryResponseEventPhaseCompleted:
				message, err := event.Payload.AsFactoryResponseEventMessagePayload()
				if err != nil {
					continue
				}
				text := piGoldenMessageText(message)
				if text == "" {
					continue
				}
				record := map[string]any{
					"type":             "message.completed",
					"eventId":          event.EventId,
					"factorySessionId": event.FactorySessionId,
					"runId":            event.RunId,
					"itemId":           piGoldenItemID(event),
					"text":             text,
					"finishReason":     "stop",
				}
				records = append(records, mustMarshalJSON(record))
			}
		case factoryapi.FactoryResponseEventKindTool:
			tool, err := event.Payload.AsFactoryResponseEventToolPayload()
			if err != nil {
				continue
			}
			recordType := ""
			switch event.Phase {
			case factoryapi.FactoryResponseEventPhaseStarted:
				recordType = "tool.started"
			case factoryapi.FactoryResponseEventPhaseCompleted:
				recordType = "tool.completed"
			default:
				continue
			}
			record := map[string]any{
				"type":             recordType,
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           piGoldenItemID(event),
				"toolName":         tool.ToolName,
			}
			records = append(records, mustMarshalJSON(record))
		}
	}
	return records
}

func observePiInvocationResultGolden(
	payload factoryapi.InferenceResponseEventPayload,
	dispatchOutput string,
) json.RawMessage {
	ok := payload.Outcome == factoryapi.InferenceOutcomeSucceeded
	content := dispatchOutput
	if payload.Response != nil && *payload.Response != "" {
		content = *payload.Response
	}
	if !ok && content == "" && payload.FailureDetail != nil {
		content = payload.FailureDetail.Message
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

func piGoldenItemID(event factoryapi.FactoryResponseEvent) string {
	if event.ItemId != nil && *event.ItemId != "" {
		return *event.ItemId
	}
	return ""
}

func piGoldenMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
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
