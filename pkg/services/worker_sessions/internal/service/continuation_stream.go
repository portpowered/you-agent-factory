package service

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// An archived terminal continuation has no live Events topic. Drain its
// committed capture without registering a session or restoring supervision.
// Cursor-based history remains owned by the public captured logs boundary.
func (r *registry) completedContinuationStream(ctx context.Context, req workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	if r.logs == nil || req.Cursor != nil {
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationSessionNotFound
	}
	request := recordings.WorkerCapturedActivityRequest{WorkerSessionID: req.WorkerSessionID, Limit: req.Limit}
	if request.Limit == 0 {
		request.Limit = workersessions.DefaultObservationStreamLimit
	}
	page, err := r.logs.reader.ReadWorkerCapturedActivity(ctx, request)
	if err != nil {
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationSessionNotFound
	}
	opening, ok := completedContinuationOpening(page.Opening.Payload)
	if !ok || opening.WorkerSessionID != req.WorkerSessionID || page.Catalog.WorkerSessionID != req.WorkerSessionID ||
		(req.FactorySessionID != "" && page.Catalog.FactorySessionID != req.FactorySessionID) ||
		opening.FactorySessionID != page.Catalog.FactorySessionID ||
		page.Health != recordings.WorkerRecordingStatusComplete || page.Terminal == nil ||
		!workersessions.State(page.Terminal.Status).Terminal() || page.Terminal.Position == 0 {
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationSessionNotFound
	}
	s := &continuationCaptureStream{reader: r.logs.reader, request: request, page: page,
		catalog: page.Catalog, terminal: uint64(page.Terminal.Position), state: page.Terminal.Status}
	r.logger.Info("worker session completed continuation stream", "workerSessionID", req.WorkerSessionID, "outcome", "captured_replay")
	return workersessions.ObservationSubscription{NextFunc: s.Next, CloseFunc: s.Close}, nil
}

type continuationCaptureStream struct {
	mu       sync.Mutex
	reader   recordings.WorkerCapturedActivityReader
	request  recordings.WorkerCapturedActivityRequest
	page     recordings.WorkerCapturedActivityPage
	catalog  recordings.WorkerSessionCatalogEntry
	terminal uint64
	state    string
	position uint64
	closed   bool
}

func (s *continuationCaptureStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

func (s *continuationCaptureStream) Next(ctx context.Context) workersessions.ObservationDelivery {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
	}
	if err := ctx.Err(); err != nil {
		s.closed = true
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: err}
	}
	if len(s.page.Records) == 0 && !s.advance(ctx) {
		s.closed = true
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceUnavailable}
	}
	captured := s.page.Records[0]
	s.page.Records = s.page.Records[1:]
	event := projectObservationEvent(captured.Record, s.request.WorkerSessionID)
	if event.Position != s.position+1 || event.Position > s.terminal ||
		(event.Position == s.terminal && !isTerminalLifecycleRecord(captured.Record)) {
		s.closed = true
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceUnavailable}
	}
	s.position = event.Position
	event.CapturedAt = captured.CapturedAt
	event.Truncated, event.ArtifactRef = captured.Truncated, captured.ArtifactRef
	event.OriginalBytes, event.ReturnedBytes = captured.OriginalBytes, captured.ReturnedBytes
	delivery := workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord, Event: event.Clone()}
	if event.Position == s.terminal {
		s.closed = true
		delivery.Kind = workersessions.ObservationDeliveryTerminalReplay
		delivery.Summary = &workersessions.ReplaySummary{Complete: true, EventsEmitted: int(s.position)}
	}
	return delivery
}

func (s *continuationCaptureStream) advance(ctx context.Context) bool {
	if s.page.NextToken == "" {
		return false
	}
	s.request.NextToken = s.page.NextToken
	page, err := s.reader.ReadWorkerCapturedActivity(ctx, s.request)
	if err != nil || page.Catalog != s.catalog || page.Health != recordings.WorkerRecordingStatusComplete ||
		page.Terminal == nil || uint64(page.Terminal.Position) != s.terminal ||
		page.Terminal.Status != s.state || len(page.Records) == 0 {
		return false
	}
	s.page = page
	return true
}
