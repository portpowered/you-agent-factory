package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

const servicePortableBundledScriptBody = "Write-Output 'portable script'\n"
const serviceStreamedRecordingTimeout = 5 * time.Second

// minimalFactoryConfig returns a minimal factory.json config for testing.
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

func serviceNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	return serviceNamedFactoryPayloadWithWorkType(t, project, "task")
}

func serviceNamedFactoryPayloadWithVersion(t *testing.T, project string, version factoryapi.HybridLogicalTimestamp) []byte {
	t.Helper()
	return withServicePayloadVersion(t, serviceNamedFactoryPayload(t, project), version)
}

func serviceNamedFactoryPayloadWithWorkType(t *testing.T, project, workType string) []byte {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "worker-a",
			"type": "MODEL_WORKER",
			"body": "You are worker " + project + ".",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": workType, "state": "complete"}},
			"onFailure": []map[string]string{{"workType": workType, "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + project + " work.",
		}},
	})
	if err != nil {
		t.Fatalf("marshal named factory payload: %v", err)
	}
	return payload
}

func serviceNamedFactoryContract(t *testing.T, name string) factoryapi.Factory {
	t.Helper()
	return serviceNamedFactoryContractWithWorkType(t, name, "task")
}

func serviceNamedFactoryContractWithBundledFiles(t *testing.T, name string) factoryapi.Factory {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "worker-a",
			"type": "MODEL_WORKER",
			"body": "You are worker " + name + ".",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + name + " work.",
		}},
		"supportingFiles": map[string]any{
			"bundledFiles": []map[string]any{
				{
					"type":       "ROOT_HELPER",
					"targetPath": "Makefile",
					"content": map[string]any{
						"encoding": string(factoryapi.Utf8),
						"inline":   "test:\n\tgo test ./...\n",
					},
				},
				{
					"type":       "DOC",
					"targetPath": "factory/docs/README.md",
					"content": map[string]any{
						"encoding": string(factoryapi.Utf8),
						"inline":   "# Portable factory\n",
					},
				},
				{
					"type":       "SCRIPT",
					"targetPath": "factory/scripts/execute-story.ps1",
					"content": map[string]any{
						"encoding": string(factoryapi.Utf8),
						"inline":   servicePortableBundledScriptBody,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal bundled named factory payload: %v", err)
	}

	generated, err := config.GeneratedFactoryFromOpenAPIJSON(payload)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(%s bundled files): %v", name, err)
	}

	generated.Name = factoryapi.FactoryName(name)
	return generated
}

func serviceNamedFactoryContractWithWorkType(t *testing.T, name, workType string) factoryapi.Factory {
	t.Helper()

	generated, err := config.GeneratedFactoryFromOpenAPIJSON([]byte(`{
		"name":"` + name + `",
		"id":"` + name + `",
		"workTypes":[{"name":"` + workType + `","states":[
			{"name":"init","type":"INITIAL"},
			{"name":"complete","type":"TERMINAL"},
			{"name":"failed","type":"FAILED"}
		]}],
		"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"You are worker ` + name + `."}],
		"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"Do the ` + name + ` work.","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"complete"}],"onFailure": [{"workType":"` + workType + `","state":"failed"}]}]
		}`))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(%s): %v", name, err)
	}

	generated.Name = factoryapi.FactoryName(name)
	return generated
}

func withServicePayloadVersion(t *testing.T, payload []byte, version factoryapi.HybridLogicalTimestamp) []byte {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal service factory payload: %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  version.Logical,
		"physical": version.Physical.UTC().Format(time.RFC3339Nano),
	}
	updated, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("marshal service factory payload with version: %v", err)
	}
	return updated
}

func submitWorkRequestsToService(ctx context.Context, svc *FactoryService, reqs []interfaces.SubmitRequest) error {
	workRequest := requests.WorkRequestFromSubmitRequests(reqs)
	_, err := svc.SubmitWorkRequest(ctx, workRequest)
	return err
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

// writeFactoryJSON writes a factory.json into the given directory.
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

func stopServiceModeRun(t *testing.T, cancel context.CancelFunc, errCh <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("service-mode run error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode run to stop")
	}
}

type aggregateSnapshotFactory struct {
	engineState              *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	engineStateErr           error
	engineStateSnapshotCalls int
	factoryEvents            []factoryapi.FactoryEvent
	factoryEventsErr         error
	factoryEventsCalls       int
	pauseErr                 error
	submitFunc               func(context.Context, interfaces.WorkRequest) error
	submitCalls              int
	submissions              []interfaces.WorkRequest
	waitToComplete           chan struct{}
}

func (f *aggregateSnapshotFactory) Run(context.Context) error { return nil }
func (f *aggregateSnapshotFactory) SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	normalized, err := requests.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{})
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	result := interfaces.WorkRequestSubmitResult{RequestID: request.RequestID, Accepted: true}
	if len(normalized) > 0 {
		result.TraceID = normalized[0].TraceID
	}
	f.submitCalls++
	f.submissions = append(f.submissions, request)
	if f.submitFunc != nil {
		return result, f.submitFunc(ctx, request)
	}
	return result, nil
}
func (f *aggregateSnapshotFactory) SubscribeFactoryEvents(context.Context) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{Events: make(chan factoryapi.FactoryEvent)}, nil
}
func (f *aggregateSnapshotFactory) Pause(context.Context) error { return f.pauseErr }
func (f *aggregateSnapshotFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	f.engineStateSnapshotCalls++
	if f.engineStateErr != nil {
		return nil, f.engineStateErr
	}
	return f.engineState, nil
}
func (f *aggregateSnapshotFactory) GetFactoryEvents(context.Context) ([]factoryapi.FactoryEvent, error) {
	f.factoryEventsCalls++
	if f.factoryEventsErr != nil {
		return nil, f.factoryEventsErr
	}
	return append([]factoryapi.FactoryEvent(nil), f.factoryEvents...), nil
}
func (f *aggregateSnapshotFactory) WaitToComplete() <-chan struct{} {
	if f.waitToComplete != nil {
		return f.waitToComplete
	}
	return make(chan struct{})
}
