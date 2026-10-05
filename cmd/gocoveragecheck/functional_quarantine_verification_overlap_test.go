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
