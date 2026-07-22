package work

import (
	"reflect"
	"testing"
)

func TestSelectFiltersStateTypeAndWorkTypeWithStableOrdering(t *testing.T) {
	t.Parallel()

	items := []Item{
		{ID: "tok-c", Name: "Third", WorkTypeName: "story", State: &State{Name: "review", Type: StateTypeProcessing}},
		{ID: "tok-a", Name: "First", WorkTypeName: "story", State: &State{Name: "init", Type: StateTypeInitial}},
		{ID: "tok-b", Name: "Second", WorkTypeName: "bug", State: &State{Name: "review", Type: StateTypeProcessing}},
		{ID: "tok-d", Name: "Fourth", WorkTypeName: "story", State: &State{Name: "review", Type: StateTypeProcessing}},
	}
	stateType := StateTypeProcessing
	workType := "story"
	selection := mustSelection(t, SelectionOptions{StateType: &stateType, WorkTypeName: &workType})
	selected := selection.Apply(items)

	if got, want := itemIDs(selected), []string{"tok-c", "tok-d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Select() IDs = %v, want %v", got, want)
	}
	if got, want := itemIDs(items), []string{"tok-c", "tok-a", "tok-b", "tok-d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Select() mutated input: IDs = %v, want %v", got, want)
	}
}

func TestSelectPreservesRepresentativeFilterBehavior(t *testing.T) {
	t.Parallel()

	items := []Item{
		{ID: "tok-story", Name: "Review PRD", WorkTypeName: "story", State: &State{Name: "review", Type: StateTypeProcessing}, TraceID: "trace-root"},
		{ID: "tok-bug", Name: "Fix bug", WorkTypeName: "bug", State: &State{Name: "init", Type: StateTypeInitial}, CurrentChainingTraceID: "trace-chain"},
		{ID: "tok-plan", Name: "Plan feature", WorkTypeName: "story", State: &State{Name: "init", Type: StateTypeInitial}, TraceID: "trace-plan"},
	}

	tests := []struct {
		name    string
		options SelectionOptions
		wantIDs []string
	}{
		{name: "empty", wantIDs: []string{"tok-bug", "tok-plan", "tok-story"}},
		{name: "name substring", options: SelectionOptions{Name: stringPointer("pRd")}, wantIDs: []string{"tok-story"}},
		{name: "state name", options: SelectionOptions{StateName: stringPointer("review")}, wantIDs: []string{"tok-story"}},
		{name: "trace", options: SelectionOptions{TraceID: stringPointer("trace-chain")}, wantIDs: []string{"tok-bug"}},
		{name: "no match", options: SelectionOptions{WorkTypeName: stringPointer("task")}, wantIDs: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := itemIDs(mustSelection(t, tt.options).Apply(items)); !reflect.DeepEqual(got, tt.wantIDs) {
				t.Fatalf("Select() IDs = %v, want %v", got, tt.wantIDs)
			}
		})
	}
}

func TestSelectSortsByStateTypeThenID(t *testing.T) {
	t.Parallel()

	items := []Item{
		{ID: "tok-terminal", State: &State{Type: StateTypeTerminal}},
		{ID: "tok-processing", State: &State{Type: StateTypeProcessing}},
		{ID: "tok-initial", State: &State{Type: StateTypeInitial}},
		{ID: "tok-failed", State: &State{Type: StateTypeFailed}},
	}
	got := itemIDs(mustSelection(t, SelectionOptions{SortBy: SortByStateType}).Apply(items))
	want := []string{"tok-failed", "tok-initial", "tok-processing", "tok-terminal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Select() IDs = %v, want %v", got, want)
	}
}

func TestValidateSelectionRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	unknown := "UNKNOWN"
	if err := ValidateSelection(SelectionOptions{StateType: &unknown}); err == nil || err.Error() != "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED" {
		t.Fatalf("ValidateSelection() state type error = %v", err)
	}
	if err := ValidateSelection(SelectionOptions{SortBy: "name"}); err == nil || err.Error() != "sortBy must be state.type" {
		t.Fatalf("ValidateSelection() sort error = %v", err)
	}
}

func itemIDs(items []Item) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func stringPointer(value string) *string {
	return &value
}

func mustSelection(t *testing.T, options SelectionOptions) Selection {
	t.Helper()
	selection, err := NewSelection(
		options.StateName, options.StateType, options.Name,
		options.WorkTypeName, options.TraceID, options.SortBy,
	)
	if err != nil {
		t.Fatalf("NewSelection() error = %v", err)
	}
	return selection
}
