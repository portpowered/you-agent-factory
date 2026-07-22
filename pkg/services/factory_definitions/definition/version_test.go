package factorydefinition

import (
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestService_RequireFreshEditableFactoryVersion_RejectsMissingBaseVersion(t *testing.T) {
	t.Parallel()

	err := New(stubDefinitionHost{}).RequireFreshEditableFactoryVersion(nil, interfaces.FactoryVersion{
		Logical:  1,
		Physical: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, apisurface.ErrFactoryVersionStale) {
		t.Fatalf("error = %v, want %v", err, apisurface.ErrFactoryVersionStale)
	}
}

func TestService_RequireFreshEditableFactoryVersion_RejectsStaleVersion(t *testing.T) {
	t.Parallel()

	current := interfaces.FactoryVersion{
		Logical:  5,
		Physical: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
	stale := interfaces.FactoryVersion{
		Logical:  4,
		Physical: time.Date(2026, 3, 1, 12, 0, 1, 0, time.UTC),
	}
	err := New(stubDefinitionHost{}).RequireFreshEditableFactoryVersion(&stale, current)
	if !errors.Is(err, apisurface.ErrFactoryVersionStale) {
		t.Fatalf("error = %v, want %v", err, apisurface.ErrFactoryVersionStale)
	}
}

func TestService_RequireFreshEditableFactoryVersion_AcceptsAdvancedVersion(t *testing.T) {
	t.Parallel()

	current := interfaces.FactoryVersion{
		Logical:  5,
		Physical: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
	advanced := interfaces.FactoryVersion{
		Logical:  6,
		Physical: time.Date(2026, 3, 1, 12, 0, 1, 0, time.UTC),
	}
	if err := New(stubDefinitionHost{}).RequireFreshEditableFactoryVersion(&advanced, current); err != nil {
		t.Fatalf("RequireFreshEditableFactoryVersion: %v", err)
	}
}

func TestService_NextEditableFactoryVersion_AdvancesLogicalAndPhysical(t *testing.T) {
	t.Parallel()

	current := interfaces.FactoryVersion{
		Logical:  7,
		Physical: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	got := New(stubDefinitionHost{}).NextEditableFactoryVersion(&current, now)
	if got.Logical != 8 {
		t.Fatalf("logical = %d, want 8", got.Logical)
	}
	if !got.Physical.After(current.Physical) {
		t.Fatalf("physical = %s, want after %s", got.Physical, current.Physical)
	}
}

func TestService_NextEditableFactoryVersion_UsesNowWhenNoCurrentVersion(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	got := New(stubDefinitionHost{}).NextEditableFactoryVersion(nil, now)
	if got.Logical != 1 || !got.Physical.Equal(now) {
		t.Fatalf("version = %#v, want logical=1 physical=%s", got, now)
	}
}
