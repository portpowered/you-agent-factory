package opencode_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	openCodeStructuredSnapshotGoldenCase = "structured-snapshot-success"
	openCodeFinalOnlyFallbackGoldenCase  = "final-only-fallback"
)

// TestOpenCodeGoldenStructuredSnapshotSuccess replays a sanitized OpenCode
// structured-snapshot transcript through the customer process boundary and
// proves successful structured snapshot outcomes with matching public metadata.
//golden: docs/temp/functional/provider-sessions/opencode/structured-snapshot-success/manifest.json
func TestOpenCodeGoldenStructuredSnapshotSuccess(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("opencode", openCodeStructuredSnapshotGoldenCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "opencode-structured-snapshot-success" {
		t.Fatalf("manifest.ID = %q, want opencode-structured-snapshot-success", loaded.Manifest.ID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelitySnapshotOnly {
		t.Fatalf("manifest.fidelityClass = %q, want %q", loaded.Manifest.FidelityClass, support.ProviderSessionFidelitySnapshotOnly)
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
		support.BuildModelWorkerConfig(modelprovider.ProviderOpenCode, request.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"opencode golden structured snapshot success"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	executablePath := writeOpenCodeFixtureExecutable(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("1.2.3\n")},
		platformprocess.CommandResult{ExitCode: 0},
		platformprocess.CommandResult{
			Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
			Stderr:   []byte(loaded.Stderr),
			ExitCode: exitCode,
		},
	)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			ProviderCommandRunner:      runner,
			WorkersExecutableLocator:   fixedExecutableLocator{path: executablePath},
			WorkersResolveSymlinks:     identityResolveSymlinks,
		},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 3 {
		t.Fatalf("provider command runner calls = %d, want discovery probes plus one invocation", runner.CallCount())
	}

	inferencePayload, dispatchOutput := openCodeGoldenInferenceObservation(t, events)
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
		ProviderSession:   observeOpenCodeProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeOpenCodeResponseEventGoldens(responseEvents),
		InvocationResult: observeOpenCodeInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestOpenCodeGoldenFinalOnlyFallback replays a sanitized OpenCode final-only
// transcript through the customer process boundary and proves authoritative
// terminal success without fabricated streaming deltas or structured snapshot
// lifecycle events.
//golden: docs/temp/functional/provider-sessions/opencode/final-only-fallback/manifest.json
func TestOpenCodeGoldenFinalOnlyFallback(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("opencode", openCodeFinalOnlyFallbackGoldenCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "opencode-final-only-fallback" {
		t.Fatalf("manifest.ID = %q, want opencode-final-only-fallback", loaded.Manifest.ID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityFinalOnly {
		t.Fatalf("manifest.fidelityClass = %q, want %q", loaded.Manifest.FidelityClass, support.ProviderSessionFidelityFinalOnly)
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
		support.BuildModelWorkerConfig(modelprovider.ProviderOpenCode, request.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"opencode golden final-only fallback"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	executablePath := writeOpenCodeFixtureExecutable(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("1.2.3\n")},
		platformprocess.CommandResult{
			Stderr:   []byte("unknown option --format\n"),
			ExitCode: 2,
		},
		platformprocess.CommandResult{
			Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
			Stderr:   []byte(loaded.Stderr),
			ExitCode: exitCode,
		},
	)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			ProviderCommandRunner:      runner,
			WorkersExecutableLocator:   fixedExecutableLocator{path: executablePath},
			WorkersResolveSymlinks:     identityResolveSymlinks,
		},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 3 {
		t.Fatalf("provider command runner calls = %d, want discovery probes plus one invocation", runner.CallCount())
	}

	inferencePayload, dispatchOutput := openCodeGoldenInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("inference outcome = %q, want SUCCEEDED", inferencePayload.Outcome)
	}
	if inferencePayload.Response == nil || *inferencePayload.Response != dispatchOutput {
		t.Fatalf("inference response text = %#v, want dispatch output %q", inferencePayload.Response, dispatchOutput)
	}
	if dispatchOutput == "" || !strings.Contains(dispatchOutput, "COMPLETE") {
		t.Fatalf("dispatch output = %q, want terminal COMPLETE-bearing success text", dispatchOutput)
	}

	assertOpenCodeFinalOnlyPublicResponseEvents(t, responseEvents)

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:   observeOpenCodeProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeOpenCodeResponseEventGoldens(responseEvents),
		InvocationResult: observeOpenCodeInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

func assertOpenCodeFinalOnlyPublicResponseEvents(t *testing.T, events []factoryapi.FactoryResponseEvent) {
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

type fixedExecutableLocator struct {
	path string
}

func (l fixedExecutableLocator) LookPath(file string) (string, error) {
	if file == "opencode" {
		return l.path, nil
	}
	return "", fmt.Errorf("executable %q not found", file)
}

func identityResolveSymlinks(path string) (string, error) {
	return path, nil
}

func writeOpenCodeFixtureExecutable(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(path, []byte("opencode-fixture-executable\n"), 0o755); err != nil {
		t.Fatalf("write opencode fixture executable: %v", err)
	}
	return path
}

func openCodeGoldenInferenceObservation(
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

func observeOpenCodeProviderSessionGolden(
	payload factoryapi.InferenceResponseEventPayload,
	manifest support.ProviderSessionGoldenManifest,
) json.RawMessage {
	status := "failed"
	if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
		status = "completed"
	}
	provider := string(modelprovider.ProviderOpenCode)
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

func observeOpenCodeResponseEventGoldens(events []factoryapi.FactoryResponseEvent) []json.RawMessage {
	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
				continue
			}
			message, err := event.Payload.AsFactoryResponseEventMessagePayload()
			if err != nil {
				continue
			}
			text := openCodeGoldenMessageText(message)
			if text == "" {
				continue
			}
			record := map[string]any{
				"type":         "message.completed",
				"eventId":      event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":        event.RunId,
				"itemId":       openCodeGoldenItemID(event),
				"text":         text,
				"finishReason": "stop",
			}
			records = append(records, mustMarshalJSON(record))
		case factoryapi.FactoryResponseEventKindTool:
			if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
				continue
			}
			tool, err := event.Payload.AsFactoryResponseEventToolPayload()
			if err != nil {
				continue
			}
			record := map[string]any{
				"type":             "tool.completed",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           openCodeGoldenItemID(event),
				"toolName":         tool.ToolName,
			}
			records = append(records, mustMarshalJSON(record))
		case factoryapi.FactoryResponseEventKindUsage:
			if event.Phase != factoryapi.FactoryResponseEventPhaseUpdated {
				continue
			}
			usage, err := event.Payload.AsFactoryResponseEventUsagePayload()
			if err != nil {
				continue
			}
			record := map[string]any{
				"type":             "usage.updated",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"inputTokens":      usage.InputTokens,
				"outputTokens":     usage.OutputTokens,
			}
			records = append(records, mustMarshalJSON(record))
		}
	}
	return records
}

func observeOpenCodeInvocationResultGolden(
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
	return mustMarshalJSON(record)
}

func openCodeGoldenItemID(event factoryapi.FactoryResponseEvent) string {
	if event.ItemId != nil && *event.ItemId != "" {
		return *event.ItemId
	}
	return ""
}

func openCodeGoldenMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
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
