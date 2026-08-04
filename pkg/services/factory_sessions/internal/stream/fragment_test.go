package stream_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestMapProgressFragment_PreservesOnlyProjectableProgressPhases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		fragment workers.ProgressFragment
		wantType responsestream.EventType
	}{
		{
			name: "reasoning delta",
			fragment: workers.ProgressFragment{
				Kind: workers.ProgressFragmentKind, Type: "delta",
				Metadata: map[string]string{"kind": "reasoning"},
			},
			wantType: responsestream.EventType("delta"),
		},
		{
			name: "session title update",
			fragment: workers.ProgressFragment{
				Kind: workers.ProgressFragmentKind, Type: "updated",
				Metadata: map[string]string{"kind": "session", "title_present": "true"},
			},
			wantType: responsestream.EventType("updated"),
		},
		{
			name: "session lifecycle remains bounded progress",
			fragment: workers.ProgressFragment{
				Kind: workers.ProgressFragmentKind, Type: "started",
				Metadata: map[string]string{"kind": "session"},
			},
			wantType: responsestream.EventTypeProgress,
		},
		{
			name: "message remains coalesced progress",
			fragment: workers.ProgressFragment{
				Kind: workers.ProgressFragmentKind, Type: "delta",
				Metadata: map[string]string{"kind": "message"},
			},
			wantType: responsestream.EventTypeProgress,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			event := stream.MapProgressFragment(tc.fragment)
			if event.Type != tc.wantType {
				t.Fatalf("event type = %q, want %q", event.Type, tc.wantType)
			}
		})
	}
}
