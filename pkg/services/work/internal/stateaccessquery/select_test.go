package stateaccessquery_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/stateaccessquery"
)

func TestSelectFiltersStateTypeAndWorkTypeWithStableOrdering(t *testing.T) {
	t.Parallel()

	items := []stateaccessquery.Item{
		{ID: "tok-c", Name: "Third", WorkTypeName: "story", State: &stateaccessquery.State{Name: "review", Type: stateaccessquery.StateTypeProcessing}},
		{ID: "tok-a", Name: "First", WorkTypeName: "story", State: &stateaccessquery.State{Name: "init", Type: stateaccessquery.StateTypeInitial}},
		{ID: "tok-b", Name: "Second", WorkTypeName: "bug", State: &stateaccessquery.State{Name: "review", Type: stateaccessquery.StateTypeProcessing}},
		{ID: "tok-d", Name: "Fourth", WorkTypeName: "story", State: &stateaccessquery.State{Name: "review", Type: stateaccessquery.StateTypeProcessing}},
	}
	stateType := stateaccessquery.StateTypeProcessing
	workType := "story"
	selection := mustSelection(t, stateaccessquery.SelectionOptions{StateType: &stateType, WorkTypeName: &workType})
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

	items := []stateaccessquery.Item{
		{ID: "tok-story", Name: "Review PRD", WorkTypeName: "story", State: &stateaccessquery.State{Name: "review", Type: stateaccessquery.StateTypeProcessing}, TraceID: "trace-root"},
		{ID: "tok-bug", Name: "Fix bug", WorkTypeName: "bug", State: &stateaccessquery.State{Name: "init", Type: stateaccessquery.StateTypeInitial}, CurrentChainingTraceID: "trace-chain"},
		{ID: "tok-plan", Name: "Plan feature", WorkTypeName: "story", State: &stateaccessquery.State{Name: "init", Type: stateaccessquery.StateTypeInitial}, TraceID: "trace-plan"},
	}

	tests := []struct {
		name    string
		options stateaccessquery.SelectionOptions
		wantIDs []string
	}{
		{name: "empty", wantIDs: []string{"tok-bug", "tok-plan", "tok-story"}},
		{name: "name substring", options: stateaccessquery.SelectionOptions{Name: stringPointer("pRd")}, wantIDs: []string{"tok-story"}},
		{name: "state name", options: stateaccessquery.SelectionOptions{StateName: stringPointer("review")}, wantIDs: []string{"tok-story"}},
		{name: "trace", options: stateaccessquery.SelectionOptions{TraceID: stringPointer("trace-chain")}, wantIDs: []string{"tok-bug"}},
		{name: "no match", options: stateaccessquery.SelectionOptions{WorkTypeName: stringPointer("task")}, wantIDs: []string{}},
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

	items := []stateaccessquery.Item{
		{ID: "tok-terminal", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeTerminal}},
		{ID: "tok-processing", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeProcessing}},
		{ID: "tok-initial", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeInitial}},
		{ID: "tok-failed", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeFailed}},
	}
	got := itemIDs(mustSelection(t, stateaccessquery.SelectionOptions{SortBy: stateaccessquery.SortByStateType}).Apply(items))
	want := []string{"tok-failed", "tok-initial", "tok-processing", "tok-terminal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Select() IDs = %v, want %v", got, want)
	}
}

func TestSelectTerminalityIncludesFailedAndExcludesUnknownStates(t *testing.T) {
	t.Parallel()

	items := []stateaccessquery.Item{
		{ID: "tok-terminal", WorkTypeName: "story", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeTerminal}},
		{ID: "tok-failed", WorkTypeName: "story", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeFailed}},
		{ID: "tok-processing", WorkTypeName: "story", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeProcessing}},
		{ID: "tok-unknown", WorkTypeName: "story", State: &stateaccessquery.State{Type: "UNKNOWN"}},
		{ID: "tok-missing", WorkTypeName: "story"},
	}
	terminal := true
	nonTerminal := true

	terminalSelection := mustSelection(t, stateaccessquery.SelectionOptions{
		Terminal:     &terminal,
		WorkTypeName: stringPointer("story"),
	})
	if got, want := itemIDs(terminalSelection.Apply(items)), []string{"tok-failed", "tok-terminal"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal selection IDs = %v, want %v", got, want)
	}
	nonTerminalSelection := mustSelection(t, stateaccessquery.SelectionOptions{
		NonTerminal:  &nonTerminal,
		WorkTypeName: stringPointer("story"),
	})
	if got, want := itemIDs(nonTerminalSelection.Apply(items)), []string{"tok-processing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("non-terminal selection IDs = %v, want %v", got, want)
	}
}

func TestAnnotateSupersessionUsesNewestSameNameSuccessor(t *testing.T) {
	t.Parallel()

	items := []stateaccessquery.Item{
		{ID: "tok-old", WorkID: "work-old", Name: "same", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeFailed}},
		{ID: "tok-processing", WorkID: "work-processing", Name: "same", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeProcessing}},
		{ID: "tok-new", WorkID: "work-new", Name: "same", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeProcessing}},
		{ID: "tok-different", WorkID: "work-different", Name: "different", State: &stateaccessquery.State{Type: stateaccessquery.StateTypeTerminal}},
	}
	admissions := []stateaccessquery.Admission{
		{WorkID: "work-old", Name: "same", Order: 0},
		{WorkID: "work-processing", Name: "same", Order: 1},
		{WorkID: "work-new", Name: "same", Order: 2},
		{WorkID: "work-different", Name: "different", Order: 3},
	}

	annotated := stateaccessquery.AnnotateSupersession(items, admissions)
	if annotated[0].SupersededBy != "work-new" {
		t.Fatalf("old item SupersededBy = %q, want work-new", annotated[0].SupersededBy)
	}
	for _, index := range []int{1, 2, 3} {
		if annotated[index].SupersededBy != "" {
			t.Fatalf("item %q SupersededBy = %q, want empty", annotated[index].ID, annotated[index].SupersededBy)
		}
	}
	if items[0].SupersededBy != "" {
		t.Fatalf("AnnotateSupersession mutated input: %#v", items[0])
	}

	defaultSelection := mustSelection(t, stateaccessquery.SelectionOptions{})
	if got := itemIDs(defaultSelection.Apply(annotated)); !reflect.DeepEqual(got, []string{"tok-new", "tok-processing", "tok-different"}) {
		t.Fatalf("default selection IDs = %v, want superseded item omitted", got)
	}
	includeSelection := mustSelection(t, stateaccessquery.SelectionOptions{IncludeSuperseded: true})
	if got := itemIDs(includeSelection.Apply(annotated)); !reflect.DeepEqual(got, []string{"tok-new", "tok-processing", "tok-old", "tok-different"}) {
		t.Fatalf("include-superseded selection IDs = %v, want complete history", got)
	}
}

func TestValidateSelectionRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	unknown := "UNKNOWN"
	if err := stateaccessquery.ValidateSelection(stateaccessquery.SelectionOptions{StateType: &unknown}); err == nil || err.Error() != "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED" {
		t.Fatalf("ValidateSelection() state type error = %v", err)
	}
	if err := stateaccessquery.ValidateSelection(stateaccessquery.SelectionOptions{SortBy: "name"}); err == nil || err.Error() != "sortBy must be state.type" {
		t.Fatalf("ValidateSelection() sort error = %v", err)
	}
	terminal, nonTerminal := true, true
	if err := stateaccessquery.ValidateSelection(stateaccessquery.SelectionOptions{Terminal: &terminal, NonTerminal: &nonTerminal}); err == nil || err.Error() != "terminal and nonTerminal cannot both be selected" {
		t.Fatalf("ValidateSelection() terminality error = %v", err)
	}
}

func itemIDs(items []stateaccessquery.Item) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func stringPointer(value string) *string {
	return &value
}

func mustSelection(t *testing.T, options stateaccessquery.SelectionOptions) stateaccessquery.Selection {
	t.Helper()
	selection, err := stateaccessquery.NewSelectionWithOptions(options)
	if err != nil {
		t.Fatalf("NewSelection() error = %v", err)
	}
	return selection
}
