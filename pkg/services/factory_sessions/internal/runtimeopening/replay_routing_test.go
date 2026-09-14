package runtimeopening

import (
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestReplayRequestsHistoricalInspectionUsesEffectiveListenerPort(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		host factorysessions.RuntimeHostRequest
		want bool
	}{
		{
			name: "offline clears port but retains parsed auto port default",
			host: factorysessions.RuntimeHostRequest{AutoPort: true},
			want: true,
		},
		{
			name: "hosted replay has a resolved listener port",
			host: factorysessions.RuntimeHostRequest{Port: 7437, AutoPort: true},
			want: false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			request := &factorysessions.RuntimeOpeningRequest{
				FactorySession: factorysessions.SessionRuntimeOpeningRequest{Host: testCase.host},
			}
			if got := replayRequestsHistoricalInspection(request); got != testCase.want {
				t.Fatalf("replayRequestsHistoricalInspection() = %t, want %t", got, testCase.want)
			}
		})
	}
}
