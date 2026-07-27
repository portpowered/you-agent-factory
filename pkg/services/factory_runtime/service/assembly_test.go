package service

import (
	"strings"
	"testing"

	"github.com/jonboulle/clockwork"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
)

func TestNewAssemblyRequiresWireConstructedRuntimeFactory(t *testing.T) {
	assembly, err := NewAssembly(nil)
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime factory is required") {
		t.Fatalf("NewAssembly(nil) error = %v, want required dependency", err)
	}
	if assembly != nil {
		t.Fatalf("NewAssembly(nil) = %#v, want nil assembly", assembly)
	}
}

func TestNewAssemblyBindsRuntimeFactory(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	assembly, err := NewAssembly(runtimeFactory)
	if err != nil {
		t.Fatalf("NewAssembly() error = %v", err)
	}
	if assembly == nil || assembly.runtimeFactory != runtimeFactory {
		t.Fatalf("NewAssembly() = %#v, want supplied Runtime Factory", assembly)
	}
}

func TestRuntimeCompositionComposesInertInstanceHost(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	lifecycle, err := instancehostwire.New(instancehost.Dependencies{Clock: clock})
	if err != nil {
		t.Fatalf("instancehostwire.New() error = %v", err)
	}
	var _ factoryruntime.Lifecycle = lifecycle
	if _, ok := lifecycle.(instancehost.Service); !ok {
		t.Fatalf("composed lifecycle type = %T, want instance_host.Service", lifecycle)
	}
}
