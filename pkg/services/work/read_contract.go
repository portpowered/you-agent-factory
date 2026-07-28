package work

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/stateaccessquery"
)

const (
	DefaultListMaxResults = 50

	FilterStateName    = "state.name"
	FilterStateType    = "state.type"
	FilterName         = "name"
	FilterWorkTypeName = "workTypeName"
	FilterTraceID      = "traceId"

	SortByStateType = "state.type"

	StateTypeInitial    = "INITIAL"
	StateTypeProcessing = "PROCESSING"
	StateTypeTerminal   = "TERMINAL"
	StateTypeFailed     = "FAILED"
)

// ErrWorkNotFound is the typed state-access failure returned when list/get /
// move-and-read cannot resolve the requested Work. Peers branch with errors.Is.
var ErrWorkNotFound = errors.New("Work not found")

// ReadModel is the detached customer-facing Work projection returned by the
// root Service state-access slice (ListWork, GetWork, MoveWorkAndRead). It
// deliberately contains no token, place, marking, or topology fields and no
// Factory Runtime or peer implementation types.
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

// ListResult is the plain Work-owned state-access list contract. Peers consume
// detached ReadModel projections and pagination facts without importing Work
// implementation packages.
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

// State is the query-owned projection of a Work state.
type State struct {
	Name string
	Type string
}

// ListOptions is the plain Work-owned state-access list request contract used by
// Service.ListWork. Filters, ordering, and pagination stay transport-independent.
type ListOptions struct {
	StateName    string
	StateType    string
	Name         string
	WorkTypeName string
	TraceID      string
	SortBy       string
	MaxResults   int
	NextToken    string
}

// PreparedListRequest is the detached, validated value returned to transport
// adapters. It contains no Work implementation or runtime reference.
type PreparedListRequest struct {
	Options       ListOptions
	FilterSummary string
}

// ListRequestPreparation is the exact Work-owned policy role used by
// transports before representing a Work list request on a protocol.
type ListRequestPreparation interface {
	PrepareListRequest(context.Context, ListOptions) (PreparedListRequest, error)
}

// ValidationError identifies the query field that failed validation. Boundary
// adapters can use Field to present transport-specific field names.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ListQuery is a validated Work-list query. Its options are immutable through
// the exported API.
type ListQuery struct {
	query stateaccessquery.ListQuery
}

// Options returns the normalized query values.
func (q ListQuery) Options() ListOptions {
	opts := q.query.Options()
	return ListOptions{
		StateName:    opts.StateName,
		StateType:    opts.StateType,
		Name:         opts.Name,
		WorkTypeName: opts.WorkTypeName,
		TraceID:      opts.TraceID,
		SortBy:       opts.SortBy,
		MaxResults:   opts.MaxResults,
		NextToken:    opts.NextToken,
	}
}

// FilterSummary returns the active filter and sort keys in canonical order.
func (q ListQuery) FilterSummary() string {
	return q.query.FilterSummary()
}

// NewListRequestPreparation constructs the pure Work list preparation role.
// Wire supplies this role to transport adapters; transports never select it.
func NewListRequestPreparation() ListRequestPreparation {
	return listRequestPreparationAdapter{}
}

type listRequestPreparationAdapter struct{}

func (listRequestPreparationAdapter) PrepareListRequest(
	ctx context.Context,
	options ListOptions,
) (PreparedListRequest, error) {
	prepared, err := stateaccessquery.NewListRequestPreparation().PrepareListRequest(
		ctx,
		listOptionsToQuery(options),
	)
	if err != nil {
		return PreparedListRequest{}, mapQueryValidationError(err)
	}
	return PreparedListRequest{
		Options:       listOptionsFromQuery(prepared.Options),
		FilterSummary: prepared.FilterSummary,
	}, nil
}

// NormalizeList validates options and returns their canonical values and
// active-filter summary.
func NormalizeList(options ListOptions) (ListQuery, error) {
	query, err := stateaccessquery.NormalizeList(listOptionsToQuery(options))
	if err != nil {
		return ListQuery{}, mapQueryValidationError(err)
	}
	return ListQuery{query: query}, nil
}

// ValidWorkStateType reports whether stateType is an allowed Work-list state
// type filter.
func ValidWorkStateType(stateType string) bool {
	return stateaccessquery.ValidWorkStateType(stateType)
}

func listOptionsToQuery(options ListOptions) stateaccessquery.ListOptions {
	return stateaccessquery.ListOptions{
		StateName:    options.StateName,
		StateType:    options.StateType,
		Name:         options.Name,
		WorkTypeName: options.WorkTypeName,
		TraceID:      options.TraceID,
		SortBy:       options.SortBy,
		MaxResults:   options.MaxResults,
		NextToken:    options.NextToken,
	}
}

func listOptionsFromQuery(options stateaccessquery.ListOptions) ListOptions {
	return ListOptions{
		StateName:    options.StateName,
		StateType:    options.StateType,
		Name:         options.Name,
		WorkTypeName: options.WorkTypeName,
		TraceID:      options.TraceID,
		SortBy:       options.SortBy,
		MaxResults:   options.MaxResults,
		NextToken:    options.NextToken,
	}
}

func mapQueryValidationError(err error) error {
	if err == nil {
		return nil
	}
	var validation *stateaccessquery.ValidationError
	if errors.As(err, &validation) {
		return &ValidationError{Field: validation.Field, Message: validation.Message}
	}
	return err
}
