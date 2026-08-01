package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
)

var _ Root = (*Service)(nil)

// Activate implements the published Service contract by delegating to the
// private activation_lifecycle owner.
func (s *Service) Activate(ctx context.Context, req ActivateRequest) (ActivateResult, error) {
	if s == nil || s.activation == nil {
		return ActivateResult{}, errors.New("activate Factory visualization: service is required")
	}
	result, err := s.activation.Activate(ctx, activationlifecycle.ActivateRequest{
		Mode: activationlifecycle.ActivateMode(req.Mode),
	})
	if err != nil {
		return ActivateResult{}, mapActivationLifecycleError(err)
	}
	return ActivateResult{State: LifecycleState(result.State)}, nil
}

// Join implements the published Service contract by delegating to the private
// activation_lifecycle owner.
func (s *Service) Join(ctx context.Context, req JoinRequest) (JoinResult, error) {
	if s == nil || s.activation == nil {
		return JoinResult{}, &LifecycleError{
			Kind:    LifecycleErrorNotActivated,
			Message: "join Factory visualization: not activated",
		}
	}
	result, err := s.activation.Join(ctx, activationlifecycle.JoinRequest{})
	if err != nil {
		return JoinResult{}, mapActivationLifecycleError(err)
	}
	return JoinResult{State: LifecycleState(result.State)}, nil
}

// StopDrain implements the published Service contract by delegating to the
// private activation_lifecycle owner.
func (s *Service) StopDrain(ctx context.Context, req StopDrainRequest) (StopDrainResult, error) {
	if s == nil || s.activation == nil {
		return StopDrainResult{State: LifecycleStateStopped}, nil
	}
	result, err := s.activation.StopDrain(ctx, activationlifecycle.StopDrainRequest{})
	if err != nil {
		return StopDrainResult{}, err
	}
	return StopDrainResult{State: LifecycleState(result.State)}, nil
}

func mapActivationLifecycleError(err error) error {
	var lifeErr *activationlifecycle.LifecycleError
	if errors.As(err, &lifeErr) {
		return &LifecycleError{
			Kind:    LifecycleErrorKind(lifeErr.Kind),
			Message: lifeErr.Message,
			Cause:   lifeErr.Cause,
		}
	}
	return err
}

// Observe implements the published Service contract by delegating live
// projection to the private live_view_projection owner.
func (s *Service) Observe(ctx context.Context, req ObserveRequest) (ObserveResult, error) {
	if s == nil || s.projection == nil {
		return ObserveResult{}, &ProjectionError{
			Kind:    ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: service is required",
		}
	}
	result, err := s.projection.Observe(ctx, mapObserveRequest(req))
	if err != nil {
		return ObserveResult{}, mapProjectionError(err)
	}
	return ObserveResult{
		View: ProjectedView{
			TickCount:          result.View.TickCount,
			RetainedEventCount: result.View.RetainedEventCount,
			ObservedAt:         result.View.ObservedAt,
		},
	}, nil
}

func mapObserveRequest(req ObserveRequest) liveviewprojection.ObserveRequest {
	mapped := liveviewprojection.ObserveRequest{Mode: liveviewprojection.ObserveMode(req.Mode)}
	if req.Reconnect != nil {
		mapped.Reconnect = &liveviewprojection.ObserveReconnectCursor{
			AfterEventID:  req.Reconnect.AfterEventID,
			AfterSequence: req.Reconnect.AfterSequence,
		}
	}
	return mapped
}

func mapProjectionError(err error) error {
	var projErr *liveviewprojection.ProjectionError
	if errors.As(err, &projErr) {
		return &ProjectionError{
			Kind:    ProjectionErrorKind(projErr.Kind),
			Message: projErr.Message,
			Cause:   projErr.Cause,
		}
	}
	return err
}

type rootPresentationSession struct {
	mode         PresentationDeliveryMode
	output       Output
	writer       *bytes.Buffer
	mu           sync.Mutex
	progressSeen bool
	finalized    bool
	closed       bool
}

// OpenPresentation implements the published Service contract by opening a
// Visualization-owned best-effort or lossless output.
func (s *Service) OpenPresentation(
	ctx context.Context,
	req OpenPresentationRequest,
) (OpenPresentationResult, error) {
	if s == nil {
		return OpenPresentationResult{}, &PresentationError{
			Kind:    PresentationErrorInvalidInput,
			Message: "open Factory visualization presentation: service is required",
		}
	}
	if ctx == nil {
		return OpenPresentationResult{}, &PresentationError{
			Kind:    PresentationErrorInvalidInput,
			Message: "open Factory visualization presentation: context is required",
		}
	}
	if err := ctx.Err(); err != nil {
		return OpenPresentationResult{}, err
	}
	if req.Mode == "" {
		return OpenPresentationResult{}, &PresentationError{
			Kind:    PresentationErrorInvalidInput,
			Message: "open Factory visualization presentation: required request parameters are missing",
		}
	}

	writer := &bytes.Buffer{}
	var output Output
	switch req.Mode {
	case PresentationDeliveryBestEffort:
		output = s.presentationOwner.OpenBestEffortOutput(writer)
	case PresentationDeliveryLossless:
		output = s.presentationOwner.OpenLosslessOutput(writer)
	default:
		return OpenPresentationResult{}, &PresentationError{
			Kind:    PresentationErrorInvalidInput,
			Message: fmt.Sprintf("open Factory visualization presentation: delivery mode %q is not supported", req.Mode),
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.presentations == nil {
		s.presentations = map[PresentationSessionID]*rootPresentationSession{}
	}
	s.presentationSeq++
	id := PresentationSessionID(fmt.Sprintf("presentation-%d", s.presentationSeq))
	s.presentations[id] = &rootPresentationSession{
		mode:   req.Mode,
		output: output,
		writer: writer,
	}
	return OpenPresentationResult{SessionID: id, Mode: req.Mode}, nil
}

// PresentProgress implements the published Service contract by enqueueing
// Visualization-owned progress records and mapping queue failures.
func (s *Service) PresentProgress(
	ctx context.Context,
	req PresentProgressRequest,
) (PresentProgressResult, error) {
	if err := requirePresentationContext(ctx); err != nil {
		return PresentProgressResult{}, err
	}
	session, err := s.presentationSession(req.SessionID)
	if err != nil {
		return PresentProgressResult{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed || session.finalized {
		return PresentProgressResult{}, &PresentationError{
			Kind:    PresentationErrorEnqueueAfterClose,
			Message: "present Factory visualization progress: presentation output is closed",
		}
	}

	accepted := 0
	for _, record := range req.Records {
		if err := session.output.Enqueue(append([]byte(nil), record.Payload...)); err != nil {
			if isPresentationClosedErr(err) {
				return PresentProgressResult{AcceptedCount: accepted}, &PresentationError{
					Kind:    PresentationErrorEnqueueAfterClose,
					Message: "present Factory visualization progress: presentation output is closed",
					Cause:   err,
				}
			}
			if isPresentationBackpressureErr(err) {
				return PresentProgressResult{AcceptedCount: accepted}, &PresentationError{
					Kind:    PresentationErrorBackpressureRejected,
					Message: "present Factory visualization progress: best-effort backlog rejected record",
					Cause:   err,
				}
			}
			return PresentProgressResult{AcceptedCount: accepted}, err
		}
		session.progressSeen = true
		accepted++
	}
	return PresentProgressResult{AcceptedCount: accepted}, nil
}

// FinalizePresentation implements the published Service contract by draining
// accepted progress before one Visualization-owned terminal write.
func (s *Service) FinalizePresentation(
	ctx context.Context,
	req FinalizePresentationRequest,
) (FinalizePresentationResult, error) {
	if err := requirePresentationContext(ctx); err != nil {
		return FinalizePresentationResult{}, err
	}
	session, err := s.presentationSession(req.SessionID)
	if err != nil {
		return FinalizePresentationResult{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.finalized {
		return FinalizePresentationResult{
			Finalized:    false,
			ProgressSeen: session.progressSeen,
		}, nil
	}
	if req.Terminal == nil {
		session.finalized = true
		session.closed = true
		_ = session.output.CloseAndDrain()
		return FinalizePresentationResult{}, &PresentationError{
			Kind:    PresentationErrorFinalizeWithoutWriter,
			Message: "finalize Factory visualization presentation: terminal writer is required",
		}
	}

	if err := session.output.CloseAndDrain(); err != nil {
		session.finalized = true
		session.closed = true
		return FinalizePresentationResult{}, err
	}
	if _, err := session.writer.Write(appendPresentationLine(req.Terminal.Payload)); err != nil {
		session.finalized = true
		session.closed = true
		return FinalizePresentationResult{}, err
	}
	session.finalized = true
	session.closed = true
	return FinalizePresentationResult{
		Finalized:    true,
		ProgressSeen: session.progressSeen,
	}, nil
}

// ClosePresentation implements the published Service contract's close and
// drain operation without a terminal write.
func (s *Service) ClosePresentation(
	ctx context.Context,
	req ClosePresentationRequest,
) (ClosePresentationResult, error) {
	if err := requirePresentationContext(ctx); err != nil {
		return ClosePresentationResult{}, err
	}
	session, err := s.presentationSession(req.SessionID)
	if err != nil {
		return ClosePresentationResult{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.closed {
		if err := session.output.CloseAndDrain(); err != nil {
			session.closed = true
			session.finalized = true
			return ClosePresentationResult{}, err
		}
		session.closed = true
		session.finalized = true
	}
	return ClosePresentationResult{DroppedCount: session.output.Dropped()}, nil
}

func (s *Service) presentationSession(id PresentationSessionID) (*rootPresentationSession, error) {
	if s == nil {
		return nil, &PresentationError{
			Kind:    PresentationErrorInvalidInput,
			Message: "Factory visualization presentation: service is required",
		}
	}
	if id == "" {
		return nil, &PresentationError{
			Kind:    PresentationErrorInvalidInput,
			Message: "Factory visualization presentation: session id is required",
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.presentations[id]
	if !ok {
		return nil, &PresentationError{
			Kind:    PresentationErrorInvalidInput,
			Message: "Factory visualization presentation: session is unknown",
		}
	}
	return session, nil
}

func requirePresentationContext(ctx context.Context) error {
	if ctx == nil {
		return &PresentationError{
			Kind:    PresentationErrorInvalidInput,
			Message: "Factory visualization presentation: context is required",
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
