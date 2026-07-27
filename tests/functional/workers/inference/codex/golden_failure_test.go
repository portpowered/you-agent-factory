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

const codexGoldenStructuredFailureCase = "structured-failure"

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

func failedInferenceResponse(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

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
		return payload
	}
	t.Fatal("missing failed inference response")
	return factoryapi.InferenceResponseEventPayload{}
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
