package cursor

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
	cursorGoldenMalformedRecordCase = "malformed-record"
	cursorGoldenProcessFailureCase  = "process-failure"
	cursorGoldenTimeoutCase         = "timeout"
)

const cursorMalformedRecordLeakProbe = "{not json}"

// TestCursorGoldenMalformedRecordReturnsStableDiagnostic replays a sanitized Cursor
// malformed-record golden through the customer process boundary and proves public
// surfaces expose a stable malformed-record diagnostic rather than silent success,
// timeout classification, or unsanitized private payload leakage.
//golden: docs/temp/functional/provider-sessions/cursor/malformed-record/manifest.json
func TestCursorGoldenMalformedRecordReturnsStableDiagnostic(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("cursor", cursorGoldenMalformedRecordCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "cursor-malformed-record" {
		t.Fatalf("manifest.ID = %q, want cursor-malformed-record", loaded.Manifest.ID)
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
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCursor, request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"cursor golden malformed record"}`))

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
		20*time.Second,
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

	inferencePayload := cursorGoldenFailedInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inferencePayload.Outcome)
	}
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inferencePayload.FailureDetail.Reason == factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want malformed-record diagnostic not timeout", inferencePayload.FailureDetail.Reason)
	}
	if inferencePayload.FailureDetail.Reason != factoryapi.WorkFailureTypePermanentBadRequest {
		t.Fatalf(
			"failure reason = %q, want %q",
			inferencePayload.FailureDetail.Reason,
			factoryapi.WorkFailureTypePermanentBadRequest,
		)
	}
	if strings.TrimSpace(inferencePayload.FailureDetail.Message) == "" {
		t.Fatal("failure message is empty, want stable malformed-record diagnostic")
	}

	encodedEvents, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events: %v", err)
	}
	encodedResponseEvents, err := json.Marshal(responseEvents)
	if err != nil {
		t.Fatalf("marshal response events: %v", err)
	}
	visible := string(encodedEvents) + string(encodedResponseEvents)
	if strings.Contains(visible, cursorMalformedRecordLeakProbe) {
		t.Fatalf("public surfaces leaked malformed record payload %q", cursorMalformedRecordLeakProbe)
	}

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:   observeCursorProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeCursorResponseEventGoldens(responseEvents),
		InvocationResult: observeCursorFailedInvocationResultGolden(inferencePayload),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestCursorGoldenProcessFailureAndTimeoutRemainDistinct replays sanitized Cursor
// process-failure and timeout goldens through the customer process boundary and
// proves those public failure classes remain distinct on Provider Session,
// FactoryResponseEvent, and invocation-result surfaces.
//golden: docs/temp/functional/provider-sessions/cursor/process-failure/manifest.json
//golden: docs/temp/functional/provider-sessions/cursor/timeout/manifest.json
func TestCursorGoldenProcessFailureAndTimeoutRemainDistinct(t *testing.T) {
	t.Run("process-failure", func(t *testing.T) {
		runCursorFailureGoldenCase(
			t,
			cursorGoldenProcessFailureCase,
			"cursor-process-failure",
			support.ProviderSessionFidelityPartialStream,
			factoryapi.WorkFailureTypeAuthFailure,
			factoryapi.WorkFailureTypeTimeout,
		)
	})
	t.Run("timeout", func(t *testing.T) {
		runCursorFailureGoldenCase(
			t,
			cursorGoldenTimeoutCase,
			"cursor-timeout",
			support.ProviderSessionFidelityPartialStream,
			factoryapi.WorkFailureTypeTimeout,
			factoryapi.WorkFailureTypeAuthFailure,
		)
	})
}

func runCursorFailureGoldenCase(
	t *testing.T,
	caseName string,
	manifestID string,
	fidelityClass string,
	wantReason factoryapi.WorkFailureType,
	notReason factoryapi.WorkFailureType,
) {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("cursor", caseName)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != manifestID {
		t.Fatalf("manifest.ID = %q, want %s", loaded.Manifest.ID, manifestID)
	}
	if loaded.Manifest.FidelityClass != fidelityClass {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			fidelityClass,
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
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCursor, request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"cursor golden `+caseName+`"}`))

	exitCode := 1
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	commandResult := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	}
	var runner *testutil.ProviderCommandRunner
	if wantReason == factoryapi.WorkFailureTypeTimeout {
		runner = testutil.NewProviderCommandRunner(
			commandResult,
			commandResult,
			commandResult,
			commandResult,
		)
	} else {
		runner = testutil.NewProviderCommandRunner(commandResult)
	}

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

	inferencePayload := cursorGoldenFailedInferenceObservationWithReason(t, events, wantReason)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inferencePayload.Outcome)
	}
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inferencePayload.FailureDetail.Reason != wantReason {
		t.Fatalf("failure reason = %q, want %q", inferencePayload.FailureDetail.Reason, wantReason)
	}
	if notReason != "" && inferencePayload.FailureDetail.Reason == notReason {
		t.Fatalf("failure reason = %q, must remain distinct from %q", inferencePayload.FailureDetail.Reason, notReason)
	}
	if strings.TrimSpace(inferencePayload.FailureDetail.Message) == "" {
		t.Fatal("failure message is empty, want stable public diagnostic")
	}
	if wantReason == factoryapi.WorkFailureTypeTimeout {
		assertCursorGoldenTimeoutDoesNotTreatPartialStdoutAsSuccess(t, events)
	}

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:   observeCursorProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeCursorResponseEventGoldens(responseEvents),
		InvocationResult: observeCursorFailedInvocationResultGolden(inferencePayload),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

func assertCursorGoldenTimeoutDoesNotTreatPartialStdoutAsSuccess(
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
			t.Fatalf("dispatch output = %q, must not treat partial stdout as success on timeout", *payload.Output)
		}
	}
}

func cursorGoldenFailedInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()
	return cursorGoldenFailedInferenceObservationWithReason(t, events, "")
}

func cursorGoldenFailedInferenceObservationWithReason(
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

func observeCursorFailedInvocationResultGolden(
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
