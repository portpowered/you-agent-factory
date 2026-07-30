// Package association owns functional coverage for Provider Session ref correlation
// on public Factory Session dispatch projections and response-exec golden metadata
// survival across CLI projection, API response events, and replay.
package association_test

import (
	"encoding/json"
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

const responseExecCodexSuccessGoldenCase = "success"

// TestResponseExecGoldenMetadataSurvivesCLIProjection replays a sanitized Codex
// provider execution transcript through root.BuildProcess and proves public
// Factory Event projection after CLI invocation preserves checked-in Provider
// Session and invocation-result golden metadata without mapper-generated
// expectations.
//
// golden: tests/functional/internal/support/testdata/provider-sessions/codex/success/manifest.json
func TestResponseExecGoldenMetadataSurvivesCLIProjection(t *testing.T) {
	t.Parallel()

	loaded := loadResponseExecCodexGoldenCase(t, responseExecCodexSuccessGoldenCase)
	replay := runResponseExecCodexGoldenReplay(t, loaded)
	observed := observeResponseExecFactoryEventGoldens(t, loaded, replay.FactoryEvents)

	if err := support.CompareProviderSessionJSON(
		loaded.Manifest.ID,
		"expected-provider-session",
		loaded.Manifest.NormalizedFields,
		loaded.Expected.ProviderSession,
		observed.ProviderSession,
	); err != nil {
		t.Fatalf("CompareProviderSessionJSON(provider-session): %v", err)
	}
	if err := support.CompareProviderSessionJSON(
		loaded.Manifest.ID,
		"expected-invocation-result",
		loaded.Manifest.NormalizedFields,
		loaded.Expected.InvocationResult,
		observed.InvocationResult,
	); err != nil {
		t.Fatalf("CompareProviderSessionJSON(invocation-result): %v", err)
	}
}

// TestResponseExecGoldenMetadataSurvivesReplay records a sanitized Codex provider
// execution transcript through root.BuildProcess, replays the public recording,
// and proves replay observation preserves checked-in Provider Session and
// invocation-result golden metadata without mapper-generated expectations.
//
// golden: tests/functional/internal/support/testdata/provider-sessions/codex/success/manifest.json
func TestResponseExecGoldenMetadataSurvivesReplay(t *testing.T) {
	t.Parallel()

	loaded := loadResponseExecCodexGoldenCase(t, responseExecCodexSuccessGoldenCase)
	replay := observeResponseExecCodexGoldenReplay(t, loaded)
	observed := observeResponseExecFactoryEventGoldens(t, loaded, replay.FactoryEvents)

	if err := support.CompareProviderSessionJSON(
		loaded.Manifest.ID,
		"expected-provider-session",
		loaded.Manifest.NormalizedFields,
		loaded.Expected.ProviderSession,
		observed.ProviderSession,
	); err != nil {
		t.Fatalf("CompareProviderSessionJSON(provider-session): %v", err)
	}
	if err := support.CompareProviderSessionJSON(
		loaded.Manifest.ID,
		"expected-invocation-result",
		loaded.Manifest.NormalizedFields,
		loaded.Expected.InvocationResult,
		observed.InvocationResult,
	); err != nil {
		t.Fatalf("CompareProviderSessionJSON(invocation-result): %v", err)
	}
}

// TestResponseExecGoldenMetadataSurvivesAPIResponseEvents replays a sanitized
// Codex provider execution transcript through root.BuildProcess and proves
// public FactoryResponseEvent history after API observation preserves
// checked-in response-event golden metadata without mapper-generated
// expectations.
//
// golden: tests/functional/internal/support/testdata/provider-sessions/codex/success/manifest.json
func TestResponseExecGoldenMetadataSurvivesAPIResponseEvents(t *testing.T) {
	t.Parallel()

	loaded := loadResponseExecCodexGoldenCase(t, responseExecCodexSuccessGoldenCase)
	replay := runResponseExecCodexGoldenReplay(t, loaded)
	observed := observeResponseExecResponseEventGoldens(t, replay.ResponseEvents)

	if err := support.CompareProviderSessionNDJSON(
		loaded.Manifest.ID,
		"expected-response-events",
		loaded.Manifest.NormalizedFields,
		loaded.Expected.ResponseEvents,
		observed,
	); err != nil {
		t.Fatalf("CompareProviderSessionNDJSON(response-events): %v", err)
	}
}

func loadResponseExecCodexGoldenCase(t *testing.T, caseName string) support.ProviderSessionCase {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("codex", caseName)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q): %v", caseName, err)
	}
	return loaded
}

type responseExecCodexReplayResult struct {
	Loaded         support.ProviderSessionCase
	Listed         factoryapi.ListWorkResponse
	FactoryEvents  []factoryapi.FactoryEvent
	ResponseEvents []factoryapi.FactoryResponseEvent
	Runner         *testutil.ProviderCommandRunner
}

func runResponseExecCodexGoldenReplay(
	t *testing.T,
	loaded support.ProviderSessionCase,
) responseExecCodexReplayResult {
	t.Helper()

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
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"response-exec cli projection"}`))

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
		t.Fatalf("provider command runner calls = %d, want exactly one mocked Codex invocation", runner.CallCount())
	}

	return responseExecCodexReplayResult{
		Loaded:         loaded,
		Listed:         listed,
		FactoryEvents:  events,
		ResponseEvents: responseEvents,
		Runner:         runner,
	}
}

func observeResponseExecCodexGoldenReplay(
	t *testing.T,
	loaded support.ProviderSessionCase,
) responseExecCodexReplayResult {
	t.Helper()

	artifactPath := recordResponseExecCodexGoldenRun(t, loaded)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath, "--no-record"},
	})
	support.WaitForTerminalStatus(t, server.URL(), 30*time.Second)

	listed := support.ListDefaultSessionWork(t, server.URL())
	events := server.GetFactoryEvents(t)
	server.Stop(t)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("replayed completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("replayed failed work = %d, want 0", got)
	}

	return responseExecCodexReplayResult{
		Loaded:        loaded,
		Listed:        listed,
		FactoryEvents: events,
	}
}

func recordResponseExecCodexGoldenRun(
	t *testing.T,
	loaded support.ProviderSessionCase,
) string {
	t.Helper()

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
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"response-exec replay survival"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})

	artifactPath := filepath.Join(t.TempDir(), "response-exec-metadata.replay.json")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Args:       []string{"--record", artifactPath},
		Edges:      serviceedges.Edges{ProviderCommandRunner: runner},
	})
	support.WaitForTerminalStatus(t, server.URL(), 30*time.Second)

	listed := support.ListDefaultSessionWork(t, server.URL())
	server.Stop(t)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("recorded completed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one mocked Codex invocation", runner.CallCount())
	}

	return artifactPath
}

func observeResponseExecResponseEventGoldens(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) []json.RawMessage {
	t.Helper()

	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		record := projectResponseExecResponseEventGolden(event)
		if len(record) == 0 {
			continue
		}
		records = append(records, record)
	}
	return records
}

func projectResponseExecResponseEventGolden(event factoryapi.FactoryResponseEvent) json.RawMessage {
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

func observeResponseExecFactoryEventGoldens(
	t *testing.T,
	loaded support.ProviderSessionCase,
	events []factoryapi.FactoryEvent,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	inference := succeededResponseExecInferenceResponse(t, events)

	providerSession := projectResponseExecProviderSessionGolden(loaded, inference)
	invocationResult := projectResponseExecInvocationResultGolden(t, loaded, inference, events)

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
		InvocationResult: resultBytes,
	}
}

func succeededResponseExecInferenceResponse(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode INFERENCE_RESPONSE: %v", err)
		}
		if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
			return payload
		}
	}
	t.Fatal("missing succeeded INFERENCE_RESPONSE factory event after CLI invocation")
	return factoryapi.InferenceResponseEventPayload{}
}

func projectResponseExecProviderSessionGolden(
	loaded support.ProviderSessionCase,
	inference factoryapi.InferenceResponseEventPayload,
) map[string]any {
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
		if threadID := responseExecCodexThreadIDFromTranscript(*inference.Response); threadID != "" {
			session["providerSessionId"] = threadID
			return session
		}
	}
	return session
}

func projectResponseExecInvocationResultGolden(
	t *testing.T,
	loaded support.ProviderSessionCase,
	inference factoryapi.InferenceResponseEventPayload,
	events []factoryapi.FactoryEvent,
) map[string]any {
	t.Helper()

	result := map[string]any{
		"ok":           inference.Outcome == factoryapi.InferenceOutcomeSucceeded,
		"finishReason": "stop",
	}

	content := ""
	if inference.Response != nil {
		content = responseExecCodexTerminalAgentMessageText(*inference.Response)
	}
	if content == "" {
		for _, record := range loaded.Stdout.Records {
			if text := responseExecCodexAgentMessageText(record); text != "" {
				content = text
				break
			}
		}
	}
	if content == "" {
		content = responseExecDispatchOutput(t, events)
	}
	if content == "" {
		t.Fatal("missing terminal invocation content in public Factory Event projection")
	}
	result["content"] = content
	return result
}

func responseExecDispatchOutput(t *testing.T, events []factoryapi.FactoryEvent) string {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output != nil && strings.TrimSpace(*payload.Output) != "" {
			return strings.TrimSpace(*payload.Output)
		}
	}
	return ""
}

func responseExecCodexThreadIDFromTranscript(transcript string) string {
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

func responseExecCodexTerminalAgentMessageText(transcript string) string {
	var terminal string
	for _, line := range strings.Split(transcript, "\n") {
		if text := responseExecCodexAgentMessageText([]byte(line)); text != "" {
			terminal = text
		}
	}
	return terminal
}

func responseExecCodexAgentMessageText(record []byte) string {
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
