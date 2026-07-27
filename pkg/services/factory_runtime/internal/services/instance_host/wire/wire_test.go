package wire_test

import (
	"testing"

	"github.com/jonboulle/clockwork"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
)

func TestNewComposesInertInstanceHostThroughRuntimeRoot(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	host, err := instancehostwire.New(instancehost.Dependencies{Clock: clock})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if host == nil {
		t.Fatal("New() = nil, want composed instance host")
	}

	var lifecycle factoryruntime.Lifecycle = host
	if lifecycle == nil {
		t.Fatal("composed host does not satisfy Factory Runtime lifecycle contract")
	}
}
