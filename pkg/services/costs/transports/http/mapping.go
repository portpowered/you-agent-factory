package http

import (
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func reportToAPI(report costs.Report) factoryapi.CostsReport {
	return factoryapi.CostsReport{
		Scope:           scopeToAPI(report.Scope),
		Currency:        factoryapi.CostsReportCurrency(report.Currency),
		Status:          factoryapi.CostsReportStatus(report.Status),
		PricedSubtotal:  report.PricedSubtotal,
		Coverage:        coverageToAPI(report.Coverage),
		LineItems:       lineItemsToAPI(report.LineItems),
		WorkItems:       rollupsToAPI(report.WorkItems),
		WorkerSessions:  rollupsToAPI(report.WorkerSessions),
		ProviderModels:  providerRollupsToAPI(report.ProviderModels),
		FactorySessions: rollupsToAPI(report.FactorySessions),
	}
}

func scopeToAPI(scope costs.Scope) factoryapi.CostsScope {
	result := factoryapi.CostsScope{Kind: factoryapi.CostsScopeKind(scope.Kind)}
	if scope.FactorySessionID != "" {
		value := scope.FactorySessionID
		result.FactorySessionId = &value
	}
	return result
}

func coverageToAPI(coverage costs.Coverage) factoryapi.CostsCoverage {
	return factoryapi.CostsCoverage{
		EncounteredRows:           coverage.EncounteredRows,
		PricedRows:                coverage.PricedRows,
		UnpricedRows:              coverage.UnpricedRows,
		EncounteredProviderModels: coverage.EncounteredProviderModels,
		PricedProviderModels:      coverage.PricedProviderModels,
		UnpricedProviderModels:    coverage.UnpricedProviderModels,
	}
}

func lineItemsToAPI(items []costs.LineItem) []factoryapi.CostsLineItem {
	result := make([]factoryapi.CostsLineItem, 0, len(items))
	for _, item := range items {
		result = append(result, factoryapi.CostsLineItem{
			FactorySessionId:      optionalString(item.FactorySessionID),
			WorkId:                optionalString(item.WorkID),
			DispatchId:            optionalString(item.DispatchID),
			WorkerSessionId:       optionalString(item.WorkerSessionID),
			Provider:              optionalString(item.Provider),
			Model:                 optionalString(item.Model),
			InputTokens:           item.InputTokens,
			OutputTokens:          item.OutputTokens,
			CachedInputTokens:     item.CachedInputTokens,
			ReasoningOutputTokens: item.ReasoningOutputTokens,
			Status:                factoryapi.CostsLineItemStatus(item.Status),
			PricedAmount:          item.PricedAmount,
			Reason:                optionalString(item.Reason),
		})
	}
	return result
}

func rollupsToAPI(rollups []costs.Rollup) []factoryapi.CostsRollup {
	result := make([]factoryapi.CostsRollup, 0, len(rollups))
	for _, rollup := range rollups {
		result = append(result, rollupToAPI(rollup))
	}
	return result
}

func providerRollupsToAPI(rollups []costs.ProviderModelRollup) []factoryapi.CostsProviderModelRollup {
	result := make([]factoryapi.CostsProviderModelRollup, 0, len(rollups))
	for _, rollup := range rollups {
		mapped := rollupToAPI(rollup.Rollup)
		result = append(result, factoryapi.CostsProviderModelRollup{
			Provider:              rollup.Provider,
			Model:                 rollup.Model,
			Key:                   mapped.Key,
			InputTokens:           mapped.InputTokens,
			OutputTokens:          mapped.OutputTokens,
			CachedInputTokens:     mapped.CachedInputTokens,
			ReasoningOutputTokens: mapped.ReasoningOutputTokens,
			Status:                factoryapi.CostsProviderModelRollupStatus(mapped.Status),
			PricedSubtotal:        mapped.PricedSubtotal,
			Coverage:              mapped.Coverage,
		})
	}
	return result
}

func rollupToAPI(rollup costs.Rollup) factoryapi.CostsRollup {
	return factoryapi.CostsRollup{
		Key:                   rollup.Key,
		InputTokens:           rollup.InputTokens,
		OutputTokens:          rollup.OutputTokens,
		CachedInputTokens:     rollup.CachedInputTokens,
		ReasoningOutputTokens: rollup.ReasoningOutputTokens,
		Status:                factoryapi.CostsRollupStatus(rollup.Status),
		PricedSubtotal:        rollup.PricedSubtotal,
		Coverage:              coverageToAPI(rollup.Coverage),
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
