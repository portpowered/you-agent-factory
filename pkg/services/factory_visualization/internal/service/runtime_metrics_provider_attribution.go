package service

import (
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const unavailableMetricProviderKey = "unavailable"

type metricProviderAttribution struct {
	byDispatch map[string]metricDispatchProvider
}

type metricDispatchProvider struct {
	provider      string
	authoritative bool
	conflict      bool
}

type metricProviderEvidence struct {
	providers           map[string]struct{}
	completionProviders map[string]struct{}
}

func newMetricProviderAttribution(records []RuntimeMetricRecord) metricProviderAttribution {
	evidenceByDispatch := make(map[string]*metricProviderEvidence)
	for _, record := range records {
		if !isProviderAttributionMetric(recordStringValue(record, "metric_name")) {
			continue
		}
		dispatchID := recordStringValue(record, "dispatch_id")
		if dispatchID == "" {
			continue
		}
		provider := concreteMetricProvider(recordStringValue(record, "provider"))
		if provider == "" {
			continue
		}
		evidence := evidenceByDispatch[dispatchID]
		if evidence == nil {
			evidence = &metricProviderEvidence{
				providers:           make(map[string]struct{}),
				completionProviders: make(map[string]struct{}),
			}
			evidenceByDispatch[dispatchID] = evidence
		}
		evidence.providers[provider] = struct{}{}
		if recordStringValue(record, "metric_name") == factoryruntime.RuntimeDispatchComplete {
			evidence.completionProviders[provider] = struct{}{}
		}
	}

	attribution := metricProviderAttribution{
		byDispatch: make(map[string]metricDispatchProvider, len(evidenceByDispatch)),
	}
	for dispatchID, evidence := range evidenceByDispatch {
		completionProviders := metricProviderKeys(evidence.completionProviders)
		providers := metricProviderKeys(evidence.providers)
		switch {
		case len(completionProviders) == 1:
			attribution.byDispatch[dispatchID] = metricDispatchProvider{
				provider:      completionProviders[0],
				authoritative: true,
			}
		case len(completionProviders) > 1:
			attribution.byDispatch[dispatchID] = metricDispatchProvider{conflict: true}
		case len(providers) == 1:
			attribution.byDispatch[dispatchID] = metricDispatchProvider{provider: providers[0]}
		default:
			attribution.byDispatch[dispatchID] = metricDispatchProvider{conflict: len(providers) > 1}
		}
	}
	return attribution
}

func (attribution metricProviderAttribution) providerFor(record RuntimeMetricRecord) string {
	provider := concreteMetricProvider(recordStringValue(record, "provider"))
	dispatchID := recordStringValue(record, "dispatch_id")
	if dispatchID == "" {
		if provider != "" {
			return provider
		}
		return unavailableMetricProviderKey
	}

	dispatchProvider, ok := attribution.byDispatch[dispatchID]
	if !ok {
		if provider != "" {
			return provider
		}
		return unavailableMetricProviderKey
	}
	if dispatchProvider.authoritative && dispatchProvider.provider != "" {
		return dispatchProvider.provider
	}
	if dispatchProvider.conflict || dispatchProvider.provider == "" {
		return unavailableMetricProviderKey
	}
	return dispatchProvider.provider
}

func providerForUsageRecord(
	attribution metricProviderAttribution,
	record RuntimeMetricRecord,
) string {
	provider := attribution.providerFor(record)
	if provider == unavailableMetricProviderKey &&
		strings.TrimSpace(recordStringValue(record, "provider")) == "" &&
		strings.TrimSpace(recordStringValue(record, "dispatch_id")) == "" {
		return ""
	}
	return provider
}

func isProviderAttributionMetric(name string) bool {
	switch name {
	case factoryruntime.RuntimeProviderInputTokens,
		factoryruntime.RuntimeProviderOutputTokens,
		factoryruntime.RuntimeProviderCachedInputTokens,
		factoryruntime.RuntimeProviderReasoningOutputTokens,
		factoryruntime.RuntimeProviderComplete,
		factoryruntime.RuntimeProviderFailed,
		factoryruntime.RuntimeProviderDuration,
		factoryruntime.RuntimeDispatchComplete,
		factoryruntime.RuntimeDispatchDuration:
		return true
	default:
		return false
	}
}

func concreteMetricProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" || strings.Contains(provider, "${") {
		return ""
	}
	return provider
}

func metricProviderKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
