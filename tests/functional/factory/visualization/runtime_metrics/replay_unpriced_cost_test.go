package runtime_metrics_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	replayUnpricedInputTokens  int64 = 1_200
	replayUnpricedOutputTokens int64 = 300
	replayUnpricedTotalTokens  int64 = 1_500
)

// TestReplayOperatorPriceTableIsReversibleInPublicCosts proves operator price-table changes are reversible in public replayed costs.
func TestReplayOperatorPriceTableIsReversibleInPublicCosts(t *testing.T) {
	t.Parallel()
	repoRoot := testutil.MustRepoRoot(t)
	fixturePath := filepath.Join(
		repoRoot,
		"tests", "functional", "factory", "visualization", "runtime_metrics", "testdata",
		"claude-sonnet-4-6.factory-recording.v1.json",
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

	beforeHuman := executeReplayCostsCLI(t, process, environment, server.URL(), false)
	t.Logf("before operator price row — human you metrics costs:\n%s", beforeHuman)
	if !strings.Contains(beforeHuman, "Status: UNPRICED") ||
		!strings.Contains(beforeHuman, "Cost (USD): ?? unknown") ||
		!strings.Contains(beforeHuman, "CLAUDE/claude-sonnet-4-6") ||
		!strings.Contains(beforeHuman, "Reason: no configured price") ||
		!strings.Contains(beforeHuman, "Input tokens: 1200") ||
		!strings.Contains(beforeHuman, "Output tokens: 300") ||
		!strings.Contains(beforeHuman, "Total tokens: 1500") ||
		strings.Contains(beforeHuman, "$0.00") || strings.Contains(beforeHuman, "Status: NO_USAGE") {
		t.Fatalf("human replay costs output before configuration = %q, want truthful UNPRICED Claude usage", beforeHuman)
	}
	beforeCLIReport := decodeReplayCostsCLI(t, process, environment, server.URL())
	assertUnpricedReplayCostsReport(t, beforeCLIReport)
	beforeAPIReport := getReplayCostsHTTP(t, server.URL())
	assertUnpricedReplayCostsReport(t, beforeAPIReport)
	if got, want := beforeAPIReport, beforeCLIReport; !reportsHaveSameReplayCostFacts(got, want) {
		t.Fatalf("HTTP and CLI before-configuration reports differ:\nHTTP=%#v\nCLI=%#v", got, want)
	}

	writeReplayOperatorPriceTable(t, homeDir, operatorsettings.PriceTableModel{
		Provider:               "claude",
		Model:                  "claude-sonnet-4-6",
		InputPerMillionTokens:  "3",
		OutputPerMillionTokens: "15",
	})
	configuredHuman := executeReplayCostsCLI(t, process, environment, server.URL(), false)
	t.Logf("configured operator price row — human you metrics costs:\n%s", configuredHuman)
	if !strings.Contains(configuredHuman, "Status: PRICED") ||
		!strings.Contains(configuredHuman, "Cost (USD): $0.01") ||
		!strings.Contains(configuredHuman, "Priced amount (USD): 0.0081") ||
		!strings.Contains(configuredHuman, "Price source: OPERATOR_SUPPLIED") ||
		!strings.Contains(configuredHuman, "Total tokens: 1500") {
		t.Fatalf("human replay costs output while configured = %q, want exact operator valuation and source", configuredHuman)
	}
	configuredCLIReport := decodeReplayCostsCLI(t, process, environment, server.URL())
	assertOperatorPricedReplayCostsReport(t, configuredCLIReport)
	configuredAPIReport := getReplayCostsHTTP(t, server.URL())
	assertOperatorPricedReplayCostsReport(t, configuredAPIReport)
	if got, want := configuredAPIReport, configuredCLIReport; !reportsHaveSameReplayCostFacts(got, want) {
		t.Fatalf("HTTP and CLI configured reports differ:\nHTTP=%#v\nCLI=%#v", got, want)
	}

	writeReplayOperatorPriceTable(t, homeDir)
	removedHuman := executeReplayCostsCLI(t, process, environment, server.URL(), false)
	t.Logf("removed operator price row — human you metrics costs:\n%s", removedHuman)
	if !strings.Contains(removedHuman, "Status: UNPRICED") ||
		!strings.Contains(removedHuman, "Reason: no configured price") ||
		!strings.Contains(removedHuman, "Input tokens: 1200") ||
		!strings.Contains(removedHuman, "Output tokens: 300") ||
		!strings.Contains(removedHuman, "Total tokens: 1500") ||
		strings.Contains(removedHuman, "Price source: OPERATOR_SUPPLIED") ||
		strings.Contains(removedHuman, "$0.00") {
		t.Fatalf("human replay costs output after removal = %q, want truthful reverted usage", removedHuman)
	}
	removedCLIReport := decodeReplayCostsCLI(t, process, environment, server.URL())
	assertUnpricedReplayCostsReport(t, removedCLIReport)
	removedAPIReport := getReplayCostsHTTP(t, server.URL())
	assertUnpricedReplayCostsReport(t, removedAPIReport)
	if got, want := removedAPIReport, removedCLIReport; !reportsHaveSameReplayCostFacts(got, want) {
		t.Fatalf("HTTP and CLI after-removal reports differ:\nHTTP=%#v\nCLI=%#v", got, want)
	}

	if providerRunner.CallCount() != 0 || scriptRunner.CallCount() != 0 {
		t.Fatalf("replay external execution calls = provider:%d script:%d, want zero", providerRunner.CallCount(), scriptRunner.CallCount())
	}
}

func decodeReplayCostsCLI(
	t *testing.T,
	process support.Process,
	environment []string,
	serverURL string,
) generatedclient.CostsReport {
	t.Helper()
	jsonOutput := executeReplayCostsCLI(t, process, environment, serverURL, true)
	var report generatedclient.CostsReport
	if err := json.Unmarshal([]byte(jsonOutput), &report); err != nil {
		t.Fatalf("decode replay costs CLI JSON: %v\nstdout=%s", err, jsonOutput)
	}
	return report
}

func assertOperatorPricedReplayCostsReport(t *testing.T, report generatedclient.CostsReport) {
	t.Helper()
	const wantAmount = "0.0081"
	if report.Status != generatedclient.CostsReportStatus("PRICED") || report.Currency != generatedclient.CostsReportCurrency("USD") {
		t.Fatalf("configured replay cost status/currency = %q/%q, want PRICED/USD", report.Status, report.Currency)
	}
	if report.KnownCost == nil || *report.KnownCost != wantAmount || report.PricedSubtotal == nil || *report.PricedSubtotal != wantAmount {
		t.Fatalf("configured replay amounts = known:%v subtotal:%v, want exact %s", report.KnownCost, report.PricedSubtotal, wantAmount)
	}
	if report.UnpricedDispatchCount != 0 || len(report.UnpricedPairs) != 0 {
		t.Fatalf("configured replay unpriced facts = dispatches:%d pairs:%#v, want none", report.UnpricedDispatchCount, report.UnpricedPairs)
	}
	if report.Coverage.EncounteredRows != 1 || report.Coverage.PricedRows != 1 || report.Coverage.UnpricedRows != 0 ||
		report.Coverage.EncounteredProviderModels != 1 || report.Coverage.PricedProviderModels != 1 || report.Coverage.UnpricedProviderModels != 0 {
		t.Fatalf("configured replay cost coverage = %#v, want one priced row and provider/model", report.Coverage)
	}
	assertReplayTokenTotals(t, report.TokenTotals, replayUnpricedInputTokens, replayUnpricedOutputTokens, replayUnpricedTotalTokens)
	if len(report.LineItems) != 1 || len(report.WorkItems) != 1 || len(report.WorkerSessions) != 1 || len(report.ProviderModels) != 1 || len(report.FactorySessions) != 1 {
		t.Fatalf("configured replay report dimensions = line:%d work:%d workers:%d providers:%d sessions:%d, want one each", len(report.LineItems), len(report.WorkItems), len(report.WorkerSessions), len(report.ProviderModels), len(report.FactorySessions))
	}
	item := report.LineItems[0]
	if item.Status != generatedclient.CostsLineItemStatus("PRICED") || item.Provider == nil || *item.Provider != "CLAUDE" ||
		item.Model == nil || *item.Model != "claude-sonnet-4-6" || item.WorkerSessionId == nil ||
		*item.WorkerSessionId != "cost-replay-unpriced-worker-session" || item.PricedAmount == nil || *item.PricedAmount != wantAmount ||
		item.PriceSource == nil || *item.PriceSource != generatedclient.CostsLineItemPriceSource("OPERATOR_SUPPLIED") || item.Reason != nil {
		t.Fatalf("configured replay line item = %#v, want priced Claude row with operator source and %s", item, wantAmount)
	}
	if item.InputTokens == nil || *item.InputTokens != replayUnpricedInputTokens || item.OutputTokens == nil || *item.OutputTokens != replayUnpricedOutputTokens {
		t.Fatalf("configured replay line-item tokens = input:%v output:%v, want %d/%d", item.InputTokens, item.OutputTokens, replayUnpricedInputTokens, replayUnpricedOutputTokens)
	}
	rollup := report.ProviderModels[0]
	if rollup.Key != "CLAUDE/claude-sonnet-4-6" || rollup.Status != generatedclient.CostsProviderModelRollupStatus("PRICED") || rollup.KnownCost == nil || *rollup.KnownCost != wantAmount {
		t.Fatalf("configured replay provider/model rollup = %#v, want priced Claude at %s", rollup, wantAmount)
	}
}

func assertUnpricedReplayCostsReport(t *testing.T, report generatedclient.CostsReport) {
	t.Helper()
	if report.Status != generatedclient.CostsReportStatus("UNPRICED") || report.Currency != generatedclient.CostsReportCurrency("USD") {
		t.Fatalf("unpriced replay cost status/currency = %q/%q, want UNPRICED/USD", report.Status, report.Currency)
	}
	if report.KnownCost != nil || report.PricedSubtotal != nil {
		t.Fatalf("unpriced replay amounts = known:%v subtotal:%v, want both absent", report.KnownCost, report.PricedSubtotal)
	}
	if report.UnpricedDispatchCount != 1 || len(report.UnpricedPairs) != 1 {
		t.Fatalf("unpriced replay facts = dispatches:%d pairs:%#v, want one dispatch and one pair", report.UnpricedDispatchCount, report.UnpricedPairs)
	}
	pair := report.UnpricedPairs[0]
	if pair.Provider == nil || *pair.Provider != "CLAUDE" || pair.Model == nil || *pair.Model != "claude-sonnet-4-6" || pair.DispatchCount != 1 {
		t.Fatalf("unpriced replay pair = %#v, want CLAUDE/claude-sonnet-4-6 with one dispatch", pair)
	}
	if report.Coverage.EncounteredRows != 1 || report.Coverage.PricedRows != 0 || report.Coverage.UnpricedRows != 1 ||
		report.Coverage.EncounteredProviderModels != 1 || report.Coverage.PricedProviderModels != 0 || report.Coverage.UnpricedProviderModels != 1 {
		t.Fatalf("unpriced replay cost coverage = %#v, want one unpriced row and provider/model", report.Coverage)
	}
	assertReplayTokenTotals(t, report.TokenTotals, replayUnpricedInputTokens, replayUnpricedOutputTokens, replayUnpricedTotalTokens)
	if len(report.LineItems) != 1 || len(report.WorkItems) != 1 || len(report.WorkerSessions) != 1 || len(report.ProviderModels) != 1 || len(report.FactorySessions) != 1 {
		t.Fatalf("unpriced replay report dimensions = line:%d work:%d workers:%d providers:%d sessions:%d, want one each", len(report.LineItems), len(report.WorkItems), len(report.WorkerSessions), len(report.ProviderModels), len(report.FactorySessions))
	}

	item := report.LineItems[0]
	if item.Status != generatedclient.CostsLineItemStatus("UNPRICED") || item.Provider == nil || *item.Provider != "CLAUDE" ||
		item.Model == nil || *item.Model != "claude-sonnet-4-6" || item.WorkerSessionId == nil ||
		*item.WorkerSessionId != "cost-replay-unpriced-worker-session" || item.PricedAmount != nil {
		t.Fatalf("unpriced replay line item = %#v, want Claude identity, Worker Session lineage, and no amount", item)
	}
	if item.InputTokens == nil || *item.InputTokens != replayUnpricedInputTokens || item.OutputTokens == nil || *item.OutputTokens != replayUnpricedOutputTokens {
		t.Fatalf("unpriced replay line-item tokens = input:%v output:%v, want %d/%d", item.InputTokens, item.OutputTokens, replayUnpricedInputTokens, replayUnpricedOutputTokens)
	}

	rollup := report.ProviderModels[0]
	if rollup.Key != "CLAUDE/claude-sonnet-4-6" || rollup.Status != generatedclient.CostsProviderModelRollupStatus("UNPRICED") || rollup.KnownCost != nil {
		t.Fatalf("unpriced replay provider/model rollup = %#v, want unpriced CLAUDE/claude-sonnet-4-6 with null cost", rollup)
	}
	assertReplayTokenTotals(t, rollup.TokenTotals, replayUnpricedInputTokens, replayUnpricedOutputTokens, replayUnpricedTotalTokens)
	if report.WorkerSessions[0].Key != "cost-replay-unpriced-worker-session" {
		t.Fatalf("unpriced replay Worker Session rollup key = %q, want recorded Worker Session lineage", report.WorkerSessions[0].Key)
	}
}
