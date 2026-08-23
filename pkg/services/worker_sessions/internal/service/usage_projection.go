package service

import (
	"encoding/json"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type usageProjectionPayload struct {
	InputTokens           *int64 `json:"inputTokens"`
	CachedInputTokens     *int64 `json:"cachedInputTokens"`
	OutputTokens          *int64 `json:"outputTokens"`
	ReasoningOutputTokens *int64 `json:"reasoningOutputTokens"`
	TotalTokens           *int64 `json:"totalTokens"`
	Model                 string `json:"model"`
}

func (r *registry) updateUsageProjection(sessionID string, draft workers.Draft) {
	usage, model, ok := usageProjectionFromDraft(draft)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	metadata := r.observations[strings.TrimSpace(sessionID)]
	if metadata == nil {
		return
	}
	metadata.tokenUsage = usage
	if strings.TrimSpace(model) != "" {
		metadata.usageModel = strings.TrimSpace(model)
	}
}

func usageProjectionFromDraft(draft workers.Draft) (*workersessions.TokenUsage, string, bool) {
	if draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated {
		return nil, "", false
	}
	var payload usageProjectionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return nil, "", false
	}
	if payload.InputTokens == nil && payload.CachedInputTokens == nil &&
		payload.OutputTokens == nil && payload.ReasoningOutputTokens == nil &&
		payload.TotalTokens == nil {
		return nil, "", false
	}
	return &workersessions.TokenUsage{
		InputTokens:           int64PointerToInt(payload.InputTokens),
		CachedInputTokens:     int64PointerToInt(payload.CachedInputTokens),
		OutputTokens:          int64PointerToInt(payload.OutputTokens),
		ReasoningOutputTokens: int64PointerToInt(payload.ReasoningOutputTokens),
		TotalTokens:           int64PointerToInt(payload.TotalTokens),
	}, payload.Model, true
}

func int64PointerToInt(value *int64) *int {
	if value == nil {
		return nil
	}
	converted := int(*value)
	return &converted
}
