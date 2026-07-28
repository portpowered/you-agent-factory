package wire

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
)

type stubSource struct{}

func (stubSource) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return nil, nil
}

func (stubSource) GetRuntimeSnapshotFacts(context.Context) (*liveviewprojection.RuntimeSnapshotFacts, error) {
	return nil, nil
}

type stubSink struct{}

func (stubSink) PresentFactoryView(liveviewprojection.View) {}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(1, 0) }

func TestNewServiceConstructsSingularLiveViewProjectionService(t *testing.T) {
	t.Parallel()

	svc, err := NewService(nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("NewService() error = nil, want missing dependency failure")
	}
	if svc != nil {
		t.Fatal("NewService() returned service with missing dependencies")
	}

	svc, err = NewService(
		stubSource{},
		&recordingsstub.Service{},
		fixedClock{},
		stubSink{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
	var _ liveviewprojection.Service = svc
}
