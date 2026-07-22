package work

import "errors"

const DefaultListMaxResults = 50

var ErrWorkNotFound = errors.New("Work not found")

// ReadModel is the detached customer-facing Work projection returned by the
// Work owner. It deliberately contains no token, place, marking, or topology.
type ReadModel struct {
	CursorID                 string
	Name                     string
	WorkID                   string
	WorkTypeName             string
	State                    *State
	ChainingTraceDepth       int
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	TraceID                  string
	Content                  []WorkContentPart
	Tags                     map[string]string
	Relations                []ReadRelation
	StopSummary              *StopSummary
}

type ReadRelation struct {
	Type           RelationType
	SourceWorkName string
	TargetWorkName string
	TargetWorkID   string
	RequiredState  string
}

type ListResult struct {
	Results    []ReadModel
	MaxResults int
	NextToken  string
}

// ReadSnapshot is the detached runtime observation consumed only by the Work
// owner. Factory Sessions adapts its live runtime into this contract, so Work
// never imports Factory Runtime and transports never observe engine types.
type ReadSnapshot struct{ Items []ReadModel }

// StopSummary is the Work-owned detached copy of the stopped-state context
// needed by a Work read. Factory Sessions remains the policy owner that
// derives it; Work prevents that owner's runtime values leaking to transports.
type StopSummary struct {
	SessionID                string
	StopKind                 string
	SessionLifecycleStatus   *string
	WorkID                   *string
	WorkName                 *string
	WorkTypeName             *string
	WorkState                *string
	LatestDispatch           *StopDispatchSummary
	LatestResultSummary      *string
	SuggestedRecoverySurface *string
	SuggestedRecoveryAction  *string
}

type StopDispatchSummary struct {
	DispatchID      string
	Status          string
	DispatchKind    string
	WorkstationName *string
	FailureDetail   *StopFailureDetail
}

type StopFailureDetail struct{ Reason, Message string }
