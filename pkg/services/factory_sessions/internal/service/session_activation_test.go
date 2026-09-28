package service

import (
	"context"
	"errors"
	"testing"
)

func TestSessionActivationCloseRetriesOnlyFailedPhase(t *testing.T) {
	workerStops := 0
	artifactCloses := 0
	activation := &sessionActivation{
		stopWorker: func(context.Context) error {
			workerStops++
			return nil
		},
		closeArtifacts: func() error {
			artifactCloses++
			if artifactCloses == 1 {
				return errors.New("artifact close failed")
			}
			return nil
		},
	}
	if err := activation.Close(context.Background()); err == nil {
		t.Fatal("first close succeeded despite artifact failure")
	}
	if err := activation.Close(context.Background()); err != nil {
		t.Fatalf("retry close: %v", err)
	}
	if workerStops != 1 || artifactCloses != 2 {
		t.Fatalf("cleanup attempts: worker=%d artifacts=%d, want 1 and 2", workerStops, artifactCloses)
	}
}
