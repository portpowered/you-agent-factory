package claude

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Claude timeout retries are retryable; queue one transcript result per process
// invocation so ProviderCommandRunner never falls back to its default mock.
const claudeGoldenTimeoutCommandInvocations = 9

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

func claudeTimeoutCommandRunner(timeoutResult platformprocess.CommandResult) *testutil.ProviderCommandRunner {
	results := make([]platformprocess.CommandResult, claudeGoldenTimeoutCommandInvocations)
	for index := range results {
		results[index] = timeoutResult
	}
	return testutil.NewProviderCommandRunner(results...)
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
