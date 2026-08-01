// Package http owns HTTP adaptation for Work operations.
//
// The top-level HTTP transport registers generated routes and composes this
// adapter when PSS fan-in arrives. Request decoding, generated contract mapping,
// Work root invocation, error mapping, and cancel/timeout policy for Work-owned
// HTTP operations remain here with the owning service.
package http

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// Adapter maps Work service values at the outward HTTP boundary.
type Adapter struct {
	root            work.Service
	defaultWorkType DefaultWorkTypeResolver
}

// DefaultWorkTypeResolver supplies the session-scoped default Work type used
// by the public submit and upsert routes. The resolver is injected by the
// session composition edge; Work HTTP remains responsible only for invoking
// Work preparation and projecting its result.
type DefaultWorkTypeResolver func(context.Context, string) (string, error)

// NewAdapterFromRoles constructs the Work HTTP adapter for transitional
// role-based composition. A complete Work root is preferred; the focused
// admission, submission, and read roles are retained for callers that have
// not yet converged on that root.
func NewAdapterFromRoles(
	primary work.Service,
	admission work.Service,
	submission apisurface.WorkAPI,
	read apisurface.WorkReadAPI,
	defaultWorkType DefaultWorkTypeResolver,
) *Adapter {
	return NewAdapterWithDefaultWorkType(
		newRoleRoot(primary, admission, submission, read),
		defaultWorkType,
	)
}

type roleRoot struct {
	work.Service
	primary    work.Service
	submission apisurface.WorkAPI
	read       apisurface.WorkReadAPI
}

func newRoleRoot(
	primary work.Service,
	admission work.Service,
	submission apisurface.WorkAPI,
	read apisurface.WorkReadAPI,
) work.Service {
	fallback := primary
	if fallback == nil {
		fallback = admission
	}
	return &roleRoot{
		Service:    fallback,
		primary:    primary,
		submission: submission,
		read:       read,
	}
}

func (root *roleRoot) SubmitWorkRequestForSession(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	if root.primary != nil {
		return root.primary.SubmitWorkRequestForSession(ctx, sessionID, request)
	}
	if root.submission != nil {
		return root.submission.SubmitWorkRequestForSession(ctx, sessionID, request)
	}
	if root.Service != nil {
		return root.Service.SubmitWorkRequestForSession(ctx, sessionID, request)
	}
	return work.WorkRequestSubmitResult{}, errors.New("Work service is unavailable")
}

func (root *roleRoot) PrepareWorkRequest(
	ctx context.Context,
	input work.WorkRequestPreparation,
) (work.WorkRequest, error) {
	if root.primary != nil {
		return root.primary.PrepareWorkRequest(ctx, input)
	}
	if root.Service != nil {
		return root.Service.PrepareWorkRequest(ctx, input)
	}
	return work.WorkRequest{}, errors.New("Work service is unavailable")
}

func (root *roleRoot) MoveWorkForSession(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	if root.primary != nil {
		return root.primary.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
	}
	if root.submission != nil {
		return root.submission.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
	}
	if root.Service != nil {
		return root.Service.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
	}
	return work.OperatorMoveResult{}, errors.New("Work service is unavailable")
}

func (root *roleRoot) ListWork(
	ctx context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	if root.primary != nil {
		return root.primary.ListWork(ctx, sessionID, options)
	}
	if root.read != nil {
		return root.read.ListWork(ctx, sessionID, options)
	}
	if root.Service != nil {
		return root.Service.ListWork(ctx, sessionID, options)
	}
	return work.ListResult{}, errors.New("Work service is unavailable")
}

func (root *roleRoot) GetWork(
	ctx context.Context,
	sessionID string,
	workID string,
) (work.ReadModel, error) {
	if root.primary != nil {
		return root.primary.GetWork(ctx, sessionID, workID)
	}
	if root.read != nil {
		return root.read.GetWork(ctx, sessionID, workID)
	}
	if root.Service != nil {
		return root.Service.GetWork(ctx, sessionID, workID)
	}
	return work.ReadModel{}, errors.New("Work service is unavailable")
}

func (root *roleRoot) MoveWorkAndRead(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.ReadModel, error) {
	if root.primary != nil {
		return root.primary.MoveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
	}
	if root.read != nil {
		return root.read.MoveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
	}
	if root.Service != nil {
		return root.Service.MoveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
	}
	return work.ReadModel{}, errors.New("Work service is unavailable")
}

func (root *roleRoot) StageContent(
	ctx context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	if root.primary != nil {
		return root.primary.StageContent(ctx, request)
	}
	if root.Service != nil {
		return root.Service.StageContent(ctx, request)
	}
	return work.StageContentResult{}, errors.New("Work service is unavailable")
}

func (root *roleRoot) PrepareContent(
	ctx context.Context,
	items []work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	if root.primary != nil {
		return root.primary.PrepareContent(ctx, items)
	}
	if root.Service != nil {
		return root.Service.PrepareContent(ctx, items)
	}
	return nil, errors.New("Work service is unavailable")
}

var _ work.Service = (*roleRoot)(nil)

// NewAdapter constructs the Work HTTP representation adapter.
func NewAdapter(root work.Service) *Adapter {
	return NewAdapterWithDefaultWorkType(root, nil)
}

// NewAdapterWithDefaultWorkType constructs the Work HTTP representation
// adapter with an optional session-scoped default Work type policy.
func NewAdapterWithDefaultWorkType(
	root work.Service,
	defaultWorkType DefaultWorkTypeResolver,
) *Adapter {
	if root == nil {
		return nil
	}
	return &Adapter{root: root, defaultWorkType: defaultWorkType}
}

// Root returns the accepted Work root consumed by adapter-owned operations.
func (a *Adapter) Root() work.Service {
	if a == nil {
		return nil
	}
	return a.root
}

// invokeListWork forwards list requests through the accepted Work root.
func (a *Adapter) invokeListWork(
	ctx context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	if a == nil || a.root == nil {
		return work.ListResult{}, errors.New("work service is required")
	}
	return a.root.ListWork(ctx, sessionID, options)
}

// invokeGetWork forwards get requests through the accepted Work root.
func (a *Adapter) invokeGetWork(
	ctx context.Context,
	sessionID string,
	workID string,
) (work.ReadModel, error) {
	if a == nil || a.root == nil {
		return work.ReadModel{}, errors.New("work service is required")
	}
	return a.root.GetWork(ctx, sessionID, workID)
}

// invokeStageContent forwards staging requests through the accepted Work root.
func (a *Adapter) invokeStageContent(
	ctx context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	if a == nil || a.root == nil {
		return work.StageContentResult{}, errors.New("work service is required")
	}
	return a.root.StageContent(ctx, request)
}

// invokeSubmitWorkRequestForSession forwards admission requests through the
// accepted Work root.
func (a *Adapter) invokeSubmitWorkRequestForSession(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	if a == nil || a.root == nil {
		return work.WorkRequestSubmitResult{}, errors.New("work service is required")
	}
	return a.root.SubmitWorkRequestForSession(ctx, sessionID, request)
}

// invokePrepareWorkRequest forwards admission preparation through the
// accepted Work root after resolving the optional session-scoped default Work
// type.
func (a *Adapter) invokePrepareWorkRequest(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
	canonicalJSON []byte,
) (work.WorkRequest, error) {
	if a == nil || a.root == nil {
		return work.WorkRequest{}, errors.New("work service is required")
	}
	defaultWorkTypeID := ""
	if a.defaultWorkType != nil {
		var err error
		defaultWorkTypeID, err = a.defaultWorkType(ctx, sessionID)
		if err != nil {
			return work.WorkRequest{}, err
		}
	}
	return a.root.PrepareWorkRequest(ctx, work.WorkRequestPreparation{
		Request:           request,
		CanonicalJSON:     canonicalJSON,
		DefaultWorkTypeID: defaultWorkTypeID,
	})
}

// invokeMoveWorkAndRead forwards move requests through the accepted Work root.
func (a *Adapter) invokeMoveWorkAndRead(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.ReadModel, error) {
	if a == nil || a.root == nil {
		return work.ReadModel{}, errors.New("work service is required")
	}
	return a.root.MoveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
}
