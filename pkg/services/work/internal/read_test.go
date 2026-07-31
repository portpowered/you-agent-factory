package internal_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
)

type readRuntime struct {
	snapshot work.ReadSnapshot
	moved    bool
	movedID  string
	movedTo  string
	source   work.WorkStateChangeSource
	request  string
}

func queryReadRuntime() *readRuntime {
	return &readRuntime{snapshot: work.ReadSnapshot{Items: []work.ReadModel{
		{CursorID: "tok-story", WorkID: "work-story", Name: "Review PRD", WorkTypeName: "story", TraceID: "trace-root", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
		{CursorID: "tok-bug", WorkID: "work-bug", Name: "Fix bug", WorkTypeName: "bug", CurrentChainingTraceID: "trace-chain-1", State: &work.State{Name: "init", Type: work.StateTypeInitial}},
		{CursorID: "tok-plan", WorkID: "work-plan", Name: "Plan feature", WorkTypeName: "story", TraceID: "trace-plan", State: &work.State{Name: "complete", Type: work.StateTypeTerminal}},
	}}}
}

func TestListWork_FiltersByWorkTypeNameNameSubstringAndTraceId(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: queryReadRuntime()}, nil, nil, nil)
	for _, tc := range []struct {
		name    string
		options work.ListOptions
		want    string
	}{
		{name: "work type", options: work.ListOptions{WorkTypeName: "bug"}, want: "work-bug"},
		{name: "name substring", options: work.ListOptions{Name: "prd"}, want: "work-story"},
		{name: "root trace", options: work.ListOptions{TraceID: "trace-root"}, want: "work-story"},
		{name: "current chaining trace", options: work.ListOptions{TraceID: "trace-chain-1"}, want: "work-bug"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := service.ListWork(context.Background(), "session-1", tc.options)
			if err != nil || len(got.Results) != 1 || got.Results[0].WorkID != tc.want {
				t.Fatalf("ListWork = %#v, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestListWork_FiltersBeforePagination(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: queryReadRuntime()}, nil, nil, nil)
	got, err := service.ListWork(context.Background(), "session-1", work.ListOptions{WorkTypeName: "story", MaxResults: 2})
	if err != nil || len(got.Results) != 2 || got.NextToken != "" {
		t.Fatalf("ListWork = %#v, %v", got, err)
	}
}

func TestListWork_FiltersByStateNameAndType(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: queryReadRuntime()}, nil, nil, nil)
	got, err := service.ListWork(context.Background(), "session-1", work.ListOptions{StateName: "review", StateType: work.StateTypeProcessing})
	if err != nil || len(got.Results) != 1 || got.Results[0].WorkID != "work-story" {
		t.Fatalf("ListWork = %#v, %v", got, err)
	}
}

func TestListWork_DefaultOrderingSurfacesActiveWorkBeforeTerminalWork(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: queryReadRuntime()}, nil, nil, nil)
	got, err := service.ListWork(context.Background(), "session-1", work.ListOptions{})
	if err != nil || len(got.Results) != 3 || got.Results[0].WorkID != "work-bug" || got.Results[1].WorkID != "work-story" || got.Results[2].WorkID != "work-plan" {
		t.Fatalf("ListWork = %#v, %v", got, err)
	}
}

func TestListWork_SortsByStateType(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: queryReadRuntime()}, nil, nil, nil)
	got, err := service.ListWork(context.Background(), "session-1", work.ListOptions{SortBy: "state.type"})
	if err != nil || len(got.Results) != 3 || got.Results[0].State.Type != work.StateTypeInitial || got.Results[2].State.Type != work.StateTypeTerminal {
		t.Fatalf("ListWork = %#v, %v", got, err)
	}
}

func TestListWork_InvalidStateTypeAndSortReturnValidationErrors(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: queryReadRuntime()}, nil, nil, nil)
	for _, options := range []work.ListOptions{{StateType: "BROKEN"}, {SortBy: "broken"}} {
		_, err := service.ListWork(context.Background(), "session-1", options)
		var validation *work.ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("ListWork(%#v) error = %v, want ValidationError", options, err)
		}
	}
}

func TestListWork_NonPositiveMaxResultsDefaultsAndNextTokenContinues(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: queryReadRuntime()}, nil, nil, nil)
	first, err := service.ListWork(context.Background(), "session-1", work.ListOptions{MaxResults: 1})
	if err != nil || len(first.Results) != 1 || first.NextToken == "" {
		t.Fatalf("first ListWork = %#v, %v", first, err)
	}
	second, err := service.ListWork(context.Background(), "session-1", work.ListOptions{MaxResults: 1, NextToken: first.NextToken})
	if err != nil || len(second.Results) != 1 || second.Results[0].WorkID == first.Results[0].WorkID {
		t.Fatalf("second ListWork = %#v, %v", second, err)
	}
	defaulted, err := service.ListWork(context.Background(), "session-1", work.ListOptions{MaxResults: 0})
	if err != nil || defaulted.MaxResults != work.DefaultListMaxResults {
		t.Fatalf("defaulted ListWork = %#v, %v", defaulted, err)
	}
}

func TestGetWork_ByTokenAndWorkIDAndNotFound(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: queryReadRuntime()}, nil, nil, nil)
	for _, id := range []string{"tok-story", "work-story"} {
		got, err := service.GetWork(context.Background(), "session-1", id)
		if err != nil || got.WorkID != "work-story" {
			t.Fatalf("GetWork(%q) = %#v, %v", id, got, err)
		}
	}
	if _, err := service.GetWork(context.Background(), "session-1", "missing"); !errors.Is(err, work.ErrWorkNotFound) {
		t.Fatalf("GetWork(missing) error = %v", err)
	}
}

func (r *readRuntime) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}
func (r *readRuntime) MoveWork(_ context.Context, workID, stateName string, source work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error) {
	r.moved = true
	r.movedID, r.movedTo, r.source, r.request = workID, stateName, source, requestID
	for index := range r.snapshot.Items {
		if r.snapshot.Items[index].WorkID != workID {
			continue
		}
		state := work.State{Name: stateName, Type: work.StateTypeTerminal}
		r.snapshot.Items[index].State = &state
	}
	return work.OperatorMoveResult{}, nil
}
func (r *readRuntime) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return r.snapshot, nil
}

func TestListWorkOwnsSelectionOrderingPaginationAndDetachesResults(t *testing.T) {
	runtime := &readRuntime{snapshot: work.ReadSnapshot{Items: []work.ReadModel{
		{CursorID: "tok-terminal", WorkID: "work-terminal", Name: "done", WorkTypeName: "task", State: &work.State{Name: "complete", Type: work.StateTypeTerminal}},
		{CursorID: "tok-active-2", WorkID: "work-active-2", Name: "Alpha second", WorkTypeName: "task", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
		{CursorID: "tok-active-1", WorkID: "work-active-1", Name: "Alpha first", WorkTypeName: "task", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
	}}}
	service := internalservice.NewService(workRuntimeResolver{runtime: runtime}, nil, nil, nil)
	first, err := service.ListWork(context.Background(), "session-1", work.ListOptions{Name: "alpha", MaxResults: 1})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(first.Results) != 1 || first.Results[0].WorkID != "work-active-1" || first.NextToken != base64.StdEncoding.EncodeToString([]byte("tok-active-1")) {
		t.Fatalf("first page = %#v", first)
	}
	first.Results[0].State.Name = "mutated"
	second, err := service.ListWork(context.Background(), "session-1", work.ListOptions{Name: "alpha", MaxResults: 1, NextToken: first.NextToken})
	if err != nil {
		t.Fatalf("ListWork second page: %v", err)
	}
	if len(second.Results) != 1 || second.Results[0].WorkID != "work-active-2" || runtime.snapshot.Items[2].State.Name != "review" {
		t.Fatalf("second page or source mutation = %#v / %#v", second, runtime.snapshot)
	}
}

func TestGetWorkAndMoveWorkAndReadOwnDetachedReadSemantics(t *testing.T) {
	runtime := &readRuntime{snapshot: work.ReadSnapshot{Items: []work.ReadModel{{CursorID: "tok-1", WorkID: "work-1", Name: "one", State: &work.State{Name: "review", Type: work.StateTypeProcessing}}}}}
	service := internalservice.NewService(workRuntimeResolver{runtime: runtime}, nil, nil, nil)
	read, err := service.GetWork(context.Background(), "session-1", "work-1")
	if err != nil || read.CursorID != "tok-1" {
		t.Fatalf("GetWork = %#v, %v", read, err)
	}
	read.State.Name = "mutated"
	moved, err := service.MoveWorkAndRead(context.Background(), "session-1", "work-1", "complete", "request-1")
	if err != nil || !runtime.moved || runtime.movedID != "work-1" || runtime.movedTo != "complete" ||
		runtime.source != work.WorkStateChangeSourceAPI || runtime.request != "request-1" ||
		moved.State.Name != "complete" || runtime.snapshot.Items[0].State.Name != "complete" {
		t.Fatalf("MoveWorkAndRead = %#v, %v, moved=%v", moved, err, runtime.moved)
	}
	moved.State.Name = "mutated"
	if runtime.snapshot.Items[0].State.Name != "complete" {
		t.Fatalf("MoveWorkAndRead result mutated source snapshot: %#v", runtime.snapshot.Items[0])
	}
}
