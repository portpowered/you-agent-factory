// Package state_access owns session-scoped Work submit, operator move, and read
// behind the published CTR-WORK state-access slice. Peers consume Work root
// contracts; this package is the parent-private nested owner for detached
// submit/move facts and detached ReadModel projections.
package state_access

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// SessionAdapter is the private Factory Session port used for session-scoped
// submit, move, and live read effects. It exposes only Work-owned request and
// result shapes and never leaks LiveRuntime or Factory Runtime Petri
// implementation types.
type SessionAdapter interface {
	SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	MoveWork(
		context.Context,
		string,
		string,
		work.WorkStateChangeSource,
		string,
	) (work.OperatorMoveResult, error)
	ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error)
}

// RecordingsAdapter is the private query-only Recordings port used when state
// access reads from Recordings-backed projections. It never writes Recordings
// stores and exposes only detached Work read snapshots.
type RecordingsAdapter interface {
	ReadWorkSnapshot(context.Context, string) (work.ReadSnapshot, error)
}

// SessionResolver resolves one session adapter for a Factory Session id.
type SessionResolver interface {
	ResolveSessionAdapter(string) (SessionAdapter, error)
}

// Service is the singular state_access subservice contract for the published
// submit, move, and read slice of the Work root Service.
type Service interface {
	SubmitWorkRequestForSession(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	MoveWorkForSession(context.Context, string, string, string, string) (work.OperatorMoveResult, error)
	ListWork(context.Context, string, work.ListOptions) (work.ListResult, error)
	GetWork(context.Context, string, string) (work.ReadModel, error)
	MoveWorkAndRead(context.Context, string, string, string, string) (work.ReadModel, error)
}
