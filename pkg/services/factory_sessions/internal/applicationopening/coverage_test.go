package applicationopening

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

func TestRuntimeReadyReturnsNilWithoutReadinessPort(t *testing.T) {
	t.Parallel()

	if got := runtimeReady(nil); got != nil {
		t.Fatalf("runtimeReady(nil) = %v, want nil", got)
	}
}

func TestCloseOpenedRuntimePreservesCauseWithoutCloseOperation(t *testing.T) {
	t.Parallel()

	cause := errors.New("binding failed")
	if got := closeOpenedRuntime(roles.OpenedApplicationRuntime{}, cause); !errors.Is(got, cause) {
		t.Fatalf("closeOpenedRuntime() = %v, want original cause", got)
	}
}
