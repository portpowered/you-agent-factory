package runtime_metrics_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	replayPricedInputTokens      int64 = 1_000_000
	replayPricedOutputTokens     int64 = 2_000_000
	replayPricedTotalTokens      int64 = 3_000_000
	replayPricedInputRateCents   int64 = 125
	replayPricedOutputRateCents  int64 = 1_000
	replayPricedMillionTokenUnit int64 = 1_000_000
)

// TestReplayPricedUsageReachesPublicCosts proves replayed priced usage reaches the public cost projection.
func TestReplayPricedUsageReachesPublicCosts(t *testing.T) {
	t.Parallel()
	repoRoot := testutil.MustRepoRoot(t)
	fixturePath := filepath.Join(
		repoRoot,
		"tests", "functional", "factory", "visualization", "runtime_metrics", "testdata",
		"codex-gpt-5-codex.factory-recording.v1.json",
	)
	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	providerRunner := support.NewRecordingCommandRunner("unexpected provider execution")
	scriptRunner := support.NewRecordingCommandRunner("unexpected script execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                t.TempDir(),
		Args:                      []string{"--replay", fixturePath, "--no-record"},
		Env:                       environment,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: providerRunner,
			ScriptCommandRunner:   scriptRunner,
		},
	})
	status := support.WaitForTerminalStatus(t, server.URL(), 15*time.Second)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("replayed Factory Session categories = %#v, want one terminal Work and no failures", status.Categories)
	}

	process := runtimeMetricsCLIProcess

	humanOutput := executeReplayCostsCLI(t, process, environment, server.URL(), false)
	wantCost := expectedPricedReplayCost()
	if !strings.Contains(humanOutput, "Status: PRICED") ||
		!strings.Contains(humanOutput, "Cost (USD): $"+wantCost) ||
		!strings.Contains(humanOutput, "Priced amount (USD): "+wantCost) ||
		!strings.Contains(humanOutput, "Price source: BUILT_IN") ||
		!strings.Contains(humanOutput, "Total tokens: 3000000") {
		t.Fatalf("human replay costs output = %q, want PRICED/$%s/3000000", humanOutput, wantCost)
	}

	jsonOutput := executeReplayCostsCLI(t, process, environment, server.URL(), true)
	var cliReport generatedclient.CostsReport
	if err := json.Unmarshal([]byte(jsonOutput), &cliReport); err != nil {
		t.Fatalf("decode replay costs CLI JSON: %v\nstdout=%s", err, jsonOutput)
	}
	assertPricedReplayCostsReport(t, cliReport)

	apiReport := getReplayCostsHTTP(t, server.URL())
	assertPricedReplayCostsReport(t, apiReport)
	if got, want := apiReport, cliReport; !reportsHaveSameReplayCostFacts(got, want) {
		t.Fatalf("HTTP and CLI replay cost reports differ:\nHTTP=%#v\nCLI=%#v", got, want)
	}

	sessionOutput := executeReplaySessionMetricsCLI(t, process, environment, server.URL(), factorysessions.DefaultSessionID)
	var sessionReport replaySessionMetricsDocument
	if err := json.Unmarshal([]byte(sessionOutput), &sessionReport); err != nil {
		t.Fatalf("decode replay metrics session JSON: %v\nstdout=%s", err, sessionOutput)
	}
	assertReplaySessionMetricsDocument(t, sessionReport)

	cachedRate, reasoningRate := "0.5", "30"
	writeReplayOperatorPriceTable(t, homeDir, operatorsettings.PriceTableModel{
		Provider:                        "codex",
		Model:                           "gpt-5-codex",
		InputPerMillionTokens:           "2",
		OutputPerMillionTokens:          "20",
		CachedInputPerMillionTokens:     &cachedRate,
		ReasoningOutputPerMillionTokens: &reasoningRate,
	})
	overrideHuman := executeReplayCostsCLI(t, process, environment, server.URL(), false)
	if !strings.Contains(overrideHuman, "Status: PRICED") ||
		!strings.Contains(overrideHuman, "Cost (USD): $42.00") ||
		!strings.Contains(overrideHuman, "Priced amount (USD): 42") ||
		!strings.Contains(overrideHuman, "Price source: OPERATOR_SUPPLIED") {
		t.Fatalf("human replay costs output for Codex override = %q, want complete operator override", overrideHuman)
	}
	overrideCLIReport := decodeReplayCostsCLI(t, process, environment, server.URL())
	assertOperatorCodexOverrideReplayCostsReport(t, overrideCLIReport)
	overrideAPIReport := getReplayCostsHTTP(t, server.URL())
	assertOperatorCodexOverrideReplayCostsReport(t, overrideAPIReport)
	if got, want := overrideAPIReport, overrideCLIReport; !reportsHaveSameReplayCostFacts(got, want) {
		t.Fatalf("HTTP and CLI Codex override reports differ:\nHTTP=%#v\nCLI=%#v", got, want)
	}

	if providerRunner.CallCount() != 0 || scriptRunner.CallCount() != 0 {
		t.Fatalf("replay external execution calls = provider:%d script:%d, want zero", providerRunner.CallCount(), scriptRunner.CallCount())
	}
}

type replaySessionMetricsDocument struct {
	FactorySessionID string                  `json:"factory_session_id"`
	Status           string                  `json:"status"`
	DispatchAttempts int                     `json:"dispatch_attempts"`
	Attempts         []replaySessionAttempt  `json:"attempts"`
	Cost             replaySessionCost       `json:"cost"`
	ByWorker         []replaySessionWorker   `json:"by_worker"`
	ByDispatch       []replaySessionDispatch `json:"by_dispatch"`
}

type replaySessionAttempt struct {
	DispatchID      string `json:"dispatch_id"`
	WorkID          string `json:"work_id"`
	WorkerSessionID string `json:"worker_session_id"`
	Outcome         string `json:"outcome"`
}

type replaySessionCost struct {
	Status    string  `json:"status"`
	Currency  string  `json:"currency"`
	KnownCost *string `json:"known_cost"`
	LineItems []struct {
		DispatchID string `json:"dispatch_id"`
	} `json:"line_items"`
}

type replaySessionWorker struct {
	Worker          string `json:"worker"`
	WorkerSessionID string `json:"worker_session_id"`
	Attempts        int    `json:"attempts"`
}

type replaySessionDispatch struct {
	DispatchID string `json:"dispatch_id"`
}

func executeReplaySessionMetricsCLI(
	t *testing.T,
	process support.Process,
	environment []string,
	serverURL string,
	sessionID string,
) string {
	t.Helper()
	args := []string{
		"you", "--json", "--server", serverURL, "metrics", "session", sessionID,
		"--lens", "cost", "--by-worker", "--by-dispatch",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("execute replay metrics session CLI: %v\nstderr=%s", err, inputs.Stderr())
	}
	return inputs.Stdout()
}

func assertReplaySessionMetricsDocument(t *testing.T, report replaySessionMetricsDocument) {
	t.Helper()
	wantCost := expectedPricedReplayCost()
	if report.FactorySessionID != factorysessions.DefaultSessionID || report.Status == "" || report.DispatchAttempts != 1 || len(report.Attempts) != 1 {
		t.Fatalf("replay session summary = %#v, want selected session with one attempt", report)
	}
	if report.Cost.Status != "PRICED" || report.Cost.Currency != "USD" || report.Cost.KnownCost == nil || *report.Cost.KnownCost != wantCost || len(report.Cost.LineItems) != 1 {
		t.Fatalf("replay session cost = %#v, want one PRICED USD line at %s", report.Cost, wantCost)
	}
	attempt := report.Attempts[0]
	if attempt.DispatchID != "cost-replay-priced-dispatch" || attempt.WorkID != "cost-replay-priced-work" ||
		attempt.WorkerSessionID != "cost-replay-priced-worker-session" || attempt.Outcome != "ACCEPTED" {
		t.Fatalf("replay session attempt = %#v, want recorded dispatch/work/Worker Session identity", attempt)
	}
	if len(report.ByWorker) != 1 || report.ByWorker[0].Worker != "priced-worker" ||
		report.ByWorker[0].WorkerSessionID != "cost-replay-priced-worker-session" || report.ByWorker[0].Attempts != 1 {
		t.Fatalf("replay session worker breakdown = %#v, want one canonical worker identity", report.ByWorker)
	}
	if len(report.ByDispatch) != 1 || report.ByDispatch[0].DispatchID != "cost-replay-priced-dispatch" {
		t.Fatalf("replay session dispatch breakdown = %#v, want one canonical dispatch identity", report.ByDispatch)
	}
}

func executeReplayCostsCLI(
	t *testing.T,
	process support.Process,
	environment []string,
	serverURL string,
	jsonOutput bool,
) string {
	t.Helper()
	args := []string{"you", "metrics", "costs", "--server", serverURL}
	if jsonOutput {
		args = []string{"you", "--json", "metrics", "costs", "--server", serverURL}
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("execute replay metrics costs CLI: %v\nstderr=%s", err, inputs.Stderr())
	}
	return inputs.Stdout()
}

func getReplayCostsHTTP(t *testing.T, serverURL string) generatedclient.CostsReport {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, strings.TrimSuffix(serverURL, "/")+"/metrics/costs", nil)
	if err != nil {
		t.Fatalf("build replay costs HTTP request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET /metrics/costs for replay: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics/costs status = %d, want 200", response.StatusCode)
	}
	var report generatedclient.CostsReport
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("decode replay costs HTTP response: %v", err)
	}
	return report
}

func assertPricedReplayCostsReport(t *testing.T, report generatedclient.CostsReport) {
	t.Helper()
	if report.Status != generatedclient.CostsReportStatus("PRICED") || report.Currency != generatedclient.CostsReportCurrency("USD") {
		t.Fatalf("replay cost status/currency = %q/%q, want PRICED/USD", report.Status, report.Currency)
	}
	wantCost := expectedPricedReplayCost()
	if report.KnownCost == nil || *report.KnownCost != wantCost {
		t.Fatalf("replay known cost = %v, want exact %s", report.KnownCost, wantCost)
	}
	if report.UnpricedDispatchCount != 0 || len(report.UnpricedPairs) != 0 {
		t.Fatalf("replay unpriced facts = count:%d pairs:%#v, want zero", report.UnpricedDispatchCount, report.UnpricedPairs)
	}
	if report.Coverage.EncounteredRows != 1 || report.Coverage.PricedRows != 1 || report.Coverage.UnpricedRows != 0 ||
		report.Coverage.EncounteredProviderModels != 1 || report.Coverage.PricedProviderModels != 1 || report.Coverage.UnpricedProviderModels != 0 {
		t.Fatalf("replay cost coverage = %#v, want one priced row and provider/model", report.Coverage)
	}
	assertReplayTokenTotals(t, report.TokenTotals, replayPricedInputTokens, replayPricedOutputTokens, replayPricedTotalTokens)
	if len(report.LineItems) != 1 || len(report.WorkerSessions) != 1 || len(report.ProviderModels) != 1 || len(report.FactorySessions) != 1 {
		t.Fatalf("replay report dimensions = line:%d workers:%d providers:%d sessions:%d, want one each", len(report.LineItems), len(report.WorkerSessions), len(report.ProviderModels), len(report.FactorySessions))
	}
	item := report.LineItems[0]
	if item.Status != generatedclient.CostsLineItemStatus("PRICED") || item.Provider == nil || *item.Provider != "CODEX" || item.Model == nil || *item.Model != "gpt-5-codex" ||
		item.WorkerSessionId == nil || *item.WorkerSessionId != "cost-replay-priced-worker-session" || item.PricedAmount == nil || *item.PricedAmount != wantCost ||
		item.PriceSource == nil || *item.PriceSource != generatedclient.CostsLineItemPriceSource("BUILT_IN") {
		t.Fatalf("replay priced line item = %#v, want Codex identity, Worker Session lineage, and %s", item, wantCost)
	}
	rollup := report.ProviderModels[0]
	if rollup.Key != "CODEX/gpt-5-codex" || rollup.Status != generatedclient.CostsProviderModelRollupStatus("PRICED") || rollup.KnownCost == nil || *rollup.KnownCost != wantCost {
		t.Fatalf("replay provider/model rollup = %#v, want priced CODEX/gpt-5-codex at %s", rollup, wantCost)
	}
	if report.WorkerSessions[0].Key != "cost-replay-priced-worker-session" {
		t.Fatalf("replay Worker Session rollup key = %q, want recorded Worker Session lineage", report.WorkerSessions[0].Key)
	}
}

func assertOperatorCodexOverrideReplayCostsReport(t *testing.T, report generatedclient.CostsReport) {
	t.Helper()
	const wantAmount = "42"
	if report.Status != generatedclient.CostsReportStatus("PRICED") || report.KnownCost == nil || *report.KnownCost != wantAmount ||
		report.PricedSubtotal == nil || *report.PricedSubtotal != wantAmount {
		t.Fatalf("Codex override amounts/status = %q/%v/%v, want PRICED and %s", report.Status, report.KnownCost, report.PricedSubtotal, wantAmount)
	}
	assertReplayTokenTotals(t, report.TokenTotals, replayPricedInputTokens, replayPricedOutputTokens, replayPricedTotalTokens)
	if len(report.LineItems) != 1 {
		t.Fatalf("Codex override line items = %d, want one", len(report.LineItems))
	}
	item := report.LineItems[0]
	if item.Status != generatedclient.CostsLineItemStatus("PRICED") || item.Provider == nil || *item.Provider != "CODEX" ||
		item.Model == nil || *item.Model != "gpt-5-codex" || item.PricedAmount == nil || *item.PricedAmount != wantAmount ||
		item.PriceSource == nil || *item.PriceSource != generatedclient.CostsLineItemPriceSource("OPERATOR_SUPPLIED") {
		t.Fatalf("Codex override line item = %#v, want complete operator-priced row at %s", item, wantAmount)
	}
}

func expectedPricedReplayCost() string {
	// Hand-compute the expected shipped-rate value in cents so the test does not
	// use the production valuation function as its oracle:
	// (1,000,000 × 1.25 + 2,000,000 × 10) / 1,000,000 = 21.25 USD.
	totalCents := (replayPricedInputTokens*replayPricedInputRateCents + replayPricedOutputTokens*replayPricedOutputRateCents) / replayPricedMillionTokenUnit
	return fmt.Sprintf("%d.%02d", totalCents/100, totalCents%100)
}

func assertReplayTokenTotals(t *testing.T, totals generatedclient.CostsTokenTotals, input, output, total int64) {
	t.Helper()
	if totals.InputTokens == nil || *totals.InputTokens != input || totals.OutputTokens == nil || *totals.OutputTokens != output || totals.TotalTokens == nil || *totals.TotalTokens != total {
		t.Fatalf("replay token totals = %#v, want input/output/total %d/%d/%d", totals, input, output, total)
	}
}

func reportsHaveSameReplayCostFacts(left, right generatedclient.CostsReport) bool {
	return reflect.DeepEqual(left, right)
}
