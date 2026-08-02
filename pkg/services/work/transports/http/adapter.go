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

// NewAdapterFromRoles is a compatibility composition helper for narrow
// transport fixtures that provide Work's published representation roles
// separately. Production composition should inject the singular Work root
// with NewAdapter.
func NewAdapterFromRoles(
	primary work.Service,
	admission work.Service,
	submission apisurface.WorkAPI,
	read apisurface.WorkReadAPI,
) *Adapter {
	root := primary
	if root == nil {
		root = &roleRoot{Service: admission, submission: submission, read: read}
	} else if submission != nil || read != nil {
		root = &roleRoot{Service: primary, submission: submission, read: read}
	}
	return NewAdapter(root)
}

type roleRoot struct {
	work.Service
	submission apisurface.WorkAPI
	read       apisurface.WorkReadAPI
}

func (r *roleRoot) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if r.submission != nil {
		return r.submission.SubmitWorkRequestForSession(ctx, sessionID, request)
	}
	if r.Service != nil {
		return r.Service.SubmitWorkRequestForSession(ctx, sessionID, request)
	}
	return work.WorkRequestSubmitResult{}, errors.New("work service is unavailable")
}

func (r *roleRoot) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	if r.submission != nil {
		return r.submission.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
	}
	return r.Service.MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (r *roleRoot) PrepareWorkRequest(ctx context.Context, input work.WorkRequestPreparation) (work.WorkRequest, error) {
	return r.Service.PrepareWorkRequest(ctx, input)
}

func (r *roleRoot) ListWork(ctx context.Context, sessionID string, options work.ListOptions) (work.ListResult, error) {
	if r.read != nil {
		return r.read.ListWork(ctx, sessionID, options)
	}
	if r.Service != nil {
		return r.Service.ListWork(ctx, sessionID, options)
	}
	return work.ListResult{}, errors.New("work service is unavailable")
}

func (r *roleRoot) GetWork(ctx context.Context, sessionID, workID string) (work.ReadModel, error) {
	if r.read != nil {
		return r.read.GetWork(ctx, sessionID, workID)
	}
	if r.Service != nil {
		return r.Service.GetWork(ctx, sessionID, workID)
	}
	return work.ReadModel{}, errors.New("work service is unavailable")
}

func (r *roleRoot) MoveWorkAndRead(ctx context.Context, sessionID, workID, stateName, requestID string) (work.ReadModel, error) {
	if r.read != nil {
		return r.read.MoveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
	}
	if r.Service != nil {
		return r.Service.MoveWorkAndRead(ctx, sessionID, workID, stateName, requestID)
	}
	return work.ReadModel{}, errors.New("work service is unavailable")
}

var _ work.Service = (*roleRoot)(nil)

// Adapter maps Work service values at the outward HTTP boundary.
type Adapter struct {
	root            work.Service
	sessionScope    func(context.Context, string) error
	defaultWorkType func(context.Context, string) (string, error)
}

// NewAdapter constructs the Work HTTP representation adapter.
func NewAdapter(root work.Service) *Adapter {
	if root == nil {
		return nil
	}
	return &Adapter{root: root}
}

// NewAdapterWithSessionScope binds a session-existence policy for content
// endpoints whose Work root operation has no session argument.
func NewAdapterWithSessionScope(root work.Service, scope func(context.Context, string) error) *Adapter {
	adapter := NewAdapter(root)
	if adapter != nil {
		adapter.sessionScope = scope
	}
	return adapter
}

// WithSessionScope returns a copy bound to a session-existence policy.
func (a *Adapter) WithSessionScope(scope func(context.Context, string) error) *Adapter {
	if a == nil {
		return nil
	}
	bound := *a
	bound.sessionScope = scope
	return &bound
}

// WithDefaultWorkTypeResolver binds the application-edge policy used to fill
// omitted Work Request types from the session's current Factory.
func (a *Adapter) WithDefaultWorkTypeResolver(
	resolver func(context.Context, string) (string, error),
) *Adapter {
	if a == nil {
		return nil
	}
	bound := *a
	bound.defaultWorkType = resolver
	return &bound
}

// WithAdmissionService returns a copy using a supplied admission/content
// service while preserving the already-bound Work representation roles.
func (a *Adapter) WithAdmissionService(admission work.Service) *Adapter {
	if a == nil {
		return nil
	}
	bound := *a
	if roles, ok := a.root.(*roleRoot); ok {
		copyRoles := *roles
		copyRoles.Service = admission
		bound.root = &copyRoles
	} else {
		bound.root = admission
	}
	return &bound
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
	var err error
	if a.defaultWorkType != nil {
		defaultWorkTypeID, err = a.defaultWorkType(ctx, sessionID)
		if err != nil {
			return work.WorkRequest{}, err
		}
	}
	return a.root.PrepareWorkRequest(ctx, work.WorkRequestPreparation{
		Request: request, CanonicalJSON: canonicalJSON,
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
