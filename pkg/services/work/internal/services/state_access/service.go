// Package state_access owns session-scoped Work submit and operator move behind
// the published CTR-WORK state-access slice. Peers consume Work root contracts;
// this package is the parent-private nested owner for detached submit/move facts.
package state_access

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// SessionAdapter is the private Factory Session port used for session-scoped
// submit and move effects. It exposes only Work-owned request and result shapes
// and never leaks LiveRuntime or Factory Runtime Petri implementation types.
type SessionAdapter interface {
	SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	MoveWork(
		context.Context,
		string,
		string,
		work.WorkStateChangeSource,
		string,
	) (work.OperatorMoveResult, error)
}

// SessionResolver resolves one session adapter for a Factory Session id.
type SessionResolver interface {
	ResolveSessionAdapter(string) (SessionAdapter, error)
}

// Service is the singular state_access subservice contract for the published
// submit and move slice of the Work root Service.
type Service interface {
	SubmitWorkRequestForSession(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	MoveWorkForSession(context.Context, string, string, string, string) (work.OperatorMoveResult, error)
}
