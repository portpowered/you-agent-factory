package factorysession_test

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryStateToLifecycleStatus_MapsLiveFactoryStates(t *testing.T) {
	cases := []struct {
		state interfaces.FactoryState
		want  factorysessionexecution.LifecycleStatus
	}{
		{interfaces.FactoryStateIdle, factorysessionexecution.LifecycleStatusRunning},
		{interfaces.FactoryStateRunning, factorysessionexecution.LifecycleStatusRunning},
		{interfaces.FactoryStatePaused, factorysessionexecution.LifecycleStatusPaused},
		{interfaces.FactoryStateCompleted, factorysessionexecution.LifecycleStatusSucceeded},
		{interfaces.FactoryStateFailed, factorysessionexecution.LifecycleStatusFailed},
	}
	for _, tc := range cases {
		if got := factorysession.FactoryStateToLifecycleStatus(tc.state); got != tc.want {
			t.Fatalf("state %q = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestLiveLifecycleControlResponse_BuildsTypedPauseOutcome(t *testing.T) {
	response := factorysession.LiveLifecycleControlResponse(
		"~default",
		factorysessionexecution.LifecycleControlPause,
		factorysessionexecution.LifecycleControlOutcomeAccepted,
		factorysessionexecution.LifecycleStatusPaused,
	)
	if response.SessionId != "~default" {
		t.Fatalf("sessionId = %q, want ~default", response.SessionId)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", response.Status)
	}
	if response.Links == nil || response.Links.Session == nil || *response.Links.Session != "/factory-sessions/~default" {
		t.Fatalf("links = %#v, want /factory-sessions/~default", response.Links)
	}
}
