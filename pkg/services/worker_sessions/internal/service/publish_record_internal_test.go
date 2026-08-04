package service

import (
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// TestPublishOutcomeLabel_CoversEveryOutcomeIncludingUnspecified proves the
// pure label mapping PublishRecord's logging depends on names every
// PublishOutcome, including the zero value no production PublishRecord call
// ever returns but the type still permits.
func TestPublishOutcomeLabel_CoversEveryOutcomeIncludingUnspecified(t *testing.T) {
	cases := map[workersessions.PublishOutcome]string{
		workersessions.PublishOutcomeAccepted:    "accepted",
		workersessions.PublishOutcomeDuplicate:   "duplicate",
		workersessions.PublishOutcomeUnspecified: "unspecified",
	}
	for outcome, want := range cases {
		if got := publishOutcomeLabel(outcome); got != want {
			t.Errorf("publishOutcomeLabel(%v) = %q, want %q", outcome, got, want)
		}
	}
}
