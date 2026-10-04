package wire_test

import (
	"testing"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
)

func TestNewComposesInertInstanceHostThroughRuntimeRoot(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	lifecycleService, err := factoryhost.NewLifecycleService(clock, platformclock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	host, err := instancehostwire.New(clock, platformclock.Real{}, lifecycleService)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if host == nil {
		t.Fatal("New() = nil, want composed instance host")
	}

	var lifecycle factoryruntime.RuntimeLifecycle = host
	if lifecycle == nil {
		t.Fatal("composed host does not satisfy Factory Runtime lifecycle contract")
	}
}
