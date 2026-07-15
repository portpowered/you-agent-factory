package cursors

// SessionTokenUsage aggregates Cursor CLI usage counters discovered while parsing store.db.
type SessionTokenUsage struct {
	InputTokens      *int
	OutputTokens     *int
	CacheReadTokens  *int
	CacheWriteTokens *int
}

func mergeSessionTokenUsage(into *SessionTokenUsage, next SessionTokenUsage) {
	if into == nil {
		return
	}
	if next.InputTokens != nil {
		into.InputTokens = next.InputTokens
	}
	if next.OutputTokens != nil {
		into.OutputTokens = next.OutputTokens
	}
	if next.CacheReadTokens != nil {
		into.CacheReadTokens = next.CacheReadTokens
	}
	if next.CacheWriteTokens != nil {
		into.CacheWriteTokens = next.CacheWriteTokens
	}
}

func tokenUsageFromData(data map[string]interface{}) SessionTokenUsage {
	if usageMap, ok := nestedMapField(data, "usage"); ok {
		if usage := tokenUsageFromUsageMap(usageMap); hasAnyTokenUsage(usage) {
			return usage
		}
	}
	return tokenUsageFromUsageMap(data)
}

func tokenUsageFromUsageMap(data map[string]interface{}) SessionTokenUsage {
	var usage SessionTokenUsage
	if value, ok := intField(data, "inputTokens", "input_tokens"); ok {
		usage.InputTokens = &value
	}
	if value, ok := intField(data, "outputTokens", "output_tokens"); ok {
		usage.OutputTokens = &value
	}
	if value, ok := intField(data, "cacheReadTokens", "cache_read_tokens", "cachedInputTokens", "cached_input_tokens"); ok {
		usage.CacheReadTokens = &value
	}
	if value, ok := intField(data, "cacheWriteTokens", "cache_write_tokens"); ok {
		usage.CacheWriteTokens = &value
	}
	return usage
}

func hasAnyTokenUsage(usage SessionTokenUsage) bool {
	return usage.InputTokens != nil ||
		usage.OutputTokens != nil ||
		usage.CacheReadTokens != nil ||
		usage.CacheWriteTokens != nil
}

func nestedMapField(data map[string]interface{}, key string) (map[string]interface{}, bool) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return nil, false
	}
	mapped, ok := raw.(map[string]interface{})
	return mapped, ok
}

func intField(data map[string]interface{}, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case float64:
			return int(typed), true
		case int:
			return typed, true
		case int64:
			return int(typed), true
		}
	}
	return 0, false
}

func computeTotalTokens(usage SessionTokenUsage) *int {
	if usage.InputTokens == nil && usage.OutputTokens == nil &&
		usage.CacheReadTokens == nil && usage.CacheWriteTokens == nil {
		return nil
	}
	total := 0
	if usage.InputTokens != nil {
		total += *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		total += *usage.OutputTokens
	}
	if usage.CacheReadTokens != nil {
		total += *usage.CacheReadTokens
	}
	if usage.CacheWriteTokens != nil {
		total += *usage.CacheWriteTokens
	}
	return &total
}
