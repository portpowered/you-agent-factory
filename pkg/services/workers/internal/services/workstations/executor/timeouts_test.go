package executor

import (
	"testing"
	"time"
)

func TestPrintTimeoutFromWorkerTimeoutPreservesValidNativePrintLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "unset", raw: "", want: 0},
		{name: "media", raw: "8m", want: 8 * time.Minute},
		{name: "invalid", raw: "not-a-duration", want: 0},
		{name: "non-positive", raw: "0s", want: 0},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := PrintTimeoutFromWorkerTimeout(test.raw); got != test.want {
				t.Fatalf("PrintTimeoutFromWorkerTimeout(%q) = %s, want %s", test.raw, got, test.want)
			}
		})
	}
}
