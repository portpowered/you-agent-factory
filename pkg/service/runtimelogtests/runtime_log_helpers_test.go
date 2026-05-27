package runtimelogtests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func minimalFactoryConfig() map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func writeFactoryJSON(t *testing.T, dir string, cfg map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeWorkRequestFile(t *testing.T, path string, req interfaces.SubmitRequest) {
	t.Helper()
	data, err := json.Marshal(requests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{req}))
	if err != nil {
		t.Fatalf("marshal work request file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write work request file: %v", err)
	}
}

func writeScriptWorkerAgentsMDWithCommand(t *testing.T, factoryDir, workerName, command string, args []string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	var argsYAML strings.Builder
	for _, arg := range args {
		argsYAML.WriteString("  - ")
		argsYAML.WriteString(strconv.Quote(arg))
		argsYAML.WriteString("\n")
	}
	agentsMD := fmt.Sprintf("---\ntype: SCRIPT_WORKER\ncommand: %s\nargs:\n%s---\n", command, argsYAML.String())
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func writeWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func serviceReplayWorkRequestEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayWorkRequestRecord {
	t.Helper()
	var out []serviceReplayWorkRequestRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode work request event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayWorkRequestRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayWorkRequestRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.WorkRequestEventPayload
}

func serviceReplayDispatchCreatedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayDispatchCreatedRecord {
	t.Helper()
	var out []serviceReplayDispatchCreatedRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch created event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayDispatchCreatedRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayDispatchCreatedRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.DispatchRequestEventPayload
}

func serviceReplayDispatchCompletedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayDispatchCompletedRecord {
	t.Helper()
	var out []serviceReplayDispatchCompletedRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch completed event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayDispatchCompletedRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayDispatchCompletedRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.DispatchResponseEventPayload
}

func serviceStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func serviceFirstStringValue(values *[]string) string {
	if values == nil {
		return ""
	}
	for _, value := range *values {
		if value != "" {
			return value
		}
	}
	return ""
}
