package canonicalledger_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type canonicalEventPeer struct {
	recordings.Service
	accepted recordings.CanonicalEvent
}

func (peer *canonicalEventPeer) Append(
	request recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	peer.accepted = request.Event
	return recordings.AppendRecordedEventResult{Event: request.Event}, nil
}

var _ recordings.Service = (*canonicalEventPeer)(nil)

func TestCanonicalEventFacts_AreRecordingsOwnedDetachedValues(t *testing.T) {
	t.Parallel()

	event := recordings.CanonicalEvent{
		ID:       "event-7",
		Sequence: 7,
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: "session-3",
		},
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-2",
			Sequence:           7,
		},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       "WORK_REQUEST",
		Payload:    `{"workId":"work-4"}`,
	}
	peer := &canonicalEventPeer{}
	var service recordings.Service = peer

	result, err := service.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	if result.Event != event || peer.accepted != event {
		t.Fatalf("Append event = %#v, accepted = %#v, want detached canonical value %#v",
			result.Event, peer.accepted, event)
	}
}
