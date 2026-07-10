package workquery

import (
	"encoding/base64"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeListAcceptedCombination(t *testing.T) {
	t.Parallel()

	nextToken := base64.StdEncoding.EncodeToString([]byte("work/42"))
	query, err := NormalizeList(ListOptions{
		Filters: map[string]string{
			FilterTraceID:      "trace value",
			FilterName:         "Alpha & Beta",
			FilterStateType:    "PROCESSING",
			FilterWorkTypeName: "story/review",
			FilterStateName:    "in review",
		},
		SortBy:     SortByStateType,
		MaxResults: 25,
		NextToken:  nextToken,
	})
	if err != nil {
		t.Fatalf("NormalizeList() error = %v", err)
	}

	wantValues := url.Values{
		"state.name":   {"in review"},
		"state.type":   {"PROCESSING"},
		"name":         {"Alpha & Beta"},
		"workTypeName": {"story/review"},
		"traceId":      {"trace value"},
		"sortBy":       {"state.type"},
		"maxResults":   {"25"},
		"nextToken":    {nextToken},
	}
	if got := query.Values(); !reflect.DeepEqual(got, wantValues) {
		t.Fatalf("Values() = %#v, want %#v", got, wantValues)
	}
	if got, want := query.FilterSummary(), "state.name,state.type,name,workTypeName,traceId,sortBy"; got != want {
		t.Fatalf("FilterSummary() = %q, want %q", got, want)
	}
	if got := query.Values().Encode(); !strings.Contains(got, "nextToken="+url.QueryEscape(nextToken)) {
		t.Fatalf("encoded values %q do not preserve nextToken %q", got, nextToken)
	}
}

func TestNormalizeListOmitsEmptyAndUnspecifiedValues(t *testing.T) {
	t.Parallel()

	query, err := NormalizeList(ListOptions{
		Filters: map[string]string{
			FilterStateName: "",
			FilterName:      "kept exactly ",
		},
		MaxResults: 0,
		NextToken:  "",
	})
	if err != nil {
		t.Fatalf("NormalizeList() error = %v", err)
	}
	if got, want := query.Values(), (url.Values{"name": {"kept exactly "}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}
	if got, want := query.FilterSummary(), "name"; got != want {
		t.Fatalf("FilterSummary() = %q, want %q", got, want)
	}
}

func TestNormalizeListEmptySummary(t *testing.T) {
	t.Parallel()

	query, err := NormalizeList(ListOptions{})
	if err != nil {
		t.Fatalf("NormalizeList() error = %v", err)
	}
	if got := query.Values(); len(got) != 0 {
		t.Fatalf("Values() = %#v, want empty", got)
	}
	if got := query.FilterSummary(); got != "none" {
		t.Fatalf("FilterSummary() = %q, want none", got)
	}
}

func TestListQueryValuesReturnsCopy(t *testing.T) {
	t.Parallel()

	query, err := NormalizeList(ListOptions{Filters: map[string]string{FilterName: "alpha"}})
	if err != nil {
		t.Fatalf("NormalizeList() error = %v", err)
	}
	values := query.Values()
	values.Set(FilterName, "changed")
	if got := query.Values().Get(FilterName); got != "alpha" {
		t.Fatalf("Values().Get(%q) = %q after caller mutation, want alpha", FilterName, got)
	}
}

func TestNormalizeListRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options ListOptions
		wantErr string
	}{
		{
			name:    "unsupported filter",
			options: ListOptions{Filters: map[string]string{"stateName": "review"}},
			wantErr: `unsupported work-list filter "stateName"`,
		},
		{
			name:    "unsupported empty filter",
			options: ListOptions{Filters: map[string]string{"unknown": ""}},
			wantErr: `unsupported work-list filter "unknown"`,
		},
		{
			name:    "unsupported state type",
			options: ListOptions{Filters: map[string]string{FilterStateType: "RUNNING"}},
			wantErr: "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED",
		},
		{
			name:    "unsupported sort",
			options: ListOptions{SortBy: "name"},
			wantErr: "sortBy must be state.type",
		},
		{
			name:    "negative page size",
			options: ListOptions{MaxResults: -1},
			wantErr: "maxResults must be zero or greater",
		},
		{
			name:    "malformed cursor",
			options: ListOptions{NextToken: "not-base64"},
			wantErr: "nextToken must be valid standard base64 for a non-empty cursor",
		},
		{
			name:    "empty decoded cursor",
			options: ListOptions{NextToken: "===="},
			wantErr: "nextToken must be valid standard base64 for a non-empty cursor",
		},
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

	for _, stateType := range []string{"INITIAL", "PROCESSING", "TERMINAL", "FAILED"} {
		stateType := stateType
		t.Run(stateType, func(t *testing.T) {
			t.Parallel()
			query, err := NormalizeList(ListOptions{Filters: map[string]string{FilterStateType: stateType}})
			if err != nil {
				t.Fatalf("NormalizeList() error = %v", err)
			}
			if got := query.Values().Get(FilterStateType); got != stateType {
				t.Fatalf("Values().Get(%q) = %q, want %q", FilterStateType, got, stateType)
			}
		})
	}
}
