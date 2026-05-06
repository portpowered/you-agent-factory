package support

import (
	"context"
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

const functionalServerReadyTimeout = 5 * time.Second

type FunctionalAPIServerConfig struct {
	FactoryDir                string
	UseMockWorkers            bool
	WaitForServiceModeRuntime bool
	Configure                 func(*service.FactoryServiceConfig)
	ExtraOptions              []factory.FactoryOption
	CaptureAPISurface         func(apisurface.APISurface)
}

type FunctionalAPIServer struct {
	httpSrv *httptest.Server
	service *service.FactoryService
	cancel  context.CancelFunc
	done    chan struct{}
}

func StartFunctionalAPIServer(t *testing.T, cfg FunctionalAPIServerConfig) *FunctionalAPIServer {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	var handler http.Handler
	readyCh := make(chan struct{})

	serviceCfg := &service.FactoryServiceConfig{
		Dir:          cfg.FactoryDir,
		Port:         1,
		Logger:       zap.NewNop(),
		ExtraOptions: cfg.ExtraOptions,
		APIServerStarter: func(ctx context.Context, surface apisurface.APISurface, port int, l *zap.Logger) error {
			if cfg.CaptureAPISurface != nil {
				cfg.CaptureAPISurface(surface)
			}
			handler = api.NewServer(surface, 0, l).Handler()
			close(readyCh)
			<-ctx.Done()
			return nil
		},
	}
	if cfg.UseMockWorkers {
		serviceCfg.MockWorkersConfig = config.NewEmptyMockWorkersConfig()
	}
	if cfg.Configure != nil {
		cfg.Configure(serviceCfg)
	}

	svc, err := service.BuildFactoryService(ctx, serviceCfg)
	if err != nil {
		cancel()
		t.Fatalf("BuildFactoryService: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := svc.Run(ctx); err != nil && err != context.Canceled {
			fmt.Printf("functional support server: svc.Run ended: %v\n", err)
		}
	}()

	waitForHandlerReadiness(t, cancel, readyCh)
	if cfg.WaitForServiceModeRuntime && serviceCfg.RuntimeMode == interfaces.RuntimeModeService {
		waitForServiceRuntimeReady(t, cancel, svc)
	}

	httpSrv := httptest.NewServer(handler)
	server := &FunctionalAPIServer{
		httpSrv: httpSrv,
		service: svc,
		cancel:  cancel,
		done:    done,
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(functionalServerReadyTimeout):
		}
		httpSrv.Close()
	})
	return server
}

func waitForHandlerReadiness(t *testing.T, cancel context.CancelFunc, readyCh <-chan struct{}) {
	t.Helper()

	select {
	case <-readyCh:
	case <-time.After(functionalServerReadyTimeout):
		cancel()
		t.Fatal("FunctionalServer: timed out waiting for API handler")
	}
}

func waitForServiceRuntimeReady(t *testing.T, cancel context.CancelFunc, svc *service.FactoryService) {
	t.Helper()

	deadline := time.Now().Add(functionalServerReadyTimeout)
	for time.Now().Before(deadline) {
		snapshot, err := svc.GetEngineStateSnapshot(context.Background())
		if err == nil && snapshot.FactoryState == string(interfaces.FactoryStateRunning) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	t.Fatal("FunctionalServer: timed out waiting for service runtime readiness")
}

func (fs *FunctionalAPIServer) URL() string {
	return fs.httpSrv.URL
}

func (fs *FunctionalAPIServer) HTTPServer() *httptest.Server {
	return fs.httpSrv
}

func (fs *FunctionalAPIServer) Service() *service.FactoryService {
	return fs.service
}

func (fs *FunctionalAPIServer) CancelFunc() context.CancelFunc {
	return fs.cancel
}

func (fs *FunctionalAPIServer) Done() chan struct{} {
	return fs.done
}

func (fs *FunctionalAPIServer) GetEngineStateSnapshot(t *testing.T) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	snapshot, err := fs.service.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return snapshot
}

func (fs *FunctionalAPIServer) GetFactoryEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()

	events, err := fs.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	return events
}
