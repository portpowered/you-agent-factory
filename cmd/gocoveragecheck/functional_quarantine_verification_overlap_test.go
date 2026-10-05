package main

import (
	"errors"
	"testing"
)

func TestFunctionalQuarantineSelectorVerificationOverlapDefersJoinWithoutDroppingFailure(t *testing.T) {
	verification := newFunctionalQuarantineSelectorVerification(1)
	verification.overlapCoverage = true
	wantErr := errors.New("selector skipped unexpectedly")
	verification.done <- wantErr

	if err := verification.waitBeforeSelection(); err != nil {
		t.Fatalf("waitBeforeSelection() with overlap = %v, want nil without joining", err)
	}
	if err := verification.waitAll(); !errors.Is(err, wantErr) {
		t.Fatalf("waitAll() = %v, want deferred selector failure %v", err, wantErr)
	}
}

func TestFunctionalQuarantineSelectorVerificationJoinsBeforeSelectionByDefault(t *testing.T) {
	verification := newFunctionalQuarantineSelectorVerification(1)
	wantErr := errors.New("selector failed")
	verification.done <- wantErr

	if err := verification.waitBeforeSelection(); !errors.Is(err, wantErr) {
		t.Fatalf("waitBeforeSelection() = %v, want %v", err, wantErr)
	}
}

func TestFunctionalQuarantineOverlapDefersRatchetAndRetainsItsFailure(t *testing.T) {
	verification := newFunctionalQuarantineSelectorVerification(1)
	verification.overlapCoverage = true
	verification.done <- nil
	wantErr := errors.New("quarantined case unexpectedly passed")
	verification.ratchet = &functionalQuarantineRatchetVerification{
		done: make(chan functionalQuarantineRatchetResult, 1),
	}
	verification.ratchet.done <- functionalQuarantineRatchetResult{err: wantErr}

	if err := verification.waitRatchetBeforeSelection(); err != nil {
		t.Fatalf("selection joined the overlapping ratchet: %v", err)
	}
	if len(verification.ratchet.done) != 1 {
		t.Fatal("selection consumed the pending ratchet result")
	}
	if err := verification.waitAll(); !errors.Is(err, wantErr) {
		t.Fatalf("final join = %v, want ratchet failure %v", err, wantErr)
	}
}

func TestFunctionalQuarantineDefaultSelectionRetainsRatchetFailure(t *testing.T) {
	verification := newFunctionalQuarantineSelectorVerification(1)
	wantErr := errors.New("quarantined case unexpectedly passed")
	verification.ratchet = &functionalQuarantineRatchetVerification{
		done: make(chan functionalQuarantineRatchetResult, 1),
	}
	verification.ratchet.done <- functionalQuarantineRatchetResult{err: wantErr}
	if err := verification.waitRatchetBeforeSelection(); !errors.Is(err, wantErr) {
		t.Fatalf("default selection = %v, want %v", err, wantErr)
	}
}
