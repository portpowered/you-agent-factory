package replay

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
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
						"kind":   "cron",
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

func TestMergeGeneratedWorkers_ReplacesExistingEntriesAndAppendsRuntimeOnlyInSortedOrder(t *testing.T) {
	factory := &factoryapi.Factory{
		Workers: &[]factoryapi.Worker{
			{
				Name:    "alpha",
				Type:    stringPtrIfNotEmpty(factoryapi.WorkerTypeScriptWorker),
				Command: stringPtrIfNotEmpty("stale-alpha"),
			},
			{
				Name:    "zeta",
				Type:    stringPtrIfNotEmpty(factoryapi.WorkerTypeScriptWorker),
				Command: stringPtrIfNotEmpty("keep-zeta"),
			},
		},
	}

	runtimeWorkers := map[string]interfaces.WorkerConfig{
		"charlie": {
			Type:      string(factoryapi.WorkerTypeScriptWorker),
			Command:   "charlie-command",
			Args:      []string{"charlie-arg"},
			StopToken: "DONE",
		},
		"alpha": {
			Type:      string(factoryapi.WorkerTypeScriptWorker),
			Command:   "fresh-alpha",
			Args:      []string{"alpha-arg"},
			StopToken: "COMPLETE",
		},
		"bravo": {
			Type:    string(factoryapi.WorkerTypeScriptWorker),
			Command: "bravo-command",
		},
	}

	if err := mergeGeneratedWorkers(factory, runtimeWorkers); err != nil {
		t.Fatalf("mergeGeneratedWorkers() error = %v", err)
	}
	if factory.Workers == nil {
		t.Fatal("merged workers = nil, want generated worker list")
	}

	got := *factory.Workers
	if len(got) != 4 {
		t.Fatalf("merged workers count = %d, want 4", len(got))
	}
	if got[0].Name != "alpha" || stringValue(got[0].Command) != "fresh-alpha" || !reflect.DeepEqual(stringSliceValue(got[0].Args), []string{"alpha-arg"}) {
		t.Fatalf("merged alpha worker = %#v, want replaced runtime definition", got[0])
	}
	if stringValue(got[0].StopToken) != "COMPLETE" {
		t.Fatalf("merged alpha stop token = %q, want COMPLETE", stringValue(got[0].StopToken))
	}
	if got[1].Name != "zeta" || stringValue(got[1].Command) != "keep-zeta" {
		t.Fatalf("merged zeta worker = %#v, want untouched existing generated entry", got[1])
	}
	if got[2].Name != "bravo" || stringValue(got[2].Command) != "bravo-command" {
		t.Fatalf("merged bravo worker = %#v, want first sorted runtime-only append", got[2])
	}
	if got[3].Name != "charlie" || stringValue(got[3].Command) != "charlie-command" || !reflect.DeepEqual(stringSliceValue(got[3].Args), []string{"charlie-arg"}) {
		t.Fatalf("merged charlie worker = %#v, want second sorted runtime-only append", got[3])
	}
}
