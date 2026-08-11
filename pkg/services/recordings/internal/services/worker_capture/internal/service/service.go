package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const defaultSubscriptionLimit = 128

const (
	workerLifecycleSourceType = events.SourceType("worker_session_lifecycle")
	openingSourceSequence     = events.SourceSequence(1)
	openingSourceEventID      = events.SourceEventID("started")
	workerDraftSchemaID       = events.SchemaID("workers.draft.v1")
)

// Service owns the Recordings side of one source-native Worker capture. It
// never appends to Events: Events is the source stream, while writer is the
// only durable acceptance port.
type Service struct {
	events events.Service
	writer recordings.WorkerRecordingWriter
	logger logging.Logger
	limit  int
}

var _ recordings.WorkerSessionRecordingService = (*Service)(nil)

// New constructs the Worker capture capability. Construction is inert; the
// topic subscription is opened only for a concrete recording request.
func New(
	eventService events.Service,
	writer recordings.WorkerRecordingWriter,
	logger logging.Logger,
	limits ...int,
) (recordings.WorkerSessionRecordingService, error) {
	if eventService == nil {
		return nil, fmt.Errorf("recordings Worker capture: Events service is required")
	}
	if writer == nil {
		return nil, recordings.ErrMissingWorkerRecordingWriter
	}
	limit := defaultSubscriptionLimit
	if len(limits) > 0 && limits[0] > 0 {
		limit = limits[0]
	}
	return &Service{
		events: eventService,
		writer: writer,
		logger: logging.EnsureLogger(logger),
		limit:  limit,
	}, nil
}

// StartWorkerSessionRecording subscribes from aggregate position zero before
// returning. The handle's opening barrier is released only after the exact
// position-one lifecycle record has been durably persisted.
func (service *Service) StartWorkerSessionRecording(
	ctx context.Context,
	request recordings.WorkerSessionRecordingRequest,
) (recordings.WorkerSessionRecording, error) {
	if service == nil || service.events == nil {
		return nil, fmt.Errorf("recordings Worker capture: Events service is required")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", recordings.ErrWorkerRecordingCanceled, err)
	}
	subscription, err := service.events.Subscribe(ctx, events.SubscribeRequest{
		Topic: request.Topic,
		From:  events.Cursor{Topic: request.Topic},
		Limit: service.limit,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w: topic %q", recordings.ErrWorkerRecordingSubscribe, err, request.Topic)
	}

	runCtx, stop := context.WithCancel(context.Background())
	capture := &capture{
		request:      request,
		writer:       service.writer,
		subscription: subscription,
		logger:       service.logger,
		runCtx:       runCtx,
		stop:         stop,
		opening:      make(chan struct{}),
		failure:      make(chan struct{}),
		done:         make(chan struct{}),
		identities:   make(map[events.AppendIdentity]events.Record),
	}
	go capture.consume()
	return capture, nil
}

type capture struct {
	request      recordings.WorkerSessionRecordingRequest
	writer       recordings.WorkerRecordingWriter
	subscription events.Subscription
	logger       logging.Logger
	runCtx       context.Context
	stop         context.CancelFunc
	stopOnce     sync.Once

	opening chan struct{}
	failure chan struct{}
	done    chan struct{}

	mu           sync.Mutex
	opened       bool
	terminal     bool
	closed       bool
	failed       error
	lastPosition events.AggregateSequence
	identities   map[events.AppendIdentity]events.Record
}

func (capture *capture) consume() {
	defer close(capture.done)
	for {
		delivery := capture.subscription.Next(capture.runCtx)
		if err := delivery.Validate(); err != nil {
			capture.fail(fmt.Errorf("%w: malformed delivery: %w", recordings.ErrWorkerRecordingDelivery, err))
			return
		}
		switch delivery.Kind {
		case events.DeliveryRecord:
			if err := capture.accept(delivery.Record); err != nil {
				capture.fail(err)
				return
			}
			if capture.isTerminal() {
				return
			}
		case events.DeliveryGap:
			capture.fail(fmt.Errorf("%w: topic %q requested=%d earliest=%d", recordings.ErrWorkerRecordingGap, capture.request.Topic, delivery.Gap.Requested, delivery.Gap.EarliestRetained))
			return
		case events.DeliveryClosed:
			capture.fail(fmt.Errorf("%w: topic %q", recordings.ErrWorkerRecordingClosed, capture.request.Topic))
			return
		case events.DeliveryCanceled:
			capture.mu.Lock()
			closed := capture.closed
			capture.mu.Unlock()
			if !closed {
				capture.fail(fmt.Errorf("%w: topic %q", recordings.ErrWorkerRecordingCanceled, capture.request.Topic))
			}
			return
		case events.DeliveryBackpressure:
			capture.fail(fmt.Errorf("%w: topic %q", recordings.ErrWorkerRecordingBackpressure, capture.request.Topic))
			return
		default:
			capture.fail(fmt.Errorf("%w: topic %q returned unspecified delivery", recordings.ErrWorkerRecordingDelivery, capture.request.Topic))
			return
		}
	}
}

func (capture *capture) accept(record events.Record) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("%w: malformed record: %w", recordings.ErrWorkerRecordingDelivery, err)
	}
	identity := record.Identity()

	capture.mu.Lock()
	if previous, exists := capture.identities[identity]; exists {
		capture.mu.Unlock()
		if sameRecord(previous, record) {
			return nil
		}
		return fmt.Errorf("%w: source identity %q/%q/%d/%q changed", recordings.ErrWorkerRecordingDuplicate, identity.SourceType, identity.SourceID, identity.SourceSequence, identity.SourceEventID)
	}
	lastPosition := capture.lastPosition
	opened := capture.opened
	capture.mu.Unlock()

	if !opened {
		if record.ID.Topic != capture.request.Topic || record.ID.Position != 1 ||
			record.SourceType != workerLifecycleSourceType ||
			record.SourceID != events.SourceID(capture.request.WorkerSessionID) ||
			record.SourceSequence != openingSourceSequence ||
			record.SourceEventID != openingSourceEventID ||
			record.SchemaID != workerDraftSchemaID {
			return fmt.Errorf("%w: expected correlated position-1 SESSION/STARTED for Worker Session %q", recordings.ErrWorkerRecordingOpening, capture.request.WorkerSessionID)
		}
		if err := validateOpeningPayload(record.Payload, capture.request.WorkerSessionID); err != nil {
			return err
		}
	} else if record.ID.Topic != capture.request.Topic {
		return fmt.Errorf("%w: record topic %q does not match %q", recordings.ErrWorkerRecordingOrder, record.ID.Topic, capture.request.Topic)
	}

	if opened && record.ID.Position != lastPosition+1 {
		return fmt.Errorf("%w: expected aggregate position %d, got %d", recordings.ErrWorkerRecordingOrder, lastPosition+1, record.ID.Position)
	}
	if err := capture.writer.PersistWorkerRecord(capture.runCtx, recordings.WorkerRecordingRecord{
		RecordingID:     capture.request.RecordingID,
		WorkerSessionID: capture.request.WorkerSessionID,
		Record:          record.Detached(),
	}); err != nil {
		return fmt.Errorf("%w: position %d: %v", recordings.ErrWorkerRecordingPersistence, record.ID.Position, err)
	}

	capture.mu.Lock()
	capture.identities[identity] = record.Detached()
	capture.lastPosition = record.ID.Position
	if record.SourceType == workerLifecycleSourceType &&
		record.SourceSequence >= openingSourceSequence+1 &&
		record.SourceEventID == events.SourceEventID("terminal") {
		capture.terminal = true
		capture.stopOnce.Do(capture.stop)
	}
	if !capture.opened {
		capture.opened = true
		close(capture.opening)
	}
	capture.mu.Unlock()
	return nil
}

func (capture *capture) isTerminal() bool {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.terminal
}

func validateOpeningPayload(payload []byte, sessionID string) error {
	var envelope struct {
		Kind    string          `json:"kind"`
		Phase   string          `json:"phase"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("%w: opening payload is not valid Worker draft JSON: %v", recordings.ErrWorkerRecordingOpening, err)
	}
	if envelope.Kind != "SESSION" || envelope.Phase != "STARTED" {
		return fmt.Errorf("%w: got kind=%q phase=%q", recordings.ErrWorkerRecordingOpening, envelope.Kind, envelope.Phase)
	}
	var session struct {
		Status          string `json:"status"`
		WorkerSessionID string `json:"workerSessionId"`
	}
	if err := json.Unmarshal(envelope.Payload, &session); err != nil {
		return fmt.Errorf("%w: opening payload facts are malformed: %v", recordings.ErrWorkerRecordingOpening, err)
	}
	if session.Status != "STARTING" || session.WorkerSessionID != sessionID {
		return fmt.Errorf("%w: payload status=%q Worker Session ID=%q does not match STARTING/%q", recordings.ErrWorkerRecordingOpening, session.Status, session.WorkerSessionID, sessionID)
	}
	return nil
}

func sameRecord(left, right events.Record) bool {
	return left.ID == right.ID && left.SourceType == right.SourceType &&
		left.SourceID == right.SourceID && left.SourceSequence == right.SourceSequence &&
		left.SourceEventID == right.SourceEventID && left.SchemaID == right.SchemaID &&
		bytes.Equal(left.Payload, right.Payload)
}

func (capture *capture) fail(err error) {
	if err == nil {
		return
	}
	capture.mu.Lock()
	if capture.failed != nil {
		capture.mu.Unlock()
		return
	}
	capture.failed = err
	close(capture.failure)
	capture.stopOnce.Do(capture.stop)
	capture.mu.Unlock()
	capture.logger.Info("Worker recording capture failed", "workerSessionID", capture.request.WorkerSessionID, "topic", capture.request.Topic, "outcome", "failed", "error", err.Error())
}

func (capture *capture) AwaitOpening(ctx context.Context) error {
	select {
	case <-capture.opening:
		return nil
	case <-capture.failure:
		return capture.failureError()
	case <-capture.done:
		if err := capture.failureError(); err != nil {
			return err
		}
		return fmt.Errorf("%w: subscription ended before position 1", recordings.ErrWorkerRecordingOpening)
	case <-ctx.Done():
		capture.fail(fmt.Errorf("%w: opening wait canceled: %w", recordings.ErrWorkerRecordingCanceled, ctx.Err()))
		return capture.failureError()
	}
}

func (capture *capture) failureError() error {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.failed
}

func (capture *capture) Close(ctx context.Context) error {
	capture.mu.Lock()
	if !capture.closed {
		capture.closed = true
		if !capture.opened || capture.terminal || capture.failed != nil {
			capture.stopOnce.Do(capture.stop)
		}
	}
	capture.mu.Unlock()

	select {
	case <-capture.done:
		return capture.failureError()
	case <-ctx.Done():
		capture.stopOnce.Do(capture.stop)
		return ctx.Err()
	}
}
