package wire

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccesswire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

// WorkSnapshotReader is the narrow, consumer-owned port Work needs to serve
// list/get reads when no live Factory Session is available: one detached,
// session-scoped Work snapshot. Work never names the owner that projects it,
// so composition is free to select any implementation.
type WorkSnapshotReader interface {
	ReadWorkSnapshot(context.Context, string) (work.ReadSnapshot, error)
}

// SnapshotStateAccessService projects snapshot-backed Work list/get reads onto
// the Work service root for peers that must depend on Service rather than
// transitional sibling or owner-internal implementation packages.
func SnapshotStateAccessService(snapshots WorkSnapshotReader) work.Service {
	if snapshots == nil {
		return nil
	}
	return snapshotStateAccessService{
		stateAccess: stateaccesswire.NewSnapshotRootService(snapshots),
	}
}

type snapshotStateAccessService struct {
	stateAccess snapshotStateAccessProjection
}

type snapshotStateAccessProjection interface {
	SubmitWorkRequestForSession(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	MoveWorkForSession(context.Context, string, string, string, string) (work.OperatorMoveResult, error)
	ListWork(context.Context, string, work.ListOptions) (work.ListResult, error)
	GetWork(context.Context, string, string) (work.ReadModel, error)
	MoveWorkAndRead(context.Context, string, string, string, string) (work.ReadModel, error)
}

func (s snapshotStateAccessService) SubmitWorkRequestForSession(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return s.stateAccess.SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (s snapshotStateAccessService) PrepareWorkRequest(
	context.Context,
	work.WorkRequestPreparation,
) (work.WorkRequest, error) {
	return work.WorkRequest{}, fmt.Errorf("Work snapshot state access service does not support admission prep")
}

func (s snapshotStateAccessService) MoveWorkForSession(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	return s.stateAccess.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (s snapshotStateAccessService) ListWork(
	ctx context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	return s.stateAccess.ListWork(ctx, sessionID, options)
}

func (s snapshotStateAccessService) GetWork(
	ctx context.Context,
	sessionID string,
	workID string,
) (work.ReadModel, error) {
	return s.stateAccess.GetWork(ctx, sessionID, workID)
}

func (s snapshotStateAccessService) MoveWorkAndRead(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.ReadModel, error) {
	return s.stateAccess.MoveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
}

func (snapshotStateAccessService) StageContent(
	context.Context,
	work.StageContentRequest,
) (work.StageContentResult, error) {
	return work.StageContentResult{}, fmt.Errorf("Work snapshot state access service does not support content staging")
}

func (snapshotStateAccessService) PrepareContent(
	context.Context,
	[]work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	return nil, fmt.Errorf("Work snapshot state access service does not support content staging")
}

func (snapshotStateAccessService) ResolveContent(
	context.Context,
	string,
) (work.ResolvedStagedContent, error) {
	return work.ResolvedStagedContent{}, fmt.Errorf("Work snapshot state access service does not support content staging")
}

func (snapshotStateAccessService) CleanupContent(context.Context, string) error {
	return fmt.Errorf("Work snapshot state access service does not support content staging")
}

func (snapshotStateAccessService) MaterializeContentURL(
	context.Context,
	string,
) (string, work.ContentCleanup, error) {
	return "", nil, fmt.Errorf("Work snapshot state access service does not support content materialization")
}

func (snapshotStateAccessService) MaterializeWorkerOutput(
	context.Context,
	work.MaterializeWorkerOutputRequest,
) (work.MaterializeWorkerOutputResult, error) {
	return work.MaterializeWorkerOutputResult{}, fmt.Errorf("Work snapshot state access service does not support worker-output materialization")
}

func (snapshotStateAccessService) PrepareInvocationInput(
	context.Context,
	work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
	return work.PreparedInvocationInput{}, fmt.Errorf("Work snapshot state access service does not support invocation policy")
}

func (snapshotStateAccessService) ResolvePrimaryResult(
	context.Context,
	work.PrimaryResultSelectionInput,
) (work.PrimaryResultSelection, error) {
	return work.PrimaryResultSelection{}, fmt.Errorf("Work snapshot state access service does not support invocation policy")
}
