package wire_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	leaseswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/wire"
)

func TestNewServiceRequiresLeaseDependencies(t *testing.T) {
	t.Parallel()

	clock := testHostClock{}
	slotFacts := modelseffects.UnconfiguredSlotFacts{}

	tests := []struct {
		name            string
		clock           modelseffects.HostClock
		slotFacts       modelseffects.SlotFactsProvider
		wantContains    string
		wantInvalidDeps bool
	}{
		{name: "valid", clock: clock, slotFacts: slotFacts},
		{
			name: "host clock", wantContains: "clock",
			slotFacts: slotFacts, wantInvalidDeps: true,
		},
		{
			name: "slot facts", wantContains: "slot facts",
			clock: clock, wantInvalidDeps: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := leaseswire.NewService(test.clock, test.slotFacts)
			if test.wantInvalidDeps {
				if service != nil || err == nil {
					t.Fatalf("NewService = (%#v, %v), want dependency error", service, err)
				}
				if !errors.Is(err, models.ErrInvalidHostDependencies) {
					t.Fatalf("error = %v, want ErrInvalidHostDependencies", err)
				}
				if test.wantContains != "" && !strings.Contains(err.Error(), test.wantContains) {
					t.Fatalf("error = %q, want substring %q", err.Error(), test.wantContains)
				}
				return
			}
			if service == nil || err != nil {
				t.Fatalf("NewService = (%#v, %v), want service", service, err)
			}
		})
	}
}

type testHostClock struct{}

func (testHostClock) Now() time.Time { return time.Unix(0, 0) }
func (testHostClock) NewTimer(time.Duration) modelseffects.HostTimer {
	panic("host timer created during inert leases construction")
}
