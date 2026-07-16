package functionalhost

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/testdeps"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/wire"
	"go.uber.org/zap"
)

// FunctionalHTTPServerConfig selects the deterministic product edges needed by
// a process-level functional HTTP server. It deliberately exposes no internal
// application handles to its callers.
type FunctionalHTTPServerConfig struct {
	Address          string
	ExecutionBaseDir string
	FactoryDir       string
	UseMockWorkers   bool
}

// FunctionalHTTPServer owns a composed application and exposes only its
// supported HTTP boundary and bounded shutdown control.
type FunctionalHTTPServer struct {
	url         string
	server      *http.Server
	serviceDone <-chan struct{}
}

// StartFunctionalHTTPServer starts a production-shaped HTTP server and waits
// for the public status endpoint before returning.
func StartFunctionalHTTPServer(ctx context.Context, cfg FunctionalHTTPServerConfig) (*FunctionalHTTPServer, error) {
	var handler http.Handler
	ready := make(chan struct{})
	serviceCfg := testdeps.QuietFactoryServiceConfig(&service.FactoryServiceConfig{
		Dir:                      cfg.FactoryDir,
		ExecutionBaseDir:         cfg.ExecutionBaseDir,
		Port:                     1,
		RuntimeFileLoggingPolicy: service.RuntimeFileLoggingPolicyDisabled,
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
		return nil, fmt.Errorf("construct functional HTTP server: %w", err)
	}
	serviceDone := make(chan struct{})
	go func() {
		defer close(serviceDone)
		_ = svc.Run(ctx)
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for functional HTTP handler: %w", ctx.Err())
	case <-time.After(functionalHTTPHostReadyTimeout):
		return nil, fmt.Errorf("timed out waiting for functional HTTP handler")
	}

	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("listen for functional HTTP server: %w", err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	host := &FunctionalHTTPServer{
		url:         "http://" + listener.Addr().String(),
		server:      server,
		serviceDone: serviceDone,
	}
	if err := host.WaitForReady(ctx); err != nil {
		_ = host.Shutdown(context.Background())
		return nil, err
	}
	return host, nil
}

func (host *FunctionalHTTPServer) URL() string { return host.url }

// WaitForReady observes the public status endpoint with the caller's timeout
// and cancellation policy.
func (host *FunctionalHTTPServer) WaitForReady(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL()+"/status", nil)
	if err != nil {
		return fmt.Errorf("build status request: %w", err)
	}
	response, err := (&http.Client{Timeout: functionalHTTPHostReadyTimeout}).Do(request)
	if err != nil {
		return fmt.Errorf("GET /status: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /status: status %s", response.Status)
	}
	return nil
}

// Shutdown stops the HTTP listener. The caller's context bounds server-side
// shutdown while the enclosing application context owns service shutdown.
func (host *FunctionalHTTPServer) Shutdown(ctx context.Context) error {
	return host.server.Shutdown(ctx)
}
