// Package service owns session-scoped Work application operations.
package service

import (
	"context"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service coordinates Work operations against the runtime registered for a
// live Factory Session.
type Service struct {
	sessions factorysessions.RuntimeResolver
}

type applicationService struct {
	runtimes          work.RuntimeResolver
	readSubmittedFile work.SubmittedFileReader
}

// New constructs the canonical Work application service.
func New(sessions factorysessions.RuntimeResolver) *Service {
	return &Service{sessions: sessions}
}

// NewService constructs the Work root contract for composition.
func NewService(runtimes work.RuntimeResolver, readSubmittedFile work.SubmittedFileReader) work.FileSubmissionService {
	return &applicationService{runtimes: runtimes, readSubmittedFile: readSubmittedFile}
}

func (s *applicationService) SubmitFileForSession(
	ctx context.Context,
	sessionID string,
	path string,
) (work.WorkRequestSubmitResult, error) {
	runtime, err := s.runtime(sessionID)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	if s.readSubmittedFile == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("submitted Work Request file reader is required")
	}
	return submitFile(ctx, path, runtime, s.readSubmittedFile)
}

func (s *applicationService) runtime(sessionID string) (work.Runtime, error) {
	if s == nil || s.runtimes == nil {
		return nil, fmt.Errorf("Factory Session runtime service is required")
	}
	runtime, err := s.runtimes.ResolveWorkRuntime(sessionID)
	if err != nil {
		return nil, err
	}
	if runtime == nil {
		return nil, fmt.Errorf("Factory Session runtime is unavailable: %s", sessionID)
	}
	return runtime, nil
}

func (s *applicationService) SubmitWorkRequestForSession(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	runtime, err := s.runtime(sessionID)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	return runtime.SubmitWorkRequest(ctx, request)
}

func (s *applicationService) MoveWorkForSession(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	runtime, err := s.runtime(sessionID)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	return runtime.MoveWork(
		ctx,
		workID,
		stateName,
		work.WorkStateChangeSourceAPI,
		requestID,
	)
}

// SubmitFile reads and submits one canonical Work Request file. It is used for
// startup Work, where the target runtime may not yet be registered as a live
// Factory Session.
type submitTarget interface {
	SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error)
}

func SubmitFile(ctx context.Context, path string, target submitTarget, readFile work.SubmittedFileReader) error {
	_, err := submitFile(ctx, path, target, readFile)
	return err
}

func submitFile(ctx context.Context, path string, target submitTarget, readFile work.SubmittedFileReader) (work.WorkRequestSubmitResult, error) {
	if readFile == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("submitted Work Request file reader is required")
	}
	data, err := readFile(path)
	if err != nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("read work file %s: %w", path, err)
	}
	request, err := work.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("parse work file %s: %w", path, err)
	}
	if target == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("factory runtime is not available")
	}
	result, err := target.SubmitWorkRequest(ctx, request)
	if err != nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("submit initial work: %w", err)
	}
	return result, nil
}

func (s *Service) runtime(sessionID string) (*factorysessions.LiveRuntime, error) {
	if s == nil || s.sessions == nil {
		return nil, fmt.Errorf("Factory Session runtime service is required")
	}
	session := s.sessions.Resolve(sessionID)
	if session == nil || session.Runtime == nil || session.Runtime.Factory == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	return session.Runtime, nil
}

// SubmitWorkRequestForSession admits a canonical Work Request to one live session.
func (s *Service) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	runtime, err := s.runtime(sessionID)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	return runtime.Factory.SubmitWorkRequest(ctx, request)
}

// MoveWorkForSession applies an API-originated operator move to one live session.
func (s *Service) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	runtime, err := s.runtime(sessionID)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	return runtime.Factory.MoveWork(ctx, workID, stateName, work.WorkStateChangeSourceAPI, requestID)
}

// SubscribeFactoryEventsForSession returns replay followed by live events for
// the selected Factory Session runtime.
func (s *Service) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	runtime, err := s.runtime(sessionID)
	if err != nil {
		return nil, err
	}
	stream, err := runtime.Factory.SubscribeFactoryEvents(ctx, reconnect, interfaces.FactoryEventReconnectScope{SessionID: sessionID})
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	if stream != nil {
		stream.BackendScopeID = strings.TrimSpace(runtime.BackendScopeID)
	}
	return stream, nil
}

// GetEngineStateSnapshotForSession returns the aggregate state snapshot for one
// live Factory Session runtime.
func (s *Service) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*factory.StateSnapshot, error) {
	runtime, err := s.runtime(sessionID)
	if err != nil {
		return nil, err
	}
	snapshot, err := runtime.Factory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snapshot, nil
}
