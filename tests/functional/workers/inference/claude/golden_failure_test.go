package claude

import (
	"encoding/json"
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

const claudeGoldenStructuredFailureCase = "structured-failure"

// TestClaudeGoldenStructuredFailure replays a sanitized Claude structured-failure
// transcript through the customer process boundary and proves public Factory
// response events, Provider Session metadata, and the terminal invocation result
// expose normalized structured failure metadata rather than success or timeout.
//
//golden: docs/temp/functional/provider-sessions/claude/structured-failure/manifest.json
func TestClaudeGoldenStructuredFailure(t *testing.T) {
	loaded := loadClaudeGoldenCase(t, claudeGoldenStructuredFailureCase)
	if loaded.Manifest.ID != "claude-structured-failure" {
		t.Fatalf("manifest.ID = %q, want claude-structured-failure", loaded.Manifest.ID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityFinalOnly {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityFinalOnly,
		)
	}

	observed := replayClaudeStructuredFailureGoldenCase(t, loaded)
	assertProviderSessionGoldensMatch(t, loaded, observed)
}

func replayClaudeStructuredFailureGoldenCase(
	t *testing.T,
	loaded support.ProviderSessionCase,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderClaude, loaded.Process.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"claude golden structured failure"}`))

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
		t.Fatalf("Claude command runner calls = %d, want 1", runner.CallCount())
	}

	inferencePayload := claudeGoldenFailedInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("inference outcome = %q, want FAILED", inferencePayload.Outcome)
	}
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if inferencePayload.FailureDetail.Reason == factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want structured failure not timeout", inferencePayload.FailureDetail.Reason)
	}
	if inferencePayload.FailureDetail.Reason != factoryapi.WorkFailureTypePermanentBadRequest {
		t.Fatalf("failure reason = %q, want PERMANENT_BAD_REQUEST", inferencePayload.FailureDetail.Reason)
	}
	if inferencePayload.FailureDetail.Message != "Reduce the request size below 20 MB." {
		t.Fatalf("failure message = %q, want actionable invalid-request correction", inferencePayload.FailureDetail.Message)
	}
	if inferencePayload.Response != nil && strings.Contains(*inferencePayload.Response, "COMPLETE") {
		t.Fatalf("structured failure must not treat COMPLETE-bearing output as success: %q", *inferencePayload.Response)
	}

	return observeClaudeFailedProviderSessionGoldens(t, inferencePayload, responseEvents)
}

func claudeGoldenFailedInferenceObservation(
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
			t.Fatalf("decode INFERENCE_RESPONSE %q: %v", event.Id, err)
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

func observeClaudeFailedProviderSessionGoldens(
	t *testing.T,
	inferenceResponse factoryapi.InferenceResponseEventPayload,
	responseEvents []factoryapi.FactoryResponseEvent,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	providerSessionRaw, err := marshalProviderSessionGoldenJSON(inferenceResponse.ProviderSession)
	if err != nil {
		t.Fatalf("marshal observed provider session: %v", err)
	}

	responseEventRecords := make([]json.RawMessage, 0, len(responseEvents))
	for index, event := range responseEvents {
		record, err := marshalProviderSessionGoldenJSON(event)
		if err != nil {
			t.Fatalf("marshal observed response event[%d]: %v", index, err)
		}
		responseEventRecords = append(responseEventRecords, record)
	}

	invocationResult, err := marshalProviderSessionGoldenJSON(claudeFailedInvocationResultGolden(inferenceResponse))
	if err != nil {
		t.Fatalf("marshal observed invocation result: %v", err)
	}

	return support.ProviderSessionObservedGoldens{
		ProviderSession:   providerSessionRaw,
		ResponseEvents:   responseEventRecords,
		InvocationResult: invocationResult,
	}
}

type claudeFailedInvocationResultGoldenView struct {
	OK            bool   `json:"ok"`
	Content       string `json:"content,omitempty"`
	FailureReason string `json:"failureReason,omitempty"`
	Message       string `json:"message,omitempty"`
}

func claudeFailedInvocationResultGolden(
	inferenceResponse factoryapi.InferenceResponseEventPayload,
) claudeFailedInvocationResultGoldenView {
	view := claudeFailedInvocationResultGoldenView{OK: false}
	if inferenceResponse.FailureDetail != nil {
		view.FailureReason = string(inferenceResponse.FailureDetail.Reason)
		view.Message = inferenceResponse.FailureDetail.Message
	}
	if inferenceResponse.Response != nil {
		view.Content = strings.TrimSpace(*inferenceResponse.Response)
	}
	return view
}
