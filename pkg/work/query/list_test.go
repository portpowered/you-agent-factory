package query

import (
	"encoding/base64"
	"testing"
)

func TestNormalizeListAcceptedCombination(t *testing.T) {
	t.Parallel()

	nextToken := base64.StdEncoding.EncodeToString([]byte("work/42"))
	options := ListOptions{
		StateName:    "in review",
		StateType:    StateTypeProcessing,
		Name:         "Alpha & Beta",
		WorkTypeName: "story/review",
		TraceID:      "trace value",
		SortBy:       SortByStateType,
		MaxResults:   25,
		NextToken:    nextToken,
	}
	query, err := NormalizeList(options)
	if err != nil {
		t.Fatalf("NormalizeList() error = %v", err)
	}
	if got := query.Options(); got != options {
		t.Fatalf("Options() = %#v, want %#v", got, options)
	}
	if got, want := query.FilterSummary(), "state.name,state.type,name,workTypeName,traceId,sortBy"; got != want {
		t.Fatalf("FilterSummary() = %q, want %q", got, want)
	}
}

func TestNormalizeListOmitsEmptyValuesFromSummary(t *testing.T) {
	t.Parallel()

	query, err := NormalizeList(ListOptions{Name: "kept exactly "})
	if err != nil {
		t.Fatalf("NormalizeList() error = %v", err)
	}
	if got, want := query.Options().Name, "kept exactly "; got != want {
		t.Fatalf("Options().Name = %q, want %q", got, want)
	}
	if got, want := query.FilterSummary(), "name"; got != want {
		t.Fatalf("FilterSummary() = %q, want %q", got, want)
	}
}

func TestNormalizeListRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options ListOptions
		wantErr string
	}{
		{name: "unsupported state type", options: ListOptions{StateType: "RUNNING"}, wantErr: "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED"},
		{name: "unsupported sort", options: ListOptions{SortBy: "name"}, wantErr: "sortBy must be state.type"},
		{name: "negative page size", options: ListOptions{MaxResults: -1}, wantErr: "maxResults must be zero or greater"},
		{name: "malformed cursor", options: ListOptions{NextToken: "not-base64"}, wantErr: "nextToken must be valid standard base64 for a non-empty cursor"},
		{name: "empty decoded cursor", options: ListOptions{NextToken: "===="}, wantErr: "nextToken must be valid standard base64 for a non-empty cursor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NormalizeList(tt.options)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("NormalizeList() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeListAcceptsEveryStateType(t *testing.T) {
	t.Parallel()

	for _, stateType := range []string{StateTypeInitial, StateTypeProcessing, StateTypeTerminal, StateTypeFailed} {
		stateType := stateType
		t.Run(stateType, func(t *testing.T) {
			t.Parallel()
			if _, err := NormalizeList(ListOptions{StateType: stateType}); err != nil {
				t.Fatalf("NormalizeList() error = %v", err)
			}
		})
	}
}
