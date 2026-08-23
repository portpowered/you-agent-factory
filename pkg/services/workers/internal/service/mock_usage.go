package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
)

// mockWorkerUsageObservationPayload is deliberately pointer-based so the
// canonical Worker draft preserves omitted token classes while retaining
// explicit zeroes.
type mockWorkerUsageObservationPayload struct {
	InputTokens           *int64 `json:"inputTokens,omitempty"`
	CachedInputTokens     *int64 `json:"cachedInputTokens,omitempty"`
	OutputTokens          *int64 `json:"outputTokens,omitempty"`
	ReasoningOutputTokens *int64 `json:"reasoningOutputTokens,omitempty"`
	TotalTokens           int64  `json:"totalTokens"`
	Model                 string `json:"model"`
}

func applyMockWorkerUsageDiagnostics(
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

func setMockUsageMetadata(metadata map[string]string, key string, value *int64) {
	if value != nil {
		metadata[key] = strconv.FormatInt(*value, 10)
	}
}

func publishMockWorkerUsage(
	ctx context.Context,
	request workers.ExecuteRequest,
	usage *workers.MockWorkerUsageConfig,
) {
	if usage == nil {
		return
	}
	publish := workerexecution.ProgressPublisherFromContext(ctx, request.Input.ProgressPublisher)
	if publish == nil {
		return
	}
	payload := mockWorkerUsageObservationPayload{
		InputTokens:           cloneInt64Pointer(usage.InputTokens),
		CachedInputTokens:     cloneInt64Pointer(usage.CachedInputTokens),
		OutputTokens:          cloneInt64Pointer(usage.OutputTokens),
		ReasoningOutputTokens: cloneInt64Pointer(usage.ReasoningOutputTokens),
		TotalTokens:           mockWorkerUsageTotal(usage),
		Model:                 strings.TrimSpace(usage.Model),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	publish(workers.ProgressFragment{
		Correlation: request.Correlation,
		DispatchID:  request.Correlation.DispatchID,
		Kind:        workers.ProgressFragmentKind,
		Type:        "usage.updated",
		Payload:     string(encoded),
		Provider:    strings.TrimSpace(usage.Provider),
		Metadata:    map[string]string{"model": strings.TrimSpace(usage.Model)},
	})
}

func mockWorkerUsageTotal(usage *workers.MockWorkerUsageConfig) int64 {
	var total int64
	if usage != nil && usage.InputTokens != nil {
		total += *usage.InputTokens
	}
	if usage != nil && usage.OutputTokens != nil {
		total += *usage.OutputTokens
	}
	return total
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
