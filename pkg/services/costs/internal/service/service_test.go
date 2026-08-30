package service

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestQueryExactValuationCoverageAndRollups(t *testing.T) {
	t.Parallel()

	pricing := &priceReader{table: providers.PriceTable{
		Currency: providers.PriceTableCurrencyUSD,
		Models: []providers.PriceTableModel{
			testPriceModel("gpt-5", "2.5", "5.25", stringPtr("0.5"), stringPtr("10")),
			testPriceModel("gpt-zero", "0", "0", nil, nil),
			testPriceModel("gpt-no-cache", "1", "1", nil, nil),
		},
	}}
	rows := []factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session-b", "work-b", "dispatch-b", "worker-b", "codex", "gpt-5", 2_000_000, 1_000_000, int64Ptr(500_000), int64Ptr(200_000)),
		usageRow("session-a", "work-a", "dispatch-a", "worker-a", "codex", "gpt-zero", 3, 4, nil, nil),
		usageRow("session-c", "work-c", "dispatch-c", "worker-c", "codex", "unknown", 10, 5, nil, nil),
		usageRow("session-a", "work-a2", "dispatch-a2", "worker-a2", "codex", "gpt-no-cache", 1_000, 1_000, int64Ptr(500), nil),
		usageRow("session-a", "work-a3", "dispatch-a3", "worker-a3", "codex", "", 8, 9, nil, nil),
	}
	query, err := New(pricing, operatorSettingsStub(), metricsQueryStub(rows, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := query.Query(context.Background(), costs.QueryRequest{
		MetricsRoot: "metrics-root", OperatorSettingsPath: "settings.json",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	assertExactReport(t, got)
}

func TestQueryCostsUsesRetainedFactorySessionIDsAsAuthoritativeFilter(t *testing.T) {
	t.Parallel()

	pricing := &priceReader{table: providers.PriceTable{
		Currency: providers.PriceTableCurrencyUSD,
		Models:   []providers.PriceTableModel{testPriceModel("known", "1", "1", nil, nil)},
	}}
	query, err := New(pricing, operatorSettingsStub(), metricsQueryStub([]factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("canonical-live-id", "work-live", "dispatch-live", "worker-live", "codex", "known", 2, 3, nil, nil),
		usageRow("~default", "work-old", "dispatch-old", "worker-old", "codex", "known", 20, 30, nil, nil),
		usageRow("foreign-id", "work-foreign", "dispatch-foreign", "worker-foreign", "codex", "known", 200, 300, nil, nil),
	}, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := query.Query(context.Background(), costs.QueryRequest{
		MetricsRoot:               "metrics-root",
		OperatorSettingsPath:      "settings.json",
		FactorySessionID:          "~default",
		RetainedFactorySessionIDs: []string{" canonical-live-id ", "canonical-live-id"},
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if got.Scope.FactorySessionID != "~default" || got.Status != costs.StatusPriced || got.KnownCost == nil || *got.KnownCost != "0.000005" {
		t.Fatalf("scoped report = %#v, want one priced canonical row under requested alias", got)
	}
	if got.Coverage.EncounteredRows != 1 || len(got.LineItems) != 1 || got.LineItems[0].DispatchID != "dispatch-live" {
		t.Fatalf("scoped report rows = coverage %#v, lines %#v, want only canonical-live-id", got.Coverage, got.LineItems)
	}
}

func assertExactReport(t *testing.T, report costs.Report) {
	t.Helper()
	if report.Scope.Kind != costs.ScopeAllFactorySessions || report.Currency != "USD" {
		t.Fatalf("scope/currency = %#v/%q, want all/USD", report.Scope, report.Currency)
	}
	if report.Status != costs.StatusPartial {
		t.Fatalf("status = %q, want PARTIAL", report.Status)
	}
	if report.PricedSubtotal == nil || *report.PricedSubtotal != "10.2" {
		t.Fatalf("priced subtotal = %v, want exact 10.2", report.PricedSubtotal)
	}
	if report.KnownCost == nil || *report.KnownCost != "10.2" {
		t.Fatalf("known cost = %v, want exact 10.2", report.KnownCost)
	}
	assertTokenTotals(t, report.TokenTotals, 2_001_021, 1_001_018, 3_002_039)
	if report.TokenTotals.CachedInputTokens == nil || *report.TokenTotals.CachedInputTokens != 500_500 || report.TokenTotals.ReasoningOutputTokens == nil || *report.TokenTotals.ReasoningOutputTokens != 200_000 {
		t.Fatalf("subclass token totals = %#v, want cached 500500 and reasoning 200000", report.TokenTotals)
	}
	if report.UnpricedDispatchCount != 3 {
		t.Fatalf("unpriced dispatch count = %d, want 3", report.UnpricedDispatchCount)
	}
	assertUnpricedPairs(t, report.UnpricedPairs, []unpricedPairWant{
		{model: nil, dispatchCount: 1},
		{model: stringPtr("gpt-no-cache"), dispatchCount: 1},
		{model: stringPtr("unknown"), dispatchCount: 1},
	})
	wantCoverage := costs.Coverage{
		EncounteredRows: 5, PricedRows: 2, UnpricedRows: 3,
		EncounteredProviderModels: 4, PricedProviderModels: 2, UnpricedProviderModels: 2,
	}
	if !reflect.DeepEqual(report.Coverage, wantCoverage) {
		t.Fatalf("coverage = %#v, want %#v", report.Coverage, wantCoverage)
	}
	assertExactLineItems(t, report.LineItems)
	assertExactRollups(t, report)
}

func assertExactLineItems(t *testing.T, lines []costs.LineItem) {
	t.Helper()
	if len(lines) != 5 || lines[0].Model != "gpt-zero" || lines[1].Model != "gpt-no-cache" || lines[2].Model != "" || lines[3].Model != "gpt-5" {
		t.Fatalf("line-item ordering = %#v, want stable identity order", lines)
	}
	assertLine(t, lines, "gpt-5", costs.StatusPriced, "10.2", "")
	assertLine(t, lines, "gpt-zero", costs.StatusPriced, "0", "")
	assertLine(t, lines, "unknown", costs.StatusUnpriced, "", "no configured price")
	assertLine(t, lines, "gpt-no-cache", costs.StatusUnpriced, "", "cached-input rate is not configured")
	assertLine(t, lines, "", costs.StatusUnpriced, "", "model identity is unavailable")
	assertLineSource(t, lines, "gpt-5", costs.StatusPriced, costs.PriceSourceBuiltIn)
	assertLineSource(t, lines, "gpt-zero", costs.StatusPriced, costs.PriceSourceBuiltIn)
	assertLineSource(t, lines, "unknown", costs.StatusUnpriced, "")
	assertLineSource(t, lines, "gpt-no-cache", costs.StatusUnpriced, "")
	assertLineSource(t, lines, "", costs.StatusUnpriced, "")
}

func assertExactRollups(t *testing.T, report costs.Report) {
	t.Helper()
	if len(report.WorkItems) != 5 || report.WorkItems[0].Key != "work-a" || report.WorkItems[4].Key != "work-c" {
		t.Fatalf("work rollups = %#v, want sorted five keys", report.WorkItems)
	}
	if len(report.ProviderModels) != 4 || report.ProviderModels[0].Model != "gpt-5" {
		t.Fatalf("provider/model rollups = %#v, want four sorted pairs", report.ProviderModels)
	}
	if report.ProviderModels[0].Key != "CODEX/gpt-5" || strings.Contains(report.ProviderModels[0].Key, "\x00") {
		t.Fatalf("provider/model rollup key = %q, want public CODEX/gpt-5", report.ProviderModels[0].Key)
	}
	if len(report.FactorySessions) != 3 {
		t.Fatalf("Factory Session rollups = %#v, want three sessions", report.FactorySessions)
	}
	for _, rollup := range report.FactorySessions {
		assertFactorySessionRollup(t, rollup)
	}
	for _, rollup := range report.WorkItems {
		if rollup.Currency != "USD" {
			t.Fatalf("work rollup currency = %q, want USD", rollup.Currency)
		}
		if rollup.TokenTotals.TotalTokens == nil {
			t.Fatalf("work rollup %#v has no total token fact", rollup)
		}
	}
}

func assertFactorySessionRollup(t *testing.T, rollup costs.Rollup) {
	t.Helper()
	switch rollup.Key {
	case "session-a":
		if rollup.Status != costs.StatusPartial || rollup.PricedSubtotal == nil || *rollup.PricedSubtotal != "0" {
			t.Fatalf("session-a rollup = %#v, want partial zero priced subtotal", rollup)
		}
	case "session-c":
		if rollup.Status != costs.StatusUnpriced || rollup.PricedSubtotal != nil {
			t.Fatalf("session-c rollup = %#v, want wholly unpriced with absent amount", rollup)
		}
	}
}

func TestQueryNoUsageAndExplicitZeroRemainDistinct(t *testing.T) {
	t.Parallel()

	pricing := &priceReader{table: providers.PriceTable{
		Currency: providers.PriceTableCurrencyUSD,
		Models:   []providers.PriceTableModel{testPriceModel("free", "0", "0", nil, nil)},
	}}
	query, err := New(pricing, operatorSettingsStub(), metricsQueryStub(nil, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	empty, err := query.Query(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("empty Query() error = %v", err)
	}
	if empty.Status != costs.StatusNoUsage || empty.PricedSubtotal != nil || empty.Coverage.EncounteredRows != 0 {
		t.Fatalf("empty report = %#v, want NO_USAGE with absent amount", empty)
	}

	query, err = New(pricing, operatorSettingsStub(), metricsQueryStub([]factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session", "work", "dispatch", "worker", "codex", "free", 0, 0, nil, nil),
	}, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New(explicit zero) error = %v", err)
	}
	free, err := query.Query(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("explicit zero Query() error = %v", err)
	}
	if free.Status != costs.StatusPriced || free.PricedSubtotal == nil || *free.PricedSubtotal != "0" {
		t.Fatalf("explicit zero report = %#v, want PRICED amount 0", free)
	}
	if free.KnownCost == nil || *free.KnownCost != "0" || free.TokenTotals.TotalTokens == nil || *free.TokenTotals.TotalTokens != 0 {
		t.Fatalf("explicit zero facts = %#v, want known zero and total zero", free)
	}
}

func TestQueryUnpricedFactsDeduplicateDispatchesAndRetainUnknownIdentity(t *testing.T) {
	t.Parallel()

	pricing := &priceReader{table: providers.PriceTable{
		Currency: providers.PriceTableCurrencyUSD,
		Models:   []providers.PriceTableModel{testPriceModel("known", "1", "1", nil, nil)},
	}}
	rows := []factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session", "work", "dispatch-a", "worker-a", "codex", "unknown", 2, 3, nil, nil),
		usageRow("session", "work", "dispatch-a", "worker-a", "CODEX", "unknown", 4, 5, nil, nil),
		usageRow("session", "work", "dispatch-b", "worker-b", "", "", 6, 7, nil, nil),
	}
	query, err := New(pricing, operatorSettingsStub(), metricsQueryStub(rows, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	report, err := query.Query(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	assertUnpricedReportFacts(t, report)
}

func assertUnpricedReportFacts(t *testing.T, report costs.Report) {
	t.Helper()
	if report.Status != costs.StatusUnpriced || report.KnownCost != nil {
		t.Fatalf("report = %#v, want wholly unpriced with no known cost", report)
	}
	assertTokenTotals(t, report.TokenTotals, 12, 15, 27)
	if report.TokenTotals.CachedInputTokens != nil || report.TokenTotals.ReasoningOutputTokens != nil {
		t.Fatalf("absent subclass totals = %#v, want nil cached/reasoning facts", report.TokenTotals)
	}
	if report.UnpricedDispatchCount != 2 {
		t.Fatalf("unpriced dispatch count = %d, want two distinct dispatches", report.UnpricedDispatchCount)
	}
	assertUnpricedPairFacts(t, report.UnpricedPairs)
}

func assertUnpricedPairFacts(t *testing.T, pairs []costs.UnpricedPair) {
	t.Helper()
	if len(pairs) != 2 {
		t.Fatalf("unpriced pairs = %#v, want canonical unknown and missing pairs", pairs)
	}
	assertMissingIdentityPair(t, pairs[0])
	assertCanonicalUnknownPair(t, pairs[1])
}

func assertMissingIdentityPair(t *testing.T, pair costs.UnpricedPair) {
	t.Helper()
	if pair.Provider != nil || pair.Model != nil || pair.DispatchCount != 1 {
		t.Fatalf("missing identity pair = %#v, want explicit nil identities", pair)
	}
}

func assertCanonicalUnknownPair(t *testing.T, pair costs.UnpricedPair) {
	t.Helper()
	if pair.Provider == nil || *pair.Provider != "CODEX" || pair.Model == nil || *pair.Model != "unknown" || pair.DispatchCount != 1 {
		t.Fatalf("canonical pair = %#v, want CODEX/unknown with one dispatch", pair)
	}
}

func TestQueryUnpricedUsageRetainsCorrelationAndCountsRepeatedRows(t *testing.T) {
	t.Parallel()

	pricing := &priceReader{table: providers.PriceTable{
		Currency: providers.PriceTableCurrencyUSD,
		Models:   []providers.PriceTableModel{testPriceModel("known", "1", "1", nil, nil)},
	}}
	rows := []factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session", "work-unknown-a", "dispatch-unknown-a", "worker-unknown-a", "codex", "missing", 10, 20, nil, nil),
		usageRow("session", "work-unknown-b", "dispatch-unknown-b", "worker-unknown-b", "codex", "missing", 30, 40, nil, nil),
	}
	query, err := New(pricing, operatorSettingsStub(), metricsQueryStub(rows, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	report, err := query.Query(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if report.Status != costs.StatusUnpriced || report.PricedSubtotal != nil {
		t.Fatalf("report = %#v, want wholly unpriced report without a fabricated amount", report)
	}
	wantCoverage := costs.Coverage{
		EncounteredRows: 2, PricedRows: 0, UnpricedRows: 2,
		EncounteredProviderModels: 1, PricedProviderModels: 0, UnpricedProviderModels: 1,
	}
	if !reflect.DeepEqual(report.Coverage, wantCoverage) {
		t.Fatalf("coverage = %#v, want %#v", report.Coverage, wantCoverage)
	}
	if len(report.LineItems) != 2 {
		t.Fatalf("line items = %#v, want two diagnostics", report.LineItems)
	}
	for _, line := range report.LineItems {
		assertUnpricedLineRetainsCorrelation(t, line)
	}
}

func assertUnpricedLineRetainsCorrelation(t *testing.T, line costs.LineItem) {
	t.Helper()
	if line.Status != costs.StatusUnpriced || line.PricedAmount != nil || line.Provider != "CODEX" || line.Model != "missing" {
		t.Fatalf("unpriced line = %#v, want safe identity and no amount", line)
	}
	if line.DispatchID == "" || line.WorkID == "" || line.WorkerSessionID == "" || !strings.Contains(line.Reason, "no configured price") {
		t.Fatalf("unpriced diagnostic = %#v, want correlation and stable reason", line)
	}
}

func TestQueryScopedSelectionAndDeterministicOutput(t *testing.T) {
	t.Parallel()

	pricing := &priceReader{table: providers.PriceTable{
		Currency: providers.PriceTableCurrencyUSD,
		Models:   []providers.PriceTableModel{testPriceModel("model", "1", "1", nil, nil)},
	}}
	rows := []factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session-b", "work-b", "dispatch-b", "worker-b", "codex", "model", 2, 2, nil, nil),
		usageRow("session-a", "work-a", "dispatch-a", "worker-a", "codex", "model", 1, 1, nil, nil),
	}
	query, err := New(pricing, operatorSettingsStub(), metricsQueryStub(rows, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	first, err := query.Query(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("first Query() error = %v", err)
	}
	second, err := query.Query(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("second Query() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated reports differ:\nfirst=%#v\nsecond=%#v", first, second)
	}

	got, err := query.Query(context.Background(), costs.QueryRequest{
		MetricsRoot: "metrics-root", OperatorSettingsPath: "settings.json", FactorySessionID: "session-a",
	})
	if err != nil {
		t.Fatalf("scoped Query() error = %v", err)
	}
	if got.Scope.Kind != costs.ScopeFactorySession || got.Scope.FactorySessionID != "session-a" || len(got.LineItems) != 1 || got.LineItems[0].FactorySessionID != "session-a" {
		t.Fatalf("scoped report = %#v, want only session-a", got)
	}
}

func TestQueryErrorsAreTypedAndLogsSafeTerminalOutcome(t *testing.T) {
	t.Parallel()
	assertSettingsReadFailureIsSafe(t)
	assertMetricsFailureIsTyped(t)
	assertInvalidRequestIsTyped(t)
}

func assertSettingsReadFailureIsSafe(t *testing.T) {
	t.Helper()
	pricingErr := errors.New("provider pricing unavailable")
	logger := &captureLogger{}
	query, err := New(&priceReader{err: pricingErr}, operatorSettingsStub(), metricsQueryStub(nil, nil), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = query.Query(context.Background(), validRequest())
	var queryErr *costs.QueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != costs.QueryErrorSettingsReadFailed || !errors.Is(err, pricingErr) {
		t.Fatalf("pricing error = %v, want typed wrapped provider-pricing failure", err)
	}
	if !logger.hasMessage("runtime costs query started") || !logger.hasMessage("runtime costs query failed") {
		t.Fatalf("log messages = %#v, want start and safe terminal failure", logger.messages)
	}
	if strings.Contains(logger.fieldsText(), "inputPerMillion") || strings.Contains(logger.fieldsText(), "gpt") {
		t.Fatalf("logs contain configuration identity/content: %#v", logger.fields)
	}
	if strings.Contains(logger.fieldsText(), pricingErr.Error()) {
		t.Fatalf("logs contain dependency error details: %#v", logger.fields)
	}
}

func assertMetricsFailureIsTyped(t *testing.T) {
	t.Helper()
	metricsErr := errors.New("metrics unavailable")
	query, err := New(&priceReader{table: providers.PriceTable{Currency: providers.PriceTableCurrencyUSD}}, operatorSettingsStub(), metricsQueryStub(nil, metricsErr), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New(metrics error) error = %v", err)
	}
	_, err = query.Query(context.Background(), validRequest())
	var queryErr *costs.QueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != costs.QueryErrorMetricsFailed || !errors.Is(err, metricsErr) {
		t.Fatalf("metrics error = %v, want typed wrapped metrics failure", err)
	}
}

func assertInvalidRequestIsTyped(t *testing.T) {
	t.Helper()
	query, err := New(&priceReader{table: providers.PriceTable{Currency: providers.PriceTableCurrencyUSD}}, operatorSettingsStub(), metricsQueryStub(nil, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New(invalid request) error = %v", err)
	}
	_, err = query.Query(context.Background(), costs.QueryRequest{})
	var queryErr *costs.QueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != costs.QueryErrorInvalidInput || !strings.Contains(err.Error(), "metrics root") {
		t.Fatalf("invalid request error = %v, want actionable invalid input", err)
	}
}

func TestQueryOperatorPriceTableSupplementsOverridesAndDoesNotMixRates(t *testing.T) {
	t.Parallel()

	builtIn := providers.PriceTable{
		Currency: providers.PriceTableCurrencyUSD,
		Models: []providers.PriceTableModel{
			testPriceModel("gpt-5", "1", "2", stringPtr("0.5"), stringPtr("3")),
			testPriceModel("fallback", "1", "2", nil, nil),
		},
	}
	settings := &operatorSettingsReaderStub{config: operatorsettings.Config{
		PriceTable: operatorsettings.PriceTable{
			Currency: operatorsettings.PriceTableCurrencyUSD,
			Models: []operatorsettings.PriceTableModel{
				{Provider: " claude ", Model: "claude-sonnet", InputPerMillionTokens: "3", OutputPerMillionTokens: "15"},
				{Provider: "codex", Model: "gpt-5", InputPerMillionTokens: "10", OutputPerMillionTokens: "20"},
			},
		},
	}}
	rows := []factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session", "work-claude", "dispatch-claude", "worker-claude", "CLAUDE", "claude-sonnet", 1_000_000, 2_000_000, nil, nil),
		usageRow("session", "work-override", "dispatch-override", "worker-override", "CODEX", "gpt-5", 1_000_000, 2_000_000, nil, nil),
		usageRow("session", "work-no-mix", "dispatch-no-mix", "worker-no-mix", "CODEX", "gpt-5", 1_000_000, 2_000_000, int64Ptr(1), nil),
		usageRow("session", "work-fallback", "dispatch-fallback", "worker-fallback", "CODEX", "fallback", 1_000_000, 1_000_000, nil, nil),
	}
	query, err := New(&priceReader{table: builtIn}, settings, metricsQueryStub(rows, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first, err := query.Query(context.Background(), costs.QueryRequest{MetricsRoot: "metrics", OperatorSettingsPath: "explicit-settings.json"})
	if err != nil {
		t.Fatalf("first Query() error = %v", err)
	}
	if first.Status != costs.StatusPartial || first.Coverage.PricedRows != 3 || first.Coverage.UnpricedRows != 1 {
		t.Fatalf("first report status/coverage = %q/%#v, want PARTIAL with three priced rows", first.Status, first.Coverage)
	}
	assertLineAmount(t, first.LineItems, "claude-sonnet", "33", "")
	assertLineAmount(t, first.LineItems, "gpt-5", "50", "")
	assertLineAmount(t, first.LineItems, "fallback", "3", "")
	assertLineAmount(t, first.LineItems, "gpt-5", "", "cached-input rate is not configured")
	assertLineSource(t, first.LineItems, "claude-sonnet", costs.StatusPriced, costs.PriceSourceOperatorSupplied)
	assertLineSource(t, first.LineItems, "gpt-5", costs.StatusPriced, costs.PriceSourceOperatorSupplied)
	assertLineSource(t, first.LineItems, "fallback", costs.StatusPriced, costs.PriceSourceBuiltIn)
	assertLineSource(t, first.LineItems, "gpt-5", costs.StatusUnpriced, "")
	if len(settings.paths) != 1 || settings.paths[0] != "explicit-settings.json" {
		t.Fatalf("settings paths = %#v, want the explicit request path", settings.paths)
	}

	settings.config.PriceTable.Models[0].InputPerMillionTokens = "4"
	second, err := query.Query(context.Background(), costs.QueryRequest{MetricsRoot: "metrics", OperatorSettingsPath: "explicit-settings.json"})
	if err != nil {
		t.Fatalf("second Query() error = %v", err)
	}
	assertLineAmount(t, second.LineItems, "claude-sonnet", "34", "")
	if len(settings.paths) != 2 {
		t.Fatalf("settings paths after repeat = %#v, want one read per query", settings.paths)
	}
}

func TestQueryRejectsInvalidOrUnreadableOperatorPriceTableBeforeMetrics(t *testing.T) {
	t.Parallel()

	var metricsCalls int
	metrics := factoryvisualization.RuntimeMetricsQuery(func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		metricsCalls++
		return factoryvisualization.RuntimeMetricsQueryResult{}, nil
	})
	builtIn := &priceReader{table: providers.PriceTable{Currency: providers.PriceTableCurrencyUSD}}
	invalidSettings := &operatorSettingsReaderStub{config: operatorsettings.Config{PriceTable: operatorsettings.PriceTable{
		Currency: operatorsettings.PriceTableCurrencyUSD,
		Models:   []operatorsettings.PriceTableModel{{Provider: "claude", Model: "sonnet", InputPerMillionTokens: "-1", OutputPerMillionTokens: "15"}},
	}}}
	query, err := New(builtIn, invalidSettings, metrics, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New(invalid settings) error = %v", err)
	}
	_, err = query.Query(context.Background(), validRequest())
	var queryErr *costs.QueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != costs.QueryErrorInvalidPriceTable || !errors.Is(err, operatorsettings.ErrPriceTableInvalid) {
		t.Fatalf("invalid operator price table error = %v, want typed actionable failure", err)
	}
	if metricsCalls != 0 {
		t.Fatalf("metrics calls after invalid settings = %d, want none", metricsCalls)
	}

	readFailure := errors.New("settings unavailable")
	settingsFailure := &operatorSettingsReaderStub{err: readFailure}
	query, err = New(builtIn, settingsFailure, metrics, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New(settings failure) error = %v", err)
	}
	_, err = query.Query(context.Background(), validRequest())
	if !errors.As(err, &queryErr) || queryErr.Kind != costs.QueryErrorSettingsReadFailed || !errors.Is(err, readFailure) {
		t.Fatalf("operator settings read error = %v, want typed wrapped failure", err)
	}
	if metricsCalls != 0 {
		t.Fatalf("metrics calls after settings failure = %d, want none", metricsCalls)
	}
}

func TestQueryCancellationDoesNotReadDependencies(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pricingCalls := 0
	pricing := providers.PriceTableReaderFunc(func() (providers.PriceTable, error) {
		pricingCalls++
		return providers.PriceTable{Currency: providers.PriceTableCurrencyUSD}, nil
	})
	settings := &operatorSettingsReaderStub{}
	query, err := New(pricing, settings, metricsQueryStub(nil, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = query.Query(ctx, validRequest())
	if !errors.Is(err, context.Canceled) || pricingCalls != 0 || len(settings.paths) != 0 {
		t.Fatalf("canceled query error/calls = %v/%d/%d, want canceled with no dependency reads", err, pricingCalls, len(settings.paths))
	}
}

func TestInternalProjectionHelpersNormalizeAndDetach(t *testing.T) {
	t.Parallel()

	scope := scopeForRequest(costs.QueryRequest{FactorySessionID: " session-a "})
	if scope.Kind != costs.ScopeFactorySession || scope.FactorySessionID != "session-a" {
		t.Fatalf("scopeForRequest() = %#v, want normalized Factory Session scope", scope)
	}
	all := scopeForRequest(costs.QueryRequest{})
	if all.Kind != costs.ScopeAllFactorySessions || all.FactorySessionID != "" {
		t.Fatalf("scopeForRequest(all) = %#v, want all-session scope", all)
	}

	input, output := int64(3), int64(4)
	row := runtimeUsageFromMetrics(factoryvisualization.RuntimeMetricsUsageRow{
		FactorySessionID: "session", InputTokens: &input, OutputTokens: &output,
	})
	input = 99
	output = 100
	if row.InputTokens == nil || *row.InputTokens != 3 || row.OutputTokens == nil || *row.OutputTokens != 4 {
		t.Fatalf("runtimeUsageFromMetrics() = %#v, want detached token pointers", row)
	}
}

func TestFormatExactDecimalDoesNotRound(t *testing.T) {
	t.Parallel()

	value, err := parseDecimal("1.234567")
	if err != nil {
		t.Fatalf("parseDecimal() error = %v", err)
	}
	value.Quo(value, big.NewRat(millionTokenCount, 1))
	got, err := formatExactDecimal(value)
	if err != nil {
		t.Fatalf("formatExactDecimal() error = %v", err)
	}
	if got != "0.000001234567" {
		t.Fatalf("formatted amount = %q, want exact decimal", got)
	}
}

func validRequest() costs.QueryRequest {
	return costs.QueryRequest{MetricsRoot: "metrics-root", OperatorSettingsPath: "settings.json"}
}

func usageRow(session, work, dispatch, worker, provider, model string, input, output int64, cached, reasoning *int64) factoryvisualization.RuntimeMetricsUsageRow {
	return factoryvisualization.RuntimeMetricsUsageRow{
		FactorySessionID: session, WorkID: work, DispatchID: dispatch, WorkerSessionID: worker,
		Provider: provider, Model: model, InputTokens: int64Ptr(input), OutputTokens: int64Ptr(output),
		CachedInputTokens: cached, ReasoningOutputTokens: reasoning,
	}
}

func metricsQueryStub(rows []factoryvisualization.RuntimeMetricsUsageRow, err error) factoryvisualization.RuntimeMetricsQuery {
	return func(_ context.Context, request factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		if request.MetricsRoot == "" {
			return factoryvisualization.RuntimeMetricsQueryResult{}, errors.New("metrics root missing")
		}
		if err != nil {
			return factoryvisualization.RuntimeMetricsQueryResult{}, err
		}
		filtered := make([]factoryvisualization.RuntimeMetricsUsageRow, 0, len(rows))
		for _, row := range rows {
			if len(request.SessionIDs) > 0 {
				if !containsSessionID(request.SessionIDs, row.FactorySessionID) {
					continue
				}
			} else if request.SessionID != "" && row.FactorySessionID != request.SessionID {
				continue
			}
			filtered = append(filtered, row)
		}
		return factoryvisualization.RuntimeMetricsQueryResult{UsageRows: filtered}, nil
	}
}

func containsSessionID(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

type priceReader struct {
	table providers.PriceTable
	err   error
}

func (reader *priceReader) ReadPriceTable() (providers.PriceTable, error) {
	if reader.err != nil {
		return providers.PriceTable{}, reader.err
	}
	return reader.table.Clone(), nil
}

func operatorSettingsStub() *operatorSettingsReaderStub {
	return &operatorSettingsReaderStub{}
}

type operatorSettingsReaderStub struct {
	operatorsettings.Service
	config operatorsettings.Config
	err    error
	paths  []string
}

func (reader *operatorSettingsReaderStub) LoadFileConfig(path string) (operatorsettings.Config, error) {
	reader.paths = append(reader.paths, path)
	if reader.err != nil {
		return operatorsettings.Config{}, reader.err
	}
	return reader.config, nil
}

func testPriceModel(model, input, output string, cached, reasoning *string) providers.PriceTableModel {
	entry := providers.PriceTableModel{
		Provider:                        providers.IDCodex,
		Model:                           model,
		InputPerMillionTokens:           input,
		OutputPerMillionTokens:          output,
		CachedInputPerMillionTokens:     cached,
		ReasoningOutputPerMillionTokens: reasoning,
		SourceURL:                       "https://example.com/test-pricing",
		AsOfDate:                        "2026-08-21",
	}
	if cached != nil && *cached == input {
		entry.EqualRateClasses = append(entry.EqualRateClasses, providers.PriceClassCachedInput)
	}
	if reasoning != nil && *reasoning == output {
		entry.EqualRateClasses = append(entry.EqualRateClasses, providers.PriceClassReasoningOutput)
	}
	return entry
}

type captureLogger struct {
	messages []string
	fields   []any
}

func (logger *captureLogger) Debug(message string, fields ...any) { logger.record(message, fields...) }
func (logger *captureLogger) Info(message string, fields ...any)  { logger.record(message, fields...) }
func (logger *captureLogger) Warn(message string, fields ...any)  { logger.record(message, fields...) }
func (logger *captureLogger) Error(message string, fields ...any) { logger.record(message, fields...) }
func (logger *captureLogger) Verbose(message string, fields ...any) {
	logger.record(message, fields...)
}
func (logger *captureLogger) record(message string, fields ...any) {
	logger.messages = append(logger.messages, message)
	logger.fields = append(logger.fields, fields...)
}
func (logger *captureLogger) hasMessage(want string) bool {
	for _, message := range logger.messages {
		if message == want {
			return true
		}
	}
	return false
}
func (logger *captureLogger) fieldsText() string { return fmt.Sprint(logger.fields...) }

func assertLine(t *testing.T, lines []costs.LineItem, model string, status costs.Status, amount, reason string) {
	t.Helper()
	for _, line := range lines {
		if line.Model != model {
			continue
		}
		if line.Status != status {
			t.Fatalf("line %q status = %q, want %q", model, line.Status, status)
		}
		if amount == "" {
			if line.PricedAmount != nil {
				t.Fatalf("line %q amount = %v, want absent", model, line.PricedAmount)
			}
		} else if line.PricedAmount == nil || *line.PricedAmount != amount {
			t.Fatalf("line %q amount = %v, want %q", model, line.PricedAmount, amount)
		}
		if reason != "" && !strings.Contains(line.Reason, reason) {
			t.Fatalf("line %q reason = %q, want substring %q", model, line.Reason, reason)
		}
		return
	}
	t.Fatalf("line for model %q not found in %#v", model, lines)
}

func assertLineSource(t *testing.T, lines []costs.LineItem, model string, status costs.Status, want costs.PriceSource) {
	t.Helper()
	for _, line := range lines {
		if line.Model != model || line.Status != status {
			continue
		}
		if line.PriceSource != want {
			t.Fatalf("line %q status %q source = %q, want %q", model, status, line.PriceSource, want)
		}
		return
	}
	t.Fatalf("line for model %q with status %q not found in %#v", model, status, lines)
}

func assertLineAmount(t *testing.T, lines []costs.LineItem, model, amount, reason string) {
	t.Helper()
	for _, line := range lines {
		if line.Model != model {
			continue
		}
		if amount != "" {
			if line.Status == costs.StatusPriced && line.PricedAmount != nil && *line.PricedAmount == amount {
				return
			}
			continue
		}
		if line.Status == costs.StatusUnpriced && strings.Contains(line.Reason, reason) {
			return
		}
	}
	t.Fatalf("line for model %q amount=%q reason=%q not found in %#v", model, amount, reason, lines)
}

func int64Ptr(value int64) *int64    { return &value }
func stringPtr(value string) *string { return &value }

type unpricedPairWant struct {
	model         *string
	dispatchCount int
}

func assertTokenTotals(t *testing.T, totals costs.TokenTotals, input, output, total int64) {
	t.Helper()
	values := []struct {
		name string
		got  *int64
		want int64
	}{
		{"input", totals.InputTokens, input},
		{"output", totals.OutputTokens, output},
		{"total", totals.TotalTokens, total},
	}
	for _, value := range values {
		if value.got == nil || *value.got != value.want {
			t.Fatalf("%s token total = %v, want %d", value.name, value.got, value.want)
		}
	}
}

func assertUnpricedPairs(t *testing.T, got []costs.UnpricedPair, want []unpricedPairWant) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unpriced pairs = %#v, want %d pairs", got, len(want))
	}
	for index, expected := range want {
		if got[index].Model == nil && expected.model != nil || got[index].Model != nil && expected.model == nil {
			t.Fatalf("unpriced pair %d model = %#v, want %#v", index, got[index].Model, expected.model)
		}
		if got[index].Model != nil && *got[index].Model != *expected.model {
			t.Fatalf("unpriced pair %d model = %q, want %q", index, *got[index].Model, *expected.model)
		}
		if got[index].Provider == nil || *got[index].Provider != "CODEX" {
			t.Fatalf("unpriced pair %d provider = %#v, want CODEX", index, got[index].Provider)
		}
		if got[index].DispatchCount != expected.dispatchCount {
			t.Fatalf("unpriced pair %d dispatch count = %d, want %d", index, got[index].DispatchCount, expected.dispatchCount)
		}
	}
}

var _ logging.Logger = (*captureLogger)(nil)
