package testlink_test

import (
	"testing"

	internaltestlink "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testlink"
	testlink "github.com/portpowered/infinite-you/pkg/services/operator_settings/testlink"
)

func TestShimRegisterCompositionDelegatesToInternal(t *testing.T) {
	t.Parallel()

	// Both packages expose the same registration hooks; exercising the shim
	// proves the transitional public path still forwards to the internal owner.
	testlink.RegisterComposition()
	internaltestlink.RegisterComposition()
}
