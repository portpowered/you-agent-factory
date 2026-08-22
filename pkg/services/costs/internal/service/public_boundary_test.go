package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	costscli "github.com/portpowered/infinite-you/pkg/services/costs/transports/cli"
	costshttp "github.com/portpowered/infinite-you/pkg/services/costs/transports/http"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestPublicCostsBoundaryCoversPricingCases(t *testing.T) {
	t.Parallel()

	knownTable := publicBoundaryPriceTable(publicBoundaryKnownModel("known"))
	knownRow := usageRow("session-a", "work-a", "dispatch-a", "worker-a", "codex", "known", 1_000_000, 2_000_000, int64Ptr(250_000), int64Ptr(500_000))
	unknownRow := usageRow("session-b", "work-b", "dispatch-b", "worker-b", "codex", "unpriced", 100, 50, int64Ptr(25), int64Ptr(10))
	unknownModelRow := usageRow("session-c", "work-c", "dispatch-c", "worker-c", "codex", "unknown-model", 20, 10, int64Ptr(5), int64Ptr(3))
	cases := publicBoundaryPricingCases(knownTable, knownRow, unknownRow, unknownModelRow)

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			query, err := New(
				&settingsReader{document: operatorsettings.Document{PriceTable: testCase.table}},
				metricsQueryStub(testCase.rows, nil),
				logging.NoopLogger{},
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			report, err := query.Query(context.Background(), validRequest())
			if err != nil {
				t.Fatalf("Costs query error = %v", err)
			}
			assertPublicDomainReport(t, report, testCase)

			apiReport, responseBody := publicBoundaryAPIReport(t, query)
			assertPublicAPIReport(t, apiReport, testCase)
			assertPublicJSONShape(t, responseBody, testCase.knownCost != "")
			assertPublicCLIOutputs(t, apiReport, testCase)
		})
	}
}

func assertPublicCLIOutputs(t *testing.T, report factoryapi.CostsReport, testCase publicBoundaryCase) {
	t.Helper()
	humanOutput := runPublicBoundaryCLI(t, report, false)
	for _, want := range testCase.human {
		if !strings.Contains(humanOutput, want) {
			t.Fatalf("human output missing %q:\n%s", want, humanOutput)
		}
	}
	for _, unwanted := range testCase.noHuman {
		if strings.Contains(humanOutput, unwanted) {
			t.Fatalf("human output contains %q:\n%s", unwanted, humanOutput)
		}
	}

	jsonOutput := runPublicBoundaryCLI(t, report, true)
	var cliReport factoryapi.CostsReport
	if err := json.Unmarshal([]byte(jsonOutput), &cliReport); err != nil {
		t.Fatalf("decode --json output: %v\n%s", err, jsonOutput)
	}
	if cliReport.Status != report.Status || cliReport.KnownCost == nil && report.KnownCost != nil || cliReport.KnownCost != nil && report.KnownCost == nil {
		t.Fatalf("CLI JSON report = %#v, want API report status/cost %#v/%#v", cliReport, report.Status, report.KnownCost)
	}
	if testCase.knownCost != "" && cliReport.KnownCost != nil && *cliReport.KnownCost != testCase.knownCost {
		t.Fatalf("CLI JSON known cost = %q, want %q", *cliReport.KnownCost, testCase.knownCost)
	}
}

func publicBoundaryPricingCases(
	knownTable operatorsettings.PriceTable,
	knownRow, unknownRow, unknownModelRow factoryvisualization.RuntimeMetricsUsageRow,
) []publicBoundaryCase {
	return []publicBoundaryCase{
		{
			name:      "all-priced",
			table:     knownTable,
			rows:      []factoryvisualization.RuntimeMetricsUsageRow{knownRow},
			status:    costs.StatusPriced,
			knownCost: "11.75",
			tokens:    publicBoundaryTokens(1_000_000, 2_000_000, 3_000_000, 250_000, 500_000),
			human:     []string{"Status: PRICED", "Cost (USD): $11.75", "Total tokens: 3000000"},
			noHuman:   []string{"?? unknown"},
		},
		{
			name:                  "none-priced",
			table:                 knownTable,
			rows:                  []factoryvisualization.RuntimeMetricsUsageRow{unknownRow},
			status:                costs.StatusUnpriced,
			tokens:                publicBoundaryTokens(100, 50, 150, 25, 10),
			unpricedDispatchCount: 1,
			unpricedPair:          "CODEX/unpriced",
			human:                 []string{"Status: UNPRICED", "Cost (USD): ?? unknown", "Unpriced dispatches: 1", "CODEX/unpriced: 1 dispatches", "Total tokens: 150"},
			noHuman:               []string{"$0.00"},
		},
		{
			name:                  "mixed",
			table:                 knownTable,
			rows:                  []factoryvisualization.RuntimeMetricsUsageRow{knownRow, unknownRow},
			status:                costs.StatusPartial,
			knownCost:             "11.75",
			tokens:                publicBoundaryTokens(1_000_100, 2_000_050, 3_000_150, 250_025, 500_010),
			unpricedDispatchCount: 1,
			unpricedPair:          "CODEX/unpriced",
			human:                 []string{"Status: PARTIAL", "Cost (USD): $11.75 + ?? unknown", "Unpriced dispatches: 1", "CODEX/unpriced: 1 dispatches", "Total tokens: 3000150"},
		},
		{
			name:                  "unknown-model",
			table:                 knownTable,
			rows:                  []factoryvisualization.RuntimeMetricsUsageRow{unknownModelRow},
			status:                costs.StatusUnpriced,
			tokens:                publicBoundaryTokens(20, 10, 30, 5, 3),
			unpricedDispatchCount: 1,
			unpricedPair:          "CODEX/unknown-model",
			human:                 []string{"Status: UNPRICED", "Cost (USD): ?? unknown", "CODEX/unknown-model: 1 dispatches", "Total tokens: 30"},
			noHuman:               []string{"$0.00"},
		},
	}
}

func TestPublicCostsTokenTotalsIgnorePriceCoverage(t *testing.T) {
	t.Parallel()

	rows := []factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session", "work-a", "dispatch-a", "worker-a", "codex", "known", 1_000_000, 2_000_000, int64Ptr(250_000), int64Ptr(500_000)),
		usageRow("session", "work-b", "dispatch-b", "worker-b", "codex", "unpriced", 100, 50, int64Ptr(25), int64Ptr(10)),
	}
	cases := []struct {
		name   string
		table  operatorsettings.PriceTable
		status costs.Status
	}{
		{name: "full", table: publicBoundaryPriceTable(publicBoundaryKnownModel("known"), publicBoundaryKnownModel("unpriced")), status: costs.StatusPriced},
		{name: "partial", table: publicBoundaryPriceTable(publicBoundaryKnownModel("known")), status: costs.StatusPartial},
		{name: "empty", table: publicBoundaryPriceTable(), status: costs.StatusUnpriced},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			query, err := New(
				&settingsReader{document: operatorsettings.Document{PriceTable: testCase.table}},
				metricsQueryStub(rows, nil),
				logging.NoopLogger{},
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			report, err := query.Query(context.Background(), validRequest())
			if err != nil {
				t.Fatalf("Costs query error = %v", err)
			}
			if report.Status != testCase.status {
				t.Fatalf("status = %q, want %q", report.Status, testCase.status)
			}
			assertTokenTotals(t, report.TokenTotals, 1_000_100, 2_000_050, 3_000_150)
			if report.TokenTotals.CachedInputTokens == nil || *report.TokenTotals.CachedInputTokens != 250_025 || report.TokenTotals.ReasoningOutputTokens == nil || *report.TokenTotals.ReasoningOutputTokens != 500_010 {
				t.Fatalf("subclass token totals = %#v, want cached 250025 and reasoning 500010", report.TokenTotals)
			}
		})
	}
}

type publicBoundaryCase struct {
	name                  string
	table                 operatorsettings.PriceTable
	rows                  []factoryvisualization.RuntimeMetricsUsageRow
	status                costs.Status
	knownCost             string
	tokens                publicBoundaryTokenWant
	unpricedDispatchCount int
	unpricedPair          string
	human                 []string
	noHuman               []string
}

type publicBoundaryTokenWant struct {
	input, output, total, cached, reasoning int64
}

func assertPublicDomainReport(t *testing.T, report costs.Report, testCase publicBoundaryCase) {
	t.Helper()
	if report.Status != testCase.status {
		t.Fatalf("domain status = %q, want %q", report.Status, testCase.status)
	}
	if testCase.knownCost == "" {
		if report.KnownCost != nil {
			t.Fatalf("domain known cost = %q, want absent", *report.KnownCost)
		}
	} else if report.KnownCost == nil || *report.KnownCost != testCase.knownCost {
		t.Fatalf("domain known cost = %v, want %q", report.KnownCost, testCase.knownCost)
	}
	assertTokenTotals(t, report.TokenTotals, testCase.tokens.input, testCase.tokens.output, testCase.tokens.total)
	if report.TokenTotals.CachedInputTokens == nil || *report.TokenTotals.CachedInputTokens != testCase.tokens.cached || report.TokenTotals.ReasoningOutputTokens == nil || *report.TokenTotals.ReasoningOutputTokens != testCase.tokens.reasoning {
		t.Fatalf("domain subclass token totals = %#v, want %#v", report.TokenTotals, testCase.tokens)
	}
	if report.UnpricedDispatchCount != testCase.unpricedDispatchCount {
		t.Fatalf("domain unpriced dispatch count = %d, want %d", report.UnpricedDispatchCount, testCase.unpricedDispatchCount)
	}
	assertDomainUnpricedPair(t, report.UnpricedPairs, testCase.unpricedPair)
}

func assertPublicAPIReport(t *testing.T, report factoryapi.CostsReport, testCase publicBoundaryCase) {
	t.Helper()
	if report.Status != factoryapi.CostsReportStatus(testCase.status) || report.Currency != "USD" {
		t.Fatalf("API status/currency = %q/%q, want %q/USD", report.Status, report.Currency, testCase.status)
	}
	if testCase.knownCost == "" {
		if report.KnownCost != nil {
			t.Fatalf("API known cost = %q, want absent", *report.KnownCost)
		}
	} else if report.KnownCost == nil || *report.KnownCost != testCase.knownCost {
		t.Fatalf("API known cost = %v, want %q", report.KnownCost, testCase.knownCost)
	}
	assertPublicTokenTotals(t, report.TokenTotals, testCase.tokens)
	if report.UnpricedDispatchCount != testCase.unpricedDispatchCount {
		t.Fatalf("API unpriced dispatch count = %d, want %d", report.UnpricedDispatchCount, testCase.unpricedDispatchCount)
	}
	assertPublicUnpricedPair(t, report.UnpricedPairs, testCase.unpricedPair)
	if len(report.LineItems) != len(testCase.rows) {
		t.Fatalf("API line items = %d, want %d", len(report.LineItems), len(testCase.rows))
	}
	for _, rollup := range report.WorkItems {
		assertPublicTokenTotalsPresent(t, rollup.TokenTotals)
	}
	for _, rollup := range report.WorkerSessions {
		assertPublicTokenTotalsPresent(t, rollup.TokenTotals)
	}
	for _, rollup := range report.FactorySessions {
		assertPublicTokenTotalsPresent(t, rollup.TokenTotals)
	}
	for _, rollup := range report.ProviderModels {
		assertPublicTokenTotalsPresent(t, rollup.TokenTotals)
	}
}

func assertPublicTokenTotals(t *testing.T, totals factoryapi.CostsTokenTotals, want publicBoundaryTokenWant) {
	t.Helper()
	values := []struct {
		name string
		got  *int64
		want int64
	}{
		{name: "total", got: totals.TotalTokens, want: want.total},
		{name: "input", got: totals.InputTokens, want: want.input},
		{name: "output", got: totals.OutputTokens, want: want.output},
		{name: "cached-input", got: totals.CachedInputTokens, want: want.cached},
		{name: "reasoning-output", got: totals.ReasoningOutputTokens, want: want.reasoning},
	}
	for _, value := range values {
		if value.got == nil || *value.got != value.want {
			t.Fatalf("API %s token total = %v, want %d", value.name, value.got, value.want)
		}
	}
}

func assertPublicTokenTotalsPresent(t *testing.T, totals factoryapi.CostsTokenTotals) {
	t.Helper()
	if totals.TotalTokens == nil || totals.InputTokens == nil || totals.OutputTokens == nil || totals.CachedInputTokens == nil || totals.ReasoningOutputTokens == nil {
		t.Fatalf("API rollup token totals = %#v, want every token class present", totals)
	}
	if *totals.TotalTokens != *totals.InputTokens+*totals.OutputTokens {
		t.Fatalf("API rollup total tokens = %d, want input plus output %d", *totals.TotalTokens, *totals.InputTokens+*totals.OutputTokens)
	}
}

func assertPublicJSONShape(t *testing.T, body string, wantKnownCost bool) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatalf("decode public Costs JSON: %v\n%s", err, body)
	}
	for _, field := range []string{"status", "currency", "known_cost", "token_totals", "unpriced_dispatch_count", "unpriced_pairs"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("public Costs JSON missing %q: %s", field, body)
		}
	}
	if wantKnownCost && string(fields["known_cost"]) != `"11.75"` {
		t.Fatalf("public Costs JSON known_cost = %s, want exact 11.75", fields["known_cost"])
	}
	if !wantKnownCost && string(fields["known_cost"]) != "null" {
		t.Fatalf("public Costs JSON known_cost = %s, want null", fields["known_cost"])
	}
	var tokenFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["token_totals"], &tokenFields); err != nil {
		t.Fatalf("decode public token_totals: %v", err)
	}
	for _, field := range []string{"total_tokens", "input_tokens", "output_tokens", "cached_input_tokens", "reasoning_output_tokens"} {
		if _, ok := tokenFields[field]; !ok {
			t.Fatalf("public token_totals missing %q: %s", field, fields["token_totals"])
		}
	}
}

func publicBoundaryAPIReport(t *testing.T, query costs.CostsQuery) (factoryapi.CostsReport, string) {
	t.Helper()
	handler := costshttp.NewHandler(costshttp.NewAdapter(query, "metrics-root", "settings.json"), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.GetMetricsCosts(
		recorder,
		httptest.NewRequest(http.MethodGet, "/metrics/costs", nil),
		factoryapi.GetMetricsCostsParams{},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("public Costs HTTP status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var report factoryapi.CostsReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode public Costs HTTP JSON: %v", err)
	}
	return report, recorder.Body.String()
}

func runPublicBoundaryCLI(t *testing.T, report factoryapi.CostsReport, jsonOutput bool) string {
	t.Helper()
	clientReport := publicBoundaryClientReport(t, report)
	command := costscli.NewCostsCommand(costscli.CostsCommandConfig{
		Operation: costscli.NewOperation(func(string) (costscli.Client, error) {
			return &publicBoundaryClient{report: clientReport}, nil
		}),
		Server: func() string { return "https://factory.example" },
		JSON:   func() bool { return jsonOutput },
	})
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(io.Discard)
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute public Costs CLI: %v", err)
	}
	return output.String()
}

type publicBoundaryClient struct {
	report generatedclient.CostsReport
}

func (client *publicBoundaryClient) GetMetricsCostsWithResponse(
	context.Context,
	*generatedclient.GetMetricsCostsParams,
	...generatedclient.RequestEditorFn,
) (*generatedclient.GetMetricsCostsClientResponse, error) {
	return &generatedclient.GetMetricsCostsClientResponse{JSON200: &client.report}, nil
}

func publicBoundaryClientReport(t *testing.T, report factoryapi.CostsReport) generatedclient.CostsReport {
	t.Helper()
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("encode public API report for client: %v", err)
	}
	var clientReport generatedclient.CostsReport
	if err := json.Unmarshal(encoded, &clientReport); err != nil {
		t.Fatalf("decode public API report for client: %v", err)
	}
	return clientReport
}

func publicBoundaryPriceTable(models ...operatorsettings.PriceTableModel) operatorsettings.PriceTable {
	return operatorsettings.PriceTable{Currency: operatorsettings.PriceTableCurrencyUSD, Models: models}
}

func publicBoundaryKnownModel(model string) operatorsettings.PriceTableModel {
	cached := "1"
	reasoning := "8"
	return operatorsettings.PriceTableModel{
		Provider: "codex", Model: model, InputPerMillionTokens: "2", OutputPerMillionTokens: "4",
		CachedInputPerMillionTokens: &cached, ReasoningOutputPerMillionTokens: &reasoning,
	}
}

func publicBoundaryTokens(input, output, total, cached, reasoning int64) publicBoundaryTokenWant {
	return publicBoundaryTokenWant{input: input, output: output, total: total, cached: cached, reasoning: reasoning}
}

func assertDomainUnpricedPair(t *testing.T, pairs []costs.UnpricedPair, want string) {
	t.Helper()
	if want == "" {
		if len(pairs) != 0 {
			t.Fatalf("domain unpriced pairs = %#v, want none", pairs)
		}
		return
	}
	if len(pairs) != 1 || pairs[0].Provider == nil || pairs[0].Model == nil || *pairs[0].Provider+"/"+*pairs[0].Model != want || pairs[0].DispatchCount != 1 {
		t.Fatalf("domain unpriced pairs = %#v, want one %s pair", pairs, want)
	}
}

func assertPublicUnpricedPair(t *testing.T, pairs []factoryapi.CostsUnpricedPair, want string) {
	t.Helper()
	if want == "" {
		if len(pairs) != 0 {
			t.Fatalf("API unpriced pairs = %#v, want none", pairs)
		}
		return
	}
	if len(pairs) != 1 || pairs[0].Provider == nil || pairs[0].Model == nil || *pairs[0].Provider+"/"+*pairs[0].Model != want || pairs[0].DispatchCount != 1 {
		t.Fatalf("API unpriced pairs = %#v, want one %s pair", pairs, want)
	}
}
