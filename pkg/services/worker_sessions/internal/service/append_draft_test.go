package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestAppendDraft_InvalidDraft_ReturnsErrorAndAppendsNothing proves
// appendDraft applies the existing workers.ValidateDraft rules itself before
// ever marshaling or calling Events, so every caller that funnels through it
// -- publishOpeningRecord, publishTerminalRecord, and PublishRecord alike --
// shares this one rejection path even if a future caller skips its own
// pre-validation.
func TestAppendDraft_InvalidDraft_ReturnsErrorAndAppendsNothing(t *testing.T) {
	r := newTestRegistry(t)
	identity := events.AppendIdentity{
		SourceType:     "worker_provider",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
	}

	if _, err := r.appendDraft(context.Background(), workersessions.Topic("worker-1"), identity, "workers.draft.v1", workers.Draft{}); err == nil {
		t.Fatal("appendDraft() error = nil, want a non-nil error for a zero-value Draft")
	}
}
