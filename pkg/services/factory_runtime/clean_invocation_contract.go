package factory

import "context"

// CleanInvocationWork is the detached work result vocabulary consumed by the
// clean-invocation transport. Runtime owns the mapping from engine state to
// these customer-facing facts.
type CleanInvocationWork struct {
	WorkID        string
	WorkTypeID    string
	StateCategory string
	Output        string
	TraceID       string
	DataType      string
}

// CleanInvocationDispatch is the detached dispatch history vocabulary needed
// to classify clean-invocation failures and accepted output mutations.
type CleanInvocationDispatch struct {
	Outcome     string
	Reason      string
	FailureType string
	Consumed    []CleanInvocationWork
	Outputs     []CleanInvocationWork
}

// CleanInvocationSnapshot is the narrow Runtime-owned projection required by
// the clean-invocation command. It deliberately contains no engine topology,
// marking, token, or provider implementation types.
type CleanInvocationSnapshot struct {
	Work            []CleanInvocationWork
	DispatchHistory []CleanInvocationDispatch
}

// CleanInvocationSnapshotProvider supplies the clean-invocation projection to
// a transport without exposing the concrete Factory Runtime engine snapshot.
type CleanInvocationSnapshotProvider interface {
	CleanInvocationSnapshot(context.Context) (CleanInvocationSnapshot, error)
}
