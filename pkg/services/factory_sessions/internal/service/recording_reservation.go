package service

import (
	"context"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func (r *RuntimeOpening) rollbackFailedRecording(ctx context.Context, sessionID string, reservation runtimeartifact.Reservation, activation *sessionActivation, closeArtifacts func() error) error {
	cleanup := &runtimeOpeningCleanup{}
	if activation != nil {
		cleanup.Add(func() error { return activation.Abort(context.WithoutCancel(ctx)) })
	} else {
		cleanup.Add(closeArtifacts)
	}
	cleanup.OwnRecordingTarget(recordingReservationRollback{owner: r.recordingsRuntime, reservation: reservation})
	if err := cleanup.Close(); err != nil {
		r.reservationCleanups.Store(sessionID, cleanup)
		return err
	}
	return nil
}

// recordingReservationRollback is registered last, after all writer/runtime
// cleanup succeeds. Failed releases stay in the existing retryable cleanup.
type recordingReservationRollback struct {
	owner       recordings.RuntimeScopeService
	reservation runtimeartifact.Reservation
}

func (rollback recordingReservationRollback) Close() error {
	owner, ok := rollback.owner.(recordings.LiveRecordingReservationOwner)
	if !ok {
		return fmt.Errorf("peer recording reservation rollback owner is unavailable")
	}
	return owner.ReleaseReservation(rollback.reservation)
}
