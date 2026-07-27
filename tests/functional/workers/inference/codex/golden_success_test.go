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

type codexGoldenReplayResult struct {
	Loaded         support.ProviderSessionCase
	Listed         factoryapi.ListWorkResponse
	FactoryEvents  []factoryapi.FactoryEvent
	ResponseEvents []factoryapi.FactoryResponseEvent
	Runner         *testutil.ProviderCommandRunner
}

// TestCodexGoldenTextAndToolSuccess replays the sanitized Codex message-tool-success
// transcript through the public process boundary and proves public text and tool success.
//golden: docs/temp/functional/provider-sessions/codex/success/manifest.json
func TestCodexGoldenTextAndToolSuccess(t *testing.T) {
	replay := runCodexGoldenSuccessReplay(t)

	if got := support.CountWorkAtCustomerState(replay.Listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, replay.Listed)
	}
	if got := support.CountWorkAtCustomerState(replay.Listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if replay.Runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", replay.Runner.CallCount())
	}
	if got := string(replay.Runner.LastRequest().Stdin); got == "" {
		t.Fatal("provider command runner stdin is empty, want rendered prompt")
	}
	if !goldenStdoutContainsToolLifecycle(replay.Loaded, "tool_fixture_1") {
		t.Fatal("golden stdout fixture must include tool lifecycle records for tool_fixture_1")
	}

	assertCodexGoldenTextAndToolSuccess(
		t,
		replay.ResponseEvents,
		replay.FactoryEvents,
		"Codex fixture answer COMPLETE",
		"tool_fixture_1",
	)
}

// TestCodexGoldenDerivesProviderSessionAndResponseEvents replays the sanitized
// Codex message-tool-success transcript and proves public Provider Session,
// FactoryResponseEvent, and invocation-result metadata match the golden contract.
//golden: docs/temp/functional/provider-sessions/codex/success/manifest.json
func TestCodexGoldenDerivesProviderSessionAndResponseEvents(t *testing.T) {
	replay := runCodexGoldenSuccessReplay(t)

	if got := support.CountWorkAtCustomerState(replay.Listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}

	observed := observeCodexProviderSessionGoldens(t, replay)
	if err := support.CompareOrUpdateProviderSessionGoldens(replay.Loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("golden files rewritten under %s: %v", support.ProviderSessionUpdateFunctionalGoldensEnv, err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

func runCodexGoldenSuccessReplay(t *testing.T) codexGoldenReplayResult {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("codex", "success")))

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "codex-message-tool-success" {
		t.Fatalf("manifest.ID = %q, want codex-message-tool-success", loaded.Manifest.ID)
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
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"codex golden success"}`))

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

	return codexGoldenReplayResult{
		Loaded:         loaded,
		Listed:         listed,
		FactoryEvents:  events,
		ResponseEvents: responseEvents,
		Runner:         runner,
	}
}

func observeCodexProviderSessionGoldens(
	t *testing.T,
	replay codexGoldenReplayResult,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	inference := succeededInferenceResponse(t, replay.FactoryEvents)
	providerSession := projectCodexProviderSessionGolden(t, replay.Loaded, inference)
	responseEvents := projectCodexResponseEventGoldens(t, replay.ResponseEvents)
	invocationResult := projectCodexInvocationResultGolden(t, inference, replay.Loaded)

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

func succeededInferenceResponse(
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
		if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
			return payload
		}
	}
	t.Fatal("missing succeeded inference response")
	return factoryapi.InferenceResponseEventPayload{}
}

func projectCodexProviderSessionGolden(
	t *testing.T,
	loaded support.ProviderSessionCase,
	inference factoryapi.InferenceResponseEventPayload,
) map[string]any {
	t.Helper()

	session := map[string]any{
		"provider":      loaded.Manifest.Provider,
		"fidelityClass": loaded.Manifest.FidelityClass,
		"status":        "completed",
	}
	if inference.Outcome != factoryapi.InferenceOutcomeSucceeded {
		session["status"] = "failed"
	}

	if inference.ProviderSession != nil && inference.ProviderSession.Id != nil &&
		strings.TrimSpace(*inference.ProviderSession.Id) != "" {
		session["providerSessionId"] = strings.TrimSpace(*inference.ProviderSession.Id)
		return session
	}

	if inference.Response != nil {
		if threadID := codexThreadIDFromTranscript(*inference.Response); threadID != "" {
			session["providerSessionId"] = threadID
			return session
		}
	}

	t.Fatal("missing public provider session identity in inference observation")
	return session
}

func projectCodexResponseEventGoldens(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) []json.RawMessage {
	t.Helper()

	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		record := projectCodexResponseEventGolden(event)
		if len(record) == 0 {
			continue
		}
		records = append(records, record)
	}
	return records
}

func projectCodexResponseEventGolden(event factoryapi.FactoryResponseEvent) json.RawMessage {
	record := map[string]any{}
	if nativeType := strings.TrimSpace(event.Provenance.NativeEventType); nativeType != "" {
		record["type"] = strings.ToLower(nativeType)
	} else {
		record["type"] = strings.ToLower(string(event.Kind) + "." + strings.ToLower(string(event.Phase)))
	}
	if event.ItemId != nil && strings.TrimSpace(*event.ItemId) != "" {
		record["itemId"] = strings.TrimSpace(*event.ItemId)
	}

	switch event.Kind {
	case factoryapi.FactoryResponseEventKindTool:
		payload, err := event.Payload.AsFactoryResponseEventToolPayload()
		if err == nil {
			if payload.ToolName != "" {
				record["toolName"] = payload.ToolName
			}
			if payload.ToolCallId != "" {
				record["itemId"] = payload.ToolCallId
			}
		}
	case factoryapi.FactoryResponseEventKindMessage:
		switch event.Phase {
		case factoryapi.FactoryResponseEventPhaseDelta:
			payload, err := event.Payload.AsFactoryResponseEventMessageDeltaPayload()
			if err == nil && payload.TextDelta != nil {
				record["text"] = *payload.TextDelta
			}
		case factoryapi.FactoryResponseEventPhaseCompleted:
			payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
			if err == nil {
				for _, block := range payload.ContentBlocks {
					text, err := block.AsFactoryResponseEventTextContentBlock()
					if err == nil && text.Text != "" {
						record["text"] = text.Text
						break
					}
				}
			}
			record["finishReason"] = "stop"
		}
	case factoryapi.FactoryResponseEventKindRun:
		payload, err := event.Payload.AsFactoryResponseEventRunPayload()
		if err == nil && payload.Status != nil && strings.TrimSpace(*payload.Status) != "" {
			record["status"] = strings.ToLower(strings.TrimSpace(*payload.Status))
		}
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return nil
	}
	return encoded
}

func projectCodexInvocationResultGolden(
	t *testing.T,
	inference factoryapi.InferenceResponseEventPayload,
	loaded support.ProviderSessionCase,
) map[string]any {
	t.Helper()

	result := map[string]any{
		"ok":           inference.Outcome == factoryapi.InferenceOutcomeSucceeded,
		"finishReason": "stop",
	}

	content := ""
	if inference.Response != nil {
		content = codexTerminalAgentMessageText(*inference.Response)
	}
	if content == "" {
		for _, record := range loaded.Stdout.Records {
			if text := codexAgentMessageText(record); text != "" {
				content = text
				break
			}
		}
	}
	if content == "" {
		t.Fatal("missing terminal invocation content in public observation")
	}
	result["content"] = content
	return result
}

func codexThreadIDFromTranscript(transcript string) string {
	for _, line := range strings.Split(transcript, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var envelope struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.Type == "thread.started" && strings.TrimSpace(envelope.ThreadID) != "" {
			return strings.TrimSpace(envelope.ThreadID)
		}
	}
	return ""
}

func codexTerminalAgentMessageText(transcript string) string {
	var terminal string
	for _, line := range strings.Split(transcript, "\n") {
		if text := codexAgentMessageText([]byte(line)); text != "" {
			terminal = text
		}
	}
	return terminal
}

func codexAgentMessageText(record []byte) string {
	var envelope struct {
		Type string `json:"type"`
		Item struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			Status string `json:"status"`
		} `json:"item"`
	}
	if err := json.Unmarshal(record, &envelope); err != nil {
		return ""
	}
	if envelope.Type != "item.completed" || envelope.Item.Type != "agent_message" {
		return ""
	}
	return strings.TrimSpace(envelope.Item.Text)
}

func assertCodexGoldenTextAndToolSuccess(
	t *testing.T,
	responseEvents []factoryapi.FactoryResponseEvent,
	factoryEvents []factoryapi.FactoryEvent,
	wantFinalText string,
	wantToolCallID string,
) {
	t.Helper()

	var (
		toolStarted    bool
		toolCompleted  bool
		messageSuccess bool
	)

	for _, event := range responseEvents {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindTool:
			payload, err := event.Payload.AsFactoryResponseEventToolPayload()
			if err != nil {
				t.Fatalf("decode tool response event: %v", err)
			}
			if payload.ToolCallId != wantToolCallID {
				continue
			}
			switch event.Phase {
			case factoryapi.FactoryResponseEventPhaseStarted:
				toolStarted = true
			case factoryapi.FactoryResponseEventPhaseCompleted:
				toolCompleted = true
			}
		case factoryapi.FactoryResponseEventKindMessage:
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
				if text.Text == wantFinalText {
					messageSuccess = true
				}
			}
		}
	}

	if toolStarted && toolCompleted && messageSuccess {
		return
	}

	var inferenceSucceeded bool
	for _, event := range factoryEvents {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response event: %v", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeSucceeded {
			continue
		}
		inferenceSucceeded = true
		if payload.Response != nil && strings.Contains(*payload.Response, wantFinalText) {
			messageSuccess = true
		}
	}

	if !inferenceSucceeded {
		t.Fatalf("missing succeeded inference response; events=%#v", factoryEvents)
	}
	if !messageSuccess {
		t.Fatalf("missing terminal text %q in public inference observation", wantFinalText)
	}

	var inferenceTranscript string
	for _, event := range factoryEvents {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			continue
		}
		if payload.Outcome != factoryapi.InferenceOutcomeSucceeded || payload.Response == nil {
			continue
		}
		inferenceTranscript = *payload.Response
		break
	}
	if inferenceTranscript == "" {
		t.Fatal("missing succeeded inference response transcript for tool lifecycle observation")
	}
	if !codexTranscriptContainsToolLifecycle(inferenceTranscript, wantToolCallID) {
		t.Fatalf(
			"public inference transcript missing tool lifecycle for %q; transcript=%q",
			wantToolCallID,
			inferenceTranscript,
		)
	}

	dispatches := support.ObserveDispatchEvents(t, factoryEvents)
	if len(dispatches) == 0 {
		t.Fatal("missing public dispatch observations")
	}
	response := dispatches[len(dispatches)-1].Response
	if response == nil || response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("terminal dispatch outcome = %#v, want ACCEPTED", response)
	}
}

func goldenStdoutContainsToolLifecycle(loaded support.ProviderSessionCase, wantToolCallID string) bool {
	var started bool
	var completed bool
	for _, record := range loaded.Stdout.Records {
		lineStarted, lineCompleted := codexTranscriptLineToolLifecycleState(record, wantToolCallID)
		if lineStarted {
			started = true
		}
		if lineCompleted {
			completed = true
		}
	}
	return started && completed
}

func codexTranscriptContainsToolLifecycle(transcript string, wantToolCallID string) bool {
	var started bool
	var completed bool
	for _, line := range strings.Split(transcript, "\n") {
		lineStarted, lineCompleted := codexTranscriptLineToolLifecycleState([]byte(line), wantToolCallID)
		if lineStarted {
			started = true
		}
		if lineCompleted {
			completed = true
		}
	}
	return started && completed
}

func codexTranscriptLineToolLifecycleState(line []byte, wantToolCallID string) (started bool, completed bool) {
	var envelope struct {
		Type string `json:"type"`
		Item struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"item"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return false, false
	}
	if envelope.Item.ID != wantToolCallID || envelope.Item.Type != "mcp_tool_call" {
		return false, false
	}
	switch envelope.Type {
	case "item.started":
		return true, false
	case "item.completed":
		return false, envelope.Item.Status == "completed"
	default:
		return false, false
	}
}
