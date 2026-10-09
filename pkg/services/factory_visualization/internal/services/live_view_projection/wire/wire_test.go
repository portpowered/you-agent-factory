package wire

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
)

type stubSource struct {
	reads *int
}

func (s stubSource) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	(*s.reads)++
	return nil, nil
}

func (s stubSource) GetRuntimeSnapshotFacts(context.Context) (*liveviewprojection.RuntimeSnapshotFacts, error) {
	(*s.reads)++
	return nil, nil
}

type stubSink struct{}

func (stubSink) PresentFactoryView(liveviewprojection.View) {}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(1, 0) }

func TestOwnerOpeningIsInert(t *testing.T) {
	t.Parallel()

	reads := 0
	behavior := NewOwner(&recordingsstub.Service{})
	svc := behavior.Open(nil, stubSource{reads: &reads}, fixedClock{}, stubSink{}, nil)
	if svc == nil {
		t.Fatal("Open() returned nil")
	}
	if reads != 0 {
		t.Fatalf("opening read the source %d times, want inert opening", reads)
	}
	if err := svc.Wait(context.Background()); err == nil {
		t.Fatal("Wait before Start returned no error")
	}
	if reads != 0 {
		t.Fatalf("Wait before Start read the source %d times", reads)
	}
}
