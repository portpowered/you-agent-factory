package service

import (
	"context"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Resume starts one next Workers attempt for the exact paused Worker Session.
// The request carries the registry-owned reference unchanged so the Workers
// provider runner must route only through Providers.Continue.
func (r *registry) Resume(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	reservation, err := r.beginControlHistory(ctx, req.ID, workersessions.ControlActionResume, req.RequestID)
	if err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	return r.resumeReserved(ctx, req, reservation)
}

func (r *registry) resumeReserved(
	ctx context.Context,
	req workersessions.ControlRequest,
	reservation *controlHistoryReservation,
) (workersessions.ControlResult, error) {
	session, supervision, err := r.controlTarget(req.ID)
	if err != nil {
		result := workersessions.ControlResult{Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeFailed}
		r.finishControlHistory(reservation, result.Outcome, "", workersessions.StateReserved)
		return result, err
	}
	if result, resumeErr, handled := r.resumeBeforeAdmission(ctx, req, session, supervision); handled {
		return r.finishResumeHistory(reservation, result, resumeErr)
	}
	if err := validateResumeAssociationForSupervision(session, supervision); err != nil {
		result, rejectedErr := r.rejectedResume(session, supervision, err)
		return r.finishResumeHistory(reservation, result, rejectedErr)
	}

	continuation, previousDispatchID, prepared := r.prepareContinuation(req.ID, supervision, session.ProviderSessionAssociation.Reference)
	if !prepared {
		current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		result := r.controlNoop(req.ID, workersessions.ControlActionResume, current, supervision)
		return r.finishResumeHistory(reservation, result, nil)
	}
	if err := r.publishAttemptLineageRecord(
		context.WithoutCancel(ctx),
		req.ID,
		continuation,
		workers.AttemptReasonResume,
		previousDispatchID,
		supervision.attemptCount(),
	); err != nil {
		return r.resumePublicationFailure(req, reservation, supervision, continuation, previousDispatchID, err)
	}
	if err := r.publishResumeDispatch(ctx, req, supervision, continuation); err != nil {
		return r.resumePublicationFailure(req, reservation, supervision, continuation, previousDispatchID, err)
	}
	return r.resumeAdmissionResult(req, reservation, supervision, continuation, previousDispatchID)
}

func (r *registry) resumeBeforeAdmission(
	ctx context.Context,
	req workersessions.ControlRequest,
	session workersessions.Session,
	supervision *supervision,
) (workersessions.ControlResult, error, bool) {
	if session.Terminal() || supervision != nil && supervision.resumeInFlight() {
		return r.controlNoop(req.ID, workersessions.ControlActionResume, session, supervision), nil, true
	}
	if session.State == workersessions.StatePaused && session.ProviderSessionAssociation == nil {
		result, err := r.rejectedResume(session, supervision, workersessions.ErrProviderSessionAssociationMissing)
		return result, err, true
	}
	if session.State != workersessions.StatePaused || supervision == nil {
		result, err := r.unsupportedControl(ctx, req, workersessions.ControlActionResume)
		return result, err, true
	}
	return workersessions.ControlResult{}, nil, false
}

func (r *registry) publishResumeDispatch(
	ctx context.Context,
	req workersessions.ControlRequest,
	supervision *supervision,
	continuation workers.WorkstationDispatchRequest,
) error {
	return r.boundary.PublishWithAdmission(
		context.WithoutCancel(ctx),
		continuation,
		func() {
			r.acceptSupervision(req.ID, supervision)
		},
		func(_ context.Context, _ workers.WorkstationDispatchRequest, result workers.WorkstationDispatchResult, dispatchErr error) {
			r.completeSupervision(req.ID, supervision, result, dispatchErr)
		},
	)
}

func (r *registry) resumePublicationFailure(
	req workersessions.ControlRequest,
	reservation *controlHistoryReservation,
	supervision *supervision,
	continuation workers.WorkstationDispatchRequest,
	previousDispatchID string,
	publicationErr error,
) (workersessions.ControlResult, error) {
	r.revertContinuation(req.ID, supervision, previousDispatchID)
	current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
	result := workersessions.ControlResult{
		Session:    current,
		Action:     workersessions.ControlActionResume,
		Outcome:    workersessions.ControlOutcomeFailed,
		DispatchID: continuation.Execution.Dispatch.DispatchID,
	}
	r.finishControlHistory(reservation, result.Outcome, result.DispatchID, result.Session.State)
	r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", result.DispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
	return result, publicationErr
}

func (r *registry) resumeAdmissionResult(
	req workersessions.ControlRequest,
	reservation *controlHistoryReservation,
	supervision *supervision,
	continuation workers.WorkstationDispatchRequest,
	previousDispatchID string,
) (workersessions.ControlResult, error) {
	r.finishContinuationPublication(supervision)
	current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
	if !supervision.continuationWasAdmitted() {
		// A Workers boundary may report a terminal callback before its admission
		// callback. Preserve that terminal history; if neither callback arrived,
		// restore the exact paused reservation instead of leaving a phantom
		// STARTING continuation behind.
		if !current.Terminal() && current.State == workersessions.StateStarting {
			r.revertContinuation(req.ID, supervision, previousDispatchID)
			current, _ = r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		}
		result := workersessions.ControlResult{
			Session:    current,
			Action:     workersessions.ControlActionResume,
			Outcome:    workersessions.ControlOutcomeFailed,
			DispatchID: continuation.Execution.Dispatch.DispatchID,
		}
		r.finishControlHistory(reservation, result.Outcome, result.DispatchID, result.Session.State)
		r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", continuation.Execution.Dispatch.DispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
		return result, workersessions.ErrStartAdmissionFailed
	}
	result := workersessions.ControlResult{
		Session:    current,
		Action:     workersessions.ControlActionResume,
		Outcome:    workersessions.ControlOutcomeApplied,
		DispatchID: continuation.Execution.Dispatch.DispatchID,
	}
	r.finishControlHistory(reservation, result.Outcome, result.DispatchID, result.Session.State)
	r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", result.DispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
	return result, nil
}

func (r *registry) finishResumeHistory(
	reservation *controlHistoryReservation,
	result workersessions.ControlResult,
	resumeErr error,
) (workersessions.ControlResult, error) {
	r.finishControlHistory(reservation, result.Outcome, result.DispatchID, result.Session.State)
	return result, resumeErr
}
