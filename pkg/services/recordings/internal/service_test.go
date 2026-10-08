package internal

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestRecordingTargetOwnershipKeepsSafePathAndCauseIdentity(t *testing.T) {
	t.Parallel()
	path := filepath.Join("recordings", "board.json")
	marker, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		marker = strings.ToLower(marker)
	}
	marker += ".recording.lock"
	cause := errors.New("PRIVATE_RECORDING_PAYLOAD")
	service := &combinedService{targetClaim: func(ctx context.Context, got string) (io.Closer, error) {
		if ctx != t.Context() || got != marker {
			t.Errorf("claim did not receive the opening context and absolute marker: %q", got)
		}
		return nil, cause
	}}
	lease, err := service.ClaimRecordingTarget(t.Context(), path)
	var diagnostic interface{ CLIErrorCode() string }
	if lease != nil || !errors.Is(err, cause) || !errors.Is(err, recordings.ErrRecordingBindingConflict) ||
		!errors.As(err, &diagnostic) || diagnostic.CLIErrorCode() != "RECORDING_TARGET_CONFLICT" {
		t.Fatalf("claim refusal lost classification/identity: lease=%v error=%v", lease, err)
	}
	message := logging.SafeErrorCause(err)
	if !strings.Contains(message, strconv.Quote(path)) || strings.Contains(message, cause.Error()) {
		t.Fatalf("unsafe or pathless claim diagnostic: %s", message)
	}
}

func TestRecordingTargetOwnershipRequiresConfirmedLease(t *testing.T) {
	t.Parallel()
	for _, claim := range []recordings.RecordingTargetClaim{nil, func(context.Context, string) (io.Closer, error) { return nil, nil }} {
		service := &combinedService{targetClaim: claim}
		lease, err := service.ClaimRecordingTarget(t.Context(), "board.json")
		if lease != nil || !errors.Is(err, recordings.ErrRecordingBindingConflict) {
			t.Fatalf("unconfirmed ownership accepted: lease=%v error=%v", lease, err)
		}
	}
}

func TestNewLifecycleRuntimeRecorderRejectsMissingClockWhenRecordingEnabled(t *testing.T) {
	t.Parallel()
	recorder, err := NewLifecycleRuntimeRecorder(0, nil, nil, "", "recording.json", nil)
	if recorder != nil || err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("NewLifecycleRuntimeRecorder = (%#v, %v), want required clock error", recorder, err)
	}
}

func TestNewLifecycleRuntimeRecorderAllowsDisabledRecordingWithoutClock(t *testing.T) {
	t.Parallel()
	recorder, err := NewLifecycleRuntimeRecorder(time.Second, nil, nil, "", "", nil)
	if recorder != nil || err != nil {
		t.Fatalf("NewLifecycleRuntimeRecorder disabled = (%#v, %v), want nil, nil", recorder, err)
	}
}
