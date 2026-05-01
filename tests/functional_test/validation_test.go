package functional_test

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestValidation_RejectsWorkstationWithNonexistentWorker verifies that
// BuildFactoryService returns a clear validation error when a workstation
// references a worker that is not declared in the workers array.
func TestValidation_RejectsWorkstationWithNonexistentWorker(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "invalid_worker_reference"))

	cfg := &service.FactoryServiceConfig{
		Dir: dir,
	}

	_, err := service.BuildFactoryService(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected BuildFactoryService to fail for workstation referencing non-existent worker")
	}

	if !strings.Contains(err.Error(), "non-existent worker") {
		t.Errorf("expected error about non-existent worker, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ghost-worker") {
		t.Errorf("expected error to mention the invalid worker name 'ghost-worker', got: %v", err)
	}
}
