package replay

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
)

func TestRunStartedPayloadFromEvent_RejectsRetiredFactoryAliases(t *testing.T) {
	rawEvent := map[string]any{
		"id":            "factory-event/run-started",
		"schemaVersion": factoryapi.AgentFactoryEventV1,
		"type":          factoryapi.FactoryEventTypeRunRequest,
		"context": map[string]any{
			"eventTime": time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"tick":      0,
		},
		"payload": map[string]any{
			"recordedAt": time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"factory": map[string]any{
				"workTypes": []map[string]any{
					{
						"name": "story",
						"states": []map[string]string{
							{"name": "ready", "type": "PROCESSING"},
							{"name": "complete", "type": "TERMINAL"},
						},
					},
				},
				"workers": []map[string]any{
					{
						"name":           "executor",
						"type":           "MODEL_WORKER",
						"modelProvider":  "CODEX",
						"model_provider": "anthropic",
					},
				},
				"workstations": []map[string]any{
					{
						"name":   "scheduled-story",
						"behavior": "CRON",
						"worker": "executor",
						"cron": map[string]any{
							"schedule":         "*/5 * * * *",
							"triggerAtStart":   false,
							"trigger_at_start": true,
							"expiryWindow":     "30s",
							"expiry_window":    "45s",
						},
						"outputs": []map[string]string{
							{"workType": "story", "state": "complete"},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(rawEvent)
	if err != nil {
		t.Fatalf("marshal raw event: %v", err)
	}

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("unmarshal factory event: %v", err)
	}

	_, err = runStartedPayloadFromEvent(event)
	if err == nil {
		t.Fatal("expected retired factory aliases to be rejected")
	}
	if want := "workers[0].model_provider is not supported; use modelProvider"; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected model_provider retirement guidance, got %v", err)
	}
}
