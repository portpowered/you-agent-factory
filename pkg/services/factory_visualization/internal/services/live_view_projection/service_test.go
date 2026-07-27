package live_view_projection_test

import (
	"errors"
	"testing"

	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
)

func TestProjectionErrorMessageAndUnwrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("underlying")
	err := &liveviewprojection.ProjectionError{
		Kind:    liveviewprojection.ProjectionErrorInvalidInput,
		Message: "invalid subscribe input",
		Cause:   cause,
	}
	if err.Error() != "invalid subscribe input" {
		t.Fatalf("Error() = %q, want invalid subscribe input", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatalf("Unwrap() = %v, want %v", errors.Unwrap(err), cause)
	}

	kindOnly := &liveviewprojection.ProjectionError{Kind: liveviewprojection.ProjectionErrorSnapshotUnavailable}
	if kindOnly.Error() != string(liveviewprojection.ProjectionErrorSnapshotUnavailable) {
		t.Fatalf("Error() = %q, want kind fallback", kindOnly.Error())
	}
}

func TestSinkFuncPresentsView(t *testing.T) {
	t.Parallel()

	var got liveviewprojection.View
	sink := liveviewprojection.SinkFunc(func(view liveviewprojection.View) {
		got = view
	})
	want := liveviewprojection.View{RetainedEventCount: 2}
	sink.PresentFactoryView(want)
	if got.RetainedEventCount != want.RetainedEventCount {
		t.Fatalf("PresentFactoryView() retained count = %d, want %d", got.RetainedEventCount, want.RetainedEventCount)
	}
}
