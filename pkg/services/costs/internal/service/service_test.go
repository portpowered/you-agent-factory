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
)

func TestQueryExactValuationCoverageAndRollups(t *testing.T) {
	t.Parallel()

	settings := &settingsReader{document: operatorsettings.Document{PriceTable: operatorsettings.PriceTable{
		Currency: operatorsettings.PriceTableCurrencyUSD,
		Models: []operatorsettings.PriceTableModel{
			{Provider: "codex", Model: "gpt-5", InputPerMillionTokens: "2.5", OutputPerMillionTokens: "5.25", CachedInputPerMillionTokens: stringPtr("0.5"), ReasoningOutputPerMillionTokens: stringPtr("10")},
			{Provider: "codex", Model: "gpt-zero", InputPerMillionTokens: "0", OutputPerMillionTokens: "0"},
			{Provider: "codex", Model: "gpt-no-cache", InputPerMillionTokens: "1", OutputPerMillionTokens: "1"},
		},
	}}}
	rows := []factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session-b", "work-b", "dispatch-b", "worker-b", "codex", "gpt-5", 2_000_000, 1_000_000, int64Ptr(500_000), int64Ptr(200_000)),
		usageRow("session-a", "work-a", "dispatch-a", "worker-a", "codex", "gpt-zero", 3, 4, nil, nil),
		usageRow("session-c", "work-c", "dispatch-c", "worker-c", "codex", "unknown", 10, 5, nil, nil),
		usageRow("session-a", "work-a2", "dispatch-a2", "worker-a2", "codex", "gpt-no-cache", 1_000, 1_000, int64Ptr(500), nil),
		usageRow("session-a", "work-a3", "dispatch-a3", "worker-a3", "codex", "", 8, 9, nil, nil),
	}
	query, err := New(settings, metricsQueryStub(rows, nil), logging.NoopLogger{})
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

	settings := &settingsReader{document: operatorsettings.Document{PriceTable: operatorsettings.PriceTable{
		Models: []operatorsettings.PriceTableModel{{Provider: "codex", Model: "free", InputPerMillionTokens: "0", OutputPerMillionTokens: "0"}},
	}}}
	query, err := New(settings, metricsQueryStub(nil, nil), logging.NoopLogger{})
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

	query, err = New(settings, metricsQueryStub([]factoryvisualization.RuntimeMetricsUsageRow{
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
}

func TestQueryUnpricedUsageRetainsCorrelationAndCountsRepeatedRows(t *testing.T) {
	t.Parallel()

	settings := &settingsReader{document: operatorsettings.Document{PriceTable: operatorsettings.PriceTable{
		Currency: operatorsettings.PriceTableCurrencyUSD,
		Models: []operatorsettings.PriceTableModel{{
			Provider: "codex", Model: "known", InputPerMillionTokens: "1", OutputPerMillionTokens: "1",
		}},
	}}}
	rows := []factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session", "work-unknown-a", "dispatch-unknown-a", "worker-unknown-a", "codex", "missing", 10, 20, nil, nil),
		usageRow("session", "work-unknown-b", "dispatch-unknown-b", "worker-unknown-b", "codex", "missing", 30, 40, nil, nil),
	}
	query, err := New(settings, metricsQueryStub(rows, nil), logging.NoopLogger{})
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
		if line.Status != costs.StatusUnpriced || line.PricedAmount != nil || line.Provider != "CODEX" || line.Model != "missing" {
			t.Fatalf("unpriced line = %#v, want safe identity and no amount", line)
		}
		if line.DispatchID == "" || line.WorkID == "" || line.WorkerSessionID == "" || !strings.Contains(line.Reason, "no configured price") {
			t.Fatalf("unpriced diagnostic = %#v, want correlation and stable reason", line)
		}
	}
}

func TestQueryScopedSelectionAndDeterministicOutput(t *testing.T) {
	t.Parallel()

	settings := &settingsReader{document: operatorsettings.Document{PriceTable: operatorsettings.PriceTable{
		Models: []operatorsettings.PriceTableModel{{Provider: "codex", Model: "model", InputPerMillionTokens: "1", OutputPerMillionTokens: "1"}},
	}}}
	rows := []factoryvisualization.RuntimeMetricsUsageRow{
		usageRow("session-b", "work-b", "dispatch-b", "worker-b", "codex", "model", 2, 2, nil, nil),
		usageRow("session-a", "work-a", "dispatch-a", "worker-a", "codex", "model", 1, 1, nil, nil),
	}
	query, err := New(settings, metricsQueryStub(rows, nil), logging.NoopLogger{})
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
	settingsErr := errors.New("settings unavailable")
	logger := &captureLogger{}
	query, err := New(&settingsReader{err: settingsErr}, metricsQueryStub(nil, nil), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = query.Query(context.Background(), validRequest())
	var queryErr *costs.QueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != costs.QueryErrorSettingsReadFailed || !errors.Is(err, settingsErr) {
		t.Fatalf("settings error = %v, want typed wrapped settings failure", err)
	}
	if !logger.hasMessage("runtime costs query started") || !logger.hasMessage("runtime costs query failed") {
		t.Fatalf("log messages = %#v, want start and safe terminal failure", logger.messages)
	}
	if strings.Contains(logger.fieldsText(), "inputPerMillion") || strings.Contains(logger.fieldsText(), "gpt") {
		t.Fatalf("logs contain configuration identity/content: %#v", logger.fields)
	}
	if strings.Contains(logger.fieldsText(), settingsErr.Error()) {
		t.Fatalf("logs contain dependency error details: %#v", logger.fields)
	}
}

func assertMetricsFailureIsTyped(t *testing.T) {
	t.Helper()
	metricsErr := errors.New("metrics unavailable")
	query, err := New(&settingsReader{document: operatorsettings.Document{}}, metricsQueryStub(nil, metricsErr), logging.NoopLogger{})
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
	query, err := New(&settingsReader{document: operatorsettings.Document{}}, metricsQueryStub(nil, nil), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New(invalid request) error = %v", err)
	}
	_, err = query.Query(context.Background(), costs.QueryRequest{MetricsRoot: "metrics-root"})
	var queryErr *costs.QueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != costs.QueryErrorInvalidInput || !strings.Contains(err.Error(), "operator settings path") {
		t.Fatalf("invalid request error = %v, want actionable invalid input", err)
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
			if request.SessionID == "" || row.FactorySessionID == request.SessionID {
				filtered = append(filtered, row)
			}
		}
		return factoryvisualization.RuntimeMetricsQueryResult{UsageRows: filtered}, nil
	}
}

type settingsReader struct {
	document operatorsettings.Document
	err      error
}

func (reader *settingsReader) LoadDocument(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
	if reader.err != nil {
		return operatorsettings.LoadDocumentResult{}, reader.err
	}
	return operatorsettings.LoadDocumentResult{Document: reader.document}, nil
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

func int64Ptr(value int64) *int64    { return &value }
func stringPtr(value string) *string { return &value }

var _ logging.Logger = (*captureLogger)(nil)
