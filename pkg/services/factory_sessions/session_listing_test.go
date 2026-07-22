package factorysessions

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type scopedLiveReader struct {
	reads []ReadProjection
	err   error
	calls int
}

func (reader *scopedLiveReader) ListScopedLiveSessions(context.Context) ([]ScopedLiveSessionSummary, error) {
	reader.calls++
	return projectScopedLiveSessions(reader.reads), reader.err
}

type scopedDurableReader struct {
	result  ListSessionsResult
	err     error
	calls   int
	request ListSessionsRequest
}

func (reader *scopedDurableReader) ListSessions(_ context.Context, request ListSessionsRequest) (ListSessionsResult, error) {
	reader.calls++
	reader.request = request
	return reader.result, reader.err
}

func TestListScopedSessionsOwnsSourceSelectionMergingAndOrdering(t *testing.T) {
	t.Parallel()

	live := &scopedLiveReader{reads: []ReadProjection{
		{Context: ProjectionContext{Session: &LiveSession{ID: "workspace-z"}}},
		{Context: ProjectionContext{Session: &LiveSession{ID: "workspace-a"}}, RuntimeAvailable: true, Runtime: RuntimeProjection{Status: "RUNNING"}},
	}}
	durable := &scopedDurableReader{result: ListSessionsResult{
		LiveSessions: []LiveSessionSummary{{ID: "durable-live-m"}},
		DurableSessions: []DurableSessionListSummary{
			{SessionID: "terminal-b", Status: LifecycleStatusSucceeded},
			{SessionID: "running-c", Status: LifecycleStatusRunning},
		},
	}}

	result, err := ListScopedSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeAll}, live, durable)
	if err != nil {
		t.Fatalf("ListScopedSessions: %v", err)
	}
	gotIDs := make([]string, 0, len(result.LiveSessions))
	for _, session := range result.LiveSessions {
		gotIDs = append(gotIDs, session.ID)
	}
	if want := []string{"durable-live-m", "workspace-a", "workspace-z"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("live ids = %v, want %v", gotIDs, want)
	}
	if len(result.DurableSessions) != 2 || result.DurableSessions[0].SessionID != "running-c" {
		t.Fatalf("durable sessions = %#v, want sorted all-scope rows", result.DurableSessions)
	}
	if durable.request.Scope != SessionListScopeAll || live.calls != 1 || durable.calls != 1 {
		t.Fatalf("source calls live=%d durable=%d durable request=%#v", live.calls, durable.calls, durable.request)
	}
	if result.LiveSessions[1].Runtime == nil || result.LiveSessions[1].Runtime.Status != "RUNNING" {
		t.Fatalf("workspace runtime projection = %#v, want detached runtime", result.LiveSessions[1].Runtime)
	}
}

func TestListScopedSessionsPersistedSkipsLiveAndAppliesRecoverabilityPolicy(t *testing.T) {
	t.Parallel()

	live := &scopedLiveReader{err: errors.New("must not read live")}
	durable := &scopedDurableReader{result: ListSessionsResult{DurableSessions: []DurableSessionListSummary{
		{SessionID: "running", Status: LifecycleStatusRunning},
		{SessionID: "interrupted", Status: LifecycleStatus("INTERRUPTED"), Recoverable: true},
		{SessionID: "completed", Status: LifecycleStatusSucceeded},
	}}}
	result, err := ListScopedSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopePersisted}, live, durable)
	if err != nil {
		t.Fatalf("ListScopedSessions: %v", err)
	}
	if live.calls != 0 {
		t.Fatalf("live calls = %d, want zero", live.calls)
	}
	got := []string{result.DurableSessions[0].SessionID, result.DurableSessions[1].SessionID}
	if want := []string{"completed", "interrupted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted ids = %v, want %v", got, want)
	}
}

func TestListScopedSessionsRequiresLiveReaderOnlyForLiveScopes(t *testing.T) {
	t.Parallel()

	if _, err := ListScopedSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeLive}, nil, nil); err == nil {
		t.Fatal("live scope succeeded without live reader")
	}
	if _, err := ListScopedSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopePersisted}, nil, nil); !errors.Is(err, ErrDurableSessionListReaderRequired) {
		t.Fatalf("persisted error = %v, want required durable reader", err)
	}
}
