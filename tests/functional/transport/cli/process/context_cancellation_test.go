package process_test

import (
	"strconv"
	"strings"
	"testing"
)

// TestContextCancellationPIDReadinessIgnoresPartialPublication proves the
// readiness parser is process-free and accepts only a complete positive PID.
func TestContextCancellationPIDReadinessIgnoresPartialPublication(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
		ok   bool
	}{
		{name: "empty publication", raw: "", want: 0, ok: false},
		{name: "whitespace publication", raw: "\n", want: 0, ok: false},
		{name: "partial publication", raw: "12x", want: 0, ok: false},
		{name: "non numeric publication", raw: "worker", want: 0, ok: false},
		{name: "complete publication", raw: "12345\n", want: 12345, ok: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseContextCancellationProviderPID([]byte(tc.raw))
			if got != tc.want || ok != tc.ok {
				t.Fatalf("parseContextCancellationProviderPID(%q) = (%d, %t), want (%d, %t)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func parseContextCancellationProviderPID(raw []byte) (int, bool) {
	contents := strings.TrimSpace(string(raw))
	if contents == "" {
		return 0, false
	}
	pid, err := strconv.Atoi(contents)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
