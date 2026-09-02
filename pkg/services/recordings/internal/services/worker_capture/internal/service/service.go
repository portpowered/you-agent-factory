package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const defaultSubscriptionLimit = 128

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
var _ recordings.WorkerRecordingReader = (*Service)(nil)
var _ recordings.WorkerRecordingWriter = (*Service)(nil)
var _ recordings.WorkerRecordingFailureWriter = (*Service)(nil)
var _ recordings.WorkerSessionRecordingFinalizer = (*capture)(nil)

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

// LoadWorkerRecording exposes the durable reader already owned by the
// composed Worker capture service. The capture service remains the lifecycle
// owner; this optional capability lets a Factory Session observation project
// Recordings health without constructing a second storage path.
func (service *Service) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	if service == nil || service.writer == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reader, ok := service.writer.(recordings.WorkerRecordingReader)
	if !ok || reader == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.LoadWorkerRecording(ctx, recordingID)
}

// PersistWorkerRecord forwards a source-native record to the same durable
// writer used by live capture. Worker Sessions uses this optional capability
// for a continuation link that is appended after the source's execution
// terminal has already closed its live capture handle.
func (service *Service) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if service == nil || service.writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	return service.writer.PersistWorkerRecord(ctx, record)
}

// PersistWorkerRecordingFailure forwards a safe loss marker when a
// post-terminal continuation record cannot be durably accepted.
func (service *Service) PersistWorkerRecordingFailure(ctx context.Context, failure recordings.WorkerRecordingFailure) error {
	if service == nil || service.writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	writer, ok := service.writer.(recordings.WorkerRecordingFailureWriter)
	if !ok || writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	return writer.PersistWorkerRecordingFailure(ctx, failure)
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
		events:       service.events,
		writer:       service.writer,
		subscription: subscription,
		logger:       service.logger,
		runCtx:       runCtx,
		stop:         stop,
		opening:      make(chan struct{}),
		failure:      make(chan struct{}),
		done:         make(chan struct{}),
		identities:   make(map[events.AppendIdentity]events.Record),
		projection: recordings.WorkerRecordingProjection{
			RecordingID:     request.RecordingID,
			WorkerSessionID: request.WorkerSessionID,
			Topic:           request.Topic,
			Status:          recordings.WorkerRecordingStatusIncomplete,
			Records:         []events.Record{},
		},
	}
	go capture.consume()
	return capture, nil
}

type capture struct {
	request      recordings.WorkerSessionRecordingRequest
	events       events.Service
	writer       recordings.WorkerRecordingWriter
	subscription events.Subscription
	logger       logging.Logger
	runCtx       context.Context
	stop         context.CancelFunc
	stopOnce     sync.Once

	opening chan struct{}
	failure chan struct{}
	done    chan struct{}

	mu                     sync.Mutex
	failureMarkerMu        sync.Mutex
	opened                 bool
	terminal               bool
	closed                 bool
	closing                bool
	failed                 error
	lastPosition           events.AggregateSequence
	identities             map[events.AppendIdentity]events.Record
	history                []events.Record
	projection             recordings.WorkerRecordingProjection
	executionTerminal      *recordings.WorkerRecordingTerminal
	failureMarkerAttempted bool
	failureMarkerCode      string
	failureMarkerTerminal  *recordings.WorkerRecordingTerminal
}

func (capture *capture) consume() {
	defer close(capture.done)
	for {
		if capture.isClosing() {
			pending, err := capture.hasPendingRecords(capture.runCtx)
			if err != nil {
				capture.fail(fmt.Errorf("%w: inspect closing Worker topic: %v", recordings.ErrWorkerRecordingDelivery, err))
				return
			}
			if !pending {
				if capture.isTerminal() {
					capture.stopOnce.Do(capture.stop)
					return
				}
				capture.fail(fmt.Errorf("%w: source closed before a durable terminal record", recordings.ErrWorkerRecordingIncomplete))
				return
			}
		}
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
	if capture.terminal {
		capture.mu.Unlock()
		return fmt.Errorf("%w: record position %d follows the durable terminal", recordings.ErrWorkerRecordingTerminal, record.ID.Position)
	}
	lastPosition := capture.lastPosition
	opened := capture.opened
	history := cloneWorkerRecords(capture.history)
	capture.mu.Unlock()

	if opened && record.ID.Position != lastPosition+1 {
		return fmt.Errorf("%w: expected aggregate position %d, got %d", recordings.ErrWorkerRecordingOrder, lastPosition+1, record.ID.Position)
	}
	history = append(history, record.Detached())
	projection, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:     capture.request.RecordingID,
		WorkerSessionID: capture.request.WorkerSessionID,
		Topic:           capture.request.Topic,
		Records:         history,
	})
	if err != nil {
		return err
	}
	if err := capture.writer.PersistWorkerRecord(capture.runCtx, recordings.WorkerRecordingRecord{
		RecordingID:      capture.request.RecordingID,
		FactorySessionID: capture.request.FactorySessionID,
		WorkerSessionID:  capture.request.WorkerSessionID,
		Record:           record.Detached(),
	}); err != nil {
		return fmt.Errorf("%w: position %d: %v", recordings.ErrWorkerRecordingPersistence, record.ID.Position, err)
	}

	capture.mu.Lock()
	capture.identities[identity] = record.Detached()
	capture.lastPosition = record.ID.Position
	capture.history = history
	capture.projection = projection
	capture.terminal = projection.Complete
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
	code := workerRecordingFailureCode(err)
	if projection, projectionErr := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:       capture.request.RecordingID,
		WorkerSessionID:   capture.request.WorkerSessionID,
		Topic:             capture.request.Topic,
		Failure:           code,
		ExecutionTerminal: capture.executionTerminal,
		Records:           capture.history,
	}); projectionErr == nil {
		capture.projection = projection
	}
	capture.mu.Unlock()
	capture.persistFailureMarker()
	capture.logger.Info("Worker recording capture failed", "workerSessionID", capture.request.WorkerSessionID, "topic", capture.request.Topic, "outcome", "failed", "code", code)
}

// persistFailureMarker writes the safe capture-loss fact at most once for a
// given failure code and authoritative terminal. A later terminal fact is a
// deliberate second write: it upgrades an already-readable prefix from
// INCOMPLETE to DEGRADED without fabricating a missing terminal record.
func (capture *capture) persistFailureMarker() {
	failureWriter, ok := capture.writer.(recordings.WorkerRecordingFailureWriter)
	if !ok {
		return
	}
	capture.failureMarkerMu.Lock()
	defer capture.failureMarkerMu.Unlock()

	capture.mu.Lock()
	if capture.failed == nil {
		capture.mu.Unlock()
		return
	}
	code := workerRecordingFailureCode(capture.failed)
	terminal := cloneWorkerRecordingTerminal(capture.executionTerminal)
	if capture.failureMarkerAttempted && capture.failureMarkerCode == code && sameWorkerRecordingTerminal(capture.failureMarkerTerminal, terminal) {
		capture.mu.Unlock()
		return
	}
	capture.failureMarkerAttempted = true
	capture.failureMarkerCode = code
	capture.failureMarkerTerminal = cloneWorkerRecordingTerminal(terminal)
	failure := recordings.WorkerRecordingFailure{
		RecordingID:       capture.request.RecordingID,
		WorkerSessionID:   capture.request.WorkerSessionID,
		Topic:             capture.request.Topic,
		Code:              code,
		ExecutionTerminal: terminal,
	}
	capture.mu.Unlock()

	if err := failureWriter.PersistWorkerRecordingFailure(context.Background(), failure); err != nil {
		fields := []any{
			"workerSessionID", capture.request.WorkerSessionID,
			"topic", capture.request.Topic,
			"outcome", "failed",
			"stage", "degradation_marker",
			"code", code,
		}
		if terminal != nil {
			fields = append(fields, "executionOutcome", terminal.Status)
		}
		capture.logger.Info("Worker recording failure persistence failed", fields...)
	}
}

func sameWorkerRecordingTerminal(left, right *recordings.WorkerRecordingTerminal) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Phase == right.Phase && left.Status == right.Status &&
		(left.Position == 0 || right.Position == 0 || left.Position == right.Position)
}

func workerRecordingFailureCode(err error) string {
	switch {
	case errors.Is(err, recordings.ErrWorkerRecordingOpening):
		return "OPENING_INVALID"
	case errors.Is(err, recordings.ErrWorkerRecordingPersistence):
		return "PERSISTENCE_FAILED"
	case errors.Is(err, recordings.ErrWorkerRecordingGap):
		return "RETENTION_GAP"
	case errors.Is(err, recordings.ErrWorkerRecordingClosed):
		return "SOURCE_CLOSED"
	case errors.Is(err, recordings.ErrWorkerRecordingCanceled):
		return "CANCELED"
	case errors.Is(err, recordings.ErrWorkerRecordingBackpressure):
		return "BACKPRESSURE"
	case errors.Is(err, recordings.ErrWorkerRecordingTerminal):
		return "TERMINAL_INVALID"
	case errors.Is(err, recordings.ErrWorkerRecordingIncomplete):
		return "INCOMPLETE"
	case errors.Is(err, recordings.ErrWorkerRecordingDuplicate):
		return "DUPLICATE_CONFLICT"
	case errors.Is(err, recordings.ErrWorkerRecordingOrder):
		return "ORDER_INVALID"
	default:
		return "DELIVERY_FAILED"
	}
}

func (capture *capture) isClosing() bool {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.closing
}

func (capture *capture) hasPendingRecords(ctx context.Context) (bool, error) {
	capture.mu.Lock()
	position := capture.lastPosition
	capture.mu.Unlock()
	result, err := capture.events.Read(ctx, events.ReadRequest{
		Topic: capture.request.Topic,
		From:  events.Cursor{Topic: capture.request.Topic, Position: position},
		Limit: defaultSubscriptionLimit,
	})
	if err != nil {
		return false, err
	}
	if err := result.Validate(); err != nil {
		return false, err
	}
	return result.Outcome == events.ReadOutcomeProgress && len(result.Records) > 0, nil
}

func cloneWorkerRecords(records []events.Record) []events.Record {
	clone := make([]events.Record, len(records))
	for i, record := range records {
		clone[i] = record.Detached()
	}
	return clone
}

func cloneWorkerProjection(projection recordings.WorkerRecordingProjection) recordings.WorkerRecordingProjection {
	clone := projection
	clone.Opening = projection.Opening.Detached()
	clone.Records = cloneWorkerRecords(projection.Records)
	if projection.Terminal != nil {
		terminal := *projection.Terminal
		clone.Terminal = &terminal
	}
	if projection.ExecutionTerminal != nil {
		terminal := *projection.ExecutionTerminal
		clone.ExecutionTerminal = &terminal
	}
	return clone
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

// Abort records a classified pre-barrier failure, stops the subscription,
// and joins the consumer. The cause is deliberately supplied by the caller:
// an Events append can fail outside the capture's source stream, but it still
// must leave a durable failure rather than an active, unclassified capture.
func (capture *capture) Abort(ctx context.Context, cause error) error {
	if cause == nil {
		cause = fmt.Errorf("%w: capture aborted before opening", recordings.ErrWorkerRecordingOpening)
	}
	capture.fail(cause)
	capture.mu.Lock()
	capture.closed = true
	capture.closing = true
	capture.stopOnce.Do(capture.stop)
	capture.mu.Unlock()

	select {
	case <-capture.done:
		return capture.failureError()
	case <-ctx.Done():
		capture.stopOnce.Do(capture.stop)
		return fmt.Errorf("%w: abort wait canceled: %w", recordings.ErrWorkerRecordingCanceled, ctx.Err())
	}
}

func (capture *capture) failureError() error {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.failed
}

func (capture *capture) setExecutionTerminal(terminal recordings.WorkerRecordingTerminal) {
	capture.mu.Lock()
	capture.executionTerminal = cloneWorkerRecordingTerminal(&terminal)
	failure := ""
	if capture.failed != nil {
		failure = workerRecordingFailureCode(capture.failed)
	}
	if projection, err := (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:       capture.request.RecordingID,
		WorkerSessionID:   capture.request.WorkerSessionID,
		Topic:             capture.request.Topic,
		Failure:           failure,
		ExecutionTerminal: capture.executionTerminal,
		Records:           capture.history,
	}); err == nil {
		capture.projection = projection
	}
	capture.mu.Unlock()
}

// WorkerRecordingProjection returns a detached live reduction. It is an
// observation seam for Recordings tests and replay callers; provider handoff
// still depends only on AwaitOpening.
func (capture *capture) WorkerRecordingProjection() (recordings.WorkerRecordingProjection, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.failed != nil {
		return cloneWorkerProjection(capture.projection), nil
	}
	return cloneWorkerProjection(capture.projection), nil
}

func (capture *capture) Close(ctx context.Context) error {
	return capture.close(ctx, nil)
}

// CloseWithTerminal is the terminal-aware finalization path used by Worker
// Sessions after its authoritative execution outcome commits. The value is a
// detached safe fact, not provider output, and may have position zero when the
// terminal Events append itself failed.
func (capture *capture) CloseWithTerminal(ctx context.Context, terminal recordings.WorkerRecordingTerminal) error {
	return capture.close(ctx, &terminal)
}

func (capture *capture) close(ctx context.Context, terminal *recordings.WorkerRecordingTerminal) error {
	if terminal != nil {
		capture.setExecutionTerminal(*terminal)
	}
	capture.markClosing(ctx)
	capture.classifyClosingState(ctx)
	return capture.awaitClose(ctx)
}

func (capture *capture) markClosing(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		// Classify an already-canceled close before marking the capture as
		// closing; otherwise the consumer can win the race and report an
		// incomplete source instead of the caller's cancellation.
		capture.fail(fmt.Errorf("%w: close wait canceled: %w", recordings.ErrWorkerRecordingCanceled, err))
	}
	capture.mu.Lock()
	if !capture.closed {
		capture.closed = true
		capture.closing = true
		if capture.terminal || capture.failed != nil {
			capture.stopOnce.Do(capture.stop)
		}
	}
	capture.mu.Unlock()
}

func (capture *capture) classifyClosingState(ctx context.Context) {
	if capture.isTerminal() || capture.failureError() != nil {
		return
	}
	if ctx.Err() != nil {
		capture.fail(fmt.Errorf("%w: close wait canceled: %w", recordings.ErrWorkerRecordingCanceled, ctx.Err()))
		return
	}
	pending, err := capture.hasPendingRecords(ctx)
	if err != nil {
		capture.classifyPendingRecordsError(ctx, err)
		return
	}
	if !pending {
		capture.fail(fmt.Errorf("%w: source closed before a durable terminal record", recordings.ErrWorkerRecordingIncomplete))
	}
}

func (capture *capture) classifyPendingRecordsError(ctx context.Context, err error) {
	if ctx.Err() != nil {
		capture.fail(fmt.Errorf("%w: close wait canceled: %w", recordings.ErrWorkerRecordingCanceled, ctx.Err()))
		return
	}
	capture.fail(fmt.Errorf("%w: inspect closing Worker topic: %v", recordings.ErrWorkerRecordingDelivery, err))
}

func (capture *capture) awaitClose(ctx context.Context) error {
	capture.persistFailureMarker()
	select {
	case <-capture.done:
		capture.persistFailureMarker()
		return capture.failureError()
	case <-ctx.Done():
		capture.fail(fmt.Errorf("%w: close wait canceled: %w", recordings.ErrWorkerRecordingCanceled, ctx.Err()))
		capture.stopOnce.Do(capture.stop)
		capture.persistFailureMarker()
		return capture.failureError()
	}
}
