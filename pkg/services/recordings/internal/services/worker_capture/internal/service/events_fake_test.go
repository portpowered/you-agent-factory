package service

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// recordingEventsService is a service-root fake. The capture tests exercise
// Recordings' subscription behavior without constructing the Events service's
// private implementation through events/wire.
type recordingEventsService struct {
	mu            sync.Mutex
	records       map[events.Topic][]events.Record
	notifications map[events.Topic][]chan struct{}
}

func newRecordingEventsService() events.Service {
	return &recordingEventsService{
		records:       make(map[events.Topic][]events.Record),
		notifications: make(map[events.Topic][]chan struct{}),
	}
}

func (service *recordingEventsService) Append(_ context.Context, request events.AppendRequest) (events.AppendResult, error) {
	if err := request.Validate(); err != nil {
		return events.AppendResult{}, err
	}

	service.mu.Lock()
	topicRecords := service.records[request.Topic]
	record := events.Record{
		ID:             events.RecordID{Topic: request.Topic, Position: events.AggregateSequence(len(topicRecords) + 1)},
		SourceType:     request.SourceType,
		SourceID:       request.SourceID,
		SourceSequence: request.SourceSequence,
		SourceEventID:  request.SourceEventID,
		SchemaID:       request.SchemaID,
		Payload:        request.Detached().Payload,
	}
	service.records[request.Topic] = append(topicRecords, record)
	notifications := append([]chan struct{}(nil), service.notifications[request.Topic]...)
	service.mu.Unlock()

	for _, notification := range notifications {
		select {
		case notification <- struct{}{}:
		default:
		}
	}
	return events.AppendResult{Record: record, Outcome: events.AppendOutcomeAccepted}, nil
}

func (service *recordingEventsService) AttachSource(_ context.Context, request events.AttachSourceRequest) (events.AttachSourceResult, error) {
	if err := request.Validate(); err != nil {
		return events.AttachSourceResult{}, err
	}
	return events.AttachSourceResult{
		ID:      events.AttachmentID{Destination: request.Destination, Source: request.Source},
		Outcome: events.AttachOutcomeAccepted,
		StartAt: request.StartAt,
	}, nil
}

func (service *recordingEventsService) Read(_ context.Context, request events.ReadRequest) (events.ReadResult, error) {
	if err := request.Validate(); err != nil {
		return events.ReadResult{}, err
	}

	service.mu.Lock()
	records := append([]events.Record(nil), service.records[request.Topic]...)
	service.mu.Unlock()
	start := int(request.From.Position)
	if start >= len(records) {
		return events.ReadResult{
			Next:     events.Cursor{Topic: request.Topic, Position: events.AggregateSequence(len(records))},
			Retained: events.RetainedRange{Topic: request.Topic, Earliest: firstPosition(records), Head: events.AggregateSequence(len(records))},
			Outcome:  events.ReadOutcomeAtHead,
		}, nil
	}
	end := start + request.Limit
	if end > len(records) {
		end = len(records)
	}
	selected := make([]events.Record, end-start)
	copy(selected, records[start:end])
	return events.ReadResult{
		Records:  selected,
		Next:     events.Cursor{Topic: request.Topic, Position: selected[len(selected)-1].ID.Position},
		Retained: events.RetainedRange{Topic: request.Topic, Earliest: firstPosition(records), Head: events.AggregateSequence(len(records))},
		Outcome:  events.ReadOutcomeProgress,
	}, nil
}

func (service *recordingEventsService) Subscribe(_ context.Context, request events.SubscribeRequest) (events.Subscription, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	notification := make(chan struct{}, 1)
	service.mu.Lock()
	service.notifications[request.Topic] = append(service.notifications[request.Topic], notification)
	service.mu.Unlock()
	nextPosition := request.From.Position + 1
	return events.Subscription(func(ctx context.Context) events.Delivery {
		for {
			service.mu.Lock()
			records := service.records[request.Topic]
			if nextPosition <= events.AggregateSequence(len(records)) {
				record := records[nextPosition-1].Detached()
				nextPosition++
				service.mu.Unlock()
				return events.Delivery{
					Kind:   events.DeliveryRecord,
					Record: record,
					Cursor: events.Cursor{Topic: request.Topic, Position: record.ID.Position},
				}
			}
			service.mu.Unlock()

			select {
			case <-ctx.Done():
				return events.Delivery{Kind: events.DeliveryCanceled}
			case <-notification:
			}
		}
	}), nil
}

func firstPosition(records []events.Record) events.AggregateSequence {
	if len(records) == 0 {
		return 0
	}
	return records[0].ID.Position
}

var _ events.Service = (*recordingEventsService)(nil)
