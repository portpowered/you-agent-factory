package cursor

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCursorGoldenTextSuccessAndSessionIdentity replays the sanitized Cursor text-success
// golden through the public process boundary and proves successful text output,
// stable Provider Session identity, and matching public metadata goldens.
//golden: docs/temp/functional/provider-sessions/cursor/success/manifest.json
func TestCursorGoldenTextSuccessAndSessionIdentity(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("cursor", "success")))

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "cursor-text-success" {
		t.Fatalf("manifest.ID = %q, want cursor-text-success", loaded.Manifest.ID)
	}

	var request struct {
		Model     string `json:"model"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" {
		t.Fatal("request.model must be non-empty")
	}
	if request.SessionID == "" {
		t.Fatal("request.session_id must be non-empty")
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCursor, request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"cursor golden success"}`))

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

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	inferencePayload, dispatchOutput := cursorGoldenInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("inference outcome = %q, want SUCCEEDED", inferencePayload.Outcome)
	}
	if inferencePayload.ProviderSession == nil || inferencePayload.ProviderSession.Id == nil {
		t.Fatal("inference response missing provider session identity")
	}
	if got := support.StringPointerValue(inferencePayload.ProviderSession.Id); got != request.SessionID {
		t.Fatalf(
			"provider session id = %q, want golden session %q",
			got,
			request.SessionID,
		)
	}
	if inferencePayload.Response == nil || *inferencePayload.Response != dispatchOutput {
		t.Fatalf(
			"inference response text = %#v, want dispatch output %q",
			inferencePayload.Response,
			dispatchOutput,
		)
	}

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:   observeCursorProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeCursorResponseEventGoldens(responseEvents),
		InvocationResult: observeCursorInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

func cursorGoldenInferenceObservation(
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

func observeCursorProviderSessionGolden(
	payload factoryapi.InferenceResponseEventPayload,
	manifest support.ProviderSessionGoldenManifest,
) json.RawMessage {
	status := "failed"
	if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
		status = "completed"
	}
	provider := string(modelprovider.ProviderCursor)
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
	encoded, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	return encoded
}

func observeCursorResponseEventGoldens(events []factoryapi.FactoryResponseEvent) []json.RawMessage {
	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		switch event.Kind {
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
					"itemId":           cursorGoldenItemID(event),
					"text":             *delta.TextDelta,
				}
				records = append(records, mustMarshalJSON(record))
			case factoryapi.FactoryResponseEventPhaseCompleted:
				message, err := event.Payload.AsFactoryResponseEventMessagePayload()
				if err != nil {
					continue
				}
				text := cursorGoldenMessageText(message)
				if text == "" {
					continue
				}
				record := map[string]any{
					"type":             "message.completed",
					"eventId":          event.EventId,
					"factorySessionId": event.FactorySessionId,
					"runId":            event.RunId,
					"itemId":           cursorGoldenItemID(event),
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
				"itemId":           cursorGoldenItemID(event),
				"toolName":         tool.ToolName,
			}
			records = append(records, mustMarshalJSON(record))
		}
	}
	return records
}

func observeCursorInvocationResultGolden(
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
	encoded, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	return encoded
}

func cursorGoldenItemID(event factoryapi.FactoryResponseEvent) string {
	if event.ItemId != nil && *event.ItemId != "" {
		return *event.ItemId
	}
	return ""
}

func cursorGoldenMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
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
