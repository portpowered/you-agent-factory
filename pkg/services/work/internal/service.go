// Package internal implements the Work composed root contract.
package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
)

type applicationService struct {
	runtimes              work.RuntimeResolver
	readSubmittedFile     work.SubmittedFileReader
	inspectSubmittedFile  work.SubmittedFilePathInspector
	contentStaging        work.ContentStagingService
	contentMaterializer   work.ContentMaterializer
	stateAccess           stateaccess.Service
	preparation           work.RequestPreparationService
	invocationPreparation work.InvocationInputPreparation
}

// Compile-time proof that production applicationService seals the published Work
// root without exporting owner-internal types through method signatures.
var (
	_ work.Service               = (*applicationService)(nil)
	_ work.FileSubmissionService = (*applicationService)(nil)
)

// NewService constructs the Work root contract for composition. Content staging
// and materialization are completed capabilities selected by composition.
func NewService(
	runtimes work.RuntimeResolver,
	readSubmittedFile work.SubmittedFileReader,
	inspectSubmittedFile work.SubmittedFilePathInspector,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
	stateAccess stateaccess.Service,
	preparation work.RequestPreparationService,
	invocationPreparation work.InvocationInputPreparation,
) work.FileSubmissionService {
	return &applicationService{
		runtimes:              runtimes,
		readSubmittedFile:     readSubmittedFile,
		inspectSubmittedFile:  inspectSubmittedFile,
		contentStaging:        contentStaging,
		contentMaterializer:   contentMaterializer,
		stateAccess:           stateAccess,
		preparation:           preparation,
		invocationPreparation: invocationPreparation,
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
	return submitFile(ctx, path, runtime, s.readSubmittedFile)
}

func (s *applicationService) runtime(sessionID string) (work.Runtime, error) {
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
	return s.stateAccess.SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (s *applicationService) PrepareWorkRequest(
	ctx context.Context,
	input work.WorkRequestPreparation,
) (work.WorkRequest, error) {
	return s.preparation.PrepareWorkRequest(ctx, input)
}

func (s *applicationService) MoveWorkForSession(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	return s.stateAccess.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (s *applicationService) StageContent(
	ctx context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	return s.contentStaging.StageContent(ctx, request)
}

func (s *applicationService) PrepareContent(
	ctx context.Context,
	items []work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	return s.contentStaging.PrepareContent(ctx, items)
}

func (s *applicationService) ResolveContent(
	ctx context.Context,
	ref string,
) (work.ResolvedStagedContent, error) {
	return s.contentStaging.ResolveContent(ctx, ref)
}

func (s *applicationService) CleanupContent(ctx context.Context, ref string) error {
	return s.contentStaging.CleanupContent(ctx, ref)
}

func (s *applicationService) MaterializeContentURL(
	ctx context.Context,
	rawURL string,
) (string, work.ContentCleanup, error) {
	return s.contentMaterializer.MaterializeContentURL(ctx, rawURL)
}

func (s *applicationService) MaterializeWorkerOutput(
	ctx context.Context,
	request work.MaterializeWorkerOutputRequest,
) (work.MaterializeWorkerOutputResult, error) {
	return materializeWorkerOutput(ctx, request)
}

func (s *applicationService) PrepareInvocationInput(
	ctx context.Context,
	request work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
	prepared, err := s.invocationPreparation.PrepareInvocationInput(ctx, request)
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

// SubmitTarget admits one canonical Work Request for path-backed submission flows.
type SubmitTarget interface {
	SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error)
}

func SubmitFile(ctx context.Context, path string, target SubmitTarget, readFile work.SubmittedFileReader) error {
	if readFile == nil {
		return fmt.Errorf("submitted Work Request file reader is required")
	}
	_, err := submitFile(ctx, path, target, readFile)
	return err
}

func submitFile(ctx context.Context, path string, target SubmitTarget, readFile work.SubmittedFileReader) (work.WorkRequestSubmitResult, error) {
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
