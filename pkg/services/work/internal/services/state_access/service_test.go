package state_access_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
	stateaccesswire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

func TestWireConstructsSingularStateAccessService(t *testing.T) {
	t.Parallel()

	svc := stateaccesswire.NewService(stateaccesswire.NewRuntimeSessionResolver(nil))
	if svc == nil {
		t.Fatal("wire.NewService() returned nil")
	}
	var _ state_access.Service = svc
}
