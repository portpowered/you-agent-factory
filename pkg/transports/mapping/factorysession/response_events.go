package factorysession

import (
	"context"
	"encoding/json"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// NewResponseEventSubscription maps a Factory Sessions-owned cursor onto the
// transport serialization boundary.
func NewResponseEventSubscription(cursor ResponseEventCursor) apisurface.FactoryResponseEventSubscription {
	return &responseEventSubscription{cursor: cursor}
}

type responseEventSubscription struct {
	cursor ResponseEventCursor
}

type ResponseEventCursor interface {
	Next(context.Context) ([]factorysessions.FactoryResponseEvent, error)
	Detach()
}

func (s *responseEventSubscription) Next(ctx context.Context) ([]apisurface.FactoryResponseEventRecord, error) {
	events, err := s.cursor.Next(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]apisurface.FactoryResponseEventRecord, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("serialize factory response event: %w", err)
		}
		records = append(records, apisurface.FactoryResponseEventRecord{Sequence: event.Sequence, Kind: string(event.Kind), Data: data})
	}
	return records, nil
}

func (s *responseEventSubscription) Detach() {
	if s != nil && s.cursor != nil {
		s.cursor.Detach()
	}
}
