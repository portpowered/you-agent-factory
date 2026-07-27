package codex

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

// TestCodexGoldenTextAndToolSuccess replays the sanitized Codex message-tool-success
// transcript through the public process boundary and proves public text and tool success.
//golden: docs/temp/functional/provider-sessions/codex/success/manifest.json
func TestCodexGoldenTextAndToolSuccess(t *testing.T) {
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
	if !goldenStdoutContainsToolLifecycle(loaded, "tool_fixture_1") {
		t.Fatal("golden stdout fixture must include tool lifecycle records for tool_fixture_1")
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

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}
	if got := string(runner.LastRequest().Stdin); got == "" {
		t.Fatal("provider command runner stdin is empty, want rendered prompt")
	}

	assertCodexGoldenTextAndToolSuccess(
		t,
		responseEvents,
		events,
		loaded,
		"Codex fixture answer COMPLETE",
		"tool_fixture_1",
	)
}

func assertCodexGoldenTextAndToolSuccess(
	t *testing.T,
	responseEvents []factoryapi.FactoryResponseEvent,
	factoryEvents []factoryapi.FactoryEvent,
	loaded support.ProviderSessionCase,
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
	if !goldenStdoutContainsToolLifecycle(loaded, wantToolCallID) {
		t.Fatalf("replayed golden stdout missing tool lifecycle for %q", wantToolCallID)
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
		var envelope struct {
			Type string `json:"type"`
			Item struct {
				ID     string `json:"id"`
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"item"`
		}
		if err := json.Unmarshal(record, &envelope); err != nil {
			continue
		}
		if envelope.Item.ID != wantToolCallID || envelope.Item.Type != "mcp_tool_call" {
			continue
		}
		switch envelope.Type {
		case "item.started":
			started = true
		case "item.completed":
			if envelope.Item.Status == "completed" {
				completed = true
			}
		}
	}
	return started && completed
}
