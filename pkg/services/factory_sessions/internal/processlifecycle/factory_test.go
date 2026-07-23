package processlifecycle

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"go.uber.org/zap"
)

type lifecycleRuntime struct{ events []string }

func (runtime *lifecycleRuntime) StartLifecycle(context.Context, context.Context) error {
	runtime.events = append(runtime.events, "runtime:start")
	return nil
}

func (runtime *lifecycleRuntime) StartWorkerLifecycle(context.Context) (factorysessions.RuntimeStop, error) {
	runtime.events = append(runtime.events, "workers:start")
	return func(context.Context) error {
		runtime.events = append(runtime.events, "workers:stop")
		return nil
	}, nil
}

func (runtime *lifecycleRuntime) CompleteStartup(context.Context) error {
	runtime.events = append(runtime.events, "runtime:complete")
	return nil
}

func (*lifecycleRuntime) WaitForRuntime(context.Context) error { return nil }

func (runtime *lifecycleRuntime) StopLifecycle(context.Context) error {
	runtime.events = append(runtime.events, "runtime:stop")
	return nil
}

func (*lifecycleRuntime) FailStartup(err error) error { return err }

func (*lifecycleRuntime) CurrentRuntimeBundle() factoryruntime.HostedInstance { return nil }

type hostOperation struct {
	events  *[]string
	request factorysessions.RuntimeHostRequest
}

func (host *hostOperation) Run(
	ctx context.Context,
	_ http.Handler,
	runtime roles.LifecycleRuntime,
	_ *zap.Logger,
	request factorysessions.RuntimeHostRequest,
	_ factorysessions.RuntimeHostObserver,
) error {
	*host.events = append(*host.events, "transport:run")
	host.request = request
	return runtime.CompleteStartup(ctx)
}

func TestFactoryBindsStateFreeProcessRuntime(t *testing.T) {
	t.Parallel()

	runtime := &lifecycleRuntime{}
	host := &hostOperation{events: &runtime.events}
	factory, err := NewFactory(host)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	process, err := factory.Bind(runtime, factorysessions.RuntimeHostRequest{Directory: "/factory", Port: 8123}, nil, zap.NewNop())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	ctx := context.Background()
	if err := process.Start(ctx, ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopWorkers, err := process.StartWorkers(ctx)
	if err != nil {
		t.Fatalf("StartWorkers: %v", err)
	}
	if err := process.RunTransport(ctx, http.NewServeMux()); err != nil {
		t.Fatalf("RunTransport: %v", err)
	}
	if err := stopWorkers(ctx); err != nil {
		t.Fatalf("stop workers: %v", err)
	}
	if err := process.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	want := []string{"runtime:start", "workers:start", "transport:run", "runtime:complete", "workers:stop", "runtime:stop"}
	if !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("events = %v, want %v", runtime.events, want)
	}
	if host.request.Directory != "/factory" || host.request.Port != 8123 {
		t.Fatalf("host request = %#v, want bound invocation values", host.request)
	}
}

func TestFactoryRejectsMissingRuntimeAndHost(t *testing.T) {
	t.Parallel()

	if _, err := NewFactory(nil); err == nil {
		t.Fatal("NewFactory error = nil, want missing host")
	}
	factory, err := NewFactory(&hostOperation{events: &[]string{}})
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	if _, err := factory.Bind(nil, factorysessions.RuntimeHostRequest{}, nil, zap.NewNop()); err == nil {
		t.Fatal("Bind error = nil, want missing runtime")
	}
}
