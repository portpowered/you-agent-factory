package service

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"

	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type normalizedPrice struct {
	input     *big.Rat
	output    *big.Rat
	cached    *big.Rat
	reasoning *big.Rat
	source    costs.PriceSource
}

type priceIndex map[string]normalizedPrice

type priceEntry struct {
	provider  string
	model     string
	input     string
	output    string
	cached    *string
	reasoning *string
	source    costs.PriceSource
}

type summary struct {
	amount                 *big.Rat
	coverage               costs.Coverage
	tokens                 tokenAccumulator
	pairs                  map[string]*pairCoverage
	unpricedDispatches     map[string]struct{}
	unpricedDispatchesNoID int
	unpricedPairs          map[string]*unpricedPairCoverage
}

type pairCoverage struct {
	provider string
	model    string
	unpriced bool
}

type unpricedPairCoverage struct {
	provider            *string
	model               *string
	dispatches          map[string]struct{}
	dispatchesWithoutID int
}

type tokenAccumulator struct {
	input     *int64
	output    *int64
	cached    *int64
	reasoning *int64
}

func calculateReport(
	ctx context.Context,
	builtInTable providers.PriceTable,
	operatorTable operatorsettings.PriceTable,
	rows []factoryvisualization.RuntimeMetricsUsageRow,
	scope costs.Scope,
) (costs.Report, error) {
	prices, err := buildPriceIndex(builtInTable)
	if err != nil {
		return costs.Report{}, err
	}
	operatorPrices, err := buildOperatorPriceIndex(operatorTable)
	if err != nil {
		return costs.Report{}, err
	}
	for key, price := range operatorPrices {
		// Operator rows are complete replacements. In particular, an omitted
		// optional subclass must not inherit a rate from the built-in row.
		prices[key] = price
	}
	report := costs.Report{
		Scope:           scope,
		Currency:        builtInTable.Currency,
		UnpricedPairs:   []costs.UnpricedPair{},
		LineItems:       []costs.LineItem{},
		WorkItems:       []costs.Rollup{},
		WorkerSessions:  []costs.Rollup{},
		ProviderModels:  []costs.ProviderModelRollup{},
		FactorySessions: []costs.Rollup{},
	}
	overall := newSummary()
	work := map[string]*summary{}
	workerSessions := map[string]*summary{}
	providerModels := map[string]*summary{}
	factorySessions := map[string]*summary{}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return costs.Report{}, err
		}
		line := runtimeUsageFromMetrics(row)
		amount, source, reason := valueLine(line, prices)
		if reason == "" {
			line.Status = costs.StatusPriced
			line.PriceSource = source
			line.PricedAmount, err = exactAmountString(amount)
			if err != nil {
				return costs.Report{}, err
			}
		} else {
			line.Status = costs.StatusUnpriced
			line.Reason = reason
		}
		report.LineItems = append(report.LineItems, line)
		priced := reason == ""
		overall.add(line, amount, priced)
		addDimension(work, line.WorkID, line, amount, priced)
		addDimension(workerSessions, line.WorkerSessionID, line, amount, priced)
		addDimension(factorySessions, line.FactorySessionID, line, amount, priced)
		if provider, model := strings.TrimSpace(line.Provider), strings.TrimSpace(line.Model); provider != "" && model != "" {
			addDimension(providerModels, providerModelKey(provider, model), line, amount, priced)
		}
	}
	sort.SliceStable(report.LineItems, func(i, j int) bool {
		return lineItemSortKey(report.LineItems[i]) < lineItemSortKey(report.LineItems[j])
	})

	report.Status, report.PricedSubtotal, report.Coverage, err = overall.result()
	if err != nil {
		return costs.Report{}, err
	}
	report.KnownCost = report.PricedSubtotal
	report.TokenTotals = overall.tokens.totals()
	report.UnpricedDispatchCount = overall.unpricedDispatchCount()
	report.UnpricedPairs = overall.unpricedPairsValue()
	report.WorkItems, err = rollups(work, builtInTable.Currency)
	if err != nil {
		return costs.Report{}, err
	}
	report.WorkerSessions, err = rollups(workerSessions, builtInTable.Currency)
	if err != nil {
		return costs.Report{}, err
	}
	report.FactorySessions, err = rollups(factorySessions, builtInTable.Currency)
	if err != nil {
		return costs.Report{}, err
	}
	report.ProviderModels, err = providerRollups(providerModels, builtInTable.Currency)
	if err != nil {
		return costs.Report{}, err
	}
	return report, nil
}

func buildPriceIndex(table providers.PriceTable) (priceIndex, error) {
	entries := make([]priceEntry, 0, len(table.Models))
	for _, model := range table.Models {
		entries = append(entries, priceEntry{
			provider:  model.Provider.String(),
			model:     model.Model,
			input:     model.InputPerMillionTokens,
			output:    model.OutputPerMillionTokens,
			cached:    model.CachedInputPerMillionTokens,
			reasoning: model.ReasoningOutputPerMillionTokens,
			source:    costs.PriceSourceBuiltIn,
		})
	}
	return buildPriceIndexFromEntries(entries)
}

func buildOperatorPriceIndex(table operatorsettings.PriceTable) (priceIndex, error) {
	entries := make([]priceEntry, 0, len(table.Models))
	for _, model := range table.Models {
		entries = append(entries, priceEntry{
			provider:  model.Provider,
			model:     model.Model,
			input:     model.InputPerMillionTokens,
			output:    model.OutputPerMillionTokens,
			cached:    model.CachedInputPerMillionTokens,
			reasoning: model.ReasoningOutputPerMillionTokens,
			source:    costs.PriceSourceOperatorSupplied,
		})
	}
	return buildPriceIndexFromEntries(entries)
}

func buildPriceIndexFromEntries(entries []priceEntry) (priceIndex, error) {
	index := make(priceIndex, len(entries))
	for _, entry := range entries {
		input, err := parseDecimal(entry.input)
		if err != nil {
			return nil, fmt.Errorf("parse input rate for %q/%q: %w", entry.provider, entry.model, err)
		}
		output, err := parseDecimal(entry.output)
		if err != nil {
			return nil, fmt.Errorf("parse output rate for %q/%q: %w", entry.provider, entry.model, err)
		}
		price := normalizedPrice{input: input, output: output, source: entry.source}
		if entry.cached != nil {
			price.cached, err = parseDecimal(*entry.cached)
			if err != nil {
				return nil, fmt.Errorf("parse cached-input rate for %q/%q: %w", entry.provider, entry.model, err)
			}
		}
		if entry.reasoning != nil {
			price.reasoning, err = parseDecimal(*entry.reasoning)
			if err != nil {
				return nil, fmt.Errorf("parse reasoning-output rate for %q/%q: %w", entry.provider, entry.model, err)
			}
		}
		index[providerModelKey(entry.provider, entry.model)] = price
	}
	return index, nil
}

func valueLine(line costs.LineItem, prices priceIndex) (*big.Rat, costs.PriceSource, string) {
	provider := canonicalProvider(line.Provider)
	model := strings.TrimSpace(line.Model)
	price, reason := validateLine(line, prices, provider, model)
	if reason != "" {
		return nil, "", reason
	}
	cached, reason := validateSubset("cached-input", "input", line.CachedInputTokens, line.InputTokens, price.cached, provider, model)
	if reason != "" {
		return nil, "", reason
	}
	reasoning, reason := validateSubset("reasoning-output", "output", line.ReasoningOutputTokens, line.OutputTokens, price.reasoning, provider, model)
	if reason != "" {
		return nil, "", reason
	}

	uncachedInput := *line.InputTokens - cached
	nonReasoningOutput := *line.OutputTokens - reasoning
	amount := new(big.Rat).Mul(big.NewRat(uncachedInput, 1), price.input)
	amount.Add(amount, new(big.Rat).Mul(big.NewRat(cached, 1), price.cachedOrZero()))
	amount.Add(amount, new(big.Rat).Mul(big.NewRat(nonReasoningOutput, 1), price.output))
	amount.Add(amount, new(big.Rat).Mul(big.NewRat(reasoning, 1), price.reasoningOrZero()))
	return amount.Quo(amount, big.NewRat(millionTokenCount, 1)), price.source, ""
}

func validateLine(
	line costs.LineItem,
	prices priceIndex,
	provider string,
	model string,
) (normalizedPrice, string) {
	if provider == "" {
		return normalizedPrice{}, "provider identity is unavailable"
	}
	if model == "" {
		return normalizedPrice{}, "model identity is unavailable"
	}
	price, ok := prices[providerModelKey(provider, model)]
	if !ok {
		return normalizedPrice{}, fmt.Sprintf("no configured price for provider/model %q/%q", provider, model)
	}
	if line.InputTokens == nil {
		return normalizedPrice{}, "input token count is unavailable"
	}
	if line.OutputTokens == nil {
		return normalizedPrice{}, "output token count is unavailable"
	}
	if *line.InputTokens < 0 {
		return normalizedPrice{}, "input token count is negative"
	}
	if *line.OutputTokens < 0 {
		return normalizedPrice{}, "output token count is negative"
	}
	return price, ""
}

func validateSubset(
	class string,
	totalClass string,
	measured *int64,
	total *int64,
	rate *big.Rat,
	provider string,
	model string,
) (int64, string) {
	if measured == nil {
		return 0, ""
	}
	value := *measured
	if value < 0 {
		return 0, class + " token count is negative"
	}
	if value > *total {
		return 0, class + " token count exceeds " + totalClass + " token count"
	}
	if rate == nil {
		return 0, fmt.Sprintf("%s rate is not configured for provider/model %q/%q", class, provider, model)
	}
	return value, ""
}

func (price normalizedPrice) cachedOrZero() *big.Rat {
	if price.cached == nil {
		return new(big.Rat)
	}
	return price.cached
}

func (price normalizedPrice) reasoningOrZero() *big.Rat {
	if price.reasoning == nil {
		return new(big.Rat)
	}
	return price.reasoning
}

func exactAmountString(amount *big.Rat) (*string, error) {
	formatted, err := formatExactDecimal(amount)
	if err != nil {
		return nil, err
	}
	return &formatted, nil
}

func newSummary() *summary {
	return &summary{
		pairs:              make(map[string]*pairCoverage),
		unpricedDispatches: make(map[string]struct{}),
		unpricedPairs:      make(map[string]*unpricedPairCoverage),
	}
}

func (target *summary) add(line costs.LineItem, amount *big.Rat, priced bool) {
	target.coverage.EncounteredRows++
	if priced {
		target.coverage.PricedRows++
	} else {
		target.coverage.UnpricedRows++
	}
	target.tokens.add(line.TokenCounts)
	if !priced {
		if dispatchID := strings.TrimSpace(line.DispatchID); dispatchID == "" {
			target.unpricedDispatchesNoID++
		} else {
			target.unpricedDispatches[dispatchID] = struct{}{}
		}
		target.addUnpricedPair(line)
	}
	provider, model := canonicalProvider(line.Provider), strings.TrimSpace(line.Model)
	if provider == "" || model == "" {
		return
	}
	key := providerModelKey(provider, model)
	pair := target.pairs[key]
	if pair == nil {
		pair = &pairCoverage{provider: provider, model: model}
		target.pairs[key] = pair
		target.coverage.EncounteredProviderModels++
	}
	if !priced {
		pair.unpriced = true
	}
	if priced {
		if target.amount == nil {
			target.amount = new(big.Rat)
		}
		target.amount.Add(target.amount, amount)
	}
}

func (target *summary) result() (costs.Status, *string, costs.Coverage, error) {
	target.coverage.PricedProviderModels = 0
	target.coverage.UnpricedProviderModels = 0
	for _, pair := range target.pairs {
		if !pair.unpriced {
			target.coverage.PricedProviderModels++
		} else {
			target.coverage.UnpricedProviderModels++
		}
	}
	status := costs.StatusNoUsage
	switch {
	case target.coverage.PricedRows == target.coverage.EncounteredRows && target.coverage.EncounteredRows > 0:
		status = costs.StatusPriced
	case target.coverage.PricedRows == 0 && target.coverage.EncounteredRows > 0:
		status = costs.StatusUnpriced
	case target.coverage.EncounteredRows > 0:
		status = costs.StatusPartial
	}
	var amount *string
	if target.amount != nil {
		formatted, err := formatExactDecimal(target.amount)
		if err != nil {
			return "", nil, costs.Coverage{}, err
		}
		amount = &formatted
	}
	return status, amount, target.coverage, nil
}

func (target *summary) addUnpricedPair(line costs.LineItem) {
	provider := canonicalProvider(line.Provider)
	model := strings.TrimSpace(line.Model)
	key := provider + "\x00" + model
	pair := target.unpricedPairs[key]
	if pair == nil {
		pair = &unpricedPairCoverage{
			provider:   optionalIdentity(provider),
			model:      optionalIdentity(model),
			dispatches: make(map[string]struct{}),
		}
		target.unpricedPairs[key] = pair
	}
	if dispatchID := strings.TrimSpace(line.DispatchID); dispatchID == "" {
		pair.dispatchesWithoutID++
	} else {
		pair.dispatches[dispatchID] = struct{}{}
	}
}

func (target *summary) unpricedDispatchCount() int {
	return len(target.unpricedDispatches) + target.unpricedDispatchesNoID
}

func (target *summary) unpricedPairsValue() []costs.UnpricedPair {
	keys := make([]string, 0, len(target.unpricedPairs))
	for key := range target.unpricedPairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]costs.UnpricedPair, 0, len(keys))
	for _, key := range keys {
		pair := target.unpricedPairs[key]
		result = append(result, costs.UnpricedPair{
			Provider:      cloneStringPointer(pair.provider),
			Model:         cloneStringPointer(pair.model),
			DispatchCount: len(pair.dispatches) + pair.dispatchesWithoutID,
		})
	}
	return result
}

func (target *summary) rollup(key string) (costs.Rollup, error) {
	return target.rollupWithCurrency(key, "")
}

func (target *summary) rollupWithCurrency(key, currency string) (costs.Rollup, error) {
	status, amount, coverage, err := target.result()
	if err != nil {
		return costs.Rollup{}, err
	}
	return costs.Rollup{
		Key:                   key,
		TokenCounts:           target.tokens.value(),
		Currency:              currency,
		Status:                status,
		KnownCost:             amount,
		PricedSubtotal:        amount,
		TokenTotals:           target.tokens.totals(),
		UnpricedDispatchCount: target.unpricedDispatchCount(),
		UnpricedPairs:         target.unpricedPairsValue(),
		Coverage:              coverage,
	}, nil
}

func addDimension(groups map[string]*summary, key string, line costs.LineItem, amount *big.Rat, priced bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	target := groups[key]
	if target == nil {
		target = newSummary()
		groups[key] = target
	}
	target.add(line, amount, priced)
}

func rollups(groups map[string]*summary, currency string) ([]costs.Rollup, error) {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]costs.Rollup, 0, len(keys))
	for _, key := range keys {
		rollup, err := groups[key].rollupWithCurrency(key, currency)
		if err != nil {
			return nil, err
		}
		result = append(result, rollup)
	}
	return result, nil
}

func providerRollups(groups map[string]*summary, currency string) ([]costs.ProviderModelRollup, error) {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]costs.ProviderModelRollup, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		pair := group.pairs[key]
		rollup, err := group.rollupWithCurrency(publicProviderModelKey(pair.provider, pair.model), currency)
		if err != nil {
			return nil, err
		}
		result = append(result, costs.ProviderModelRollup{Provider: pair.provider, Model: pair.model, Rollup: rollup})
	}
	return result, nil
}

func (tokens *tokenAccumulator) add(value costs.TokenCounts) {
	tokens.input = addToken(tokens.input, value.InputTokens)
	tokens.output = addToken(tokens.output, value.OutputTokens)
	tokens.cached = addToken(tokens.cached, value.CachedInputTokens)
	tokens.reasoning = addToken(tokens.reasoning, value.ReasoningOutputTokens)
}

func (tokens tokenAccumulator) value() costs.TokenCounts {
	return costs.TokenCounts{
		InputTokens:           cloneInt64(tokens.input),
		OutputTokens:          cloneInt64(tokens.output),
		CachedInputTokens:     cloneInt64(tokens.cached),
		ReasoningOutputTokens: cloneInt64(tokens.reasoning),
	}
}

func (tokens tokenAccumulator) totals() costs.TokenTotals {
	input := cloneInt64(tokens.input)
	output := cloneInt64(tokens.output)
	var total *int64
	if input != nil && output != nil {
		value := *input + *output
		total = &value
	}
	return costs.TokenTotals{
		TotalTokens:           total,
		InputTokens:           input,
		OutputTokens:          output,
		CachedInputTokens:     cloneInt64(tokens.cached),
		ReasoningOutputTokens: cloneInt64(tokens.reasoning),
	}
}

func addToken(current, value *int64) *int64 {
	if value == nil {
		return current
	}
	if current == nil {
		cloned := *value
		return &cloned
	}
	*current += *value
	return current
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func optionalIdentity(value string) *string {
	if value == "" {
		return nil
	}
	cloned := value
	return &cloned
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func providerModelKey(provider, model string) string {
	provider = canonicalProvider(provider)
	return provider + "\x00" + strings.TrimSpace(model)
}

func publicProviderModelKey(provider, model string) string {
	return canonicalProvider(provider) + "/" + strings.TrimSpace(model)
}

func canonicalProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if canonical, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(provider); ok {
		return canonical
	}
	return provider
}

func lineItemSortKey(line costs.LineItem) string {
	return strings.Join([]string{
		line.FactorySessionID,
		line.WorkID,
		line.WorkerSessionID,
		line.DispatchID,
		line.Provider,
		line.Model,
		tokenSortValue(line.InputTokens),
		tokenSortValue(line.OutputTokens),
		tokenSortValue(line.CachedInputTokens),
		tokenSortValue(line.ReasoningOutputTokens),
		string(line.Status),
		line.Reason,
	}, "\x00")
}

func tokenSortValue(value *int64) string {
	if value == nil {
		return "<absent>"
	}
	return fmt.Sprintf("%020d", *value)
}

func runtimeUsageFromMetrics(row factoryvisualization.RuntimeMetricsUsageRow) costs.LineItem {
	return costs.LineItem{
		FactorySessionID: row.FactorySessionID,
		WorkID:           row.WorkID,
		DispatchID:       row.DispatchID,
		WorkerSessionID:  row.WorkerSessionID,
		Provider:         canonicalProvider(row.Provider),
		Model:            row.Model,
		TokenCounts: costs.TokenCounts{
			InputTokens:           cloneInt64Pointer(row.InputTokens),
			OutputTokens:          cloneInt64Pointer(row.OutputTokens),
			CachedInputTokens:     cloneInt64Pointer(row.CachedInputTokens),
			ReasoningOutputTokens: cloneInt64Pointer(row.ReasoningOutputTokens),
		},
	}
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
