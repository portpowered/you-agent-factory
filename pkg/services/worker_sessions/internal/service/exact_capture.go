package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// The caller holds pub.mu across opening acknowledgement and this binding.
// Only the admitted capture supplies generation/epoch; later catalog entries
// must not upgrade a stale execution handle's authority.
func (r *registry) bindOpeningCapture(ctx context.Context, id string, payload workers.SessionPayload, pub *publication) error {
	if r.logs == nil {
		return nil
	}
	entry, err := r.logs.reader.LookupWorkerSessionCapture(ctx, publicWorkerID(id))
	if err != nil {
		return fmt.Errorf("%w: capture identity unavailable", recordings.ErrWorkerRecordingOpening)
	}
	if entry.WorkerSessionID != publicWorkerID(id) || entry.RecordingID != payload.RecordingID ||
		entry.FactorySessionID != payload.FactorySessionID || entry.RecordingGenerationID == "" || entry.OwnerEpoch == "" {
		return fmt.Errorf("%w: capture identity does not match opening", recordings.ErrWorkerRecordingOpening)
	}
	pub.capture = recordings.WorkerControlTarget{
		RecordingID: entry.RecordingID, WorkerSessionID: entry.WorkerSessionID,
		FactorySessionID: entry.FactorySessionID, RecordingGenerationID: entry.RecordingGenerationID,
		OwnerEpoch: entry.OwnerEpoch,
	}
	return nil
}

// Publication -> registry -> attempt is the admission lock order. Keep the
// immutable capture pinned through the ownership comparison/control claim,
// then release every lock before callbacks, publication, or joining.
func (r *registry) lockFrozenCapture(id string, target frozenControlTarget) (func(), error) {
	pub := r.publicationFor(id)
	if pub != target.publication {
		return nil, staleControlTargetError()
	}
	if pub == nil {
		return func() {}, nil
	}
	pub.mu.Lock()
	if pub.capture != target.capture {
		pub.mu.Unlock()
		return nil, staleControlTargetError()
	}
	return pub.mu.Unlock, nil
}
