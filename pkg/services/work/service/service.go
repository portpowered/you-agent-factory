// Package service owns session-scoped Work application operations.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions 	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
	stateaccesswire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

// Service coordinates Work operations against the runtime registered for a
// live Factory Session.
type Service struct {
	sessions    RuntimeResolver
	stateAccess stateaccess.Service
}

type RuntimeResolver interface {
	Resolve(string) *factorysessions.LiveRuntime
}

type applicationService struct {
	runtimes            work.RuntimeResolver
	readSubmittedFile   work.SubmittedFileReader
	contentStaging      work.ContentStagingService
	contentMaterializer work.ContentMaterializer
	stateAccess         stateaccess.Service
}

// New constructs the canonical Work application service.
func New(sessions RuntimeResolver) *Service {
	return &Service{
		sessions: sessions,
		stateAccess: stateaccesswire.NewService(
			stateaccesswire.NewRuntimeSessionResolver(liveSessionRuntimeResolver{sessions: sessions}),
			nil,
		),
	}
}

// NewService constructs the Work root contract for composition. Content staging
// and materialization may be nil when a caller only needs admission/state-access
// slices; content methods then return a deterministic configuration error.
func NewService(
	runtimes work.RuntimeResolver,
	readSubmittedFile work.SubmittedFileReader,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
) work.FileSubmissionService {
	return &applicationService{
		runtimes:            runtimes,
		readSubmittedFile:   readSubmittedFile,
		contentStaging:      contentStaging,
		contentMaterializer: contentMaterializer,
		stateAccess: stateaccesswire.NewService(
			stateaccesswire.NewRuntimeSessionResolver(runtimes),
			nil,
		),
	}
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
	if s == nil || s.stateAccess == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("Work state access is required")
	}
	return s.stateAccess.SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (s *applicationService) MoveWorkForSession(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	if s == nil || s.stateAccess == nil {
		return work.OperatorMoveResult{}, fmt.Errorf("Work state access is required")
	}
	return s.stateAccess.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (s *applicationService) StageContent(
	ctx context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	if s == nil || s.contentStaging == nil {
		return work.StageContentResult{}, fmt.Errorf("Work content staging is required")
	}
	return s.contentStaging.StageContent(ctx, request)
}

func (s *applicationService) PrepareContent(
	ctx context.Context,
	items []work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	if s == nil || s.contentStaging == nil {
		return nil, fmt.Errorf("Work content staging is required")
	}
	return s.contentStaging.PrepareContent(ctx, items)
}

func (s *applicationService) ResolveContent(
	ctx context.Context,
	ref string,
) (work.ResolvedStagedContent, error) {
	if s == nil || s.contentStaging == nil {
		return work.ResolvedStagedContent{}, fmt.Errorf("Work content staging is required")
	}
	return s.contentStaging.ResolveContent(ctx, ref)
}

func (s *applicationService) CleanupContent(ctx context.Context, ref string) error {
	if s == nil || s.contentStaging == nil {
		return fmt.Errorf("Work content staging is required")
	}
	return s.contentStaging.CleanupContent(ctx, ref)
}

func (s *applicationService) MaterializeContentURL(
	ctx context.Context,
	rawURL string,
) (string, work.ContentCleanup, error) {
	if s == nil || s.contentMaterializer == nil {
		return "", nil, fmt.Errorf("Work content materializer is required")
	}
	return s.contentMaterializer.MaterializeContentURL(ctx, rawURL)
}

func (s *applicationService) PrepareInvocationInput(
	ctx context.Context,
	request work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
	prepared, err := work.NewInvocationInputPreparation().PrepareInvocationInput(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return work.PreparedInvocationInput{}, err
		}
		return work.PreparedInvocationInput{}, fmt.Errorf("%w: %w", work.ErrInvalidInvocationInput, err)
	}
	return prepared, nil
}

func (s *applicationService) ResolvePrimaryResult(
	ctx context.Context,
	input work.PrimaryResultSelectionInput,
) (work.PrimaryResultSelection, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return work.PrimaryResultSelection{}, err
		}
	}
	return work.ResolvePrimaryResult(input)
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
	runtime := s.sessions.Resolve(sessionID)
	if runtime == nil || runtime.Factory == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	return runtime, nil
}

// SubmitWorkRequestForSession admits a canonical Work Request to one live session.
func (s *Service) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if s == nil || s.stateAccess == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("Work state access is required")
	}
	return s.stateAccess.SubmitWorkRequestForSession(ctx, sessionID, request)
}

// MoveWorkForSession applies an API-originated operator move to one live session.
func (s *Service) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	if s == nil || s.stateAccess == nil {
		return work.OperatorMoveResult{}, fmt.Errorf("Work state access is required")
	}
	return s.stateAccess.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

// SubscribeFactoryEventsForSession returns replay followed by live events for
// the selected Factory Session runtime.
func (s *Service) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	runtime, err := s.runtime(sessionID)
	if err != nil {
		return nil, err
	}
	legacyRuntime, ok := runtime.Factory.(factory.APIFactory)
	if !ok {
		return nil, fmt.Errorf("legacy Factory Runtime event subscription is required")
	}
	stream, err := legacyRuntime.SubscribeFactoryEvents(ctx, reconnect, interfaces.FactoryEventReconnectScope{SessionID: sessionID})
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	if stream != nil {
		stream.BackendScopeID = strings.TrimSpace(runtime.BackendScopeID)
	}
	return stream, nil
}
