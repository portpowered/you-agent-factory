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
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	replayUnpricedInputTokens  int64 = 1_200
	replayUnpricedOutputTokens int64 = 300
	replayUnpricedTotalTokens  int64 = 1_500
)

func TestReplayUnpricedUsageRemainsTruthfulInPublicCosts(t *testing.T) {
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

	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
		ScriptCommandRunner:   scriptRunner,
	})
	support.CleanupProcess(t, process)

	humanOutput := executeReplayCostsCLI(t, process, environment, server.URL(), false)
	if !strings.Contains(humanOutput, "Status: UNPRICED") ||
		!strings.Contains(humanOutput, "Cost (USD): ?? unknown") ||
		!strings.Contains(humanOutput, "CLAUDE/claude-sonnet-4-6") ||
		!strings.Contains(humanOutput, "Total tokens: 1500") ||
		strings.Contains(humanOutput, "$0.00") || strings.Contains(humanOutput, "Status: NO_USAGE") {
		t.Fatalf("human replay costs output = %q, want truthful UNPRICED Claude usage", humanOutput)
	}

	jsonOutput := executeReplayCostsCLI(t, process, environment, server.URL(), true)
	var cliReport generatedclient.CostsReport
	if err := json.Unmarshal([]byte(jsonOutput), &cliReport); err != nil {
		t.Fatalf("decode unpriced replay costs CLI JSON: %v\nstdout=%s", err, jsonOutput)
	}
	assertUnpricedReplayCostsReport(t, cliReport)

	apiReport := getReplayCostsHTTP(t, server.URL())
	assertUnpricedReplayCostsReport(t, apiReport)
	if got, want := apiReport, cliReport; !reportsHaveSameReplayCostFacts(got, want) {
		t.Fatalf("HTTP and CLI unpriced replay cost reports differ:\nHTTP=%#v\nCLI=%#v", got, want)
	}

	if providerRunner.CallCount() != 0 || scriptRunner.CallCount() != 0 {
		t.Fatalf("replay external execution calls = provider:%d script:%d, want zero", providerRunner.CallCount(), scriptRunner.CallCount())
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
