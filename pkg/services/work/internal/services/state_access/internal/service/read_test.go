package service_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/internal/service"
)

func querySnapshot() work.ReadSnapshot {
	return work.ReadSnapshot{Items: []work.ReadModel{
		{CursorID: "tok-story", WorkID: "work-story", Name: "Review PRD", WorkTypeName: "story", TraceID: "trace-root", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
		{CursorID: "tok-bug", WorkID: "work-bug", Name: "Fix bug", WorkTypeName: "bug", CurrentChainingTraceID: "trace-chain-1", State: &work.State{Name: "init", Type: work.StateTypeInitial}},
		{CursorID: "tok-plan", WorkID: "work-plan", Name: "Plan feature", WorkTypeName: "story", TraceID: "trace-plan", State: &work.State{Name: "complete", Type: work.StateTypeTerminal}},
	}}
}

func TestListWorkReturnsDetachedReadModels(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshot: querySnapshot()}
	svc := internalservice.New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()

	got, err := svc.ListWork(ctx, "session-1", work.ListOptions{WorkTypeName: "bug"})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].WorkID != "work-bug" {
		t.Fatalf("ListWork = %#v, want one bug work item", got)
	}
	got.Results[0].State.Name = "mutated"
	second, err := svc.ListWork(ctx, "session-1", work.ListOptions{WorkTypeName: "bug"})
	if err != nil || second.Results[0].State.Name != "init" {
		t.Fatalf("ListWork mutated source snapshot: %#v, %v", second, err)
	}
}

func TestListWorkHonorsPaginationNextToken(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshot: work.ReadSnapshot{Items: []work.ReadModel{
		{CursorID: "tok-active-1", WorkID: "work-active-1", Name: "Alpha first", WorkTypeName: "task", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
		{CursorID: "tok-active-2", WorkID: "work-active-2", Name: "Alpha second", WorkTypeName: "task", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
	}}}
	svc := internalservice.New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()

	first, err := svc.ListWork(ctx, "session-1", work.ListOptions{Name: "alpha", MaxResults: 1})
	if err != nil || len(first.Results) != 1 || first.NextToken != base64.StdEncoding.EncodeToString([]byte("tok-active-1")) {
		t.Fatalf("first ListWork = %#v, %v", first, err)
	}
	second, err := svc.ListWork(ctx, "session-1", work.ListOptions{Name: "alpha", MaxResults: 1, NextToken: first.NextToken})
	if err != nil || len(second.Results) != 1 || second.Results[0].WorkID != "work-active-2" {
		t.Fatalf("second ListWork = %#v, %v", second, err)
	}
}

func TestGetWorkByCursorOrWorkIDAndNotFound(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshot: querySnapshot()}
	svc := internalservice.New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()

	for _, id := range []string{"tok-story", "work-story"} {
		got, err := svc.GetWork(ctx, "session-1", id)
		if err != nil || got.WorkID != "work-story" {
			t.Fatalf("GetWork(%q) = %#v, %v", id, got, err)
		}
	}
	if _, err := svc.GetWork(ctx, "session-1", "missing"); !errors.Is(err, work.ErrWorkNotFound) {
		t.Fatalf("GetWork(missing) error = %v, want ErrWorkNotFound", err)
	}
}

func TestMoveWorkAndReadReturnsDetachedPostMoveReadModel(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshot: work.ReadSnapshot{Items: []work.ReadModel{{
		CursorID: "tok-1",
		WorkID:   "work-1",
		Name:     "one",
		State:    &work.State{Name: "review", Type: work.StateTypeProcessing},
	}}}}
	svc := internalservice.New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()

	read, err := svc.MoveWorkAndRead(ctx, "session-1", "work-1", "complete", "request-1")
	if err != nil {
		t.Fatalf("MoveWorkAndRead: %v", err)
	}
	if read.WorkID != "work-1" || adapter.movedID != "work-1" || adapter.requestID != "request-1" {
		t.Fatalf("MoveWorkAndRead = %#v, adapter move = (%q, %q)", read, adapter.movedID, adapter.requestID)
	}
	read.State.Name = "mutated"
	got, err := svc.GetWork(ctx, "session-1", "work-1")
	if err != nil || got.State.Name != "review" {
		t.Fatalf("detached read mutated source: %#v, %v", got, err)
	}
}

func TestReadSnapshotUsesSessionAdapterOnly(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshotErr: errors.New("snapshot unavailable")}
	svc := internalservice.New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()

	_, err := svc.ListWork(ctx, "session-1", work.ListOptions{})
	if err == nil || err.Error() != "read Work snapshot: snapshot unavailable" {
		t.Fatalf("ListWork error = %v, want wrapped snapshot error", err)
	}
}
