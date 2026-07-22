package service

import (
	"strings"
	"testing"
)

func TestNewRuntimeRecorderRejectsMissingClockWhenRecordingEnabled(t *testing.T) {
	t.Parallel()
	recorder, err := NewRuntimeRecorder(nil, 0, nil, nil, "recording.json", nil)
	if recorder != nil || err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("NewRuntimeRecorder = (%#v, %v), want required clock error", recorder, err)
	}
}

func TestNewRuntimeRecorderAllowsDisabledRecordingWithoutClock(t *testing.T) {
	t.Parallel()
	recorder, err := NewRuntimeRecorder(nil, 0, nil, nil, "", nil)
	if recorder != nil || err != nil {
		t.Fatalf("NewRuntimeRecorder disabled = (%#v, %v), want nil, nil", recorder, err)
	}
}
