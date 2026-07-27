//go:build functionallong

package claude

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

// TestClaudeGoldenFullStreamTextSuccess replays a sanitized Claude full-stream
// transcript through the customer process boundary and proves public Factory
// response events, Provider Session metadata, and the terminal invocation
// result expose streaming text deltas and a truthful final snapshot.
//
//golden: docs/temp/functional/provider-sessions/claude/full-stream-text-success/manifest.json
func TestClaudeGoldenFullStreamTextSuccess(t *testing.T) {
	support.SkipLongFunctional(t, "slow Claude golden full-stream replay")

	loaded := loadClaudeGoldenCase(t, "full-stream-text-success")
	observed := replayClaudeGoldenCase(t, loaded)
	assertProviderSessionGoldensMatch(t, loaded, observed)
}

func loadClaudeGoldenCase(t *testing.T, caseName string) support.ProviderSessionCase {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("claude", caseName)))
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q): %v", caseName, err)
	}
	return loaded
}

func replayClaudeGoldenCase(t *testing.T, loaded support.ProviderSessionCase) support.ProviderSessionObservedGoldens {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderClaude, loaded.Process.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"claude golden full-stream text success"}`))

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
		t.Fatalf("Claude command runner calls = %d, want 1", runner.CallCount())
	}

	return observeProviderSessionGoldens(t, events, responseEvents)
}

func observeProviderSessionGoldens(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	inferenceResponse, ok := successfulInferenceResponsePayload(t, events)
	if !ok {
		t.Fatal("missing successful INFERENCE_RESPONSE factory event")
	}

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

	invocationResult, err := marshalProviderSessionGoldenJSON(claudeInvocationResultGolden(inferenceResponse))
	if err != nil {
		t.Fatalf("marshal observed invocation result: %v", err)
	}

	return support.ProviderSessionObservedGoldens{
		ProviderSession:   providerSessionRaw,
		ResponseEvents:   responseEventRecords,
		InvocationResult: invocationResult,
	}
}

func successfulInferenceResponsePayload(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) (factoryapi.InferenceResponseEventPayload, bool) {
	t.Helper()

	var payload factoryapi.InferenceResponseEventPayload
	found := false
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		response, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode INFERENCE_RESPONSE %q: %v", event.Id, err)
		}
		if response.Outcome != factoryapi.InferenceOutcomeSucceeded {
			continue
		}
		payload = response
		found = true
	}
	return payload, found
}

type claudeInvocationResultGoldenView struct {
	OK      bool   `json:"ok"`
	Content string `json:"content,omitempty"`
}

func claudeInvocationResultGolden(
	inferenceResponse factoryapi.InferenceResponseEventPayload,
) claudeInvocationResultGoldenView {
	content := ""
	if inferenceResponse.Response != nil {
		content = strings.TrimSpace(*inferenceResponse.Response)
	}
	return claudeInvocationResultGoldenView{
		OK:      inferenceResponse.Outcome == factoryapi.InferenceOutcomeSucceeded,
		Content: content,
	}
}

func marshalProviderSessionGoldenJSON(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage("null"), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func assertProviderSessionGoldensMatch(
	t *testing.T,
	loaded support.ProviderSessionCase,
	observed support.ProviderSessionObservedGoldens,
) {
	t.Helper()

	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}
