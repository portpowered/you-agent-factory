package runtime_api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/api"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

type functionalAPIServer struct {
	httpSrv *httptest.Server
	factory apisurface.APISurface
	service *service.FactoryService
	cancel  context.CancelFunc
	done    chan struct{}
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
			"onFailure": map[string]string{"workType": "task", "state": "failed"},
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

// portos:func-length-exception owner=agent-factory reason=runtime-api-functional-server-fixture review=2026-07-22 removal=split-server-build-run-and-runtime-surface-capture-helpers-before-next-runtime-api-functional-server-change
func startFunctionalServerWithConfig(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	configure func(*service.FactoryServiceConfig),
	extraOpts ...factory.FactoryOption,
) *functionalAPIServer {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	var handler http.Handler
	var runtimeFactory apisurface.APISurface
	readyCh := make(chan struct{})

	cfg := &service.FactoryServiceConfig{
		Dir:          factoryDir,
		Port:         1,
		Logger:       zap.NewNop(),
		ExtraOptions: extraOpts,
		APIServerStarter: func(ctx context.Context, f apisurface.APISurface, port int, l *zap.Logger) error {
			runtimeFactory = f
			handler = api.NewServer(f, 0, l).Handler()
			close(readyCh)
			<-ctx.Done()
			return nil
		},
	}
	if useMockWorkers {
		cfg.MockWorkersConfig = config.NewEmptyMockWorkersConfig()
	}
	if configure != nil {
		configure(cfg)
	}

	svc, err := service.BuildFactoryService(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatalf("BuildFactoryService: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := svc.Run(ctx); err != nil && err != context.Canceled {
			fmt.Printf("runtime_api FunctionalServer: svc.Run ended: %v\n", err)
		}
	}()

	select {
	case <-readyCh:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("FunctionalServer: timed out waiting for API handler")
	}

	if cfg.RuntimeMode == interfaces.RuntimeModeService {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			snapshot, err := svc.GetEngineStateSnapshot(context.Background())
			if err == nil && snapshot.FactoryState == string(interfaces.FactoryStateRunning) {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	httpSrv := httptest.NewServer(handler)
	server := &functionalAPIServer{
		httpSrv: httpSrv,
		factory: runtimeFactory,
		service: svc,
		cancel:  cancel,
		done:    done,
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		httpSrv.Close()
	})
	return server
}

func startFunctionalServer(t *testing.T, factoryDir string, useMockWorkers bool, extraOpts ...factory.FactoryOption) *functionalAPIServer {
	t.Helper()
	return startFunctionalServerWithConfig(t, factoryDir, useMockWorkers, nil, extraOpts...)
}

func (fs *functionalAPIServer) URL() string {
	return fs.httpSrv.URL
}

func (fs *functionalAPIServer) SubmitRuntimeWork(t *testing.T, requests ...interfaces.SubmitRequest) []interfaces.SubmitRequest {
	t.Helper()

	normalized := normalizeSubmitRequestsForFunctionalTest(requests)
	workRequest := workRequestFromSubmitRequests(normalized)
	if _, err := fs.factory.SubmitWorkRequest(context.Background(), workRequest); err != nil {
		t.Fatalf("factory.SubmitWorkRequest: %v", err)
	}
	return normalized
}

func (fs *functionalAPIServer) GetEngineStateSnapshot(t *testing.T) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	snapshot, err := fs.service.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return snapshot
}
