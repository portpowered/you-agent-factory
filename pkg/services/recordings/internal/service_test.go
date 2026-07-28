package internal

import (
	"strings"
	"testing"
	"time"
)

func TestNewLifecycleRuntimeRecorderRejectsMissingClockWhenRecordingEnabled(t *testing.T) {
	t.Parallel()
	recorder, err := NewLifecycleRuntimeRecorder(0, nil, nil, "recording.json", nil)
	if recorder != nil || err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("NewLifecycleRuntimeRecorder = (%#v, %v), want required clock error", recorder, err)
	}
}

func TestNewLifecycleRuntimeRecorderAllowsDisabledRecordingWithoutClock(t *testing.T) {
	t.Parallel()
	recorder, err := NewLifecycleRuntimeRecorder(time.Second, nil, nil, "", nil)
	if recorder != nil || err != nil {
		t.Fatalf("NewLifecycleRuntimeRecorder disabled = (%#v, %v), want nil, nil", recorder, err)
	}
}
