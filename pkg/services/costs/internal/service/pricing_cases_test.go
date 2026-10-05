package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestCostsValuationPricingCases(t *testing.T) {
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
				&priceReader{table: testCase.table},
				operatorSettingsStub(),
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

		})
	}
}

func publicBoundaryPricingCases(
	knownTable providers.PriceTable,
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
		},
		{
			name:                  "none-priced",
			table:                 knownTable,
			rows:                  []factoryvisualization.RuntimeMetricsUsageRow{unknownRow},
			status:                costs.StatusUnpriced,
			tokens:                publicBoundaryTokens(100, 50, 150, 25, 10),
			unpricedDispatchCount: 1,
			unpricedPair:          "CODEX/unpriced",
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
		},
		{
			name:                  "unknown-model",
			table:                 knownTable,
			rows:                  []factoryvisualization.RuntimeMetricsUsageRow{unknownModelRow},
			status:                costs.StatusUnpriced,
			tokens:                publicBoundaryTokens(20, 10, 30, 5, 3),
			unpricedDispatchCount: 1,
			unpricedPair:          "CODEX/unknown-model",
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
		table  providers.PriceTable
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
				&priceReader{table: testCase.table},
				operatorSettingsStub(),
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
	table                 providers.PriceTable
	rows                  []factoryvisualization.RuntimeMetricsUsageRow
	status                costs.Status
	knownCost             string
	tokens                publicBoundaryTokenWant
	unpricedDispatchCount int
	unpricedPair          string
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

func publicBoundaryPriceTable(models ...providers.PriceTableModel) providers.PriceTable {
	return providers.PriceTable{Currency: providers.PriceTableCurrencyUSD, Models: models}
}

func publicBoundaryKnownModel(model string) providers.PriceTableModel {
	cached := "1"
	reasoning := "8"
	return providers.PriceTableModel{
		Provider: "codex", Model: model, InputPerMillionTokens: "2", OutputPerMillionTokens: "4",
		CachedInputPerMillionTokens: &cached, ReasoningOutputPerMillionTokens: &reasoning,
		SourceURL: "https://example.com/test-pricing", AsOfDate: "2026-08-21",
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
