package responsebridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	workerEventSourceType events.SourceType = "worker_session_event"
	workerReadBatchLimit                    = 64
)

var (
	// ErrMalformedWorkerAssociation reports a Factory association record that
	// cannot establish the canonical ownership relationship needed to consume a
	// Worker Session topic.
	ErrMalformedWorkerAssociation = errors.New("chat sessions: malformed worker session association")
	// ErrConflictingWorkerAssociation reports attempts to give one dispatch or
	// Worker Session more than one canonical counterpart.
	ErrConflictingWorkerAssociation = errors.New("chat sessions: conflicting worker session association")
	// ErrWorkerChildOpeningRequired reports a Worker lifecycle update that has
	// no already-sequenced parent tool-call item.
	ErrWorkerChildOpeningRequired = errors.New("chat sessions: worker child opening is required")
	// ErrDuplicateWorkerChildOpening reports a second, distinct opening record
	// for one associated Worker Session.
	ErrDuplicateWorkerChildOpening = errors.New("chat sessions: duplicate worker child opening")
	// ErrWorkerChildAfterTerminal reports lifecycle activity after an accepted
	// terminal record; it is not sequenced as a fabricated recovery state.
	ErrWorkerChildAfterTerminal = errors.New("chat sessions: worker child record after terminal")
	// ErrWorkerChildHistoryGap reports a Worker stream retention gap. The
	// explicit ACP-visible gap projection belongs to the later bounding slice;
	// this lifecycle bridge fails closed rather than pretending its parent was
	// observed.
	ErrWorkerChildHistoryGap = errors.New("chat sessions: worker child history gap")
	// ErrMalformedWorkerChildRecord reports a Worker topic payload that cannot
	// be decoded as the published workers.Draft vocabulary. It is local to one
	// child: ingestion logs and skips it so a malformed sibling cannot stop
	// already-associated Worker Sessions from continuing to sequence.
	ErrMalformedWorkerChildRecord = errors.New("chat sessions: malformed worker child record")
)

// workerChildren owns only the child-source ingestion lifecycle for one
// response bridge Run. It never allocates ACP identities: the Chat Session
// sequencer returns the opening item ID, which becomes the stored parent for
// every later Worker lifecycle record.
type workerChildren struct {
	service          *Service
	liveCtx          context.Context
	deliveryCtx      context.Context
	chatSessionID    string
	factorySessionID string
	state            *drainState

	factoryDone sync.WaitGroup
	workerDone  sync.WaitGroup
	errMu       sync.Mutex
	err         error
}

// workerChild is state owned under drainState.mu. Its openingItemID is a
// sequencer-assigned Chat aggregate identity, never a Worker or transport
// identifier, and is therefore stable across retained and live consumption.
type workerChild struct {
	association    chatsessions.WorkerSessionAssociation
	openingItemID  string
	openingSource  events.AppendIdentity
	terminalSource events.AppendIdentity
	terminal       bool
	// sequencedSources records only source identities whose Chat Session
	// sequence and stream-head advancement both succeeded in this Run. The
	// retained terminal sweep restarts each Worker cursor at zero, so this
	// per-run evidence prevents its prefix from being reordered through the
	// lifecycle guards after the same records were already accepted live.
	sequencedSources map[events.AppendIdentity]struct{}
}

// startWorkerChildren attaches to canonical Factory Events before invocation
// starts, processes its retained prefix, then watches its live continuation.
// The association is the membership edge: the direct child route can publish
// its Worker opening before that edge is recorded, so the per-child
// subscription always starts at the topic cursor and replays the retained
// prefix. A nil Events collaborator intentionally leaves the existing generic
// response bridge unchanged for narrow, pre-W5 constructions.
func (s *Service) startWorkerChildren(
	liveCtx, deliveryCtx context.Context,
	chatSessionID, factorySessionID string,
	state *drainState,
) (*workerChildren, error) {
	children := &workerChildren{
		service: s, liveCtx: liveCtx, deliveryCtx: deliveryCtx, chatSessionID: chatSessionID,
		factorySessionID: factorySessionID, state: state,
	}
	if s.workerEvents == nil {
		return children, nil
	}

	stream, err := s.factoryTarget.SubscribeFactoryEventsForSession(liveCtx, factorySessionID, nil)
	if err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, errors.New("factory event stream is unavailable")
	}
	if err := children.presentFactoryEvents(stream.History, true); err != nil {
		return nil, err
	}
	if stream.Events == nil {
		return children, nil
	}

	children.factoryDone.Add(1)
	go func() {
		defer children.factoryDone.Done()
		for {
			select {
			case <-liveCtx.Done():
				return
			case event, open := <-stream.Events:
				if !open {
					return
				}
				if err := children.presentFactoryEvents([]factorydefinitions.FactoryEvent{event}, true); err != nil {
					children.setError(err)
					return
				}
			}
		}
	}()
	return children, nil
}

// finish joins all live consumers after Run has cancelled liveCtx, then reads
// the final retained Factory association prefix and each associated Worker
// topic. Replaying a retained Worker record is safe: Chat Sessions resolves
// its source identity to the original ItemID and does not emit another item.
func (c *workerChildren) finish(ctx context.Context) error {
	if c == nil || c.service.workerEvents == nil {
		return nil
	}
	c.factoryDone.Wait()
	c.workerDone.Wait()

	tailCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := c.service.factoryTarget.SubscribeFactoryEventsForSession(tailCtx, c.factorySessionID, nil)
	if err != nil {
		return err
	}
	if stream == nil {
		return errors.New("factory event tail stream is unavailable")
	}
	if err := c.presentFactoryEvents(stream.History, false); err != nil {
		return err
	}
	for _, child := range c.children() {
		if err := c.service.drainWorkerLifecycleTail(ctx, c.chatSessionID, c.state, child.association); err != nil {
			return err
		}
	}
	return c.error()
}

func (c *workerChildren) presentFactoryEvents(eventsToPresent []factorydefinitions.FactoryEvent, startLive bool) error {
	for _, event := range eventsToPresent {
		association, associated, err := workerAssociationFromFactoryEvent(event)
		if err != nil {
			return err
		}
		if !associated {
			continue
		}
		child, added, err := c.registerAssociation(association)
		if err != nil || !added || !startLive {
			if err != nil {
				return err
			}
			continue
		}
		c.workerDone.Add(1)
		go func() {
			defer c.workerDone.Done()
			if err := c.service.drainWorkerLifecycleLive(c.liveCtx, c.deliveryCtx, c.chatSessionID, c.state, child.association); err != nil {
				c.setError(err)
			}
		}()
	}
	return nil
}

func workerAssociationFromFactoryEvent(event factorydefinitions.FactoryEvent) (chatsessions.WorkerSessionAssociation, bool, error) {
	if event.Type != factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc {
		return chatsessions.WorkerSessionAssociation{}, false, nil
	}
	if event.Context.DispatchID == nil {
		return chatsessions.WorkerSessionAssociation{}, false, fmt.Errorf("%w: dispatch id is required", ErrMalformedWorkerAssociation)
	}
	var payload factorydefinitions.DispatchWorkerSessionAssociationEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return chatsessions.WorkerSessionAssociation{}, false, fmt.Errorf("%w: %v", ErrMalformedWorkerAssociation, err)
	}
	association := chatsessions.WorkerSessionAssociation{
		DispatchID: *event.Context.DispatchID, WorkerSessionID: payload.WorkerSessionID,
	}
	if err := association.Validate(); err != nil {
		return chatsessions.WorkerSessionAssociation{}, false, fmt.Errorf("%w: %v", ErrMalformedWorkerAssociation, err)
	}
	return association, true, nil
}

func (c *workerChildren) registerAssociation(association chatsessions.WorkerSessionAssociation) (*workerChild, bool, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if existing := c.state.childrenByWorkerSessionID[association.WorkerSessionID]; existing != nil {
		if existing.association == association {
			return existing, false, nil
		}
		return nil, false, ErrConflictingWorkerAssociation
	}
	if workerID := c.state.workerSessionIDByDispatch[association.DispatchID]; workerID != "" && workerID != association.WorkerSessionID {
		return nil, false, ErrConflictingWorkerAssociation
	}
	child := &workerChild{association: association, sequencedSources: make(map[events.AppendIdentity]struct{})}
	c.state.childrenByWorkerSessionID[association.WorkerSessionID] = child
	c.state.workerSessionIDByDispatch[association.DispatchID] = association.WorkerSessionID
	return child, true, nil
}

func (c *workerChildren) children() []workerChild {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	children := make([]workerChild, 0, len(c.state.childrenByWorkerSessionID))
	for _, child := range c.state.childrenByWorkerSessionID {
		children = append(children, *child)
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].association.WorkerSessionID < children[j].association.WorkerSessionID
	})
	return children
}

func (c *workerChildren) setError(err error) {
	if err == nil {
		return
	}
	c.errMu.Lock()
	defer c.errMu.Unlock()
	if c.err == nil {
		c.err = err
	}
}

func (c *workerChildren) error() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.err
}

func (s *Service) drainWorkerLifecycleLive(
	liveCtx, deliveryCtx context.Context,
	chatSessionID string,
	state *drainState,
	association chatsessions.WorkerSessionAssociation,
) error {
	topic := workersessions.Topic(association.WorkerSessionID)
	subscription, err := s.workerEvents.Subscribe(liveCtx, events.SubscribeRequest{
		Topic: topic, From: events.Cursor{Topic: topic}, Limit: workerReadBatchLimit,
	})
	if err != nil {
		return err
	}
	for {
		delivery := subscription.Next(liveCtx)
		switch delivery.Kind {
		case events.DeliveryRecord:
			if err := s.sequenceWorkerLifecycleRecord(deliveryCtx, chatSessionID, state, association, delivery.Record); err != nil {
				if isIsolatedWorkerChildRecordError(err) {
					s.logWorkerChildRecordSkipped(association, delivery.Record, err)
					continue
				}
				return err
			}
		case events.DeliveryGap:
			return ErrWorkerChildHistoryGap
		default:
			return nil
		}
	}
}

func (s *Service) drainWorkerLifecycleTail(
	ctx context.Context,
	chatSessionID string,
	state *drainState,
	association chatsessions.WorkerSessionAssociation,
) error {
	topic := workersessions.Topic(association.WorkerSessionID)
	cursor := events.Cursor{Topic: topic}
	for {
		read, err := s.workerEvents.Read(ctx, events.ReadRequest{Topic: topic, From: cursor, Limit: workerReadBatchLimit})
		if err != nil {
			return err
		}
		switch read.Outcome {
		case events.ReadOutcomeAtHead:
			return nil
		case events.ReadOutcomeGap, events.ReadOutcomeInvalidCursor:
			return ErrWorkerChildHistoryGap
		case events.ReadOutcomeProgress:
			for _, record := range read.Records {
				if err := s.sequenceWorkerLifecycleRecord(ctx, chatSessionID, state, association, record); err != nil {
					if isIsolatedWorkerChildRecordError(err) {
						s.logWorkerChildRecordSkipped(association, record, err)
						continue
					}
					return err
				}
			}
			cursor = read.Next
		default:
			return fmt.Errorf("worker child read returned unexpected outcome %d", read.Outcome)
		}
	}
}

func (s *Service) sequenceWorkerLifecycleRecord(
	ctx context.Context,
	chatSessionID string,
	state *drainState,
	association chatsessions.WorkerSessionAssociation,
	record events.Record,
) error {
	var draft workers.Draft
	if err := json.Unmarshal(record.Payload, &draft); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedWorkerChildRecord, err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	child := state.childrenByWorkerSessionID[association.WorkerSessionID]
	if child == nil || child.association != association {
		return ErrMalformedWorkerAssociation
	}
	identity := record.Identity()
	if _, alreadySequenced := child.sequencedSources[identity]; alreadySequenced {
		return nil
	}
	parentItemID, err := child.parentForRecord(draft, identity)
	if err != nil {
		return err
	}
	result, err := s.sequencer.Sequence(ctx, chatsessions.SequenceRequest{
		SessionID: chatSessionID, SourceType: workerEventSourceType,
		SourceID:       events.SourceID(association.WorkerSessionID),
		SourceSequence: record.SourceSequence, SourceEventID: record.SourceEventID,
		SchemaID: record.SchemaID, Kind: draft.Kind, Phase: draft.Phase,
		ParentItemID: parentItemID, WorkerSessionAssociation: &child.association,
		Payload: draft.Payload,
	})
	if err != nil {
		return err
	}
	advance, err := s.sequencer.AdvanceStreamHead(ctx, chatsessions.AdvanceStreamHeadRequest{
		SessionID: chatSessionID, ExpectedVersion: state.currentVersion,
		AggregateSequence: result.AggregateSequence, SourceType: workerEventSourceType,
		SourceID: events.SourceID(association.WorkerSessionID), SourceSequence: record.SourceSequence,
		SourceEventID: record.SourceEventID,
	})
	if err != nil {
		return err
	}
	if isWorkerChildOpening(draft) {
		child.openingItemID = result.ItemID
		child.openingSource = identity
	}
	if isWorkerChildTerminal(draft) {
		child.terminal = true
		child.terminalSource = identity
	}
	child.sequencedSources[identity] = struct{}{}
	state.currentVersion = advance.Session.Version
	return nil
}

func (child *workerChild) parentForRecord(draft workers.Draft, identity events.AppendIdentity) (string, error) {
	if isWorkerChildOpening(draft) {
		if child.openingItemID != "" && child.openingSource != identity {
			return "", ErrDuplicateWorkerChildOpening
		}
		return "", nil
	}
	if child.openingItemID == "" {
		return "", ErrWorkerChildOpeningRequired
	}
	if child.terminal && child.terminalSource != identity {
		return "", ErrWorkerChildAfterTerminal
	}
	return child.openingItemID, nil
}

func isWorkerChildOpening(draft workers.Draft) bool {
	return draft.Kind == workers.KindSession && draft.Phase == workers.PhaseStarted
}

func isWorkerChildTerminal(draft workers.Draft) bool {
	return draft.Kind == workers.KindSession &&
		(draft.Phase == workers.PhaseCompleted || draft.Phase == workers.PhaseFailed || draft.Phase == workers.PhaseCanceled)
}

func isIsolatedWorkerChildRecordError(err error) bool {
	return errors.Is(err, ErrMalformedWorkerChildRecord) ||
		errors.Is(err, ErrWorkerChildOpeningRequired) ||
		errors.Is(err, ErrDuplicateWorkerChildOpening) ||
		errors.Is(err, ErrWorkerChildAfterTerminal)
}

// logWorkerChildRecordSkipped makes a malformed record observable without
// exposing worker payloads or letting one child end the entire Factory turn.
func (s *Service) logWorkerChildRecordSkipped(
	association chatsessions.WorkerSessionAssociation,
	record events.Record,
	err error,
) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Warn(
		"chat sessions skipped malformed worker child record",
		"op", responseBridgeOperation,
		"dispatch_id", association.DispatchID,
		"worker_session_id", association.WorkerSessionID,
		"source_sequence", uint64(record.SourceSequence),
		"error_class", workerChildRecordErrorClass(err),
	)
}

func workerChildRecordErrorClass(err error) string {
	switch {
	case errors.Is(err, ErrMalformedWorkerChildRecord):
		return "malformed_record"
	case errors.Is(err, ErrWorkerChildOpeningRequired):
		return "opening_required"
	case errors.Is(err, ErrDuplicateWorkerChildOpening):
		return "duplicate_opening"
	case errors.Is(err, ErrWorkerChildAfterTerminal):
		return "after_terminal"
	default:
		return "unknown"
	}
}
