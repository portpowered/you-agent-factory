package http

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestMergeScopedSessionList_HistorySelectsRecordedProjectionOnly(t *testing.T) {
	t.Parallel()

	live := &recordedListingLiveReader{err: errors.New("live reader must not be called")}
	durable := &recordedListingDurableReader{result: factorysessions.ListSessionsResult{
		LiveSessions: []factorysessions.LiveSessionSummary{{ID: "live-ignored"}},
		DurableSessions: []factorysessions.DurableSessionListSummary{{
			SessionID: "durable-ignored",
		}},
		RecordedSessions: []factorysessions.RecordedSessionListSummary{
			{SessionID: "session-b", Source: factorysessions.RecordedSessionListSourceHistory, ArtifactReference: "2026/08/24/session-b.jsonl", Format: factorysessions.RecordedSessionListFormatV2JSONL},
			{SessionID: "session-a", Source: factorysessions.RecordedSessionListSourceHistory, ArtifactReference: "2026/08/23/session-a.json", Format: factorysessions.RecordedSessionListFormatV1JSON},
		},
	}}

	result, err := mergeScopedSessionList(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopeHistory,
	}, live, durable)
	if err != nil {
		t.Fatalf("merge history: %v", err)
	}
	if live.calls != 0 {
		t.Fatalf("live calls = %d, want zero", live.calls)
	}
	if durable.request.Scope != factorysessions.SessionListScopeHistory || durable.request.ExcludeRecordedHistory {
		t.Fatalf("recorded inventory request = %#v, want history without history exclusion", durable.request)
	}
	if len(result.LiveSessions) != 0 || len(result.DurableSessions) != 0 || len(result.RecordedSessions) != 2 {
		t.Fatalf("history result = %#v, want recorded rows only", result)
	}
	if result.RecordedSessions[0].SessionID != "session-a" || result.RecordedSessions[1].SessionID != "session-b" {
		t.Fatalf("recorded ordering = %#v, want canonical session ordering", result.RecordedSessions)
	}
}

func TestMergeScopedSessionList_LiveExcludesRecordedHistoryAtSourceBoundary(t *testing.T) {
	t.Parallel()

	live := &recordedListingLiveReader{rows: []factorysessions.ScopedLiveSessionSummary{{ID: "live-session"}}}
	durable := &recordedListingDurableReader{result: factorysessions.ListSessionsResult{
		RecordedSessions: []factorysessions.RecordedSessionListSummary{{SessionID: "recorded-session"}},
	}}

	result, err := mergeScopedSessionList(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopeLive,
	}, live, durable)
	if err != nil {
		t.Fatalf("merge live: %v", err)
	}
	if durable.request.Scope != factorysessions.SessionListScopeAll || !durable.request.ExcludeRecordedHistory {
		t.Fatalf("live inventory request = %#v, want all with recorded history excluded", durable.request)
	}
	if len(result.LiveSessions) != 1 || result.LiveSessions[0].ID != "live-session" || len(result.RecordedSessions) != 0 {
		t.Fatalf("live result = %#v, want live only", result)
	}
}

func TestMergeScopedSessionList_PersistedExcludesRecordedHistoryAtSourceBoundary(t *testing.T) {
	t.Parallel()

	durable := &recordedListingDurableReader{result: factorysessions.ListSessionsResult{
		DurableSessions: []factorysessions.DurableSessionListSummary{{
			SessionID: "durable-session", Status: factorysessions.LifecycleStatusSucceeded,
		}},
		RecordedSessions: []factorysessions.RecordedSessionListSummary{{
			SessionID: "recorded-session",
		}},
	}}

	result, err := mergeScopedSessionList(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopePersisted,
	}, nil, durable)
	if err != nil {
		t.Fatalf("merge persisted: %v", err)
	}
	if durable.request.Scope != factorysessions.SessionListScopeAll || !durable.request.ExcludeRecordedHistory {
		t.Fatalf("persisted inventory request = %#v, want all with recorded history excluded", durable.request)
	}
	if len(result.DurableSessions) != 1 || len(result.RecordedSessions) != 0 {
		t.Fatalf("persisted result = %#v, want durable rows without history", result)
	}
}

type recordedListingLiveReader struct {
	rows  []factorysessions.ScopedLiveSessionSummary
	err   error
	calls int
}

func (reader *recordedListingLiveReader) ListScopedLiveSessions(context.Context) ([]factorysessions.ScopedLiveSessionSummary, error) {
	reader.calls++
	return reader.rows, reader.err
}

type recordedListingDurableReader struct {
	result  factorysessions.ListSessionsResult
	request factorysessions.ListSessionsRequest
	calls   int
}

func (reader *recordedListingDurableReader) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	reader.calls++
	reader.request = request
	return reader.result, nil
}
