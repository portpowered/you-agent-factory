package service_test

import (
	"testing"

	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/internal/service"
)

func TestOrchestrationServiceDoesNotImplementDispatchPlanning(t *testing.T) {
	t.Parallel()

	var service orchestration.Service = (*internalservice.Compiler)(nil)
	if _, ok := any(service).(dispatchplanning.Service); ok {
		t.Fatal("orchestration.Service must not implement dispatch_planning.Service")
	}
}
