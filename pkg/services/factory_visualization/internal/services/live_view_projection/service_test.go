package live_view_projection_test

import (
	"testing"

	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	liveviewprojectionwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/wire"
)

func TestWireConstructsSingularLiveViewProjectionService(t *testing.T) {
	t.Parallel()

	svc, err := liveviewprojectionwire.NewService(nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("wire.NewService() error = nil, want missing dependency failure")
	}
	if svc != nil {
		t.Fatal("wire.NewService() returned service with missing dependencies")
	}

	svc, err = liveviewprojectionwire.NewService(
		stubSource{},
		projectionStub{},
		fixedClock{},
		stubSink{},
		nil,
	)
	if err != nil {
		t.Fatalf("wire.NewService() error = %v", err)
	}
	if svc == nil {
		t.Fatal("wire.NewService() returned nil")
	}
	var _ liveviewprojection.Service = svc
}
