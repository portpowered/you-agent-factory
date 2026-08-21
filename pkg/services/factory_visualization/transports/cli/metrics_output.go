package cli

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

func renderHumanMetrics(
	groupBy string,
	result factoryvisualization.RuntimeMetricsQueryResult,
) string {
	var output strings.Builder
	fmt.Fprintln(&output, "Scope: all Factory Sessions")
	fmt.Fprintf(&output, "Group by: %s\n", groupBy)
	fmt.Fprintln(&output, "Cost: unavailable")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Totals:")
	renderMetricsAggregate(&output, "  ", result.Totals)
	fmt.Fprintln(&output)

	breakdowns := metricsBreakdowns(groupBy, result)
	fmt.Fprintf(&output, "Breakdown by %s: %d rows\n", groupBy, len(breakdowns))
	for _, breakdown := range breakdowns {
		fmt.Fprintf(&output, "  %s:\n", breakdown.Key)
		renderMetricsAggregate(&output, "    ", breakdown.Aggregate)
	}
	return output.String()
}

func metricsBreakdowns(
	groupBy string,
	result factoryvisualization.RuntimeMetricsQueryResult,
) []factoryvisualization.RuntimeMetricsBreakdown {
	switch groupBy {
	case metricsGroupByWorker:
		return result.WorkerTypes
	case metricsGroupByProvider:
		return result.Providers
	default:
		return result.Workstations
	}
}

func renderMetricsAggregate(
	output *strings.Builder,
	indent string,
	aggregate factoryvisualization.RuntimeMetricsAggregate,
) {
	fmt.Fprintf(output, "%sInput tokens: %s\n", indent, formatMetricValue(aggregate.InputTokens))
	fmt.Fprintf(output, "%sOutput tokens: %s\n", indent, formatMetricValue(aggregate.OutputTokens))
	fmt.Fprintf(output, "%sCompleted dispatches: %s\n", indent, formatMetricValue(aggregate.CompletedDispatches))
	renderFailureReasons(output, indent, aggregate.FailuresByReason)
	renderLatency(output, indent, "Dispatch latency (milliseconds)", aggregate.DispatchDuration)
	renderLatency(output, indent, "Provider latency (milliseconds)", aggregate.ProviderDuration)
}

func renderFailureReasons(output *strings.Builder, indent string, failures map[string]float64) {
	if len(failures) == 0 {
		fmt.Fprintf(output, "%sFailures by reason: none\n", indent)
		return
	}
	keys := make([]string, 0, len(failures))
	for key := range failures {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Fprintf(output, "%sFailures by reason:\n", indent)
	for _, key := range keys {
		fmt.Fprintf(output, "%s  %s: %s\n", indent, key, formatMetricValue(failures[key]))
	}
}

func renderLatency(
	output *strings.Builder,
	indent string,
	label string,
	duration *factoryvisualization.RuntimeMetricsDuration,
) {
	if duration == nil || duration.Samples == 0 || duration.P50 == nil || duration.P95 == nil {
		fmt.Fprintf(output, "%s%s: no samples\n", indent, label)
		return
	}
	fmt.Fprintf(output, "%s%s: p50=%s, p95=%s, samples=%d\n",
		indent, label, formatMetricValue(*duration.P50), formatMetricValue(*duration.P95), duration.Samples)
}

func formatMetricValue(value float64) string {
	if value == 0 {
		return "0"
	}
	if math.Trunc(value) == value {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
