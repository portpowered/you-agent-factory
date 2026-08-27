package mock

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// assertMockGateTimeoutDispatch proves the configured mock-worker gate is a
// public execution timeout, rather than a test-level wait expiring. The
// timeout remains customer-safe, and no mock gate output leaks into the
// dispatch response.
func assertMockGateTimeoutDispatch(
	t *testing.T,
	observation support.DispatchEventObservation,
) {
	t.Helper()

	if observation.Response == nil {
		t.Fatalf("gate-timeout dispatch response missing: %#v", observation)
	}
	payload := observation.Response
	if payload.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("gate-timeout outcome = %s, want %s", payload.Outcome, factoryapi.WorkOutcomeFailed)
	}
	if payload.FailureDetail == nil || payload.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("gate-timeout failure detail = %#v, want timeout", payload.FailureDetail)
	}
	if !strings.Contains(strings.ToLower(payload.FailureDetail.Message), "timed out") {
		t.Fatalf("gate-timeout failure message = %q, want timeout diagnostic", payload.FailureDetail.Message)
	}
	if payload.Error == nil || !strings.Contains(strings.ToLower(*payload.Error), "timed out") {
		t.Fatalf("gate-timeout error = %#v, want timeout diagnostic", payload.Error)
	}
	if payload.Output != nil {
		t.Fatalf("gate-timeout output = %q, want no output", *payload.Output)
	}
	if observation.Request.TransitionId != gateTimeoutWorkstation {
		t.Fatalf(
			"gate-timeout transition = %q, want %q",
			observation.Request.TransitionId,
			gateTimeoutWorkstation,
		)
	}
}
