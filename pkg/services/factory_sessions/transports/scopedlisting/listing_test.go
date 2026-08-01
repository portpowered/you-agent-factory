package scopedlisting_test

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	scopedlisting "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/scopedlisting"
)

type durableReader struct {
	result factorysessions.ListSessionsResult
}

func (reader durableReader) ListSessions(
	context.Context,
	factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	return reader.result, nil
}

func TestListUsesRootOwnedScopeAndFilterPolicy(t *testing.T) {
	t.Parallel()

	reader := durableReader{result: factorysessions.ListSessionsResult{
		DurableSessions: []factorysessions.DurableSessionListSummary{
			{
				SessionID: "matching-session",
				Status:    factorysessions.LifecycleStatusSucceeded,
				ResolvedSource: factorysessions.ResolvedSource{
					SourceRef: "factory/review",
					Metadata:  map[string]string{"project": "alpha/project"},
				},
			},
			{
				SessionID: "different-project",
				Status:    factorysessions.LifecycleStatusSucceeded,
				ResolvedSource: factorysessions.ResolvedSource{
					SourceRef: "factory/review",
					Metadata:  map[string]string{"project": "beta/project"},
				},
			},
		},
	}}

	result, err := scopedlisting.List(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopePersisted,
		Filters: factorysessions.SessionListFilters{
			Statuses:        []factorysessions.LifecycleStatus{factorysessions.LifecycleStatusSucceeded},
			SourceRef:       "review",
			ProjectBoundary: "alpha",
		},
	}, nil, reader)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.DurableSessions) != 1 || result.DurableSessions[0].SessionID != "matching-session" {
		t.Fatalf("durable sessions = %#v, want only matching-session", result.DurableSessions)
	}
}
