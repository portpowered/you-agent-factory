package agy_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"
)

func TestConstructionRejectsMissingPTYAllocator(t *testing.T) {
	t.Parallel()

	adapter, err := agy.NewAdapterWithAllocator(t.TempDir(), nil)
	if adapter != nil || err == nil || !strings.Contains(err.Error(), "PTY allocator is required") {
		t.Fatalf("NewAdapterWithAllocator() = (%v, %v), want nil adapter and required-allocator error", adapter, err)
	}
}
