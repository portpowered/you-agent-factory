package service

import (
	"context"
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Fixed identity Start uses to commit the one opening SESSION/STARTED record
// onto workersessions.Topic(id) before Workers invocation. The record is
// Worker-Sessions-owned lifecycle, not a source-native Workers observation, so
// its SourceID is the Worker Session's own stable identity and its
// SourceSequence/SourceEventID are fixed constants scoped by that SourceID:
// a genuine retry can never occur (transitionToStarting succeeds at most once
// per identity), so no richer sequencing is required here.
const (
	lifecycleSourceType   events.SourceType     = "worker_session_lifecycle"
	openingSourceSequence events.SourceSequence = 1
	openingSourceEventID  events.SourceEventID  = "started"
	workerDraftSchemaID   events.SchemaID       = "workers.draft.v1"
)

// publishOpeningRecord commits the one opening KindSession/PhaseStarted
// workers.Draft onto workersessions.Topic(id), detached from any
// caller-owned backing array, before Start ever calls
// workers.WorkstationExecutionService.DispatchWorkstation. A non-nil return
// means no record was committed: Start must not proceed to Workers handoff.
func (r *registry) publishOpeningRecord(ctx context.Context, id, attemptID string) error {
	// workers.SessionPayload{Status: string} has one string field, so
	// json.Marshal cannot fail here; the error is intentionally discarded
	// rather than defended against.
	draftPayload, _ := json.Marshal(workers.SessionPayload{Status: string(workersessions.StateStarting)})
	draft := workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseStarted,
		Payload:    draftPayload,
		DispatchID: attemptID,
	}
	identity := events.AppendIdentity{
		SourceType:     lifecycleSourceType,
		SourceID:       events.SourceID(id),
		SourceSequence: openingSourceSequence,
		SourceEventID:  openingSourceEventID,
	}
	_, err := r.appendDraft(ctx, workersessions.Topic(id), identity, workerDraftSchemaID, draft)
	return err
}
