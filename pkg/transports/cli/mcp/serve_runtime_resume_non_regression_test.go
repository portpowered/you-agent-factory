package mcpcli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPResumeSmokeLane_FixtureBackedInstallSmokeRegression(t *testing.T) {
	client, stdinWrite, serveErr := startRunServeSmokeServer(t, installSmokeExecutionScript{})
	assertInstallSmokeInitialize(t, client)
	assertInstallSmokeDiscovery(t, client)
	closeRunServeSmokeServer(t, stdinWrite, serveErr)
}

func TestMCPResumeSmokeLane_ToolDiscoveryAvoidsForbiddenVocabulary(t *testing.T) {
	service := installSmokeExecutionScript{}
	client, stdinWrite, serveErr := startRunServeSmokeServer(t, service)
	assertInstallSmokeInitialize(t, client)

	toolsResult := client.call("tools/list", map[string]any{})
	encoded, err := json.Marshal(toolsResult.Result)
	if err != nil {
		t.Fatalf("marshal tools/list: %v", err)
	}
	assertMCPResumeSmokeLaneOutputExcludesForbiddenVocabulary(t, string(encoded))

	closeRunServeSmokeServer(t, stdinWrite, serveErr)
}

func assertMCPResumeSmokeLaneOutputExcludesForbiddenVocabulary(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, term := range []string{"DynamicWorkflowRun", "workflow run"} {
		if strings.Contains(lower, strings.ToLower(term)) {
			t.Fatalf("output introduced forbidden vocabulary %q:\n%s", term, text)
		}
	}
}
