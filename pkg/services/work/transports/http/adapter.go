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
)

// Adapter maps Work service values at the outward HTTP boundary.
type Adapter struct {
	root work.Service
}

// NewAdapter constructs the Work HTTP representation adapter.
func NewAdapter(root work.Service) *Adapter {
	if root == nil {
		return nil
	}
	return &Adapter{root: root}
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
