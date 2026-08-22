// Package processlifecycle binds an opened Factory Session runtime to the
// state-free process lifecycle consumed by Initializer.
package processlifecycle

import (
	"context"
	"errors"
	"net/http"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"go.uber.org/zap"
)

// Factory is the process-scoped, inert lifecycle binder constructed by Wire.
type Factory struct {
	host roles.RuntimeHostOperation
}

// NewFactory constructs a lifecycle binder from the stable runtime host.
func NewFactory(host roles.RuntimeHostOperation) (*Factory, error) {
	if host == nil {
		return nil, errors.New("construct Factory Session process lifecycle: runtime host is required")
	}
	return &Factory{host: host}, nil
}

// Bind retains only the exact opened runtime and invocation-local host values.
func (factory *Factory) Bind(
	runtime roles.LifecycleRuntime,
	request factorysessions.RuntimeHostRequest,
	observer factorysessions.RuntimeHostObserver,
	logger *zap.Logger,
) (roles.ProcessRuntime, error) {
	if factory == nil || factory.host == nil || runtime == nil {
		return nil, errors.New("bind Factory Session process lifecycle: factory and runtime are required")
	}
	return &processRuntime{
		runtime: runtime, host: factory.host, request: request, logger: logger, observer: observer,
		ready: make(chan factorysessions.RuntimeHostBinding, 1),
	}, nil
}

type processRuntime struct {
	runtime   roles.LifecycleRuntime
	host      roles.RuntimeHostOperation
	request   factorysessions.RuntimeHostRequest
	logger    *zap.Logger
	observer  factorysessions.RuntimeHostObserver
	ready     chan factorysessions.RuntimeHostBinding
	readyOnce sync.Once
}

func (runtime *processRuntime) Start(ctx, runCtx context.Context) error {
	return runtime.runtime.StartLifecycle(ctx, runCtx)
}

func (runtime *processRuntime) StartWorkers(ctx context.Context) (factorysessions.RuntimeStop, error) {
	return runtime.runtime.StartWorkerLifecycle(ctx)
}

func (runtime *processRuntime) RunTransport(ctx context.Context, handler http.Handler) error {
	return runtime.host.Run(ctx, handler, runtime.runtime, runtime.logger, runtime.request, func(binding factorysessions.RuntimeHostBinding) {
		runtime.PublishRuntimeHostBinding(binding)
		if runtime.observer != nil {
			runtime.observer(binding)
		}
	})
}

func (runtime *processRuntime) Stop(ctx context.Context) error {
	return runtime.runtime.StopLifecycle(ctx)
}

func (runtime *processRuntime) PublishRuntimeHostBinding(binding factorysessions.RuntimeHostBinding) {
	if runtime == nil || runtime.ready == nil {
		return
	}
	runtime.readyOnce.Do(func() {
		runtime.ready <- binding
		close(runtime.ready)
	})
}

func (runtime *processRuntime) RuntimeHostReady() <-chan factorysessions.RuntimeHostBinding {
	if runtime == nil {
		return nil
	}
	return runtime.ready
}

var _ roles.ProcessRuntimeFactory = (*Factory)(nil)
var _ roles.ProcessRuntime = (*processRuntime)(nil)
