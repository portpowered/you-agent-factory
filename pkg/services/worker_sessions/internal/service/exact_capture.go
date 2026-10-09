package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// A public direct start does not require the customer to choose a recording
// identity. Bind its opening and recipe to one deterministic store identity.
func (r *registry) bindDirectRecording(req *workersessions.StartRequest) {
	if r.logs == nil || r.recording == nil || req.Execution.Execution.RecordingID != "" {
		return
	}
	digest := sha256.Sum256([]byte(req.ID))
	req.Execution.Execution.RecordingID = fmt.Sprintf("direct-%x", digest)
}

// Unreconstructible requests remain invocable. Only an actual artifact-store
// failure rejects admission; unsafe settings never become a changed recipe.
func (r *registry) saveDirectRestartRecipe(ctx context.Context, req workersessions.InvokeSessionRequest) error {
	if _, metadata, ok := r.loadObservationState(req.ID); !ok || !metadata.direct {
		return nil
	}
	pub := r.publicationFor(req.ID)
	if pub == nil {
		return nil
	}
	pub.mu.Lock()
	target := pub.capture
	pub.mu.Unlock()
	if target.RecordingID == "" {
		return nil
	}
	target.ExpectedAttemptID = req.Execution.Execution.Dispatch.DispatchID
	if !directRestartRecipeSafe(req.Execution) {
		r.logger.Info("worker session restart recipe unavailable", "sessionID", publicWorkerID(req.ID), "attemptID", target.ExpectedAttemptID, "outcome", "unsafe_input")
		return nil
	}
	err := r.restart.SaveWorkerRestartRecipe(ctx, target, req.Execution)
	if errors.Is(err, recordings.ErrInvalidRecordingRedactionRequest) {
		r.logger.Info("worker session restart recipe unavailable", "sessionID", publicWorkerID(req.ID), "attemptID", target.ExpectedAttemptID, "outcome", "unsafe_input")
		return nil
	}
	return err
}

func directRestartRecipeSafe(execution workers.WorkstationDispatchRequest) bool {
	payload, err := json.Marshal(execution)
	return err == nil && interruptExecutionReplaySafe(execution.Execution) &&
		interruptRecipeSafe(payload, execution.Execution.ProcessEnvironment)
}

// Validate the immutable direct input before source cancellation, rather than
// discovering an unusable recipe in Continue after the source has stopped.
// Capture identity remains pinned by the existing interrupt fence.
func (r *registry) capturedInterruptPlan(ctx context.Context, plan interruptPlan, target recordings.WorkerControlTarget) (interruptPlan, error) {
	_, metadata, exists := r.loadObservationState(plan.sourceAddressOrID())
	if r.logs == nil || !exists || !metadata.direct {
		// Component fixtures and legacy non-direct interruption have no direct
		// recipe. Preserve those paths; they do not authorize captured restart.
		return plan, nil
	}
	target.ExpectedAttemptID = plan.dispatchID
	catalog, err := r.logs.reader.LookupWorkerSessionCapture(ctx, plan.request.SourceWorkerSessionID)
	selected := recordings.WorkerControlTarget{
		RecordingID: catalog.RecordingID, WorkerSessionID: catalog.WorkerSessionID,
		FactorySessionID: catalog.FactorySessionID, RecordingGenerationID: catalog.RecordingGenerationID,
		OwnerEpoch: catalog.OwnerEpoch, ExpectedAttemptID: plan.dispatchID,
	}
	if err != nil || selected != target {
		return plan, workersessions.ErrInterruptExecutionUnavailable
	}
	captured, err := r.restart.ReadWorkerRestartRecipe(ctx, target)
	start := workersessions.StartRequest{RequestID: plan.request.RequestID, ID: plan.request.SourceWorkerSessionID, Execution: captured}
	if err != nil || start.Validate() != nil || !directRestartRecipeSafe(captured) ||
		captured.Execution.Dispatch.DispatchID != plan.dispatchID || captured.Execution.FactorySessionID != target.FactorySessionID {
		return plan, workersessions.ErrInterruptExecutionUnavailable
	}
	// The environment is owned by the current host and never restored from
	// storage. Check the detached recipe against that environment for secrets.
	captured.Execution.ProcessEnvironment = append([]string(nil), plan.execution.Execution.ProcessEnvironment...)
	if !directRestartRecipeSafe(captured) {
		return plan, workersessions.ErrInterruptExecutionUnavailable
	}
	plan.execution = captured
	return plan, nil
}

// Read outside the registry lock; reservation later rechecks the immutable
// source attempt. A replay already reserved in this host needs no storage read.
func (r *registry) readContinuationRecipe(req workersessions.ContinueRequest, callers ...context.Context) (*workers.WorkstationDispatchRequest, error) {
	r.mu.RLock()
	address := r.workerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	source, exists := r.sessions[address]
	metadata := r.observations[address]
	_, replay := r.continueReplays[req.RequestID]
	read := !replay && exists && source.Terminal() && metadata != nil && metadata.direct && r.logs != nil
	factorySessionID, terminalPublished := r.continuationPublicationLocked(address)
	source = cloneSession(source)
	r.mu.RUnlock()
	if !read {
		return nil, nil
	}
	if err := validateContinuationSourceAssociation(source); err != nil {
		return nil, err
	}
	ctx := r.serverOwnedContext()
	waitCtx := ctx
	if len(callers) != 0 && callers[0] != nil {
		waitCtx = callers[0]
	}
	if err := waitContinuationPublication(waitCtx, terminalPublished); err != nil {
		return nil, err
	}
	catalog, err := r.logs.reader.LookupWorkerSessionCapture(ctx, source.ID)
	if err != nil || catalog.WorkerSessionID != source.ID || catalog.FactorySessionID != factorySessionID {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	target := recordings.WorkerControlTarget{
		RecordingID: catalog.RecordingID, WorkerSessionID: catalog.WorkerSessionID,
		FactorySessionID: catalog.FactorySessionID, RecordingGenerationID: catalog.RecordingGenerationID,
		OwnerEpoch: catalog.OwnerEpoch, ExpectedAttemptID: source.ProviderSessionAssociation.AttemptID,
	}
	return r.readCapturedContinuationRecipe(ctx, target, source)
}

// The caller holds r.mu while selecting the exact attempt's publication.
func (r *registry) continuationPublicationLocked(address string) (string, <-chan struct{}) {
	factorySessionID := ""
	var terminalPublished <-chan struct{}
	if supervision := r.supervisions[address]; supervision != nil {
		supervision.mu.Lock()
		factorySessionID = supervision.execution.Execution.FactorySessionID
		if supervision.accepted {
			terminalPublished = supervision.done
		}
		supervision.mu.Unlock()
	}
	return factorySessionID, terminalPublished
}

func waitContinuationPublication(ctx context.Context, terminalPublished <-chan struct{}) error {
	// Terminal state is visible before capture finalization. Join the exact
	// admitted attempt's publication, outside the registry lock, before reading
	// the durable recipe. A peer's completion cannot release this barrier.
	if terminalPublished != nil {
		select {
		case <-terminalPublished:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (r *registry) readCapturedContinuationRecipe(ctx context.Context, target recordings.WorkerControlTarget, source workersessions.Session) (*workers.WorkstationDispatchRequest, error) {
	captured, err := r.restart.ReadWorkerContinuationSource(ctx, target)
	if err != nil || !directRestartRecipeSafe(captured.Execution) ||
		captured.Execution.Execution.FactorySessionID != target.FactorySessionID {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	if captured.Reference != source.ProviderSessionAssociation.Reference || captured.Terminal.Status != string(source.State) {
		return nil, workersessions.ErrContinuationProviderSessionInvalid
	}
	return &captured.Execution, nil
}

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
