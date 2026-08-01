package service

import (
	"testing"
	"time"
)

func TestScriptPollerRestartBackoff_ProgressionBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: scriptPollerRestartBackoffMin},
		{attempt: 2, want: 2 * scriptPollerRestartBackoffMin},
		{attempt: 3, want: 4 * scriptPollerRestartBackoffMin},
		{attempt: 4, want: 8 * scriptPollerRestartBackoffMin},
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
