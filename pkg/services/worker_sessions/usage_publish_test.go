package workersessions_test

import (
	"encoding/json"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPublish_CanonicalMockUsagePreservesExplicitZeroesAndModel(t *testing.T) {
	spy := &workerRecordSpy{}
	publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) {})
	publisher.Bind(spy)
	publisher.Publish(workers.ProgressFragment{
		DispatchID: "worker-1",
		Kind:       workers.ProgressFragmentKind,
		Type:       "usage.updated",
		Provider:   "codex",
		Payload:    `{"inputTokens":0,"outputTokens":5,"reasoningOutputTokens":0,"totalTokens":5,"model":"gpt-5-codex"}`,
	})

	if len(spy.published) != 1 {
		t.Fatalf("published records = %d, want exactly one usage record", len(spy.published))
	}
	draft := spy.published[0].Draft
	if draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated || draft.Provenance.Provider != "codex" {
		t.Fatalf("draft = %#v, want codex USAGE/UPDATED", draft)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("draft payload is not valid JSON: %v", err)
	}
	for _, field := range []string{"inputTokens", "outputTokens", "reasoningOutputTokens", "totalTokens", "model"} {
		if _, ok := payload[field]; !ok {
			t.Fatalf("draft payload = %s, missing %q", draft.Payload, field)
		}
	}
	if _, ok := payload["cachedInputTokens"]; ok {
		t.Fatalf("draft payload = %s, cachedInputTokens should remain omitted", draft.Payload)
	}
}
