package factory_visualization_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
)

func TestRuntimeMetricsQueryReconcilesProviderAttributionWithoutDuplicatingFacts(t *testing.T) {
	t.Parallel()

	records := providerAttributionRecords()
	originalRecords := cloneRuntimeMetricRecords(records)
	result, repeated := queryProviderAttributionMetrics(t, records)
	assertProviderAttributionDeterminism(t, records, originalRecords, result, repeated)
	assertProviderAttributionTotals(t, result)
	assertProviderAttributionBreakdowns(t, result.Providers)
	assertProviderAttributionHasNoTemplateKeys(t, result)
}

func providerAttributionRecords() []factoryvisualization.RuntimeMetricRecord {
	var records []factoryvisualization.RuntimeMetricRecord
	appendDispatch := func(dispatchID, name string, value float64, provider, reason, unit string) {
		record := metricRecord(name, value, "session-a", "runtime-a", "ws", "worker", provider, reason, unit)
		record["dispatch_id"] = dispatchID
		records = append(records, record)
	}

	appendDispatch("authoritative", "dispatch.completed", 1, "codex", "", "")
	appendDispatch("authoritative", "dispatch.duration", 12, "${executorProvider}", "", "ms")
	appendDispatch("authoritative", "provider.input_tokens", 3, "${branchProvider}", "", "tokens")
	appendDispatch("authoritative", "provider.failed", 1, "${plannerProvider}", "timeout", "")
	appendDispatch("authoritative", "provider.duration", 8, "${reviewerProvider}", "", "ms")
	appendDispatch("fallback", "dispatch.completed", 1, "", "", "")
	appendDispatch("fallback", "provider.completed", 1, "claude", "", "")
	appendDispatch("fallback", "provider.input_tokens", 2, "${workerProvider}", "", "tokens")
	appendDispatch("fallback", "provider.duration", 4, "", "", "ms")
	appendDispatch("conflict", "dispatch.completed", 1, "", "", "")
	appendDispatch("conflict", "provider.input_tokens", 5, "provider-a", "", "tokens")
	appendDispatch("conflict", "provider.duration", 7, "provider-b", "", "ms")
	appendDispatch("missing", "dispatch.completed", 1, "", "", "")
	appendDispatch("missing", "dispatch.duration", 9, "", "", "ms")
	appendDispatch("missing", "provider.failed", 1, "", "lost", "")
	appendDispatch("", "dispatch.completed", 1, "", "", "")
	appendDispatch("", "provider.input_tokens", 7, "orphan-provider", "", "tokens")
	appendDispatch("", "provider.duration", 3, "${secondProvider}", "", "ms")

	for _, placeholder := range []string{
		"${branchProvider}", "${executorProvider}", "${plannerProvider}",
		"${reviewerProvider}", "${secondProvider}", "${workerProvider}",
	} {
		dispatchID := "placeholder-" + strings.Trim(placeholder, "${}Provider")
		appendDispatch(dispatchID, "dispatch.completed", 1, "", "", "")
		appendDispatch(dispatchID, "provider.input_tokens", 1, placeholder, "", "tokens")
		appendDispatch(dispatchID, "provider.duration", 2, "codex", "", "ms")
	}
	return records
}

func cloneRuntimeMetricRecords(records []factoryvisualization.RuntimeMetricRecord) []factoryvisualization.RuntimeMetricRecord {
	clones := make([]factoryvisualization.RuntimeMetricRecord, len(records))
	for index, record := range records {
		clone := make(factoryvisualization.RuntimeMetricRecord, len(record))
		for key, value := range record {
			clone[key] = value
		}
		clones[index] = clone
	}
	return clones
}

func queryProviderAttributionMetrics(
	t *testing.T,
	records []factoryvisualization.RuntimeMetricRecord,
) (factoryvisualization.RuntimeMetricsQueryResult, factoryvisualization.RuntimeMetricsQueryResult) {
	t.Helper()
	reader := &runtimeMetricsReaderStub{records: records}
	query, err := factoryvisualizationwire.NewRuntimeMetricsQuery(reader, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsQuery() error = %v", err)
	}
	request := factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: t.TempDir()}
	result, err := query.QueryRuntimeMetrics(context.Background(), request)
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics() error = %v", err)
	}
	repeated, err := query.QueryRuntimeMetrics(context.Background(), request)
	if err != nil {
		t.Fatalf("QueryRuntimeMetrics(repeated) error = %v", err)
	}
	return result, repeated
}

func assertProviderAttributionDeterminism(
	t *testing.T,
	records, originalRecords []factoryvisualization.RuntimeMetricRecord,
	result, repeated factoryvisualization.RuntimeMetricsQueryResult,
) {
	t.Helper()
	if !reflect.DeepEqual(result, repeated) {
		t.Fatalf("repeated query result changed: first=%#v second=%#v", result, repeated)
	}
	if !reflect.DeepEqual(records, originalRecords) {
		t.Fatalf("query mutated retained metric records: got=%#v want=%#v", records, originalRecords)
	}
}

func assertProviderAttributionTotals(t *testing.T, result factoryvisualization.RuntimeMetricsQueryResult) {
	t.Helper()
	if result.Totals.CompletedDispatches != 11 || result.Totals.InputTokens != 23 {
		t.Fatalf("totals = %#v, want 11 completed dispatches and 23 input tokens", result.Totals)
	}
}

func assertProviderAttributionBreakdowns(t *testing.T, breakdowns []factoryvisualization.RuntimeMetricsBreakdown) {
	t.Helper()
	wants := []struct {
		key         string
		completed   float64
		inputTokens float64
	}{
		{key: "codex", completed: 7, inputTokens: 9},
		{key: "claude", completed: 1, inputTokens: 2},
		{key: factoryvisualization.RuntimeMetricsUnavailableProviderKey, completed: 3, inputTokens: 5},
		{key: "orphan-provider", inputTokens: 7},
	}
	for _, want := range wants {
		got := breakdownForKey(t, breakdowns, want.key)
		if got.CompletedDispatches != want.completed || got.InputTokens != want.inputTokens {
			t.Fatalf("%s aggregate = %#v, want completion %v and input %v", want.key, got, want.completed, want.inputTokens)
		}
	}
}

func assertProviderAttributionHasNoTemplateKeys(t *testing.T, result factoryvisualization.RuntimeMetricsQueryResult) {
	t.Helper()
	for _, breakdown := range result.Providers {
		if strings.Contains(breakdown.Key, "${") {
			t.Fatalf("provider breakdown leaked template key: %#v", breakdown)
		}
	}
	for _, row := range result.UsageRows {
		if strings.Contains(row.Provider, "${") {
			t.Fatalf("usage row leaked template provider: %#v", row)
		}
	}
}

func breakdownForKey(t *testing.T, breakdowns []factoryvisualization.RuntimeMetricsBreakdown, key string) factoryvisualization.RuntimeMetricsAggregate {
	t.Helper()
	for _, breakdown := range breakdowns {
		if breakdown.Key == key {
			return breakdown.Aggregate
		}
	}
	t.Fatalf("breakdown key %q not found in %#v", key, breakdowns)
	return factoryvisualization.RuntimeMetricsAggregate{}
}
