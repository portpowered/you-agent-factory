package runtime_metrics_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const (
	endToEndPricedModel   = "gpt-5-codex"
	endToEndUnpricedModel = "mystery-model"
	cachedPricingInput    = int64(10_000)
	cachedPricingCached   = int64(9_984)
	cachedPricingOutput   = int64(100)
)

// TestRuntimeCostsEndToEndFromProviderCompletion proves the corrected public
// path: a real provider command result becomes canonical token metrics, and
// the Costs service values those rows through the public CLI route. The
// unconsumed dispatch.cost/provider.cost samples intentionally remain absent.
func TestRuntimeCostsEndToEndFromProviderCompletion(t *testing.T) {
	functionalevidence.Covers(t, "cli/you.metrics.costs")
	factoryDir := scaffoldEndToEndCostsFactory(t)
	testutil.WriteSeedFile(t, factoryDir, "task", []byte(`{"title":"cost rollup"}`))
	providerRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdoutWithUsage("priced COMPLETE", 1_000_000, 2_000_000),
		},
		platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdoutWithUsage("unpriced COMPLETE", 3, 4),
		},
	)

	var homeDir string
	session, listed, events := support.RunFactoryToCompletionWithConfiguredHome(
		t,
		factoryDir,
		serviceedges.Edges{ProviderCommandRunner: providerRunner},
		30*time.Second,
		func(configuredHome string) {
			homeDir = configuredHome
		},
	)
	if homeDir == "" {
		t.Fatal("configured home directory was not returned")
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress = %+v, want one terminal work item and no failures", session.Runtime.Progress.Categories)
	}
	if len(listed.Results) != 1 {
		t.Fatalf("listed work = %d, want one chained work item", len(listed.Results))
	}
	if providerRunner.CallCount() != 2 {
		t.Fatalf("provider command calls = %d, want priced and unpriced completions", providerRunner.CallCount())
	}
	observations := support.ObserveDispatchEvents(t, events)
	if len(observations) != 2 {
		t.Fatalf("dispatch observations = %d, want two public dispatch records", len(observations))
	}

	usageRecords := readEndToEndUsageRecords(t, homeDir)
	assertEndToEndUsageRecords(t, usageRecords)

	// The default runtime writes its internal "~default" metrics scope while
	// the public session projection has a generated ID. Query the isolated home
	// without a session filter so this proof does not redefine obs-11 scoping.
	report, output := queryCostsReport(t, factoryDir, homeDir, "")
	if report.Status != generatedclient.CostsReportStatus("PARTIAL") {
		t.Fatalf("cost report status = %q, want PARTIAL", report.Status)
	}
	if report.Currency != generatedclient.CostsReportCurrency("USD") {
		t.Fatalf("cost report currency = %q, want USD", report.Currency)
	}
	if report.PricedSubtotal == nil || *report.PricedSubtotal != "21.25" {
		t.Fatalf("priced subtotal = %v, want exact 21.25 USD", report.PricedSubtotal)
	}
	if report.Coverage.EncounteredRows != 2 || report.Coverage.PricedRows != 1 ||
		report.Coverage.UnpricedRows != 1 || report.Coverage.EncounteredProviderModels != 2 ||
		report.Coverage.PricedProviderModels != 1 || report.Coverage.UnpricedProviderModels != 1 {
		t.Fatalf("cost report coverage = %#v, want one priced and one unpriced row/model", report.Coverage)
	}
	assertCostLineItem(t, report, endToEndPricedModel, generatedclient.CostsLineItemStatus("PRICED"), "21.25")
	assertCostLineItem(t, report, endToEndUnpricedModel, generatedclient.CostsLineItemStatus("UNPRICED"), "")
	if !strings.Contains(output, `"status":"UNPRICED"`) || !strings.Contains(output, `"priced_subtotal":"21.25"`) {
		t.Fatalf("public costs JSON omitted priced or unpriced evidence: %s", output)
	}

	encodedRecords, err := json.Marshal(usageRecords)
	if err != nil {
		t.Fatalf("encode captured usage records: %v", err)
	}
	t.Logf("captured canonical provider usage records: %s", encodedRecords)
	t.Logf("captured public costs rollup: %s", strings.TrimSpace(output))
}

// TestRuntimeCostsCachedInputConfigurationMatrix proves one recorded usage
// row is valued from the built-in cached rate, a complete operator replacement,
// an omitted cached rate, and an explicit zero without another provider call.
func TestRuntimeCostsCachedInputConfigurationMatrix(t *testing.T) {
	functionalevidence.Covers(t, "cli/you.metrics.costs")
	factoryDir := support.ScaffoldSingleStepFactory(t, "cached-input-pricing-functional")
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig("codex", endToEndPricedModel))
	testutil.WriteSeedFile(t, factoryDir, "task", []byte(`{"title":"cached cost rollup"}`))
	providerRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdoutWithUsageAndCachedInput(
			"cached COMPLETE", cachedPricingInput, cachedPricingCached, cachedPricingOutput,
		),
	})

	var homeDir string
	session, listed, _ := support.RunFactoryToCompletionWithConfiguredHome(
		t,
		factoryDir,
		serviceedges.Edges{ProviderCommandRunner: providerRunner},
		30*time.Second,
		func(configuredHome string) { homeDir = configuredHome },
	)
	if homeDir == "" {
		t.Fatal("configured home directory was not returned")
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress = %+v, want one terminal work item and no failures", session.Runtime.Progress.Categories)
	}
	if len(listed.Results) != 1 {
		t.Fatalf("listed work = %d, want one completed work item", len(listed.Results))
	}
	if providerRunner.CallCount() != 1 {
		t.Fatalf("provider command calls after recording = %d, want exactly one", providerRunner.CallCount())
	}

	defaultReport, _ := queryCostsReport(t, factoryDir, homeDir, "")
	assertCachedPricingReport(t, defaultReport, "PRICED", "0.002268", "BUILT_IN", "")

	cachedRate := "0.50"
	writeReplayOperatorPriceTable(t, homeDir, operatorsettings.PriceTableModel{
		Provider:                    "codex",
		Model:                       endToEndPricedModel,
		InputPerMillionTokens:       "1.25",
		OutputPerMillionTokens:      "10",
		CachedInputPerMillionTokens: &cachedRate,
	})
	overrideReport, _ := queryCostsReport(t, factoryDir, homeDir, "")
	assertCachedPricingReport(t, overrideReport, "PRICED", "0.006012", "OPERATOR_SUPPLIED", "")
	if providerRunner.CallCount() != 1 {
		t.Fatalf("provider command calls after operator override query = %d, want one recorded call", providerRunner.CallCount())
	}

	writeReplayOperatorPriceTable(t, homeDir, operatorsettings.PriceTableModel{
		Provider:               "codex",
		Model:                  endToEndPricedModel,
		InputPerMillionTokens:  "1.25",
		OutputPerMillionTokens: "10",
	})
	omittedReport, _ := queryCostsReport(t, factoryDir, homeDir, "")
	assertCachedPricingReport(t, omittedReport, "UNPRICED", "", "", "cached-input rate is not configured")

	zeroRate := "0"
	writeReplayOperatorPriceTable(t, homeDir, operatorsettings.PriceTableModel{
		Provider:                    "codex",
		Model:                       endToEndPricedModel,
		InputPerMillionTokens:       "1.25",
		OutputPerMillionTokens:      "10",
		CachedInputPerMillionTokens: &zeroRate,
	})
	zeroReport, _ := queryCostsReport(t, factoryDir, homeDir, "")
	// Cost amounts use the shortest exact decimal representation, so the
	// mathematically exact $0.001020 result is serialized as $0.00102.
	assertCachedPricingReport(t, zeroReport, "PRICED", "0.00102", "OPERATOR_SUPPLIED", "")
	if providerRunner.CallCount() != 1 {
		t.Fatalf("provider command calls after zero-rate query = %d, want one recorded call", providerRunner.CallCount())
	}
}

func assertCachedPricingReport(
	t *testing.T,
	report generatedclient.CostsReport,
	wantStatus generatedclient.CostsReportStatus,
	wantCost string,
	wantSource generatedclient.CostsLineItemPriceSource,
	wantReason string,
) {
	t.Helper()
	if report.Status != wantStatus {
		t.Fatalf("cached cost report status = %q, want %q", report.Status, wantStatus)
	}
	if len(report.LineItems) != 1 {
		t.Fatalf("cached cost line items = %d, want one: %#v", len(report.LineItems), report.LineItems)
	}
	item := report.LineItems[0]
	if item.InputTokens == nil || *item.InputTokens != cachedPricingInput ||
		item.CachedInputTokens == nil || *item.CachedInputTokens != cachedPricingCached ||
		item.OutputTokens == nil || *item.OutputTokens != cachedPricingOutput {
		t.Fatalf("cached cost usage = input:%v cached:%v output:%v, want %d/%d/%d", item.InputTokens, item.CachedInputTokens, item.OutputTokens, cachedPricingInput, cachedPricingCached, cachedPricingOutput)
	}
	if wantCost == "" {
		if report.KnownCost != nil || item.PricedAmount != nil {
			t.Fatalf("unpriced cached cost = report:%v line:%v, want null amounts", report.KnownCost, item.PricedAmount)
		}
		if item.Reason == nil || !strings.Contains(*item.Reason, wantReason) {
			t.Fatalf("unpriced cached cost reason = %v, want %q", item.Reason, wantReason)
		}
		if item.PriceSource != nil {
			t.Fatalf("unpriced cached cost source = %v, want absent", item.PriceSource)
		}
		return
	}
	if report.KnownCost == nil || *report.KnownCost != wantCost || item.PricedAmount == nil || *item.PricedAmount != wantCost {
		t.Fatalf("cached cost amount = report:%v line:%v, want exact %s", stringPointerValue(report.KnownCost), stringPointerValue(item.PricedAmount), wantCost)
	}
	if item.PriceSource == nil || *item.PriceSource != wantSource {
		t.Fatalf("cached cost source = %v, want %q", item.PriceSource, wantSource)
	}
	if item.Reason != nil {
		t.Fatalf("priced cached cost reason = %v, want absent", item.Reason)
	}
}

func stringPointerValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

// TestRuntimeCostsNoUsageThroughPublicCLI proves an empty metrics root is a
// successful, explicit report rather than an empty stdout or command error.
func TestRuntimeCostsNoUsageThroughPublicCLI(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.MkdirAll(platformmetrics.RuntimeMetricsRoot(homeDir), 0o755); err != nil {
		t.Fatalf("create empty runtime metrics root: %v", err)
	}
	factoryDir := support.ScaffoldSingleStepFactory(t, "costs-no-usage-functional")

	report, output := queryCostsReport(t, factoryDir, homeDir, "")
	if report.Status != generatedclient.CostsReportStatus("NO_USAGE") {
		t.Fatalf("empty cost report status = %q, want NO_USAGE", report.Status)
	}
	if report.PricedSubtotal != nil || report.Coverage.EncounteredRows != 0 || len(report.LineItems) != 0 {
		t.Fatalf("empty cost report = %#v, want no amount or usage rows", report)
	}
	if !strings.Contains(output, `"status":"NO_USAGE"`) {
		t.Fatalf("empty costs output = %q, want explicit NO_USAGE state", output)
	}
}

func scaffoldEndToEndCostsFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "costs-provider-functional",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "priced", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{
			{"name": "priced-worker"},
			{"name": "unpriced-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "price-usage",
				"worker":    "priced-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "priced"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":      "leave-unpriced",
				"worker":    "unpriced-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "priced"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	})
	support.WriteAgentConfig(t, dir, "priced-worker", support.BuildModelWorkerConfig("codex", endToEndPricedModel))
	support.WriteAgentConfig(t, dir, "unpriced-worker", support.BuildModelWorkerConfig("codex", endToEndUnpricedModel))
	return dir
}

func readEndToEndUsageRecords(
	t *testing.T,
	homeDir string,
) []platformmetrics.RuntimeMetricRecord {
	t.Helper()
	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct runtime metrics reader: %v", err)
	}
	records, err := reader.Read(context.Background(), platformmetrics.RuntimeMetricsRoot(homeDir))
	if err != nil {
		t.Fatalf("read runtime metrics: %v", err)
	}
	usage := make([]platformmetrics.RuntimeMetricRecord, 0, 4)
	for _, record := range records {
		name, _ := record["metric_name"].(string)
		if name == "provider.input_tokens" || name == "provider.output_tokens" {
			usage = append(usage, record)
		}
	}
	return usage
}

func assertEndToEndUsageRecords(t *testing.T, records []platformmetrics.RuntimeMetricRecord) {
	t.Helper()
	if len(records) != 4 {
		t.Fatalf("captured usage records = %d, want four input/output records: %#v", len(records), records)
	}
	wantValues := map[string]float64{
		endToEndPricedModel + ":provider.input_tokens":    1_000_000,
		endToEndPricedModel + ":provider.output_tokens":   2_000_000,
		endToEndUnpricedModel + ":provider.input_tokens":  3,
		endToEndUnpricedModel + ":provider.output_tokens": 4,
	}
	for _, record := range records {
		model, _ := record["model"].(string)
		name, _ := record["metric_name"].(string)
		key := strings.TrimSpace(model) + ":" + strings.TrimSpace(name)
		value, ok := record["value"].(float64)
		if !ok {
			t.Fatalf("usage record value = %#v, want number: %#v", record["value"], record)
		}
		want, ok := wantValues[key]
		if !ok || want != value {
			t.Fatalf("usage record %q value = %v, want %v: %#v", key, value, want, record)
		}
		for _, field := range []string{"provider", "work_id", "dispatch_id", "worker_session_id"} {
			if value, _ := record[field].(string); strings.TrimSpace(value) == "" {
				t.Fatalf("usage record field %q is empty: %#v", field, record)
			}
		}
	}
	if len(wantValues) != len(records) {
		t.Fatalf("usage records did not contain every expected model/token pair: %#v", records)
	}
}

func queryCostsReport(
	t *testing.T,
	factoryDir string,
	homeDir string,
	sessionID string,
) (generatedclient.CostsReport, string) {
	t.Helper()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
	})
	defer server.Stop(t)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	args := []string{"you", "--json", "--server", server.URL(), "metrics", "costs"}
	if strings.TrimSpace(sessionID) != "" {
		args = append(args, "--session", sessionID)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Env = environment
	inputs.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("execute public metrics costs CLI: %v\nstderr=%s", err, inputs.Stderr())
	}
	var report generatedclient.CostsReport
	if err := json.Unmarshal([]byte(inputs.Stdout()), &report); err != nil {
		t.Fatalf("decode public metrics costs JSON: %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	return report, inputs.Stdout()
}

func assertCostLineItem(
	t *testing.T,
	report generatedclient.CostsReport,
	model string,
	wantStatus generatedclient.CostsLineItemStatus,
	wantAmount string,
) {
	t.Helper()
	for _, item := range report.LineItems {
		if item.Model == nil || *item.Model != model {
			continue
		}
		if item.Status != wantStatus {
			t.Fatalf("%s line-item status = %q, want %q", model, item.Status, wantStatus)
		}
		if wantAmount == "" {
			if item.PricedAmount != nil {
				t.Fatalf("%s line-item priced amount = %q, want absent", model, *item.PricedAmount)
			}
			if item.Reason == nil || !strings.Contains(*item.Reason, "no configured price") {
				t.Fatalf("%s line-item reason = %v, want missing-price diagnostic", model, item.Reason)
			}
			return
		}
		if item.PricedAmount == nil || *item.PricedAmount != wantAmount {
			t.Fatalf("%s line-item priced amount = %v, want %q", model, item.PricedAmount, wantAmount)
		}
		return
	}
	t.Fatalf("cost report has no line item for model %q: %#v", model, report.LineItems)
}
