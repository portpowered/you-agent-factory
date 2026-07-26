package automations

import "context"

// UnimplementedService provides typed root-slice defaults so concrete
// implementers stay assignable to Service before private reconciliation and
// source collaborators are wired. Embed it and override only implemented
// operations.
type UnimplementedService struct{}

var _ Service = UnimplementedService{}

// ReadyUnimplementedService reports boundary availability while retaining
// typed not-ready outcomes for private slices whose collaborators are not yet
// wired.
type ReadyUnimplementedService struct {
	UnimplementedService
}

var _ Service = ReadyUnimplementedService{}

func (ReadyUnimplementedService) Ready(context.Context, ReadyRequest) (ReadyResult, error) {
	return ReadyResult{Ready: true}, nil
}

func (UnimplementedService) Ready(context.Context, ReadyRequest) (ReadyResult, error) {
	return ReadyResult{}, unimplementedError("Ready")
}

func (UnimplementedService) Reconcile(
	context.Context,
	ReconcileRequest,
) (ReconcileResult, error) {
	return ReconcileResult{}, unimplementedError("Reconcile")
}

func (UnimplementedService) StartSource(
	context.Context,
	StartSourceRequest,
) (StartSourceResult, error) {
	return StartSourceResult{}, unimplementedError("StartSource")
}

func (UnimplementedService) StopSource(
	context.Context,
	StopSourceRequest,
) (StopSourceResult, error) {
	return StopSourceResult{}, unimplementedError("StopSource")
}

func (UnimplementedService) WaitSource(
	context.Context,
	WaitSourceRequest,
) (WaitSourceResult, error) {
	return WaitSourceResult{}, unimplementedError("WaitSource")
}

func (UnimplementedService) SourceStatus(
	context.Context,
	SourceStatusRequest,
) (SourceStatusResult, error) {
	return SourceStatusResult{}, unimplementedError("SourceStatus")
}

func (UnimplementedService) GetStatus(
	context.Context,
	GetStatusRequest,
) (GetStatusResult, error) {
	return GetStatusResult{}, unimplementedError("GetStatus")
}

func (UnimplementedService) GetCursor(
	context.Context,
	GetCursorRequest,
) (GetCursorResult, error) {
	return GetCursorResult{}, unimplementedError("GetCursor")
}

func unimplementedError(op string) *Error {
	return &Error{
		Op:   op,
		Code: ErrorCodeNotReady,
		Err:  ErrNotReady,
	}
}
