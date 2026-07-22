package work

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	FilterStateName    = "state.name"
	FilterStateType    = "state.type"
	FilterName         = "name"
	FilterWorkTypeName = "workTypeName"
	FilterTraceID      = "traceId"

	SortByStateType = "state.type"
)

// ListOptions contains transport-independent filters, ordering, and pagination
// controls for a Work list request.
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

type listRequestPreparation struct{}

// NewListRequestPreparation constructs the pure Work list preparation role.
// Wire supplies this role to transport adapters; transports never select it.
func NewListRequestPreparation() ListRequestPreparation {
	return listRequestPreparation{}
}

func (listRequestPreparation) PrepareListRequest(
	ctx context.Context,
	options ListOptions,
) (PreparedListRequest, error) {
	if ctx == nil {
		return PreparedListRequest{}, errors.New("Work list preparation context is required")
	}
	if err := ctx.Err(); err != nil {
		return PreparedListRequest{}, err
	}
	query, err := NormalizeList(options)
	if err != nil {
		return PreparedListRequest{}, err
	}
	return PreparedListRequest{
		Options:       query.Options(),
		FilterSummary: query.FilterSummary(),
	}, nil
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

func validationError(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

// ListQuery is a validated Work-list query. Its options are immutable through
// the exported API.
type ListQuery struct {
	options       ListOptions
	filterSummary string
}

// NormalizeList validates options and returns their canonical values and
// active-filter summary.
func NormalizeList(options ListOptions) (ListQuery, error) {
	if options.StateType != "" && !ValidWorkStateType(options.StateType) {
		return ListQuery{}, validationError(
			FilterStateType,
			fmt.Sprintf("%s must be one of INITIAL, PROCESSING, TERMINAL, or FAILED", FilterStateType),
		)
	}
	if options.SortBy != "" && options.SortBy != SortByStateType {
		return ListQuery{}, validationError("sortBy", fmt.Sprintf("sortBy must be %s", SortByStateType))
	}
	if options.MaxResults < 0 {
		return ListQuery{}, validationError("maxResults", "maxResults must be zero or greater")
	}
	if err := validateNextToken(options.NextToken); err != nil {
		return ListQuery{}, err
	}

	active := make([]string, 0, 6)
	for _, entry := range []struct {
		key   string
		value string
	}{
		{FilterStateName, options.StateName},
		{FilterStateType, options.StateType},
		{FilterName, options.Name},
		{FilterWorkTypeName, options.WorkTypeName},
		{FilterTraceID, options.TraceID},
		{"sortBy", options.SortBy},
	} {
		if entry.value != "" {
			active = append(active, entry.key)
		}
	}

	summary := "none"
	if len(active) > 0 {
		summary = strings.Join(active, ",")
	}
	return ListQuery{options: options, filterSummary: summary}, nil
}

// Options returns the normalized query values.
func (q ListQuery) Options() ListOptions {
	return q.options
}

// FilterSummary returns the active filter and sort keys in canonical order.
func (q ListQuery) FilterSummary() string {
	return q.filterSummary
}

func validateNextToken(nextToken string) error {
	if nextToken == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(nextToken)
	if err != nil || len(decoded) == 0 {
		return validationError("nextToken", "nextToken must be valid standard base64 for a non-empty cursor")
	}
	return nil
}

// ValidWorkStateType reports whether stateType is an allowed Work-list state
// type filter.
func ValidWorkStateType(stateType string) bool {
	switch stateType {
	case StateTypeInitial, StateTypeProcessing, StateTypeTerminal, StateTypeFailed:
		return true
	default:
		return false
	}
}
