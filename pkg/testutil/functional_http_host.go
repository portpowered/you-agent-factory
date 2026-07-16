package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/testdeps"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/wire"
	"go.uber.org/zap"
)

const functionalHTTPHostReadyTimeout = 5 * time.Second

// FunctionalHTTPHostConfig selects deterministic product edges for a
// functional HTTP host. It deliberately contains no service, handler, runtime,
// projection, or generated-configuration handles.
type FunctionalHTTPHostConfig struct {
	FactoryDir     string
	UseMockWorkers bool
	RuntimeMode    interfaces.RuntimeMode
	ExtraOptions   []factory.FactoryOption
}

// FunctionalHTTPHost starts the production service graph and exposes only the
// customer HTTP boundary and bounded lifecycle controls to functional tests.
type FunctionalHTTPHost struct {
	url    string
	client *http.Client
	cancel context.CancelFunc
	done   <-chan struct{}
	close  func()
}

// StartFunctionalHTTPHost starts an isolated HTTP host and waits for its
// supported status endpoint before returning.
func StartFunctionalHTTPHost(t *testing.T, cfg FunctionalHTTPHostConfig) *FunctionalHTTPHost {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	var handler http.Handler
	ready := make(chan struct{})
	serviceCfg := testdeps.QuietFactoryServiceConfig(&service.FactoryServiceConfig{
		Dir:          cfg.FactoryDir,
		Port:         1,
		RuntimeMode:  cfg.RuntimeMode,
		ExtraOptions: cfg.ExtraOptions,
		APIServerStarter: func(runCtx context.Context, surface apisurface.APISurface, _ int, logger *zap.Logger) error {
			handler = api.NewServer(surface, 0, logger).Handler()
			close(ready)
			<-runCtx.Done()
			return nil
		},
	})
	if cfg.UseMockWorkers {
		serviceCfg.MockWorkersConfig = config.NewEmptyMockWorkersConfig()
	}

	svc, err := wire.InjectFactoryService(ctx, serviceCfg)
	if err != nil {
		cancel()
		t.Fatalf("construct functional HTTP host: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := svc.Run(ctx); err != nil && err != context.Canceled {
			t.Logf("functional HTTP host stopped: %v", err)
		}
	}()

	select {
	case <-ready:
	case <-time.After(functionalHTTPHostReadyTimeout):
		cancel()
		waitForFunctionalHTTPHostStop(t, done, "API handler")
		t.Fatal("functional HTTP host: timed out waiting for API handler")
	}

	server := httptest.NewServer(handler)
	host := &FunctionalHTTPHost{url: server.URL, client: server.Client(), cancel: cancel, done: done, close: server.Close}
	host.WaitForReady(t)
	t.Cleanup(func() { host.Stop(t) })
	return host
}

func (host *FunctionalHTTPHost) URL() string { return host.url }

func (host *FunctionalHTTPHost) Client() *http.Client { return host.client }

func (host *FunctionalHTTPHost) Done() <-chan struct{} { return host.done }

// WaitForReady observes the documented status endpoint rather than an
// in-process runtime state.
func (host *FunctionalHTTPHost) WaitForReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(functionalHTTPHostReadyTimeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var lastStatus string
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, host.URL()+"/status", nil)
		if err != nil {
			t.Fatalf("build status readiness request: %v", err)
		}
		response, err := host.Client().Do(request)
		if err == nil {
			lastStatus = response.Status
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		} else {
			lastStatus = err.Error()
		}
		select {
		case <-t.Context().Done():
			host.Stop(t)
			t.Fatalf("functional HTTP host: readiness canceled; last public observation: %s", lastStatus)
		case <-ticker.C:
		}
	}
	host.Stop(t)
	t.Fatalf("functional HTTP host: timed out waiting for GET /status; last public observation: %s", lastStatus)
}

func (host *FunctionalHTTPHost) Stop(t *testing.T) {
	t.Helper()
	host.cancel()
	waitForFunctionalHTTPHostStop(t, host.done, "service shutdown")
	host.close()
}

func waitForFunctionalHTTPHostStop(t *testing.T, done <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(functionalHTTPHostReadyTimeout):
		t.Fatalf("functional HTTP host: timed out waiting for %s", operation)
	}
}
