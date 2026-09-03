package agy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	agyFinalOnlySuccessGoldenCase = "final-only-success"
	agyTimeoutGoldenCase          = "timeout"
	agyTimeoutFailureMessage      = "Agy request timed out."
)

// TestAgyGoldenFinalOnlySuccess replays a sanitized Agy final-only-success
// transcript through the customer process boundary and proves public final-only
// success without fabricated streaming deltas or structured snapshot events.
// golden: tests/functional/internal/support/testdata/provider-sessions/agy/final-only-success/manifest.json
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestAgyGoldenFinalOnlySuccess(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcessForTest(t)
	scenario := fixture.scenario(t, agyFinalOnlySuccessGoldenCase)
	replay := fixture.runScenario(t, scenario, "agy golden final-only success")
	loaded := scenario.loaded
	listed, events, responseEvents := replay.Listed, replay.FactoryEvents, replay.ResponseEvents

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if got := replay.RouteCalls; got != 1 {
		t.Fatalf("agy %q route calls = %d, want one invocation", scenario.selector, got)
	}

	inferencePayload, dispatchOutput := agyGoldenInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("inference outcome = %q, want SUCCEEDED", inferencePayload.Outcome)
	}
	if inferencePayload.ProviderSession != nil &&
		inferencePayload.ProviderSession.Id != nil &&
		strings.TrimSpace(support.StringPointerValue(inferencePayload.ProviderSession.Id)) != "" {
		t.Fatalf(
			"success-path inference unexpectedly retained provider session id %q",
			support.StringPointerValue(inferencePayload.ProviderSession.Id),
		)
	}
	if inferencePayload.Response == nil || *inferencePayload.Response != dispatchOutput {
		t.Fatalf("inference response text = %#v, want dispatch output %q", inferencePayload.Response, dispatchOutput)
	}
	if dispatchOutput == "" || !strings.Contains(dispatchOutput, "COMPLETE") {
		t.Fatalf("dispatch output = %q, want terminal COMPLETE-bearing success text", dispatchOutput)
	}

	assertAgyFinalOnlyPublicResponseEvents(t, responseEvents)

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeAgyProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeAgyResponseEventGoldens(responseEvents),
		InvocationResult: observeAgyInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
	fixture.assertProcessTopology(t)
}

// TestAgyGoldenTimeout replays a sanitized Agy timeout transcript through the
// customer process boundary and proves a public timeout outcome distinct from
// silent success, using the canonical command-runner native deadline error
// rather than a PTY-specific exit-code sentinel.
// golden: tests/functional/internal/support/testdata/provider-sessions/agy/timeout/manifest.json
func TestAgyGoldenTimeout(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcessForTest(t)
	scenario := fixture.scenario(t, agyTimeoutGoldenCase)
	replay := fixture.runScenario(t, scenario, "agy golden timeout")
	loaded := scenario.loaded
	listed, events, responseEvents := replay.Listed, replay.FactoryEvents, replay.ResponseEvents

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := replay.RouteCalls; got != 9 {
		t.Fatalf("agy %q route calls = %d, want nine retry invocations", scenario.selector, got)
	}

	inferencePayload := agyGoldenFailedInferenceObservation(t, events)
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inferencePayload.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want TIMEOUT", inferencePayload.FailureDetail.Reason)
	}
	if inferencePayload.ProviderSession == nil || inferencePayload.ProviderSession.Provider == nil {
		t.Fatal("inference response missing provider session metadata")
	}
	if got := support.StringPointerValue(inferencePayload.ProviderSession.Provider); got != string(modelprovider.ProviderAntigravity) {
		t.Fatalf("provider session provider = %q, want %q", got, modelprovider.ProviderAntigravity)
	}
	assertAgyFailureDoesNotLeakSensitiveOutput(t, events, responseEvents)
	assertAgyGoldenTimeoutResponseStream(t, responseEvents)

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeAgyProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeAgyResponseEventGoldens(responseEvents),
		InvocationResult: observeAgyInvocationResultGolden(inferencePayload, ""),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
	fixture.assertProcessTopology(t)
}

// agyDeadlineExceededCommandRunner is a test double proving the print-mode
// command adapter's native context.DeadlineExceeded classification, including
// partial-output publication, matches the recorded timeout golden contract.
type agyDeadlineExceededCommandRunner struct {
	stdout []byte
	starts atomic.Int64
}

func newAgyDeadlineExceededCommandRunner(stdout []byte) *agyDeadlineExceededCommandRunner {
	return &agyDeadlineExceededCommandRunner{stdout: stdout}
}

func (r *agyDeadlineExceededCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.starts.Add(1)
	return platformprocess.CommandResult{Stdout: append([]byte(nil), r.stdout...)}, context.DeadlineExceeded
}

// RunStreaming forwards the immutable partial fixture output directly to the
// provider observer. This avoids the completed-output fallback copy on every
// one of the timeout golden's nine controlled retry calls.
func (r *agyDeadlineExceededCommandRunner) RunStreaming(
	_ context.Context,
	_ platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	r.starts.Add(1)
	stdout := r.stdout
	if observer != nil && len(stdout) > 0 {
		observer(platformprocess.OutputStreamStdout, stdout)
	}
	return platformprocess.CommandResult{Stdout: stdout}, context.DeadlineExceeded
}

func (r *agyDeadlineExceededCommandRunner) callCount() int {
	return int(r.starts.Load())
}

type agyGoldenRequest struct {
	Model     string `json:"model"`
	SessionID string `json:"session_id"`
}

func readAgyGoldenCase(
	repoRoot string,
	caseName string,
	manifestID string,
	fidelityClass string,
) (support.ProviderSessionCase, agyGoldenRequest, error) {
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("agy", caseName)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		return support.ProviderSessionCase{}, agyGoldenRequest{}, fmt.Errorf("LoadProviderSessionCase: %w", err)
	}
	if loaded.Manifest.ID != manifestID {
		return support.ProviderSessionCase{}, agyGoldenRequest{}, fmt.Errorf("manifest.ID = %q, want %s", loaded.Manifest.ID, manifestID)
	}
	if loaded.Manifest.FidelityClass != fidelityClass {
		return support.ProviderSessionCase{}, agyGoldenRequest{}, fmt.Errorf("manifest.fidelityClass = %q, want %q", loaded.Manifest.FidelityClass, fidelityClass)
	}
	var request agyGoldenRequest
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		return support.ProviderSessionCase{}, agyGoldenRequest{}, fmt.Errorf("decode request.json: %w", err)
	}
	if request.Model == "" || request.SessionID == "" {
		return support.ProviderSessionCase{}, agyGoldenRequest{}, fmt.Errorf("request.json = %#v, want model and session_id", request)
	}
	return loaded, request, nil
}

func assertAgyFailureDoesNotLeakSensitiveOutput(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	forbidden := []string{
		"invalid api key",
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
			t.Fatalf("public observation leaked sensitive Agy output containing %q", needle)
		}
	}
}

func assertAgyGoldenTimeoutResponseStream(
	t *testing.T,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	if len(responseEvents) == 0 {
		t.Fatal("response stream missing events; want closed terminal stream")
	}
	last := responseEvents[len(responseEvents)-1]
	if last.Phase != factoryapi.FactoryResponseEventPhaseFailed {
		t.Fatalf("terminal response event phase = %q, want FAILED", last.Phase)
	}
	if last.Kind == factoryapi.FactoryResponseEventKindError {
		payload, err := last.Payload.AsFactoryResponseEventErrorPayload()
		if err != nil {
			t.Fatalf("decode terminal ERROR response event: %v", err)
		}
		if payload.Code != "" && payload.Code != "timeout" {
			t.Fatalf("terminal response error code = %q, want timeout", payload.Code)
		}
		if payload.Message != agyTimeoutFailureMessage {
			t.Fatalf(
				"terminal response error message = %q, want %q",
				payload.Message,
				agyTimeoutFailureMessage,
			)
		}
	}
	for _, event := range responseEvents {
		if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
			continue
		}
		if event.Kind == factoryapi.FactoryResponseEventKindRun {
			payload, err := event.Payload.AsFactoryResponseEventRunPayload()
			if err != nil {
				t.Fatalf("decode RUN response event: %v", err)
			}
			if payload.Status != nil && *payload.Status == "completed" {
				t.Fatalf("response stream invented successful run completion: %#v", event)
			}
		}
	}
}

func assertAgyFinalOnlyPublicResponseEvents(t *testing.T, events []factoryapi.FactoryResponseEvent) {
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

func agyGoldenInferenceObservation(
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
		case factoryapi.FactoryEventTypeModelResponse:
			payload, err := support.AsInferenceResponseObservation(event)
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

func agyGoldenFailedInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	var (
		inferencePayload factoryapi.InferenceResponseEventPayload
		foundInference   bool
	)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
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

func observeAgyProviderSessionGolden(
	payload factoryapi.InferenceResponseEventPayload,
	manifest support.ProviderSessionGoldenManifest,
) json.RawMessage {
	status := "failed"
	if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
		status = "completed"
	}
	provider := string(modelprovider.ProviderAntigravity)
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

func observeAgyResponseEventGoldens(events []factoryapi.FactoryResponseEvent) []json.RawMessage {
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
				"itemId":           agyGoldenItemID(event),
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
			text := agyGoldenMessageText(message)
			if text == "" {
				continue
			}
			record := map[string]any{
				"type":             "message.completed",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           agyGoldenItemID(event),
				"text":             text,
				"finishReason":     "stop",
			}
			records = append(records, mustMarshalJSON(record))
		}
	}
	return records
}

func observeAgyInvocationResultGolden(
	payload factoryapi.InferenceResponseEventPayload,
	dispatchOutput string,
) json.RawMessage {
	ok := payload.Outcome == factoryapi.InferenceOutcomeSucceeded
	if !ok {
		record := map[string]any{
			"ok":            false,
			"failureReason": "unknown",
			"message":       "provider invocation failed",
		}
		if payload.FailureDetail != nil {
			record["failureReason"] = string(payload.FailureDetail.Reason)
			if payload.FailureDetail.Reason == factoryapi.WorkFailureTypeTimeout {
				record["message"] = agyTimeoutFailureMessage
			}
		}
		return mustMarshalJSON(record)
	}
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

func agyGoldenItemID(event factoryapi.FactoryResponseEvent) string {
	if event.ItemId != nil && *event.ItemId != "" {
		return *event.ItemId
	}
	return ""
}

func agyGoldenMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
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
