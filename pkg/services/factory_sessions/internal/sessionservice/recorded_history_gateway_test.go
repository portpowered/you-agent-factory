package service

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestServiceListSessionsUsesBoundRecordedHistoryForHistoryScope(t *testing.T) {
	var got factorysessions.ListSessionsRequest
	service := &Service{}
	service.bindRecordedSessionHistory(func(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
		got = request
		return factorysessions.ListSessionsResult{
			Scope: factorysessions.SessionListScopeHistory,
			RecordedSessions: []factorysessions.RecordedSessionListSummary{{
				SessionID:         "recorded-session",
				Source:            factorysessions.RecordedSessionListSourceHistory,
				ArtifactReference: "2026/08/24/recorded-session.jsonl",
				Format:            factorysessions.RecordedSessionListFormatV2JSONL,
			}},
		}, nil
	})

	result, err := service.ListSessions(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopeHistory,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if got.Scope != factorysessions.SessionListScopeHistory {
		t.Fatalf("history request scope = %q, want history", got.Scope)
	}
	if len(result.RecordedSessions) != 1 || result.RecordedSessions[0].SessionID != "recorded-session" {
		t.Fatalf("recorded sessions = %#v, want bound history row", result.RecordedSessions)
	}
}
