package internal_test

import (
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
)

func TestComposedLifecycleHostExercisesVersionSurface(t *testing.T) {
	t.Parallel()

	service := lifecycle.New(nil, lifecycle.StubActivationGateway())
	next := service.NextEditableFactoryVersion(nil, time.Unix(42, 0).UTC())
	if next.Logical != 1 {
		t.Fatalf("NextEditableFactoryVersion logical = %d, want 1", next.Logical)
	}
	if !next.Physical.Equal(time.Unix(42, 0).UTC()) {
		t.Fatalf("NextEditableFactoryVersion physical = %v, want unix 42", next.Physical)
	}

	current := factorydefinitions.FactoryVersion{Logical: 3, Physical: time.Unix(100, 0).UTC()}
	base := factorydefinitions.FactoryVersion{Logical: 4, Physical: time.Unix(101, 0).UTC()}
	if err := service.RequireFreshEditableFactoryVersion(&base, current); err != nil {
		t.Fatalf("RequireFreshEditableFactoryVersion() error = %v, want success", err)
	}
}
