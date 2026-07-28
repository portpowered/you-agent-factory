package service

import (
	"testing"
	"time"

	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
)

func TestScriptPollerRestartBackoff_ProgressionBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: scriptpollers.ScriptPollerRestartBackoffMin},
		{attempt: 2, want: 2 * scriptpollers.ScriptPollerRestartBackoffMin},
		{attempt: 3, want: 4 * scriptpollers.ScriptPollerRestartBackoffMin},
		{attempt: 4, want: 8 * scriptpollers.ScriptPollerRestartBackoffMin},
		{attempt: 5, want: scriptPollerRestartBackoffMax},
		{attempt: 10, want: scriptPollerRestartBackoffMax},
	}
	for _, tc := range cases {
		tc := tc
		t.Run("", func(t *testing.T) {
			t.Parallel()
			if got := scriptPollerRestartBackoff(tc.attempt); got != tc.want {
				t.Fatalf("scriptPollerRestartBackoff(%d) = %s, want %s", tc.attempt, got, tc.want)
			}
		})
	}
}
