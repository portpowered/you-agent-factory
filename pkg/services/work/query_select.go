package work

import (
	"sort"
	"strings"
)

const (
	StateTypeInitial    = "INITIAL"
	StateTypeProcessing = "PROCESSING"
	StateTypeTerminal   = "TERMINAL"
	StateTypeFailed     = "FAILED"
)

// State is the query-owned projection of a Work state.
type State struct {
	Name string
	Type string
}

// Item contains only the Work facts needed for selection and ordering.
type Item struct {
	ID                     string
	Name                   string
	WorkTypeName           string
	State                  *State
	TraceID                string
	CurrentChainingTraceID string
}

// SelectionOptions contains filters and ordering for canonical Work selection.
// Nil filters are absent; non-nil filters, including empty values, are explicit.
type SelectionOptions struct {
	StateName    *string
	StateType    *string
	Name         *string
	WorkTypeName *string
	TraceID      *string
	SortBy       string
}

// Selection is a validated, immutable Work selection policy.
type Selection struct {
	options SelectionOptions
}

// NewSelection validates options before any Work values are selected.
func NewSelection(
	stateName *string,
	stateType *string,
	name *string,
	workTypeName *string,
	traceID *string,
	sortBy string,
) (Selection, error) {
	options := SelectionOptions{
		StateName: stateName, StateType: stateType, Name: name,
		WorkTypeName: workTypeName, TraceID: traceID, SortBy: sortBy,
	}
	if err := ValidateSelection(options); err != nil {
		return Selection{}, err
	}
	return Selection{options: options}, nil
}

// ValidateSelection rejects unsupported state types and ordering modes.
func ValidateSelection(options SelectionOptions) error {
	if options.StateType != nil && !ValidWorkStateType(*options.StateType) {
		return validationError(
			FilterStateType,
			FilterStateType+" must be one of INITIAL, PROCESSING, TERMINAL, or FAILED",
		)
	}
	if options.SortBy != "" && options.SortBy != SortByStateType {
		return validationError("sortBy", "sortBy must be "+SortByStateType)
	}
	return nil
}

// Apply returns matching items in deterministic order without mutating input.
func (s Selection) Apply(items []Item) []Item {
	selected := make([]Item, 0, len(items))
	for _, item := range items {
		if matches(item, s.options) {
			selected = append(selected, item)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		left, right := selected[i], selected[j]
		if s.options.SortBy == SortByStateType {
			if leftType, rightType := stateType(left.State), stateType(right.State); leftType != rightType {
				return leftType < rightType
			}
			return left.ID < right.ID
		}

		if leftOrder, rightOrder := stateOrder(left.State), stateOrder(right.State); leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if leftType, rightType := stateType(left.State), stateType(right.State); leftType != rightType {
			return leftType < rightType
		}
		return left.ID < right.ID
	})
	return selected
}

func matches(item Item, options SelectionOptions) bool {
	return matchesState(item, options) &&
		matchesName(item, options.Name) &&
		matchesWorkType(item, options.WorkTypeName) &&
		matchesTrace(item, options.TraceID)
}

func matchesState(item Item, options SelectionOptions) bool {
	if options.StateName != nil && (item.State == nil || item.State.Name != *options.StateName) {
		return false
	}
	if options.StateType != nil && (item.State == nil || item.State.Type != *options.StateType) {
		return false
	}
	return true
}

func matchesName(item Item, filter *string) bool {
	return filter == nil || *filter == "" ||
		strings.Contains(strings.ToLower(item.Name), strings.ToLower(*filter))
}

func matchesWorkType(item Item, filter *string) bool {
	return filter == nil || *filter == "" || item.WorkTypeName == *filter
}

func matchesTrace(item Item, filter *string) bool {
	return filter == nil || *filter == "" || item.TraceID == *filter || item.CurrentChainingTraceID == *filter
}

func stateOrder(state *State) int {
	if state == nil {
		return 4
	}
	switch state.Type {
	case StateTypeInitial:
		return 0
	case StateTypeProcessing:
		return 1
	case StateTypeFailed:
		return 2
	case StateTypeTerminal:
		return 3
	default:
		return 4
	}
}

func stateType(state *State) string {
	if state == nil {
		return ""
	}
	return state.Type
}
