package timedisplay

import (
	"testing"
	"time"
)

func TestTimestampUsesExplicitUTCDisplay(t *testing.T) {
	offset := time.FixedZone("UTC+07", 7*60*60)
	value := time.Date(2026, 4, 3, 18, 59, 15, 123, offset)

	got := Timestamp(value)
	want := "2026-04-03 11:59:15 UTC"
	if got != want {
		t.Fatalf("Timestamp() = %q, want %q", got, want)
	}
}

func TestTimestampMissingValue(t *testing.T) {
	if got := Timestamp(time.Time{}); got != "n/a" {
		t.Fatalf("Timestamp(zero) = %q, want n/a", got)
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "zero", d: 0, want: "0s"},
		{name: "negative", d: -time.Second, want: "0s"},
		{name: "subsecond", d: 500 * time.Millisecond, want: "500ms"},
		{name: "seconds", d: 5 * time.Second, want: "5s"},
		{name: "minutes seconds", d: 90 * time.Second, want: "1m30s"},
		{name: "minutes", d: 5 * time.Minute, want: "5m"},
		{name: "hours minutes", d: 2*time.Hour + 15*time.Minute, want: "2h15m"},
		{name: "hours", d: 2 * time.Hour, want: "2h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Duration(tt.d); got != tt.want {
				t.Fatalf("Duration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestElapsedSinceMissingInput(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)
	if got := ElapsedSince(time.Time{}, now); got != "n/a" {
		t.Fatalf("ElapsedSince(zero, now) = %q, want n/a", got)
	}
}
