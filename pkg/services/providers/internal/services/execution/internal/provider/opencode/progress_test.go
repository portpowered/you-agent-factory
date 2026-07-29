package opencode

import (
	"encoding/json"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestUsageEventPayloadIncludesTokenCounts(t *testing.T) {
	t.Parallel()

	event, ok := usageEvent("run", providers.ExecuteProgress{
		Phase:    "usage.updated",
		Metadata: map[string]string{"input_tokens": "12", "output_tokens": "7"},
	})
	if !ok {
		t.Fatal("usageEvent returned false")
	}
	if err := workerexecution.ValidateDraft(event.Draft()); err != nil {
		t.Fatalf("ValidateDraft: %v payload=%s", err, event.Draft().Payload)
	}
	var payload workerexecution.UsagePayload
	if err := json.Unmarshal(event.Draft().Payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.InputTokens != 12 || payload.OutputTokens != 7 {
		t.Fatalf("payload = %#v, want input=12 output=7", payload)
	}
}
