package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// rootFake is a focused Work root fake for adapter-edge tests. It avoids
// constructing state-access, content-staging, or materialization graphs.
type rootFake struct {
	work.Service

	listWork func(context.Context, string, work.ListOptions) (work.ListResult, error)
	getWork  func(context.Context, string, string) (work.ReadModel, error)

	stageContent func(context.Context, work.StageContentRequest) (work.StageContentResult, error)
	prepareContent func(context.Context, []work.StagedSubmissionItem) ([]work.WorkContentPart, error)
	submitWorkRequestForSession func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	moveWorkAndRead func(context.Context, string, string, string, string) (work.ReadModel, error)
}

func (fake *rootFake) ListWork(
	ctx context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	if fake.listWork != nil {
		return fake.listWork(ctx, sessionID, options)
	}
	return work.ListResult{}, work.ErrWorkNotFound
}

func (fake *rootFake) GetWork(
	ctx context.Context,
	sessionID string,
	workID string,
) (work.ReadModel, error) {
	if fake.getWork != nil {
		return fake.getWork(ctx, sessionID, workID)
	}
	return work.ReadModel{}, work.ErrWorkNotFound
}

func (fake *rootFake) StageContent(
	ctx context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	if fake.stageContent != nil {
		return fake.stageContent(ctx, request)
	}
	return work.StageContentResult{}, &work.ContentStagingError{Message: "staging unavailable"}
}

func (fake *rootFake) PrepareContent(
	ctx context.Context,
	items []work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	if fake.prepareContent != nil {
		return fake.prepareContent(ctx, items)
	}
	return nil, &work.ContentStagingError{Message: "prepare content unavailable"}
}

func (fake *rootFake) SubmitWorkRequestForSession(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	if fake.submitWorkRequestForSession != nil {
		return fake.submitWorkRequestForSession(ctx, sessionID, request)
	}
	return work.WorkRequestSubmitResult{}, work.ErrInvalidWorkRequest
}

func (fake *rootFake) MoveWorkAndRead(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.ReadModel, error) {
	if fake.moveWorkAndRead != nil {
		return fake.moveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
	}
	return work.ReadModel{}, work.ErrWorkNotFound
}
