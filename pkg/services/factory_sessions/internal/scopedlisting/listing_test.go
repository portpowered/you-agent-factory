package scopedlisting_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/scopedlisting"
)

type scopedLiveReader struct {
	reads []factorysessions.ReadProjection
	err   error
	calls int
}

func (reader *scopedLiveReader) ListScopedLiveSessions(context.Context) ([]factorysessions.ScopedLiveSessionSummary, error) {
	reader.calls++
	return scopedlisting.ProjectLiveSessions(reader.reads), reader.err
}

type scopedDurableReader struct {
	result  factorysessions.ListSessionsResult
	err     error
	calls   int
	request factorysessions.ListSessionsRequest
}

func (reader *scopedDurableReader) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	reader.calls++
	reader.request = request
	return reader.result, reader.err
}

func TestListOwnsSourceSelectionMergingAndOrdering(t *testing.T) {
	t.Parallel()
	live := &scopedLiveReader{reads: []factorysessions.ReadProjection{
		{Context: factorysessions.ProjectionContext{
			Session: &factorysessions.LiveSession{ID: "workspace-z"}, FactorySessionID: "workspace-z",
		}},
		{Context: factorysessions.ProjectionContext{
			Session: &factorysessions.LiveSession{ID: "workspace-a"}, FactorySessionID: "workspace-a",
		}, RuntimeAvailable: true, Runtime: factorysessions.RuntimeProjection{Status: "RUNNING"}},
	}}
	durable := &scopedDurableReader{result: factorysessions.ListSessionsResult{
		LiveSessions: []factorysessions.LiveSessionSummary{{ID: "durable-live-m"}},
		DurableSessions: []factorysessions.DurableSessionListSummary{
			{SessionID: "terminal-b", Status: factorysessions.LifecycleStatusSucceeded},
			{SessionID: "running-c", Status: factorysessions.LifecycleStatusRunning},
		},
	}}

	result, err := scopedlisting.List(context.Background(), factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeAll}, live, durable)
	if err != nil {
		t.Fatalf("List: %v", err)
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
	if durable.request.Scope != factorysessions.SessionListScopeAll || live.calls != 1 || durable.calls != 1 {
		t.Fatalf("source calls live=%d durable=%d durable request=%#v", live.calls, durable.calls, durable.request)
	}
	if result.LiveSessions[1].Runtime == nil || result.LiveSessions[1].Runtime.Status != "RUNNING" {
		t.Fatalf("workspace runtime projection = %#v, want detached runtime", result.LiveSessions[1].Runtime)
	}
}

func TestListPersistedSkipsLiveAndAppliesRecoverabilityPolicy(t *testing.T) {
	t.Parallel()
	live := &scopedLiveReader{err: errors.New("must not read live")}
	durable := &scopedDurableReader{result: factorysessions.ListSessionsResult{DurableSessions: []factorysessions.DurableSessionListSummary{
		{SessionID: "running", Status: factorysessions.LifecycleStatusRunning},
		{SessionID: "interrupted", Status: factorysessions.LifecycleStatus("INTERRUPTED"), Recoverable: true},
		{SessionID: "completed", Status: factorysessions.LifecycleStatusSucceeded},
	}}}
	result, err := scopedlisting.List(context.Background(), factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopePersisted}, live, durable)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if live.calls != 0 {
		t.Fatalf("live calls = %d, want zero", live.calls)
	}
	got := []string{result.DurableSessions[0].SessionID, result.DurableSessions[1].SessionID}
	if want := []string{"completed", "interrupted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted ids = %v, want %v", got, want)
	}
}

func TestListRequiresReadersForSelectedScopes(t *testing.T) {
	t.Parallel()
	if _, err := scopedlisting.List(context.Background(), factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeLive}, nil, nil); err == nil {
		t.Fatal("live scope succeeded without live reader")
	}
	if _, err := scopedlisting.List(context.Background(), factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopePersisted}, nil, nil); !errors.Is(err, scopedlisting.ErrDurableReaderRequired) {
		t.Fatalf("persisted error = %v, want required durable reader", err)
	}
}
