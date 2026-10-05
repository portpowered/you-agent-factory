package wire_test

import (
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
)

func TestNewConstructsOrchestrationService(t *testing.T) {
	t.Parallel()

	service := orchestrationwire.New(testDefinitionMapper(t), nil, nil)
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

func testDefinitionMapper(t *testing.T) *definitionmapping.Mapper {
	t.Helper()
	mapper, err := definitionmapping.New(testIDGenerator())
	if err != nil {
		t.Fatal(err)
	}
	return mapper
}
