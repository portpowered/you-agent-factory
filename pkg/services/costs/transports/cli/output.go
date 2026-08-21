package cli

import (
	"encoding/json"
	"fmt"
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

func renderHumanCosts(report generatedclient.CostsReport) string {
	var output strings.Builder
	renderCostsScope(&output, report.Scope)
	fmt.Fprintf(&output, "Currency: %s\n", report.Currency)
	fmt.Fprintf(&output, "Status: %s\n", report.Status)
	renderPricedAmount(&output, "Priced subtotal", report.PricedSubtotal)
	renderCoverage(&output, report.Coverage)
	fmt.Fprintln(&output)

	renderRollupDimension(&output, "Work items", rollupViews(report.WorkItems))
	renderRollupDimension(&output, "Worker Sessions", rollupViews(report.WorkerSessions))
	renderRollupDimension(&output, "Provider/models", providerModelViews(report.ProviderModels))
	renderRollupDimension(&output, "Factory Sessions", rollupViews(report.FactorySessions))
	renderUnpricedItems(&output, report.LineItems)
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

func renderCoverage(output *strings.Builder, coverage generatedclient.CostsCoverage) {
	fmt.Fprintf(output, "Coverage: rows priced %d/%d; provider/models priced %d/%d\n",
		coverage.PricedRows, coverage.EncounteredRows,
		coverage.PricedProviderModels, coverage.EncounteredProviderModels)
}

type humanRollup struct {
	Key                   string
	Status                string
	PricedSubtotal        *string
	Coverage              generatedclient.CostsCoverage
	InputTokens           *int64
	CachedInputTokens     *int64
	OutputTokens          *int64
	ReasoningOutputTokens *int64
}

func rollupViews(rollups []generatedclient.CostsRollup) []humanRollup {
	views := make([]humanRollup, 0, len(rollups))
	for _, rollup := range rollups {
		views = append(views, humanRollup{
			Key: rollup.Key, Status: string(rollup.Status), PricedSubtotal: rollup.PricedSubtotal,
			Coverage: rollup.Coverage, InputTokens: rollup.InputTokens,
			CachedInputTokens: rollup.CachedInputTokens, OutputTokens: rollup.OutputTokens,
			ReasoningOutputTokens: rollup.ReasoningOutputTokens,
		})
	}
	return views
}

func providerModelViews(rollups []generatedclient.CostsProviderModelRollup) []humanRollup {
	views := make([]humanRollup, 0, len(rollups))
	for _, rollup := range rollups {
		views = append(views, humanRollup{
			Key: rollup.Provider + "/" + rollup.Model, Status: string(rollup.Status),
			PricedSubtotal: rollup.PricedSubtotal, Coverage: rollup.Coverage,
			InputTokens: rollup.InputTokens, CachedInputTokens: rollup.CachedInputTokens,
			OutputTokens: rollup.OutputTokens, ReasoningOutputTokens: rollup.ReasoningOutputTokens,
		})
	}
	return views
}

func renderRollupDimension(output *strings.Builder, name string, rollups []humanRollup) {
	fmt.Fprintf(output, "%s: %d\n", name, len(rollups))
	for _, rollup := range rollups {
		fmt.Fprintf(output, "  %s:\n", displayValue(rollup.Key))
		fmt.Fprintf(output, "    Status: %s\n", rollup.Status)
		renderPricedAmount(output, "    Priced subtotal", rollup.PricedSubtotal)
		fmt.Fprintf(output, "    Coverage: rows priced %d/%d; provider/models priced %d/%d\n",
			rollup.Coverage.PricedRows, rollup.Coverage.EncounteredRows,
			rollup.Coverage.PricedProviderModels, rollup.Coverage.EncounteredProviderModels)
		renderTokenCounts(output, "    ", rollup.InputTokens, rollup.CachedInputTokens, rollup.OutputTokens, rollup.ReasoningOutputTokens)
	}
}

func renderUnpricedItems(output *strings.Builder, items []generatedclient.CostsLineItem) {
	count := 0
	for _, item := range items {
		if string(item.Status) == "UNPRICED" {
			count++
		}
	}
	fmt.Fprintf(output, "Unpriced usage: %d rows\n", count)
	for _, item := range items {
		if string(item.Status) != "UNPRICED" {
			continue
		}
		fmt.Fprintf(output, "  UNPRICED provider=%s model=%s\n",
			displayPointer(item.Provider), displayPointer(item.Model))
		if item.Reason != nil && strings.TrimSpace(*item.Reason) != "" {
			fmt.Fprintf(output, "    Reason: %s\n", *item.Reason)
		}
		renderTokenCounts(output, "    ", item.InputTokens, item.CachedInputTokens, item.OutputTokens, item.ReasoningOutputTokens)
	}
}

func renderTokenCounts(output *strings.Builder, indent string, input, cached, outputTokens, reasoning *int64) {
	fmt.Fprintf(output, "%sInput tokens: %s\n", indent, displayInt64(input))
	fmt.Fprintf(output, "%sCached-input tokens: %s\n", indent, displayInt64(cached))
	fmt.Fprintf(output, "%sOutput tokens: %s\n", indent, displayInt64(outputTokens))
	fmt.Fprintf(output, "%sReasoning-output tokens: %s\n", indent, displayInt64(reasoning))
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
