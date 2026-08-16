package internal

import (
	"strings"
	"testing"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func stubWorkerSessionsFactory(workers.WorkstationPoolBoundary, platformclock.Source) (workersessions.Service, error) {
	return nil, nil
}

type stubWorkersService struct{ workers.Service }

func TestNewAssemblyRequiresWireConstructedRuntimeFactory(t *testing.T) {
	assembly, err := NewAssembly(nil, stubWorkerSessionsFactory, nil)
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime factory is required") {
		t.Fatalf("NewAssembly(nil) error = %v, want required dependency", err)
	}
	if assembly != nil {
		t.Fatalf("NewAssembly(nil) = %#v, want nil assembly", assembly)
	}
}

func TestNewAssemblyRequiresWorkerSessionsFactory(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	assembly, err := NewAssembly(runtimeFactory, nil, stubWorkersService{})
	if err == nil || !strings.Contains(err.Error(), "Worker Sessions factory is required") {
		t.Fatalf("NewAssembly(nil factory) error = %v, want required dependency", err)
	}
	if assembly != nil {
		t.Fatalf("NewAssembly(nil factory) = %#v, want nil assembly", assembly)
	}
}

func TestNewAssemblyRequiresWorkersService(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	assembly, err := NewAssembly(runtimeFactory, stubWorkerSessionsFactory, nil)
	if err == nil || !strings.Contains(err.Error(), "Workers service is required") {
		t.Fatalf("NewAssembly(nil Workers service) error = %v, want required dependency", err)
	}
	if assembly != nil {
		t.Fatalf("NewAssembly(nil Workers service) = %#v, want nil assembly", assembly)
	}
}

func TestNewAssemblyBindsRuntimeFactory(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	workerService := stubWorkersService{}
	assembly, err := NewAssembly(runtimeFactory, stubWorkerSessionsFactory, workerService)
	if err != nil {
		t.Fatalf("NewAssembly() error = %v", err)
	}
	if assembly == nil || assembly.runtimeFactory != runtimeFactory {
		t.Fatalf("NewAssembly() = %#v, want supplied Runtime Factory", assembly)
	}
	if assembly.workerService != workerService {
		t.Fatalf("NewAssembly() worker service = %#v, want supplied service", assembly.workerService)
	}
}

func TestRuntimeCompositionComposesInertInstanceHost(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	lifecycle, err := instancehostwire.New(instancehost.Dependencies{Clock: clock})
	if err != nil {
		t.Fatalf("instancehostwire.New() error = %v", err)
	}
	var _ factoryruntime.RuntimeLifecycle = lifecycle
	if _, ok := lifecycle.(instancehost.Service); !ok {
		t.Fatalf("composed lifecycle type = %T, want instance_host.Service", lifecycle)
	}
}
