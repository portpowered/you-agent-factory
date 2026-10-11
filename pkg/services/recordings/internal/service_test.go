package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
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
	service := &combinedService{caseInsensitive: runtime.GOOS == "windows", targetClaim: func(ctx context.Context, target, got string) (io.Closer, error) {
		if ctx != t.Context() || target != path || got != marker {
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

func TestRecordingTargetValidationKeepsSafePathAndCauseIdentity(t *testing.T) {
	t.Parallel()
	cause := errors.New("PRIVATE_RECORDING_PAYLOAD")
	service := &combinedService{caseInsensitive: runtime.GOOS == "windows", targetClaim: func(context.Context, string, string) (io.Closer, error) {
		return &changedRecordingTarget{cause: cause}, nil
	}}
	lease, err := service.ClaimRecordingTarget(t.Context(), "board.json")
	if err != nil {
		t.Fatal(err)
	}
	err = lease.(recordings.RecordingTargetValidator).Validate()
	var coded interface{ CLIErrorCode() string }
	if !errors.Is(err, cause) || !errors.Is(err, recordings.ErrRecordingBindingConflict) ||
		!errors.As(err, &coded) || coded.CLIErrorCode() != "RECORDING_TARGET_CONFLICT" {
		t.Fatalf("validation lost typed classification or cause: %v", err)
	}
	message := logging.SafeErrorCause(err)
	if !strings.Contains(message, `"board.json"`) || strings.Contains(message, cause.Error()) {
		t.Fatalf("unsafe validation diagnostic: %s", message)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
}

type changedRecordingTarget struct{ cause error }

func (*changedRecordingTarget) Close() error          { return nil }
func (lease *changedRecordingTarget) Validate() error { return lease.cause }

func TestRecordingTargetOwnershipRequiresConfirmedLease(t *testing.T) {
	t.Parallel()
	service := &combinedService{targetClaim: func(context.Context, string, string) (io.Closer, error) { return nil, nil }}
	lease, err := service.ClaimRecordingTarget(t.Context(), "board.json")
	if lease != nil || !errors.Is(err, recordings.ErrRecordingBindingConflict) {
		t.Fatalf("unconfirmed ownership accepted: lease=%v error=%v", lease, err)
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

type ownedPublicationLifecycle struct {
	recordinglifecycle.Service
	cause  error
	writes int
	status recordings.RecordingStatusFacts
}

func (owner *ownedPublicationLifecycle) QueryRecordingStatus(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{Status: owner.status}, nil
}
func (owner *ownedPublicationLifecycle) FlushRecording(recordings.FlushRecordingRequest) (recordings.FlushRecordingResult, error) {
	owner.writes++
	if owner.cause != nil {
		return recordings.FlushRecordingResult{}, owner.cause
	}
	owner.status.FlushedThrough = &recordings.CanonicalEventCursor{StreamGenerationID: "generation", Sequence: 1}
	return recordings.FlushRecordingResult{Status: owner.status}, nil
}

func TestOwnedRecordingPublicationValidatesInputThenRetainsWriterOwnership(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed-write=%t", fail), func(t *testing.T) {
			t.Parallel()
			target := &changedRecordingTarget{}
			writeFailure := errors.New("controlled write failure")
			lifecycle := &ownedPublicationLifecycle{status: recordings.RecordingStatusFacts{Artifact: "owned.json"}}
			if fail {
				lifecycle.cause = writeFailure
			}
			owner := &combinedService{Service: lifecycle, targetClaim: func(context.Context, string, string) (io.Closer, error) { return target, nil }}
			lease, err := owner.ClaimRecordingTarget(t.Context(), "owned.json")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := lease.Close(); err != nil {
					t.Error(err)
				}
			}()
			request := recordings.FlushRecordingRequest{RecordingID: "owned"}
			_, err = owner.FlushRecording(request)
			if !errors.Is(err, lifecycle.cause) {
				t.Fatalf("first publication = %v", err)
			}
			target.cause = errors.New("input fingerprint changed")
			validationErr := lease.(recordings.RecordingTargetValidator).Validate()
			_, flushErr := owner.FlushRecording(request)
			if fail {
				if !errors.Is(validationErr, target.cause) || !errors.Is(flushErr, target.cause) || lifecycle.writes != 1 {
					t.Fatalf("failed publication dropped input protection: validation=%v flush=%v writes=%d", validationErr, flushErr, lifecycle.writes)
				}
			} else if validationErr != nil || flushErr != nil || lifecycle.writes != 2 {
				t.Fatalf("owned publication mistaken for external replacement: validation=%v flush=%v writes=%d", validationErr, flushErr, lifecycle.writes)
			}
		})
	}
}

func unavailableTargetClaim(context.Context, string, string) (io.Closer, error) {
	return nil, recordings.ErrRecordingBindingConflict
}

type claimLease struct {
	cause  error
	closed int
}

func (lease *claimLease) Close() error {
	lease.closed++
	return lease.cause
}

func TestRecordingTargetDirectConstructionAndCasePolicy(t *testing.T) {
	for _, insensitive := range []bool{false, true} {
		t.Run(fmt.Sprintf("case-insensitive=%t", insensitive), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("Recordings", "Board.JSON")
			key, err := filepath.Abs(path)
			if err != nil {
				t.Fatal(err)
			}
			if insensitive {
				key = strings.ToLower(key)
			}
			calls := 0
			lease := &claimLease{}
			claim := func(ctx context.Context, target, marker string) (io.Closer, error) {
				calls++
				if ctx != t.Context() || target != path || marker != key+".recording.lock" {
					t.Fatalf("claim arguments = %v, %q, %q", ctx, target, marker)
				}
				return lease, nil
			}
			service := NewCombinedService(nil, nil, nil, nil, nil, nil, nil,
				staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil, claim, insensitive).(*combinedService)
			if calls != 0 {
				t.Fatal("construction acquired ownership")
			}
			owned, err := service.ClaimRecordingTarget(t.Context(), path)
			if err != nil || calls != 1 {
				t.Fatalf("claim = %v; calls = %d", err, calls)
			}
			if service.recordingTargetValidator(path) != owned.(recordings.RecordingTargetValidator) {
				t.Fatal("selected target has no validator")
			}
			if got := service.recordingTargetValidator(strings.ToUpper(path)); (got == owned.(recordings.RecordingTargetValidator)) != insensitive {
				t.Fatal("case policy did not select the expected ownership")
			}
			if err := owned.Close(); err != nil || lease.closed != 1 || service.recordingTargetValidator(path) != nil {
				t.Fatalf("release = %v; closes = %d", err, lease.closed)
			}
		})
	}
}

func TestRecordingTargetReleaseFailureRetryAndStalePeerIsolation(t *testing.T) {
	t.Parallel()
	cause := errors.New("release unavailable")
	selected, replacement, peer := &claimLease{cause: cause}, &claimLease{}, &claimLease{}
	leases := []*claimLease{selected, peer, replacement}
	service := &combinedService{targetClaim: func(context.Context, string, string) (io.Closer, error) {
		lease := leases[0]
		leases = leases[1:]
		return lease, nil
	}}
	first, err := service.ClaimRecordingTarget(t.Context(), "selected.json")
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.ClaimRecordingTarget(t.Context(), "peer.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); !errors.Is(err, cause) || service.recordingTargetValidator("selected.json") != first.(recordings.RecordingTargetValidator) {
		t.Fatalf("failed release lost ownership/cause: %v", err)
	}
	selected.cause = nil
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	later, err := service.ClaimRecordingTarget(t.Context(), "selected.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil || service.recordingTargetValidator("selected.json") != later.(recordings.RecordingTargetValidator) {
		t.Fatalf("stale close removed later ownership: %v", err)
	}
	if service.recordingTargetValidator("peer.json") != other.(recordings.RecordingTargetValidator) || peer.closed != 0 {
		t.Fatal("selected release affected peer")
	}
	if err := later.Close(); err != nil {
		t.Fatal(err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
}
