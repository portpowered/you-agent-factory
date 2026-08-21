package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

func renderHumanMetrics(
	groupBy string,
	sessionID string,
	result factoryvisualization.RuntimeMetricsQueryResult,
) string {
	var output strings.Builder
	if strings.TrimSpace(sessionID) == "" {
		fmt.Fprintln(&output, "Scope: all Factory Sessions")
	} else {
		fmt.Fprintf(&output, "Scope: Factory Session %s\n", sessionID)
	}
	fmt.Fprintf(&output, "Group by: %s\n", groupBy)
	fmt.Fprintf(&output, "Cost: %s\n", normalizeMetricsCostAvailability(result.Cost.Availability))
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

type metricsJSONDocument struct {
	Scope   metricsJSONScope     `json:"scope"`
	GroupBy string               `json:"group_by"`
	Units   metricsJSONUnits     `json:"units"`
	Cost    metricsJSONCost      `json:"cost"`
	Totals  metricsJSONAggregate `json:"totals"`
	Groups  []metricsJSONGroup   `json:"groups"`
}

type metricsJSONScope struct {
	Kind             string  `json:"kind"`
	FactorySessionID *string `json:"factory_session_id"`
}

type metricsJSONUnits struct {
	Tokens  string `json:"tokens"`
	Counts  string `json:"counts"`
	Latency string `json:"latency"`
}

type metricsJSONCost struct {
	Availability string `json:"availability"`
}

type metricsJSONGroup struct {
	Key       string               `json:"key"`
	Aggregate metricsJSONAggregate `json:"aggregate"`
}

type metricsJSONAggregate struct {
	InputTokens         float64            `json:"input_tokens"`
	OutputTokens        float64            `json:"output_tokens"`
	CompletedDispatches float64            `json:"completed_dispatches"`
	FailuresByReason    map[string]float64 `json:"failures_by_reason"`
	DispatchLatency     metricsJSONLatency `json:"dispatch_latency"`
	ProviderLatency     metricsJSONLatency `json:"provider_latency"`
}

type metricsJSONLatency struct {
	Unit    string   `json:"unit"`
	Samples int      `json:"samples"`
	P50     *float64 `json:"p50"`
	P95     *float64 `json:"p95"`
}

func renderMetricsOutput(
	groupBy string,
	sessionID string,
	jsonOutput bool,
	result factoryvisualization.RuntimeMetricsQueryResult,
) (string, error) {
	if !jsonOutput {
		return renderHumanMetrics(groupBy, sessionID, result), nil
	}
	document := metricsJSONDocument{
		Scope:   metricsJSONScopeForSession(sessionID),
		GroupBy: groupBy,
		Units: metricsJSONUnits{
			Tokens:  "tokens",
			Counts:  "count",
			Latency: "milliseconds",
		},
		Cost:   metricsJSONCost{Availability: normalizeMetricsCostAvailability(result.Cost.Availability)},
		Totals: toJSONAggregate(result.Totals),
		Groups: toJSONGroups(metricsBreakdowns(groupBy, result)),
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode metrics JSON: %w", err)
	}
	return string(append(encoded, '\n')), nil
}

func normalizeMetricsCostAvailability(availability factoryvisualization.RuntimeMetricsCostAvailability) string {
	normalized := strings.ToLower(strings.TrimSpace(string(availability)))
	if normalized == "" {
		return "unavailable"
	}
	return normalized
}

func metricsJSONScopeForSession(sessionID string) metricsJSONScope {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return metricsJSONScope{Kind: "all_factory_sessions"}
	}
	return metricsJSONScope{Kind: "factory_session", FactorySessionID: &sessionID}
}

func toJSONGroups(breakdowns []factoryvisualization.RuntimeMetricsBreakdown) []metricsJSONGroup {
	groups := make([]metricsJSONGroup, 0, len(breakdowns))
	for _, breakdown := range breakdowns {
		groups = append(groups, metricsJSONGroup{
			Key:       breakdown.Key,
			Aggregate: toJSONAggregate(breakdown.Aggregate),
		})
	}
	return groups
}

func toJSONAggregate(aggregate factoryvisualization.RuntimeMetricsAggregate) metricsJSONAggregate {
	return metricsJSONAggregate{
		InputTokens:         aggregate.InputTokens,
		OutputTokens:        aggregate.OutputTokens,
		CompletedDispatches: aggregate.CompletedDispatches,
		FailuresByReason:    cloneMetricFailures(aggregate.FailuresByReason),
		DispatchLatency:     toJSONLatency(aggregate.DispatchDuration),
		ProviderLatency:     toJSONLatency(aggregate.ProviderDuration),
	}
}

func cloneMetricFailures(failures map[string]float64) map[string]float64 {
	if len(failures) == 0 {
		return map[string]float64{}
	}
	clone := make(map[string]float64, len(failures))
	for key, value := range failures {
		clone[key] = value
	}
	return clone
}

func toJSONLatency(duration *factoryvisualization.RuntimeMetricsDuration) metricsJSONLatency {
	if duration == nil {
		return metricsJSONLatency{Unit: "milliseconds"}
	}
	unit := duration.Unit
	if strings.TrimSpace(unit) == "" {
		unit = "milliseconds"
	}
	if duration.Samples == 0 {
		return metricsJSONLatency{Unit: unit}
	}
	return metricsJSONLatency{
		Unit:    unit,
		Samples: duration.Samples,
		P50:     duration.P50,
		P95:     duration.P95,
	}
}

func metricsBreakdowns(
	groupBy string,
	result factoryvisualization.RuntimeMetricsQueryResult,
) []factoryvisualization.RuntimeMetricsBreakdown {
	var source []factoryvisualization.RuntimeMetricsBreakdown
	switch groupBy {
	case metricsGroupByWorker:
		source = result.WorkerTypes
	case metricsGroupByProvider:
		source = result.Providers
	default:
		source = result.Workstations
	}
	breakdowns := append([]factoryvisualization.RuntimeMetricsBreakdown(nil), source...)
	sort.SliceStable(breakdowns, func(i, j int) bool {
		return breakdowns[i].Key < breakdowns[j].Key
	})
	return breakdowns
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
