package cursors

import "testing"

func TestTokenUsageFromData_NestedUsageObject(t *testing.T) {
	t.Parallel()
	data := map[string]interface{}{
		"usage": map[string]interface{}{
			"inputTokens":      float64(120),
			"outputTokens":     float64(25),
			"cacheReadTokens":  float64(40),
			"cacheWriteTokens": float64(10),
		},
	}
	usage := tokenUsageFromData(data)
	if usage.InputTokens == nil || *usage.InputTokens != 120 {
		t.Fatalf("input tokens = %#v, want 120", usage.InputTokens)
	}
	if usage.CacheWriteTokens == nil || *usage.CacheWriteTokens != 10 {
		t.Fatalf("cache write tokens = %#v, want 10", usage.CacheWriteTokens)
	}
}

func TestComputeTotalTokens_SumsPresentFields(t *testing.T) {
	t.Parallel()
	input := 100
	output := 25
	cacheRead := 40
	cacheWrite := 10
	total := computeTotalTokens(SessionTokenUsage{
		InputTokens:      &input,
		OutputTokens:     &output,
		CacheReadTokens:  &cacheRead,
		CacheWriteTokens: &cacheWrite,
	})
	if total == nil || *total != 175 {
		t.Fatalf("total = %#v, want 175", total)
	}
}
