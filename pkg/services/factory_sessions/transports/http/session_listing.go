package http

import (
	"context"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/scopedlisting"
)

// ErrDurableReaderRequired is retained as the transport compatibility symbol;
// source selection and persisted-scope policy live in scopedlisting.
var ErrDurableReaderRequired = scopedlisting.ErrDurableReaderRequired

// LiveSessionListReader is the exact detached live-list role consumed by the
// Factory Sessions HTTP adapter.
type LiveSessionListReader = scopedlisting.LiveReader

// LiveSessionProjectionReader is the canonical root-service read role adapted
// to the detached HTTP listing input.
type LiveSessionProjectionReader interface {
	ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error)
}

// ReadProjectionSessionListReader adapts canonical live projections without
// applying generated HTTP representation policy.
type ReadProjectionSessionListReader struct {
	Reader LiveSessionProjectionReader
}

func (reader ReadProjectionSessionListReader) ListScopedLiveSessions(
	ctx context.Context,
) ([]factorysessions.ScopedLiveSessionSummary, error) {
	if reader.Reader == nil {
		return nil, fmt.Errorf("list scoped Factory Sessions: live session projection reader is required")
	}
	reads, err := reader.Reader.ListFactorySessions(ctx)
	if err != nil {
		return nil, err
	}
	return scopedlisting.ProjectLiveSessions(reads), nil
}

type scopedLiveReader = scopedlisting.LiveReader
type scopedDurableReader = scopedlisting.DurableReader

func mergeScopedSessionList(
	ctx context.Context,
	request factorysessions.ListSessionsRequest,
	live scopedLiveReader,
	durable scopedDurableReader,
) (factorysessions.ScopedSessionListResult, error) {
	return scopedlisting.List(ctx, request, live, durable)
}
