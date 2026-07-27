package wire_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	leaseswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/wire"
)

func TestNewServiceRequiresLeaseDependencies(t *testing.T) {
	t.Parallel()

	clock := testHostClock{}

	tests := []struct {
		name            string
		clock           models.HostClock
		wantContains    string
		wantInvalidDeps bool
	}{
		{name: "valid", clock: clock},
		{
			name: "host clock", wantContains: "clock",
			wantInvalidDeps: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := leaseswire.NewService(test.clock)
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
func (testHostClock) NewTimer(time.Duration) models.HostTimer {
	panic("host timer created during inert leases construction")
}
