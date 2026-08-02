package events_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// fakeService proves that events.Service can be implemented entirely from
// outside the package, using only exported detached values. It holds no
// private events state and imports no internal package.
type fakeService struct {
	records []events.Record
}

func (f *fakeService) Append(_ context.Context, req events.AppendRequest) (events.AppendResult, error) {
	if err := req.Validate(); err != nil {
		return events.AppendResult{}, err
	}
	rec := events.Record{
		ID:             events.RecordID{Topic: req.Topic, Position: events.AggregateSequence(len(f.records) + 1)},
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
		SchemaID:       req.SchemaID,
		Payload:        req.Detached().Payload,
	}
	f.records = append(f.records, rec)
	return events.AppendResult{Record: rec, Outcome: events.AppendOutcomeAccepted}, nil
}

func (f *fakeService) AttachSource(_ context.Context, req events.AttachSourceRequest) (events.AttachSourceResult, error) {
	if err := req.Validate(); err != nil {
		return events.AttachSourceResult{}, err
	}
	id := events.AttachmentID{Destination: req.Destination, Source: req.Source}
	return events.AttachSourceResult{ID: id, Outcome: events.AttachOutcomeAccepted, StartAt: req.StartAt}, nil
}

func (f *fakeService) Read(_ context.Context, req events.ReadRequest) (events.ReadResult, error) {
	if err := req.Validate(); err != nil {
		return events.ReadResult{}, err
	}
	if len(f.records) == 0 {
		return events.ReadResult{Outcome: events.ReadOutcomeAtHead, Next: req.From}, nil
	}
	return events.ReadResult{
		Records: f.records,
		Next:    events.Cursor{Topic: req.Topic, Position: f.records[len(f.records)-1].ID.Position},
		Outcome: events.ReadOutcomeProgress,
	}, nil
}

func (f *fakeService) Subscribe(_ context.Context, req events.SubscribeRequest) (events.Subscription, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	index := 0
	return events.Subscription(func(ctx context.Context) events.Delivery {
		if ctx.Err() != nil {
			return events.Delivery{Kind: events.DeliveryCanceled}
		}
		if index >= len(f.records) {
			return events.Delivery{Kind: events.DeliveryClosed}
		}
		rec := f.records[index]
		index++
		return events.Delivery{Kind: events.DeliveryRecord, Record: rec, Cursor: events.Cursor{Topic: req.Topic, Position: rec.ID.Position}}
	}), nil
}

var _ events.Service = (*fakeService)(nil)

func TestExternalConsumerImplementsService(t *testing.T) {
	svc := &fakeService{}
	ctx := context.Background()
	topic := events.Topic("chat-session/external/events")

	appendResult, err := svc.Append(ctx, events.AppendRequest{
		Topic:          topic,
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"status":"ok"}`),
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if appendResult.Outcome != events.AppendOutcomeAccepted {
		t.Fatalf("Append().Outcome = %v, want AppendOutcomeAccepted", appendResult.Outcome)
	}

	readResult, err := svc.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if readResult.Outcome != events.ReadOutcomeProgress {
		t.Fatalf("Read().Outcome = %v, want ReadOutcomeProgress", readResult.Outcome)
	}
	if len(readResult.Records) != 1 {
		t.Fatalf("Read() returned %d records, want 1", len(readResult.Records))
	}

	subscription, err := svc.Subscribe(ctx, events.SubscribeRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	first := subscription.Next(ctx)
	if first.Kind != events.DeliveryRecord {
		t.Fatalf("Next().Kind = %v, want DeliveryRecord", first.Kind)
	}
	second := subscription.Next(ctx)
	if second.Kind != events.DeliveryClosed {
		t.Fatalf("Next().Kind = %v, want DeliveryClosed once records are exhausted", second.Kind)
	}
}

func TestExternalConsumerRejectsSelfAttachment(t *testing.T) {
	svc := &fakeService{}
	topic := events.Topic("chat-session/external/events")

	_, err := svc.AttachSource(context.Background(), events.AttachSourceRequest{
		Destination: topic,
		Source:      topic,
		StartAt:     events.Cursor{Topic: topic},
		Mode:        events.AttachModeRetainedThenLive,
	})
	if !errors.Is(err, events.ErrSelfAttachment) {
		t.Fatalf("AttachSource() error = %v, want ErrSelfAttachment", err)
	}
}
