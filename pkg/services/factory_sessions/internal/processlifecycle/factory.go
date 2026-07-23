// Package processlifecycle binds an opened Factory Session runtime to the
// state-free process lifecycle consumed by Initializer.
package processlifecycle

import (
	"context"
	"errors"
	"net/http"

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
	return &processRuntime{runtime: runtime, host: factory.host, request: request, observer: observer, logger: logger}, nil
}

type processRuntime struct {
	runtime  roles.LifecycleRuntime
	host     roles.RuntimeHostOperation
	request  factorysessions.RuntimeHostRequest
	observer factorysessions.RuntimeHostObserver
	logger   *zap.Logger
}

func (runtime *processRuntime) Start(ctx, runCtx context.Context) error {
	return runtime.runtime.StartLifecycle(ctx, runCtx)
}

func (runtime *processRuntime) StartWorkers(ctx context.Context) (factorysessions.RuntimeStop, error) {
	return runtime.runtime.StartWorkerLifecycle(ctx)
}

func (runtime *processRuntime) RunTransport(ctx context.Context, handler http.Handler) error {
	return runtime.host.Run(ctx, handler, runtime.runtime, runtime.logger, runtime.request, runtime.observer)
}

func (runtime *processRuntime) Stop(ctx context.Context) error {
	return runtime.runtime.StopLifecycle(ctx)
}

var _ roles.ProcessRuntimeFactory = (*Factory)(nil)
var _ roles.ProcessRuntime = (*processRuntime)(nil)
