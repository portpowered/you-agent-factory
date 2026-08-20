package events

import (
	"math"
	"strconv"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// dispatchUsageEventPayload derives the usage facts that belong on the
// existing Petri DISPATCH_RESPONSE. Duration comes from the completed
// dispatch; token facts come only from normalized provider response metadata.
func dispatchUsageEventPayload(
	result workers.WorkResult,
	completed interfaces.CompletedDispatch,
) *workers.DispatchUsageEventPayload {
	durationMillis := completed.Duration.Milliseconds()
	usage := &workers.DispatchUsageEventPayload{DurationMillis: &durationMillis}
	inputTokens, hasInput := providerResponseToken(result.Diagnostics, workers.ProviderResponseMetadataInputTokens)
	outputTokens, hasOutput := providerResponseToken(result.Diagnostics, workers.ProviderResponseMetadataOutputTokens)
	if hasInput {
		usage.InputTokens = &inputTokens
	}
	if hasOutput {
		usage.OutputTokens = &outputTokens
	}
	if hasInput && hasOutput && inputTokens <= math.MaxInt64-outputTokens {
		totalTokens := inputTokens + outputTokens
		usage.TotalTokens = &totalTokens
	}
	return usage
}

func providerResponseToken(diagnostics *workers.WorkDiagnostics, key string) (int64, bool) {
	if diagnostics == nil || diagnostics.Provider == nil || diagnostics.Provider.ResponseMetadata == nil {
		return 0, false
	}
	value := strings.TrimSpace(diagnostics.Provider.ResponseMetadata[key])
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}
