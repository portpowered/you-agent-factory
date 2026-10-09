package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// An archived capture has no live Events topic. Drain its
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
		if errors.Is(err, os.ErrNotExist) {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationSessionNotFound
		}
		if ctx.Err() != nil {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationCanceled
		}
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationSourceUnavailable
	}
	opening, ok := capturedReplayOpening(page.Opening.Payload)
	if !ok || opening.WorkerSessionID != req.WorkerSessionID || page.Catalog.WorkerSessionID != req.WorkerSessionID ||
		(req.FactorySessionID != "" && page.Catalog.FactorySessionID != req.FactorySessionID) ||
		opening.FactorySessionID != page.Catalog.FactorySessionID ||
		(page.Terminal != nil && !workersessions.State(page.Terminal.Status).Terminal()) {
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationSessionNotFound
	}
	s := &continuationCaptureStream{reader: r.logs.reader, request: request, page: page,
		catalog: page.Catalog, head: uint64(page.Catalog.CommittedPosition), health: page.Health, topic: page.Opening.ID.Topic}
	if page.Terminal != nil {
		s.terminal, s.state = uint64(page.Terminal.Position), page.Terminal.Status
	}
	r.logger.Info("worker session captured observation stream", "workerSessionID", req.WorkerSessionID, "outcome", "captured_replay")
	return workersessions.ObservationSubscription{NextFunc: s.Next, CloseFunc: s.Close}, nil
}

// Read identity does not require execution continuation lineage or a provider reference.
func capturedReplayOpening(payload json.RawMessage) (workers.SessionPayload, bool) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted ||
		json.Unmarshal(draft.Payload, &opening) != nil {
		return workers.SessionPayload{}, false
	}
	return opening, true
}

type continuationCaptureStream struct {
	mu       sync.Mutex
	reader   recordings.WorkerCapturedActivityReader
	request  recordings.WorkerCapturedActivityRequest
	page     recordings.WorkerCapturedActivityPage
	catalog  recordings.WorkerSessionCatalogEntry
	topic    events.Topic
	head     uint64
	health   recordings.WorkerRecordingStatus
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
	head := s.head
	if s.position == head && len(s.page.Records) == 0 {
		s.closed = true
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryReplaySummary,
			Summary: &workersessions.ReplaySummary{Complete: false, Reason: "capture-" + string(s.health), EventsEmitted: int(s.position)}}
	}
	if len(s.page.Records) == 0 && !s.advance(ctx) {
		s.closed = true
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceUnavailable}
	}
	captured := s.page.Records[0]
	s.page.Records = s.page.Records[1:]
	event := projectObservationEvent(captured.Record, s.request.WorkerSessionID)
	if captured.Record.ID.Topic != s.topic ||
		event.Position != s.position+1 || event.Position > head ||
		(event.Position == s.terminal && !isTerminalLifecycleRecord(captured.Record)) {
		s.closed = true
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceUnavailable}
	}
	s.position = event.Position
	event.CapturedAt = captured.CapturedAt
	event.Truncated, event.ArtifactRef = captured.Truncated, captured.ArtifactRef
	event.OriginalBytes, event.ReturnedBytes = captured.OriginalBytes, captured.ReturnedBytes
	delivery := workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord, Event: event.Clone()}
	if event.Position == s.terminal && event.Position == s.head && s.health == recordings.WorkerRecordingStatusComplete {
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
	if err != nil || page.Catalog != s.catalog || !s.pageMetadataMatches(page) || len(page.Records) == 0 {
		return false
	}
	s.page = page
	return true
}

func (s *continuationCaptureStream) pageMetadataMatches(page recordings.WorkerCapturedActivityPage) bool {
	if page.Health != s.health {
		return false
	}
	if page.Terminal == nil {
		return s.terminal == 0 && s.state == ""
	}
	return uint64(page.Terminal.Position) == s.terminal && page.Terminal.Status == s.state
}
