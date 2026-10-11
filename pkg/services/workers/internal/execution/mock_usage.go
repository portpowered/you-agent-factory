package workerexecution

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// mockWorkerUsageObservationPayload is deliberately pointer-based so the
// canonical Worker draft preserves omitted token classes while retaining
// explicit zeroes.
type mockWorkerUsageObservationPayload struct {
	Origin                string `json:"origin,omitempty"`
	InputTokens           *int64 `json:"inputTokens,omitempty"`
	CachedInputTokens     *int64 `json:"cachedInputTokens,omitempty"`
	OutputTokens          *int64 `json:"outputTokens,omitempty"`
	ReasoningOutputTokens *int64 `json:"reasoningOutputTokens,omitempty"`
	TotalTokens           int64  `json:"totalTokens"`
	Model                 string `json:"model"`
}

// PublishMockWorkerUsage emits the one canonical usage observation for a
// matched mock dispatch. It is shared by the explicit mock Runner and the
// contextual command-runner path so the public mock-worker boundary cannot
// silently skip usage when the selected composition changes.
func PublishMockWorkerUsage(
	ctx context.Context,
	correlation workers.ExecutionCorrelation,
	provider string,
	usage *workers.MockWorkerUsageConfig,
) {
	if usage == nil {
		return
	}
	publish := ProgressPublisherFromContext(ctx, nil)
	if publish == nil {
		return
	}
	payload := mockWorkerUsageObservationPayload{
		Origin:                "SYNTHETIC",
		InputTokens:           cloneInt64Pointer(usage.InputTokens),
		CachedInputTokens:     cloneInt64Pointer(usage.CachedInputTokens),
		OutputTokens:          cloneInt64Pointer(usage.OutputTokens),
		ReasoningOutputTokens: cloneInt64Pointer(usage.ReasoningOutputTokens),
		TotalTokens:           MockWorkerUsageTotal(usage),
		Model:                 strings.TrimSpace(usage.Model),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	attemptID := strings.TrimSpace(correlation.AttemptID)
	if attemptID == "" {
		// Legacy direct executions use their dispatch as the physical attempt.
		attemptID = strings.TrimSpace(correlation.DispatchID)
	}
	provider = strings.TrimSpace(provider)
	publish(workers.ProgressFragment{
		Correlation: correlation,
		DispatchID:  correlation.DispatchID,
		Kind:        workers.ProgressFragmentKind,
		Type:        "usage.updated",
		Payload:     string(encoded),
		Provider:    provider,
		CanonicalDraft: workers.Draft{
			Kind: workers.KindUsage, Phase: workers.PhaseUpdated,
			DispatchID: attemptID, Payload: encoded,
			Provenance: workers.Provenance{
				Provider: provider, NativeEventType: "usage.updated",
				Delivery: workers.DeliverySynthesized, Representation: workers.RepresentationSnapshot,
				Fidelity: workers.FidelityNormalized,
			},
		},
	})
}

// ApplyMockWorkerUsageDiagnostics overlays the declared provider identity and
// token classes onto a detached runner result for metrics and failure shaping.
func ApplyMockWorkerUsageDiagnostics(
	result workers.RunnerExecutionResult,
	usage *workers.MockWorkerUsageConfig,
) workers.RunnerExecutionResult {
	if usage == nil {
		return result
	}
	result.Diagnostics = workers.CloneWorkDiagnostics(result.Diagnostics)
	if result.Continuation != nil {
		continuation := result.Continuation.Clone()
		result.Continuation = &continuation
	}
	if result.Diagnostics == nil {
		result.Diagnostics = &workers.WorkDiagnostics{}
	}
	metadata := cloneStringMap(result.Diagnostics.Metadata)
	if metadata == nil {
		metadata = make(map[string]string)
	}
	for _, key := range []string{
		workers.ProviderResponseMetadataInputTokens,
		workers.ProviderResponseMetadataCachedInputTokens,
		workers.ProviderResponseMetadataOutputTokens,
		workers.ProviderResponseMetadataReasoningOutputTokens,
	} {
		delete(metadata, key)
	}
	setMockUsageMetadata(metadata, workers.ProviderResponseMetadataInputTokens, usage.InputTokens)
	setMockUsageMetadata(metadata, workers.ProviderResponseMetadataCachedInputTokens, usage.CachedInputTokens)
	setMockUsageMetadata(metadata, workers.ProviderResponseMetadataOutputTokens, usage.OutputTokens)
	setMockUsageMetadata(metadata, workers.ProviderResponseMetadataReasoningOutputTokens, usage.ReasoningOutputTokens)
	result.Diagnostics.Metadata = metadata
	if result.Diagnostics.Provider == nil {
		result.Diagnostics.Provider = &workers.ProviderDiagnostic{}
	}
	result.Diagnostics.Provider.Provider = strings.TrimSpace(usage.Provider)
	result.Diagnostics.Provider.Model = strings.TrimSpace(usage.Model)
	result.Diagnostics.Provider.ResponseMetadata = cloneStringMap(metadata)
	return result
}

// MockWorkerUsageTotal returns the provider-neutral total represented by a
// mock declaration. Cached input is a subset of input and reasoning output is
// a subset of output, so neither is added a second time.
func MockWorkerUsageTotal(usage *workers.MockWorkerUsageConfig) int64 {
	var total int64
	if usage != nil && usage.InputTokens != nil {
		total += *usage.InputTokens
	}
	if usage != nil && usage.OutputTokens != nil {
		total += *usage.OutputTokens
	}
	return total
}

func setMockUsageMetadata(metadata map[string]string, key string, value *int64) {
	if value != nil {
		metadata[key] = strconv.FormatInt(*value, 10)
	}
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
