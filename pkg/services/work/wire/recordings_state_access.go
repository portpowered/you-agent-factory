package wire

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccesswire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

// RecordingsStateAccessService projects Recordings-backed Work list/get reads onto
// the Work service root for peers that must depend on Service rather than
// transitional sibling or owner-internal implementation packages.
func RecordingsStateAccessService(root recordings.Service) work.Service {
	if root == nil {
		return nil
	}
	return recordingsStateAccessService{
		stateAccess: stateaccesswire.NewRecordingsRootService(root),
	}
}

type recordingsStateAccessService struct {
	stateAccess recordingsStateAccessProjection
}

type recordingsStateAccessProjection interface {
	SubmitWorkRequestForSession(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	MoveWorkForSession(context.Context, string, string, string, string) (work.OperatorMoveResult, error)
	ListWork(context.Context, string, work.ListOptions) (work.ListResult, error)
	GetWork(context.Context, string, string) (work.ReadModel, error)
	MoveWorkAndRead(context.Context, string, string, string, string) (work.ReadModel, error)
}

func (s recordingsStateAccessService) SubmitWorkRequestForSession(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return s.stateAccess.SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (s recordingsStateAccessService) PrepareWorkRequest(
	context.Context,
	work.WorkRequestPreparation,
) (work.WorkRequest, error) {
	return work.WorkRequest{}, fmt.Errorf("Work recordings state access service does not support admission prep")
}

func (s recordingsStateAccessService) MoveWorkForSession(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	return s.stateAccess.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (s recordingsStateAccessService) ListWork(
	ctx context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	return s.stateAccess.ListWork(ctx, sessionID, options)
}

func (s recordingsStateAccessService) GetWork(
	ctx context.Context,
	sessionID string,
	workID string,
) (work.ReadModel, error) {
	return s.stateAccess.GetWork(ctx, sessionID, workID)
}

func (s recordingsStateAccessService) MoveWorkAndRead(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.ReadModel, error) {
	return s.stateAccess.MoveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
}

func (recordingsStateAccessService) StageContent(
	context.Context,
	work.StageContentRequest,
) (work.StageContentResult, error) {
	return work.StageContentResult{}, fmt.Errorf("Work recordings state access service does not support content staging")
}

func (recordingsStateAccessService) PrepareContent(
	context.Context,
	[]work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	return nil, fmt.Errorf("Work recordings state access service does not support content staging")
}

func (recordingsStateAccessService) ResolveContent(
	context.Context,
	string,
) (work.ResolvedStagedContent, error) {
	return work.ResolvedStagedContent{}, fmt.Errorf("Work recordings state access service does not support content staging")
}

func (recordingsStateAccessService) CleanupContent(context.Context, string) error {
	return fmt.Errorf("Work recordings state access service does not support content staging")
}

func (recordingsStateAccessService) MaterializeContentURL(
	context.Context,
	string,
) (string, work.ContentCleanup, error) {
	return "", nil, fmt.Errorf("Work recordings state access service does not support content materialization")
}

func (recordingsStateAccessService) PrepareInvocationInput(
	context.Context,
	work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
	return work.PreparedInvocationInput{}, fmt.Errorf("Work recordings state access service does not support invocation policy")
}

func (recordingsStateAccessService) ResolvePrimaryResult(
	context.Context,
	work.PrimaryResultSelectionInput,
) (work.PrimaryResultSelection, error) {
	return work.PrimaryResultSelection{}, fmt.Errorf("Work recordings state access service does not support invocation policy")
}
