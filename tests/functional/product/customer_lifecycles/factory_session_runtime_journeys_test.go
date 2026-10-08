package customer_lifecycles_test

import (
	"context"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

// TestFactorySessionRuntimeJourneys shares startup while every scenario owns
// its Factory Session, Factory files and controlled command route.
func TestFactorySessionRuntimeJourneys(t *testing.T) {
	t.Parallel()
	resetorchestrationpetricross2State()
	resetorchestrationpetridispatch5State()
	resetorchestrationpetriguards3State()
	initializeOrchestrationpetricrossFixture(t)
	t.Run("LifecycleParity", runFactorySessionLifecycleParityJourneys)
	t.Run("Dispatch", runFactoryDispatchJourneys)
	t.Run("Eligibility", runFactoryEligibilityJourneys)
}

// A request can consume only the route registered for its Factory directory.
// Checking dispatch membership never invokes a responder from another group.
type sharedSessionRuntimeCommands struct {
	cross    *sharedCrossCommandRouter
	dispatch *sharedPetriCommandRouter
	guards   *sharedGuardCommandRouter
}

func (router sharedSessionRuntimeCommands) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	router.dispatch.mu.Lock()
	matched := router.dispatch.routeForRequest(request) != nil
	router.dispatch.mu.Unlock()
	if matched {
		return router.dispatch.Run(ctx, request)
	}
	router.guards.mu.Lock()
	guarded := router.guards.routeForRequest(request) != nil
	router.guards.mu.Unlock()
	if guarded {
		return router.guards.Run(ctx, request)
	}
	return router.cross.Run(ctx, request)
}
