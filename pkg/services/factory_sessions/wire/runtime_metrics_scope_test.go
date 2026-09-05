package wire

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestRuntimeMetricsScopeResolverRetainsPublicSelectorAlongsideCanonicalLineage(t *testing.T) {
	const canonicalID = "canonical-runtime-id"
	reader := runtimeMetricsSessionReaderFunc(func(context.Context, string) (factorysessions.SessionProjection, error) {
		return factorysessions.SessionProjection{
			Runtime: factorysessions.RuntimeProjection{
				StreamIdentity: &factorysessions.RuntimeStreamIdentity{
					FactorySessionID: canonicalID,
				},
				RetainedMetricsSessionIDs: []string{canonicalID},
			},
		}, nil
	})
	resolver := NewRuntimeMetricsScopeResolver(reader)

	got, err := resolver.ResolveRuntimeMetricsScope(context.Background(), factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatalf("ResolveRuntimeMetricsScope() error = %v", err)
	}
	want := []string{canonicalID, factorysessions.DefaultSessionID}
	if len(got.RetainedFactorySessionIDs) != len(want) {
		t.Fatalf("retained metrics IDs = %#v, want %#v", got.RetainedFactorySessionIDs, want)
	}
	for index, value := range want {
		if got.RetainedFactorySessionIDs[index] != value {
			t.Fatalf("retained metrics IDs = %#v, want %#v", got.RetainedFactorySessionIDs, want)
		}
	}
}

type runtimeMetricsSessionReaderFunc func(context.Context, string) (factorysessions.SessionProjection, error)

func (reader runtimeMetricsSessionReaderFunc) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	return reader(ctx, sessionID)
}
