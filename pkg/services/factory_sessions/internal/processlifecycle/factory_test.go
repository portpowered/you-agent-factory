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

type bindingHostOperation struct {
	binding factorysessions.RuntimeHostBinding
}

func (host *bindingHostOperation) Run(
	ctx context.Context,
	_ http.Handler,
	runtime roles.LifecycleRuntime,
	_ *zap.Logger,
	_ factorysessions.RuntimeHostRequest,
	observer factorysessions.RuntimeHostObserver,
) error {
	if observer != nil {
		observer(host.binding)
	}
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

func TestFactoryPublishesRuntimeHostBindingAndObserverNotification(t *testing.T) {
	runtime := &lifecycleRuntime{}
	host := &bindingHostOperation{binding: factorysessions.RuntimeHostBinding{
		Host: "127.0.0.1", Port: 8123,
	}}
	factory, err := NewFactory(host)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	observerCalls := 0
	process, err := factory.Bind(runtime, factorysessions.RuntimeHostRequest{}, func(binding factorysessions.RuntimeHostBinding) {
		observerCalls++
		if binding.Port != 8123 {
			t.Fatalf("observed binding = %#v, want port 8123", binding)
		}
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := process.RunTransport(context.Background(), http.NewServeMux()); err != nil {
		t.Fatalf("RunTransport: %v", err)
	}
	ready, ok := process.(interface {
		RuntimeHostReady() <-chan factorysessions.RuntimeHostBinding
	})
	if !ok {
		t.Fatal("process does not expose initializer readiness")
	}
	binding, open := <-ready.RuntimeHostReady()
	if !open || binding.Port != 8123 || binding.Host != "127.0.0.1" {
		t.Fatalf("readiness = (%#v, open=%t), want published binding", binding, open)
	}
	if _, open := <-ready.RuntimeHostReady(); open {
		t.Fatal("readiness channel remained open after first publication")
	}
	if observerCalls != 1 {
		t.Fatalf("observer calls = %d, want 1", observerCalls)
	}
}

func TestNilProcessRuntimeHasNoHostReadiness(t *testing.T) {
	var runtime *processRuntime
	if runtime.RuntimeHostReady() != nil {
		t.Fatal("nil process runtime returned a readiness channel")
	}
}
