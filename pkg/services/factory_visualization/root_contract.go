package factory_visualization

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
)

// Root is the singular peer-facing Factory Visualization contract.
//
// Cross-service consumers depend on this named root for request-activated
// lifecycle, live projection, and presentation/drain slices. Collaborator
// ports and legacy presentation helpers are not additional Visualization
// authority interfaces for those published slices.
//
// Concrete Service retains Start/Stop/Wait for initializer lifecycle.Component
// compatibility; peers use the request-parameter methods below.
type Root interface {
	// Activate leaves the inert constructed state through explicit request
	// parameters and starts retained-then-live Factory event projection.
	Activate(context.Context, ActivateRequest) (ActivateResult, error)
	// Join waits for the live subscription to exit. Calling Join before
	// Activate returns a typed not-activated failure while the root remains
	// inert.
	Join(context.Context, JoinRequest) (JoinResult, error)
	// StopDrain cancels the live subscription and emits one final projected
	// view through the Visualization-owned drain path.
	StopDrain(context.Context, StopDrainRequest) (StopDrainResult, error)
	// Observe returns one detached retained-then-live Factory view projection
	// through Visualization-owned plain contracts.
	Observe(context.Context, ObserveRequest) (ObserveResult, error)
	// OpenPresentation opens one Visualization-owned presentation output using
	// best-effort or lossless drain policy. Transports do not supply writers,
	// queue capacity, or backpressure policy through this seam.
	OpenPresentation(context.Context, OpenPresentationRequest) (OpenPresentationResult, error)
	// PresentProgress enqueues ordered progress records onto an opened
	// presentation session.
	PresentProgress(context.Context, PresentProgressRequest) (PresentProgressResult, error)
	// FinalizePresentation drains accepted progress then commits one terminal
	// write owned by Visualization final-write ordering.
	FinalizePresentation(context.Context, FinalizePresentationRequest) (FinalizePresentationResult, error)
	// ClosePresentation closes and drains a presentation session without a
	// terminal write.
	ClosePresentation(context.Context, ClosePresentationRequest) (ClosePresentationResult, error)
}

// ActivateMode selects how visualization leaves the inert constructed state.
type ActivateMode string

const (
	// ActivateModeRetainedThenLive activates retained history projection then
	// live event observation — the existing Visualization Start vocabulary.
	ActivateModeRetainedThenLive ActivateMode = "RETAINED_THEN_LIVE"
)

// LifecycleState is the published request-activated lifecycle vocabulary.
type LifecycleState string

const (
	LifecycleStateInert   LifecycleState = "INERT"
	LifecycleStateStarted LifecycleState = "STARTED"
	LifecycleStateStopped LifecycleState = "STOPPED"
)

// LifecycleErrorKind distinguishes typed Visualization lifecycle outcomes.
type LifecycleErrorKind string

const (
	LifecycleErrorMissingParameters LifecycleErrorKind = "MISSING_PARAMETERS"
	LifecycleErrorAlreadyActivated  LifecycleErrorKind = "ALREADY_ACTIVATED"
	LifecycleErrorNotActivated      LifecycleErrorKind = "NOT_ACTIVATED"
)

// ActivateRequest carries the explicit parameters required to leave the inert
// constructed state. A zero-value request is rejected.
type ActivateRequest struct {
	Mode ActivateMode
}

// ActivateResult is the published outcome of a successful Activate call.
type ActivateResult struct {
	State LifecycleState
}

// JoinRequest carries wait/join parameters for the live subscription.
type JoinRequest struct{}

// JoinResult is the published outcome of a successful Join call.
type JoinResult struct {
	State LifecycleState
}

// StopDrainRequest carries stop-and-drain-final-view parameters.
type StopDrainRequest struct{}

// StopDrainResult is the published outcome of a successful StopDrain call.
type StopDrainResult struct {
	State LifecycleState
}

// LifecycleError is a typed Visualization lifecycle failure peers can branch on.
type LifecycleError struct {
	Kind    LifecycleErrorKind
	Message string
	Cause   error
}

func (e *LifecycleError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *LifecycleError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ObserveMode selects how live projection is obtained through the root.
type ObserveMode string

const (
	// ObserveModeRetainedThenLive projects retained history then live
	// observation facts into one detached Visualization view.
	ObserveModeRetainedThenLive ObserveMode = "RETAINED_THEN_LIVE"
)

// ObserveReconnectCursor is the Visualization-owned reconnect observe input.
// Peers supply AfterEventID and/or AfterSequence; both empty is invalid.
type ObserveReconnectCursor struct {
	AfterEventID  string
	AfterSequence *int
}

// ObserveRequest carries explicit live-projection parameters. A zero-value
// request is rejected as invalid input.
type ObserveRequest struct {
	Mode      ObserveMode
	Reconnect *ObserveReconnectCursor
}

// ProjectedView is a Visualization-owned detached live view. Engine-state and
// observation facts are plain values so peers do not import Recordings ledger
// storage or Runtime Petri/JavaScript internals.
type ProjectedView struct {
	TickCount          int
	RetainedEventCount int
	ObservedAt         time.Time
}

// ObserveResult is the published outcome of a successful Observe call.
type ObserveResult struct {
	View ProjectedView
}

// ProjectionErrorKind distinguishes typed Visualization live-projection outcomes.
type ProjectionErrorKind string

const (
	ProjectionErrorInvalidInput         ProjectionErrorKind = "INVALID_INPUT"
	ProjectionErrorSnapshotUnavailable  ProjectionErrorKind = "SNAPSHOT_UNAVAILABLE"
	ProjectionErrorReconstructionFailed ProjectionErrorKind = "RECONSTRUCTION_FAILED"
)

// ProjectionError is a typed Visualization live-projection failure peers can branch on.
type ProjectionError struct {
	Kind    ProjectionErrorKind
	Message string
	Cause   error
}

func (e *ProjectionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *ProjectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Compile-time proof that the existing lifecycle Service remains reachable
// through the singular Root seam.
var _ Root = (*Service)(nil)

// Activate implements Root by delegating to the activation_lifecycle owner.
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

// Join implements Root by delegating to the activation_lifecycle owner.
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

// StopDrain implements Root by delegating to the activation_lifecycle owner.
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

// Observe implements Root by delegating live projection to the private
// live_view_projection owner and mapping detached view facts to the published
// root contract.
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

// PresentationDeliveryMode selects Visualization-owned drain/backpressure policy.
type PresentationDeliveryMode string

const (
	// PresentationDeliveryBestEffort may reject progress under backlog pressure.
	PresentationDeliveryBestEffort PresentationDeliveryMode = "BEST_EFFORT"
	// PresentationDeliveryLossless retains every accepted progress record until
	// close/finalize drain completes.
	PresentationDeliveryLossless PresentationDeliveryMode = "LOSSLESS"
)

// PresentationSessionID identifies one opened presentation/drain session.
type PresentationSessionID string

// PresentationErrorKind distinguishes typed Visualization presentation/drain outcomes.
type PresentationErrorKind string

const (
	PresentationErrorInvalidInput          PresentationErrorKind = "INVALID_INPUT"
	PresentationErrorEnqueueAfterClose     PresentationErrorKind = "ENQUEUE_AFTER_CLOSE"
	PresentationErrorFinalizeWithoutWriter PresentationErrorKind = "FINALIZE_WITHOUT_WRITER"
	PresentationErrorBackpressureRejected  PresentationErrorKind = "BACKPRESSURE_REJECTED"
)

// PresentationError is a typed Visualization presentation/drain failure peers can branch on.
type PresentationError struct {
	Kind    PresentationErrorKind
	Message string
	Cause   error
}

func (e *PresentationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *PresentationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// OpenPresentationRequest carries explicit presentation open parameters.
type OpenPresentationRequest struct {
	Mode PresentationDeliveryMode
}

// OpenPresentationResult is the published outcome of opening a presentation session.
type OpenPresentationResult struct {
	SessionID PresentationSessionID
	Mode      PresentationDeliveryMode
}

// ProgressRecord is one Visualization-owned progress payload.
type ProgressRecord struct {
	Payload []byte
}

// PresentProgressRequest enqueues ordered progress onto an opened session.
type PresentProgressRequest struct {
	SessionID PresentationSessionID
	Records   []ProgressRecord
}

// PresentProgressResult reports how many progress records were accepted.
type PresentProgressResult struct {
	AcceptedCount int
}

// TerminalWrite is the Visualization-owned terminal payload committed after drain.
type TerminalWrite struct {
	Payload []byte
}

// FinalizePresentationRequest finalizes one presentation session after drain.
// A nil Terminal is the typed finalize-without-writer failure.
type FinalizePresentationRequest struct {
	SessionID PresentationSessionID
	Terminal  *TerminalWrite
}

// FinalizePresentationResult is the published finalize outcome.
type FinalizePresentationResult struct {
	Finalized    bool
	ProgressSeen bool
}

// ClosePresentationRequest closes and drains without a terminal write.
type ClosePresentationRequest struct {
	SessionID PresentationSessionID
}

// ClosePresentationResult reports close-and-drain outcomes peers need.
type ClosePresentationResult struct {
	DroppedCount int
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

// OpenPresentation implements Root by opening a Visualization-owned best-effort
// or lossless output. Writer/codec transport types stay off the peer contract.
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
		output = newBestEffortOutput(writer)
	case PresentationDeliveryLossless:
		output = newLosslessOutput(writer)
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

// PresentProgress implements Root by enqueueing Visualization-owned progress
// records and mapping closed/backpressure outcomes to typed errors.
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

// FinalizePresentation implements Root by draining accepted progress then
// appending one Visualization-owned terminal payload.
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
	if _, err := session.writer.Write(appendLine(req.Terminal.Payload)); err != nil {
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

// ClosePresentation implements Root close-and-drain without a terminal write.
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

func isPresentationClosedErr(err error) bool {
	return errors.Is(err, errPresentationOutputClosed)
}

func isPresentationBackpressureErr(err error) bool {
	return errors.Is(err, errPresentationBacklogFull)
}
