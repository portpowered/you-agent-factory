package wire_test

import (
	"fmt"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
)

func TestNewConstructsOrchestrationService(t *testing.T) {
	t.Parallel()

	service := orchestrationwire.New(testIDGenerator(), nil, nil)
	if service == nil {
		t.Fatal("New() = nil, want orchestration.Service")
	}
	if _, ok := service.(orchestration.Service); !ok {
		t.Fatalf("New() = %T, want orchestration.Service", service)
	}
}

func testIDGenerator() factoryruntime.IDGenerator {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("orchestration-wire-test-id-%d", next)
	}
}
