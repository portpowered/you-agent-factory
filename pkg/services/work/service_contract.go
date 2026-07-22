package work

import (
	"context"
)

// Runtime is the narrow live-session capability consumed by Work operations.
type Runtime interface {
	SubmitWorkRequest(context.Context, WorkRequest) (WorkRequestSubmitResult, error)
	ReadWorkSnapshot(context.Context) (ReadSnapshot, error)
	MoveWork(
		context.Context,
		string,
		string,
		WorkStateChangeSource,
		string,
	) (OperatorMoveResult, error)
}

// RuntimeResolver resolves the Work-facing runtime for one Factory Session.
// Factory Sessions adapts its registry to this consumer-owned port.
type RuntimeResolver interface {
	ResolveWorkRuntime(string) (Runtime, error)
}

// RequestIDGenerator supplies opaque identity components for Work Requests and
// chaining traces. Wire selects the production generator; callers that submit
// fully identified requests do not need to invoke it.
type RequestIDGenerator func() string

// SubmittedFileReader reads a canonical Work Request submitted by path.
// Filesystem policy is selected at the process edge rather than by Work.
type SubmittedFileReader func(string) ([]byte, error)

// Service is the public Work submission and movement contract. Runtime state
// and event queries belong to Factory Runtime and Recordings.
type Service interface {
	SubmitWorkRequestForSession(context.Context, string, WorkRequest) (WorkRequestSubmitResult, error)
	MoveWorkForSession(context.Context, string, string, string, string) (OperatorMoveResult, error)
	ListWork(context.Context, string, ListOptions) (ListResult, error)
	GetWork(context.Context, string, string) (ReadModel, error)
	MoveWorkAndRead(context.Context, string, string, string, string) (ReadModel, error)
}

// FileSubmissionService is the exact Work role for path-backed submission.
// General Work consumers should depend on Service unless they submit files.
type FileSubmissionService interface {
	Service
	SubmitFileForSession(context.Context, string, string) (WorkRequestSubmitResult, error)
}
