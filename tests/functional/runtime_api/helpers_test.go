package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type functionalAPIServer struct {
	factory apisurface.APISurface
	*support.FunctionalAPIServer
}

func generatedWorkStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func generatedWorkStateType(state *factoryapi.WorkState) factoryapi.WorkStateType {
	if state == nil {
		return ""
	}
	return state.Type
}

func simplePipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func persistTestPipelineConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "stage1", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "step-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "step1", WorkerTypeName: "step-worker", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage1"}}},
			{Name: "finish", WorkerTypeName: "step-worker", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage1"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}}},
		},
	}
}

func startFunctionalServerWithConfig(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	configure func(*service.FactoryServiceConfig),
	extraOpts ...factory.FactoryOption,
) *functionalAPIServer {
	t.Helper()

	server := &functionalAPIServer{}
	var runtimeFactory apisurface.APISurface
	base := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            useMockWorkers,
		WaitForServiceModeRuntime: true,
		Configure:                 configure,
		ExtraOptions:              extraOpts,
		CaptureAPISurface: func(surface apisurface.APISurface) {
			runtimeFactory = surface
		},
	})
	server.factory = runtimeFactory
	server.FunctionalAPIServer = base
	return server
}

func startFunctionalServer(t *testing.T, factoryDir string, useMockWorkers bool, extraOpts ...factory.FactoryOption) *functionalAPIServer {
	t.Helper()
	return startFunctionalServerWithConfig(t, factoryDir, useMockWorkers, nil, extraOpts...)
}

func (fs *functionalAPIServer) SubmitRuntimeWork(t *testing.T, submitted ...work.SubmitRequest) []work.SubmitRequest {
	t.Helper()

	normalized := normalizeSubmitRequestsForFunctionalTest(submitted)
	workRequest := requests.WorkRequestFromSubmitRequests(normalized)
	if _, err := fs.factory.SubmitWorkRequest(context.Background(), workRequest); err != nil {
		t.Fatalf("factory.SubmitWorkRequest: %v", err)
	}
	return normalized
}

func postJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("%s: marshal request: %v", failurePrefix, err)
		}
		body = bytes.NewReader(encoded)
	}
	resp, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: POST %s status = %d, want 200: %s", failurePrefix, endpoint, resp.StatusCode, string(payload))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return out
}

func mustGeneratedFunctionalTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
	}); err != nil {
		t.Fatalf("encode generated text part: %v", err)
	}
	return part
}

func generatedAudioPath(audio factoryapi.WorkAudioContentPart) string {
	if audio.File != nil && strings.TrimSpace(string(*audio.File)) != "" {
		return string(*audio.File)
	}
	if strings.TrimSpace(string(audio.Url)) != "" {
		return strings.TrimPrefix(string(audio.Url), "file://")
	}
	return ""
}
