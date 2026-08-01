package scopedlisting

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	transportlisting "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/scopedlisting"
)

var ErrDurableReaderRequired = transportlisting.ErrDurableReaderRequired

type LiveReader = transportlisting.LiveReader
type DurableReader = transportlisting.DurableReader

func List(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
	live LiveReader,
	durable DurableReader,
) (factorysessions.ScopedSessionListResult, error) {
	return transportlisting.List(ctx, request, live, durable)
}

func ProjectLiveSessions(
	reads []factorysessions.ReadProjection,
) []factorysessions.ScopedLiveSessionSummary {
	return transportlisting.ProjectLiveSessions(reads)
}
