package bootstrap_portability

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactoryValidation rejects factories whose workstation wiring references
// undeclared workers before runtime bootstrap succeeds.
func TestFactoryValidation_RejectsWorkstationWithNonexistentWorker(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "invalid_worker_reference"))

	cfg := &service.FactoryServiceConfig{
		Dir: dir,
	}

	_, err := service.BuildFactoryService(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected BuildFactoryService to fail for workstation referencing non-existent worker")
	}

	if !strings.Contains(err.Error(), "invalid named factory") {
		t.Errorf("expected load-boundary invalid factory error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid graph references") {
		t.Errorf("expected blocking structural validation summary, got: %v", err)
	}
	if !strings.Contains(err.Error(), "blocking validation targets") {
		t.Errorf("expected blocking validation target count in error, got: %v", err)
	}
}
