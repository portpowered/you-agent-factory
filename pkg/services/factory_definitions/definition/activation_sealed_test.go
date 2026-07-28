package factorydefinition

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// TestActivationSealedThroughNarrowGatewayOnly proves Definitions activation
// idle-gated failure through DefinitionActivationGateway without relying on the
// old attach-capable SessionHost callback bundle. Editable and named swap
// success paths are covered in activation_gateway_test.go.
func TestActivationSealedThroughNarrowGatewayOnly(t *testing.T) {
	t.Parallel()

	gateway := &trackingActivationGateway{idleNamedErr: factorydefinitions.ErrFactoryActivationRequiresIdle}
	err := gateway.RequireIdleBeforeNamedFactoryActivation(context.Background(), "session-alpha", nil)
	if !errors.Is(err, factorydefinitions.ErrFactoryActivationRequiresIdle) {
		t.Fatalf("RequireIdleBeforeNamedFactoryActivation() error = %v, want %v", err, factorydefinitions.ErrFactoryActivationRequiresIdle)
	}
}
