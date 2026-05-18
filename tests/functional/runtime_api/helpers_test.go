package runtime_api

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
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
		Workers: []interfaces.WorkerConfig{{Name: "step-worker"}},
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

func (fs *functionalAPIServer) SubmitRuntimeWork(t *testing.T, requests ...interfaces.SubmitRequest) []interfaces.SubmitRequest {
	t.Helper()

	normalized := normalizeSubmitRequestsForFunctionalTest(requests)
	workRequest := factory.WorkRequestFromSubmitRequests(normalized)
	if _, err := fs.factory.SubmitWorkRequest(context.Background(), workRequest); err != nil {
		t.Fatalf("factory.SubmitWorkRequest: %v", err)
	}
	return normalized
}
