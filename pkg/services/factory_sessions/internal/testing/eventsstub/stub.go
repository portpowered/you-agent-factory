// Package eventsstub provides a hand-rolled events.Service test double for
// Factory Sessions unit tests. It exists so those tests can exercise a
// working Events root without constructing the owning service's real
// implementation across a package boundary: pkg/services/events/wire is
// reserved for pkg/wire and the Events service itself, and Factory Sessions'
// own production code only ever calls events.Service.Append and
// events.Service.Read, never Subscribe or AttachSource.
package eventsstub

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// Service is a minimal events.Service double built only against the
// published pkg/services/events contract. Append accepts records in commit
// order per topic, and Read serves back a bounded, optionally
// retention-limited slice using the same boundary semantics (at-head,
// invalid-cursor, gap) as the real implementation. Subscribe and
// AttachSource are never called by Factory Sessions production code and are
// left unimplemented.
type Service struct {
	mu          sync.Mutex
	maxRetained int // 0 means unbounded retention
	topics      map[events.Topic]*topicState
}

type topicState struct {
	records  []events.Record // retained window only, oldest first
	head     events.AggregateSequence
	earliest events.AggregateSequence
}

// New constructs a Service with unbounded retention.
func New() *Service {
	return &Service{topics: make(map[events.Topic]*topicState)}
}

// NewWithRetention constructs a Service that evicts the oldest retained
// record from a topic once more than maxRetained records have been
// committed to it, mirroring the real Events implementation's bounded
// per-topic retention contract closely enough to exercise
// partial-eviction/gap behavior in tests.
func NewWithRetention(maxRetained int) *Service {
	return &Service{topics: make(map[events.Topic]*topicState), maxRetained: maxRetained}
}

func (s *Service) topic(topic events.Topic) *topicState {
	ts, ok := s.topics[topic]
	if !ok {
		ts = &topicState{}
		s.topics[topic] = ts
	}
	return ts
}

// Append implements events.Service.
func (s *Service) Append(_ context.Context, req events.AppendRequest) (events.AppendResult, error) {
	if err := req.Validate(); err != nil {
		return events.AppendResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := s.topic(req.Topic)
	position := ts.head + 1
	record := events.Record{
		ID:             events.RecordID{Topic: req.Topic, Position: position},
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
		SchemaID:       req.SchemaID,
		Payload:        append(json.RawMessage(nil), req.Payload...),
	}
	ts.records = append(ts.records, record)
	ts.head = position
	if ts.earliest == 0 {
		ts.earliest = 1
	}
	if s.maxRetained > 0 && len(ts.records) > s.maxRetained {
		ts.records = ts.records[1:]
		ts.earliest++
	}
	return events.AppendResult{Record: record.Detached(), Outcome: events.AppendOutcomeAccepted}, nil
}

// Read implements events.Service. Its outcome resolution mirrors
// pkg/services/events/internal/service.Store.Read's boundary contract: a
// From naming exactly EarliestRetained-1 is a valid, non-gap cursor whose
// first returned record is EarliestRetained itself.
func (s *Service) Read(_ context.Context, req events.ReadRequest) (events.ReadResult, error) {
	if err := req.Validate(); err != nil {
		return events.ReadResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := s.topic(req.Topic)
	topic := req.Topic
	from := req.From.Position
	head := ts.head
	earliest := ts.earliest
	if earliest == 0 {
		earliest = 1
	}

	switch {
	case from == head:
		return events.ReadResult{
			Outcome:  events.ReadOutcomeAtHead,
			Next:     events.Cursor{Topic: topic, Position: head},
			Retained: events.RetainedRange{Topic: topic, Earliest: earliest, Head: head},
		}, nil
	case from > head:
		return events.ReadResult{Outcome: events.ReadOutcomeInvalidCursor}, nil
	case earliest > 1 && from+1 < earliest:
		return events.ReadResult{
			Outcome: events.ReadOutcomeGap,
			Gap:     &events.GapFacts{Topic: topic, Requested: from, EarliestRetained: earliest, Head: head},
		}, nil
	}

	startIndex := int(from + 1 - earliest)
	available := ts.records[startIndex:]
	if req.Limit > 0 && len(available) > req.Limit {
		available = available[:req.Limit]
	}
	records := make([]events.Record, len(available))
	for i, rec := range available {
		records[i] = rec.Detached()
	}
	last := records[len(records)-1]
	return events.ReadResult{
		Records:  records,
		Next:     events.Cursor{Topic: topic, Position: last.ID.Position},
		Retained: events.RetainedRange{Topic: topic, Earliest: earliest, Head: head},
		Outcome:  events.ReadOutcomeProgress,
	}, nil
}

// Subscribe is not called by any Factory Sessions production path and is
// left unimplemented.
func (s *Service) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return nil, errors.New("eventsstub: Subscribe is not implemented")
}

// AttachSource is not called by any Factory Sessions production path and is
// left unimplemented.
func (s *Service) AttachSource(context.Context, events.AttachSourceRequest) (events.AttachSourceResult, error) {
	return events.AttachSourceResult{}, errors.New("eventsstub: AttachSource is not implemented")
}

var _ events.Service = (*Service)(nil)
