package mock

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const mockUsageWorkID = "mock-usage-costs"

// TestMockWorkerUsageIsVisibleAndPriceableThroughPublicCLI proves the
// documented mock-worker path produces one correlated, priceable usage row
// without invoking a live provider or reading a recording fixture.
func TestMockWorkerUsageIsVisibleAndPriceableThroughPublicCLI(t *testing.T) {
	t.Parallel()
	factoryDir := testutil.CopyFixtureDir(t, support.AgentFactoryPath(t, "examples/simple-tasks"))
	support.WriteAgentConfig(t, factoryDir, "executor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.ClearSeedInputs(t, factoryDir)
	testutil.WriteSeedRequest(t, factoryDir, work.SubmitRequest{
		WorkID:     mockUsageWorkID,
		Name:       "mock usage pricing",
		WorkTypeID: "story",
		TraceID:    "trace-mock-usage-costs",
		Payload:    []byte(`{"title":"price mock usage"}`),
	})

	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:        factoryDir,
		MockWorkersConfig: mockUsageWorkersConfig(),
		Args:              []string{"--provider", "codex", "--model", "gpt-5-codex"},
		Env:               environment,
	})
	defer server.Stop(t)

	support.WaitForTerminalStatus(t, server.URL(), 15*time.Second)
	workItem := singleMockUsageWork(t, server.URL())
	workerSessionID := usageWorkerSessionID(t, server.URL(), workItem)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	showOutput := executeMockUsageCLI(t, process, environment, factoryDir,
		"--server", server.URL(), "worker-sessions", "show", "--worker-session-id", workerSessionID)

	costOutput := executeMockUsageCLI(t, process, environment, factoryDir,
		"--server", server.URL(), "metrics", "costs")
	assertMockUsageShowOutput(t, showOutput)
	assertMockUsageCostOutput(t, costOutput)

	costJSON := executeMockUsageCLI(t, process, environment, factoryDir,
		"--json", "--server", server.URL(), "metrics", "costs")
	assertMockUsageCostReport(t, costJSON, workerSessionID, workItem)
}

func mockUsageWorkersConfig() *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkstationName: "execute-story",
			RunType:         workers.MockWorkerRunTypeAccept,
			Usage: &workers.MockWorkerUsageConfig{
				Provider:              "codex",
				Model:                 "gpt-5-codex",
				InputTokens:           mockUsageInt64(1_000_000),
				CachedInputTokens:     mockUsageInt64(400_000),
				OutputTokens:          mockUsageInt64(500_000),
				ReasoningOutputTokens: mockUsageInt64(100_000),
			},
		}},
	}
}

func mockUsageInt64(value int64) *int64 {
	return &value
}

func singleMockUsageWork(t testing.TB, baseURL string) factoryapi.Work {
	t.Helper()
	listed := support.ListDefaultSessionWork(t, baseURL)
	if len(listed.Results) != 1 {
		t.Fatalf("mock usage Work count = %d, want one: %#v", len(listed.Results), listed.Results)
	}
	return listed.Results[0]
}

func usageWorkerSessionID(t testing.TB, baseURL string, workItem factoryapi.Work) string {
	t.Helper()
	if workItem.WorkId == nil || strings.TrimSpace(*workItem.WorkId) == "" {
		t.Fatalf("mock usage Work has no Work ID: %#v", workItem)
	}
	sessions, err := support.WaitForObservation(
		2*time.Second,
		func() (factoryapi.ListWorkerSessionsResponse, error) {
			return support.ListDefaultSessionWorkerSessions(t, baseURL, *workItem.WorkId), nil
		},
		func(value factoryapi.ListWorkerSessionsResponse) bool {
			return usageWorkerSessionIDFromResponse(value) != ""
		},
	)
	if err != nil {
		t.Fatalf("waiting for usage-bearing Worker Session: %v", err)
	}
	return usageWorkerSessionIDFromResponse(sessions)
}

func usageWorkerSessionIDFromResponse(sessions factoryapi.ListWorkerSessionsResponse) string {
	var usageSessionID string
	usageCount := 0
	for _, session := range sessions.Sessions {
		if session.TokenUsage == nil {
			continue
		}
		usageSessionID = session.WorkerSessionId
		usageCount++
	}
	if usageCount == 1 {
		return usageSessionID
	}
	return ""
}

func executeMockUsageCLI(
	t testing.TB,
	process support.Process,
	environment []string,
	workingDirectory string,
	args ...string,
) string {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("execute public CLI %v: %v\nstdout=%s\nstderr=%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs.Stdout()
}

func assertMockUsageShowOutput(t testing.TB, output string) {
	t.Helper()
	for _, expected := range []string{
		"Provider:\tcodex",
		"Model:\tgpt-5-codex",
		"Token usage:\tinput=1000000 cached-input=400000 cache-write=- output=500000 reasoning=100000 total=1500000",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("worker-sessions show output missing %q:\n%s", expected, output)
		}
	}
}

func assertMockUsageCostOutput(t testing.TB, output string) {
	t.Helper()
	for _, expected := range []string{
		"Status: PRICED",
		"Cost (USD): $5.80",
		"Unpriced dispatches: 0",
		"Unpriced usage: 0 rows",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("metrics costs output missing %q:\n%s", expected, output)
		}
	}
}

func assertMockUsageCostReport(
	t testing.TB,
	output string,
	workerSessionID string,
	workItem factoryapi.Work,
) {
	t.Helper()
	var report generatedclient.CostsReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode metrics costs JSON: %v\noutput=%s", err, output)
	}
	if report.Status != generatedclient.CostsReportStatus("PRICED") {
		t.Fatalf("metrics costs status = %q, want PRICED", report.Status)
	}
	if report.KnownCost == nil || *report.KnownCost != "5.8" || report.PricedSubtotal == nil || *report.PricedSubtotal != "5.8" {
		t.Fatalf("metrics costs amounts = known=%v subtotal=%v, want exact 5.8", report.KnownCost, report.PricedSubtotal)
	}
	if report.UnpricedDispatchCount != 0 || report.Coverage.EncounteredRows != 1 || report.Coverage.PricedRows != 1 ||
		report.Coverage.UnpricedRows != 0 || report.Coverage.EncounteredProviderModels != 1 || report.Coverage.PricedProviderModels != 1 ||
		report.Coverage.UnpricedProviderModels != 0 || len(report.LineItems) != 1 {
		t.Fatalf("metrics costs coverage = %#v, line items=%d, want one fully priced row", report.Coverage, len(report.LineItems))
	}
	item := report.LineItems[0]
	if item.Provider == nil || *item.Provider != "CODEX" || item.Model == nil || *item.Model != "gpt-5-codex" ||
		item.WorkerSessionId == nil || *item.WorkerSessionId != workerSessionID || item.WorkId == nil ||
		workItem.WorkId == nil || *item.WorkId != *workItem.WorkId || item.Status != generatedclient.CostsLineItemStatus("PRICED") ||
		item.PricedAmount == nil || *item.PricedAmount != "5.8" {
		t.Fatalf("metrics costs line item = %#v, want correlated priced codex row", item)
	}
	assertMockUsageToken(t, item.InputTokens, 1_000_000, "input")
	assertMockUsageToken(t, item.CachedInputTokens, 400_000, "cached input")
	assertMockUsageToken(t, item.OutputTokens, 500_000, "output")
	assertMockUsageToken(t, item.ReasoningOutputTokens, 100_000, "reasoning output")
}

func assertMockUsageToken(t testing.TB, got *int64, want int64, name string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("metrics costs %s tokens = %v, want %d", name, got, want)
	}
}
