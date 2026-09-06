package cli

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

func renderCostsOutput(report generatedclient.CostsReport, jsonOutput bool) (string, error) {
	if !jsonOutput {
		return renderHumanCosts(report), nil
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("encode metrics costs JSON: %w", err)
	}
	return string(append(encoded, '\n')), nil
}

// RenderHumanReport exposes the Costs-owned human representation to composed
// commands while keeping the standalone `you metrics costs` output unchanged.
func RenderHumanReport(report generatedclient.CostsReport) string {
	return renderHumanCosts(report)
}

func renderHumanCosts(report generatedclient.CostsReport) string {
	var output strings.Builder
	renderCostsScope(&output, report.Scope)
	fmt.Fprintf(&output, "Currency: %s\n", report.Currency)
	fmt.Fprintf(&output, "Status: %s\n", report.Status)
	renderCostSummary(&output, "", string(report.Status), report.KnownCost)
	renderPricedAmount(&output, "Priced subtotal", report.PricedSubtotal)
	renderTokenCounts(&output, "", report.TokenTotals)
	renderCoverage(&output, report.Coverage)
	renderUnpricedCoverage(&output, report.UnpricedDispatchCount, report.UnpricedPairs)
	fmt.Fprintln(&output)

	renderRollupDimension(&output, "Work items", rollupViews(report.WorkItems))
	renderRollupDimension(&output, "Worker Sessions", rollupViews(report.WorkerSessions))
	renderRollupDimension(&output, "Provider/models", providerModelViews(report.ProviderModels))
	renderRollupDimension(&output, "Factory Sessions", rollupViews(report.FactorySessions))
	renderLineItems(&output, report.LineItems)
	return output.String()
}

func renderCostsScope(output *strings.Builder, scope generatedclient.CostsScope) {
	if scope.FactorySessionId != nil && strings.TrimSpace(*scope.FactorySessionId) != "" {
		fmt.Fprintf(output, "Scope: Factory Session %s\n", *scope.FactorySessionId)
		return
	}
	fmt.Fprintln(output, "Scope: all Factory Sessions")
}

func renderPricedAmount(output *strings.Builder, label string, amount *string) {
	if amount == nil {
		fmt.Fprintf(output, "%s (USD): unavailable\n", label)
		return
	}
	fmt.Fprintf(output, "%s (USD): %s\n", label, *amount)
}

func renderCostSummary(output *strings.Builder, indent, status string, knownCost *string) {
	switch status {
	case "PARTIAL":
		fmt.Fprintf(output, "%sCost (USD): %s + ?? unknown\n", indent, formatHumanUSD(knownCost))
	case "UNPRICED":
		fmt.Fprintf(output, "%sCost (USD): ?? unknown\n", indent)
	case "NO_USAGE":
		fmt.Fprintf(output, "%sCost (USD): no usage\n", indent)
	default:
		fmt.Fprintf(output, "%sCost (USD): %s\n", indent, formatHumanUSD(knownCost))
	}
}

// formatHumanUSD rounds an exact API decimal to cents with math/big's
// deterministic half-away-from-zero rule. The API retains the unrounded
// decimal; this conversion is only for human-readable USD output.
func formatHumanUSD(amount *string) string {
	if amount == nil {
		return "?? unknown"
	}
	raw := strings.TrimSpace(*amount)
	value, ok := new(big.Rat).SetString(raw)
	if !ok || value.Sign() < 0 {
		return "?? unknown"
	}
	return "$" + value.FloatString(2)
}

func renderCoverage(output *strings.Builder, coverage generatedclient.CostsCoverage) {
	fmt.Fprintf(output, "Coverage: rows priced %d/%d; provider/models priced %d/%d\n",
		coverage.PricedRows, coverage.EncounteredRows,
		coverage.PricedProviderModels, coverage.EncounteredProviderModels)
}

type humanRollup struct {
	Key            string
	Status         string
	KnownCost      *string
	PricedSubtotal *string
	Coverage       generatedclient.CostsCoverage
	TokenTotals    generatedclient.CostsTokenTotals
}

func rollupViews(rollups []generatedclient.CostsRollup) []humanRollup {
	views := make([]humanRollup, 0, len(rollups))
	for _, rollup := range rollups {
		views = append(views, humanRollup{
			Key: rollup.Key, Status: string(rollup.Status), KnownCost: rollup.KnownCost,
			PricedSubtotal: rollup.PricedSubtotal, Coverage: rollup.Coverage,
			TokenTotals: rollup.TokenTotals,
		})
	}
	return views
}

func providerModelViews(rollups []generatedclient.CostsProviderModelRollup) []humanRollup {
	views := make([]humanRollup, 0, len(rollups))
	for _, rollup := range rollups {
		views = append(views, humanRollup{
			Key: rollup.Provider + "/" + rollup.Model, Status: string(rollup.Status),
			KnownCost: rollup.KnownCost, PricedSubtotal: rollup.PricedSubtotal,
			Coverage: rollup.Coverage, TokenTotals: rollup.TokenTotals,
		})
	}
	return views
}

func renderRollupDimension(output *strings.Builder, name string, rollups []humanRollup) {
	fmt.Fprintf(output, "%s: %d\n", name, len(rollups))
	for _, rollup := range rollups {
		fmt.Fprintf(output, "  %s:\n", displayValue(rollup.Key))
		fmt.Fprintf(output, "    Status: %s\n", rollup.Status)
		renderCostSummary(output, "    ", rollup.Status, rollup.KnownCost)
		renderPricedAmount(output, "    Priced subtotal", rollup.PricedSubtotal)
		fmt.Fprintf(output, "    Coverage: rows priced %d/%d; provider/models priced %d/%d\n",
			rollup.Coverage.PricedRows, rollup.Coverage.EncounteredRows,
			rollup.Coverage.PricedProviderModels, rollup.Coverage.EncounteredProviderModels)
		renderTokenCounts(output, "    ", rollup.TokenTotals)
	}
}

func renderLineItems(output *strings.Builder, items []generatedclient.CostsLineItem) {
	pricedCount := 0
	for _, item := range items {
		if string(item.Status) == "PRICED" {
			pricedCount++
		}
	}
	fmt.Fprintf(output, "Priced usage: %d rows\n", pricedCount)
	for _, item := range items {
		if string(item.Status) != "PRICED" {
			continue
		}
		fmt.Fprintf(output, "  PRICED provider=%s model=%s\n",
			displayPointer(item.Provider), displayPointer(item.Model))
		renderPricedAmount(output, "    Priced amount", item.PricedAmount)
		fmt.Fprintf(output, "    Price source: %s\n", displayPriceSource(item.PriceSource))
		renderTokenCounts(output, "    ", generatedclient.CostsTokenTotals{
			TotalTokens:           sumTokenClasses(item.InputTokens, item.OutputTokens),
			InputTokens:           item.InputTokens,
			CachedInputTokens:     item.CachedInputTokens,
			OutputTokens:          item.OutputTokens,
			ReasoningOutputTokens: item.ReasoningOutputTokens,
		})
	}

	unpricedCount := 0
	for _, item := range items {
		if string(item.Status) == "UNPRICED" {
			unpricedCount++
		}
	}
	fmt.Fprintf(output, "Unpriced usage: %d rows\n", unpricedCount)
	for _, item := range items {
		if string(item.Status) != "UNPRICED" {
			continue
		}
		fmt.Fprintf(output, "  UNPRICED provider=%s model=%s\n",
			displayPointer(item.Provider), displayPointer(item.Model))
		if item.Reason != nil && strings.TrimSpace(*item.Reason) != "" {
			fmt.Fprintf(output, "    Reason: %s\n", *item.Reason)
		}
		renderTokenCounts(output, "    ", generatedclient.CostsTokenTotals{
			TotalTokens:           sumTokenClasses(item.InputTokens, item.OutputTokens),
			InputTokens:           item.InputTokens,
			CachedInputTokens:     item.CachedInputTokens,
			OutputTokens:          item.OutputTokens,
			ReasoningOutputTokens: item.ReasoningOutputTokens,
		})
	}
}

func displayPriceSource(source *generatedclient.CostsLineItemPriceSource) string {
	if source == nil || strings.TrimSpace(string(*source)) == "" {
		return "<unknown>"
	}
	return string(*source)
}

func renderUnpricedCoverage(output *strings.Builder, dispatchCount int, pairs []generatedclient.CostsUnpricedPair) {
	fmt.Fprintf(output, "Unpriced dispatches: %d\n", dispatchCount)
	fmt.Fprintf(output, "Unpriced provider/models: %d\n", len(pairs))
	for _, pair := range orderedUnpricedPairs(pairs) {
		fmt.Fprintf(output, "  %s/%s: %d dispatches\n",
			displayPointer(pair.Provider), displayPointer(pair.Model), pair.DispatchCount)
	}
}

func orderedUnpricedPairs(pairs []generatedclient.CostsUnpricedPair) []generatedclient.CostsUnpricedPair {
	ordered := append([]generatedclient.CostsUnpricedPair(nil), pairs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return unpricedPairKey(ordered[i]) < unpricedPairKey(ordered[j])
	})
	return ordered
}

func unpricedPairKey(pair generatedclient.CostsUnpricedPair) string {
	return displayPointer(pair.Provider) + "/" + displayPointer(pair.Model)
}

func renderTokenCounts(output *strings.Builder, indent string, totals generatedclient.CostsTokenTotals) {
	fmt.Fprintf(output, "%sTotal tokens: %s\n", indent, displayInt64(totals.TotalTokens))
	fmt.Fprintf(output, "%sInput tokens: %s\n", indent, displayInt64(totals.InputTokens))
	fmt.Fprintf(output, "%sCached-input tokens: %s\n", indent, displayInt64(totals.CachedInputTokens))
	fmt.Fprintf(output, "%sOutput tokens: %s\n", indent, displayInt64(totals.OutputTokens))
	fmt.Fprintf(output, "%sReasoning-output tokens: %s\n", indent, displayInt64(totals.ReasoningOutputTokens))
}

func sumTokenClasses(input, output *int64) *int64 {
	if input == nil || output == nil {
		return nil
	}
	total := *input + *output
	return &total
}

func displayInt64(value *int64) string {
	if value == nil {
		return "absent"
	}
	return fmt.Sprintf("%d", *value)
}

func displayPointer(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "<unknown>"
	}
	return *value
}

func displayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<unknown>"
	}
	return value
}
