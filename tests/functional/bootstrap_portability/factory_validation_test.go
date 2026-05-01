package bootstrap_portability

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/pkg/service"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
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

	if !strings.Contains(err.Error(), "non-existent worker") {
		t.Errorf("expected error about non-existent worker, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ghost-worker") {
		t.Errorf("expected error to mention the invalid worker name 'ghost-worker', got: %v", err)
	}
}
