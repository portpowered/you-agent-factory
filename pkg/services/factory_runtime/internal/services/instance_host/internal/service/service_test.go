package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/jonboulle/clockwork"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
)

func TestNewRequiresClock(t *testing.T) {
	t.Parallel()

	host, err := New(instancehost.Dependencies{})
	if host != nil || err == nil || !errors.Is(err, instancehost.ErrInvalidDependencies) ||
		!strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("New() = (%v, %v), want invalid-dependencies clock error", host, err)
	}
}

func TestNewConstructsInertHost(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	host, err := New(instancehost.Dependencies{Clock: clock})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if host == nil {
		t.Fatal("New() = nil, want inert instance host")
	}

	concrete, ok := host.(*Host)
	if !ok {
		t.Fatalf("New() type = %T, want *Host", host)
	}
	if concrete.lifecycle == nil {
		t.Fatal("constructed host has nil lifecycle delegate")
	}
	if concrete.handles == nil {
		t.Fatal("constructed host did not allocate handle state")
	}
	if len(concrete.handles) != 0 {
		t.Fatalf("constructed host handles = %d, want empty inert registry", len(concrete.handles))
	}
}
