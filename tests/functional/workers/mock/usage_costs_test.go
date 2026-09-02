package mock

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const mockUsageWorkID = "mock-usage-costs"

// testMockWorkerUsageIsVisibleAndPriceableThroughSharedProcess proves the
// documented mock-worker path produces one correlated, priceable usage row
// without invoking a live provider or reading a recording fixture.
func testMockWorkerUsageIsVisibleAndPriceableThroughSharedProcess(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
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

	fixture.useCommandRunnersFor(t, factoryDir, nil, nil)
	session := fixture.openSession(t, factoryDir)
	listed, _ := session.terminalObservations(t, 15*time.Second)
	defer session.closeAndAssertGone(t)
	workItem := singleMockUsageWork(t, listed)
	workerSessionID, observation := usageWorkerSession(t, fixture.server.URL(), session.id, workItem)

	listOutput := executeMockUsageCLI(t, fixture, factoryDir,
		"--server", fixture.server.URL(), "worker-sessions", "list", "--session", session.id,
		"--work-id", *workItem.WorkId)

	costOutput := executeMockUsageCLI(t, fixture, factoryDir,
		"--server", fixture.server.URL(), "metrics", "costs", "--session", session.id)
	assertMockUsageListOutput(t, listOutput, workerSessionID)
	assertMockUsageObservation(t, observation, workerSessionID, session.id, workItem)
	assertMockUsageCostOutput(t, costOutput)

	costJSON := executeMockUsageCLI(t, fixture, factoryDir,
		"--json", "--server", fixture.server.URL(), "metrics", "costs", "--session", session.id)
	assertMockUsageCostReport(t, costJSON, workerSessionID, workItem)
}

func singleMockUsageWork(t testing.TB, listed factoryapi.ListWorkResponse) factoryapi.Work {
	t.Helper()
	if len(listed.Results) != 1 {
		t.Fatalf("mock usage Work count = %d, want one: %#v", len(listed.Results), listed.Results)
	}
	return listed.Results[0]
}

func usageWorkerSession(
	t testing.TB,
	baseURL, sessionID string,
	workItem factoryapi.Work,
) (string, factoryapi.WorkerSessionObservation) {
	t.Helper()
	if workItem.WorkId == nil || strings.TrimSpace(*workItem.WorkId) == "" {
		t.Fatalf("mock usage Work has no Work ID: %#v", workItem)
	}
	sessions, err := support.WaitForObservation(
		2*time.Second,
		func() (factoryapi.ListWorkerSessionsResponse, error) {
			endpoint := strings.TrimSuffix(baseURL, "/") +
				"/factory-sessions/" + url.PathEscape(sessionID) +
				"/worker-sessions?workId=" + url.QueryEscape(*workItem.WorkId)
			return support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint), nil
		},
		func(value factoryapi.ListWorkerSessionsResponse) bool {
			return usageWorkerSessionObservationFromResponse(value).WorkerSessionId != ""
		},
	)
	if err != nil {
		t.Fatalf("waiting for usage-bearing Worker Session: %v", err)
	}
	observation := usageWorkerSessionObservationFromResponse(sessions)
	return observation.WorkerSessionId, observation
}

func usageWorkerSessionObservationFromResponse(sessions factoryapi.ListWorkerSessionsResponse) factoryapi.WorkerSessionObservation {
	var usageObservation factoryapi.WorkerSessionObservation
	usageCount := 0
	for _, session := range sessions.Sessions {
		if session.TokenUsage == nil {
			continue
		}
		usageObservation = session
		usageCount++
	}
	if usageCount == 1 {
		return usageObservation
	}
	return factoryapi.WorkerSessionObservation{}
}

func executeMockUsageCLI(
	t testing.TB,
	fixture *sharedWorkersMockFixture,
	workingDirectory string,
	args ...string,
) string {
	t.Helper()
	inputs, err := fixture.executeCLI(t, workingDirectory, args...)
	if err != nil {
		t.Fatalf("execute public CLI %v: %v\nstdout=%s\nstderr=%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs.Stdout()
}

func assertMockUsageListOutput(t testing.TB, output, workerSessionID string) {
	t.Helper()
	for _, expected := range []string{
		"WORKER SESSION ID",
		workerSessionID,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("worker-sessions list output missing %q:\n%s", expected, output)
		}
	}
}

func assertMockUsageObservation(
	t testing.TB,
	observation factoryapi.WorkerSessionObservation,
	workerSessionID, sessionID string,
	workItem factoryapi.Work,
) {
	t.Helper()
	if observation.WorkerSessionId != workerSessionID || observation.FactorySessionId == nil ||
		*observation.FactorySessionId != sessionID || observation.WorkId == nil || workItem.WorkId == nil ||
		*observation.WorkId != *workItem.WorkId || observation.Model == nil || *observation.Model != "gpt-5-codex" ||
		observation.TokenUsage == nil {
		t.Fatalf("Worker Session observation = %#v, want correlated session/work/model/usage", observation)
	}
	usage := observation.TokenUsage
	assertMockUsageObservationToken(t, usage.InputTokens, 1_000_000, "input")
	assertMockUsageObservationToken(t, usage.CachedInputTokens, 400_000, "cached input")
	assertMockUsageObservationToken(t, usage.OutputTokens, 500_000, "output")
	assertMockUsageObservationToken(t, usage.ReasoningOutputTokens, 100_000, "reasoning output")
}

func assertMockUsageObservationToken(t testing.TB, got *int, want int, name string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("Worker Session %s tokens = %v, want %d", name, got, want)
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
