package bootstrap_portability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
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
	service *service.FactoryService
	cancel  context.CancelFunc
	done    chan struct{}
}

// portos:func-length-exception owner=agent-factory reason=bootstrap-functional-server-fixture review=2026-07-22 removal=split-server-build-run-and-service-mode-readiness-helpers-before-next-bootstrap-functional-server-change
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
	readyCh := make(chan struct{})

	cfg := &service.FactoryServiceConfig{
		Dir:          factoryDir,
		Port:         1,
		Logger:       zap.NewNop(),
		ExtraOptions: extraOpts,
		APIServerStarter: func(ctx context.Context, f apisurface.APISurface, port int, l *zap.Logger) error {
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
			fmt.Printf("bootstrap_portability FunctionalServer: svc.Run ended: %v\n", err)
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

func (fs *functionalAPIServer) URL() string {
	return fs.httpSrv.URL
}

func (fs *functionalAPIServer) SubmitWork(t *testing.T, workTypeID string, payload json.RawMessage) string {
	t.Helper()

	req := factoryapi.SubmitWorkRequest{
		WorkTypeName: workTypeID,
		Payload:      payload,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}

	resp, err := http.Post(fs.httpSrv.URL+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /work: expected 201 Created, got %d", resp.StatusCode)
	}

	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	return result.TraceId
}

func (fs *functionalAPIServer) GetEngineStateSnapshot(t *testing.T) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	snapshot, err := fs.service.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return snapshot
}

func getGeneratedJSON[T any](t *testing.T, endpoint string) T {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", endpoint, resp.StatusCode)
	}

	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", endpoint, err)
	}
	return out
}

func waitForGeneratedWorkAtPlace(
	t *testing.T,
	baseURL string,
	traceID string,
	placeID string,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		for _, token := range work.Results {
			if token.TraceId == traceID && token.PlaceId == placeID {
				return work
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
}
