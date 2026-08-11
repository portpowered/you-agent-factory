package support

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitForObservationReturnsImmediateSuccess(t *testing.T) {
	calls := 0
	value, err := WaitForObservation(time.Second, func() (string, error) {
		calls++
		return "ready", nil
	}, func(value string) bool {
		return value == "ready"
	})
	if err != nil {
		t.Fatalf("WaitForObservation() error = %v", err)
	}
	if value != "ready" || calls != 1 {
		t.Fatalf("WaitForObservation() = %q after %d calls, want ready after one call", value, calls)
	}
}

func TestWaitForObservationReturnsDelayedSuccess(t *testing.T) {
	calls := 0
	value, err := WaitForObservation(100*time.Millisecond, func() (int, error) {
		calls++
		return calls, nil
	}, func(value int) bool {
		return value >= 3
	})
	if err != nil {
		t.Fatalf("WaitForObservation() error = %v", err)
	}
	if value != 3 || calls != 3 {
		t.Fatalf("observation = %d after %d calls, want 3 after three calls", value, calls)
	}
}

func TestWaitForObservationTimeoutReturnsLastObservation(t *testing.T) {
	value, err := WaitForObservation(5*time.Millisecond, func() (string, error) {
		return "still-processing", nil
	}, func(string) bool {
		return false
	})
	if err == nil {
		t.Fatal("WaitForObservation() error = nil, want timeout")
	}
	if value != "still-processing" {
		t.Fatalf("last observation = %q, want still-processing", value)
	}
	if !strings.Contains(err.Error(), "last observation=\"still-processing\"") {
		t.Fatalf("timeout error = %q, want last observation diagnostic", err)
	}
}

func TestWaitForObservationTimeoutIncludesObserverError(t *testing.T) {
	wantErr := errors.New("status endpoint unavailable")
	_, err := WaitForObservation(5*time.Millisecond, func() (int, error) {
		return 7, wantErr
	}, func(int) bool {
		return true
	})
	if err == nil {
		t.Fatal("WaitForObservation() error = nil, want timeout")
	}
	if !strings.Contains(err.Error(), "last observation=7") || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("timeout error = %q, want observation and observer error diagnostics", err)
	}
}
