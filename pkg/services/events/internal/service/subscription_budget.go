package service

import "github.com/portpowered/infinite-you/pkg/services/events"

// recordBytes conservatively accounts the native payload, all variable identity
// bytes and fixed envelope/channel bookkeeping. It measures retained data, not
// JSON-encoded size (escaping would require allocating another large envelope).
func recordBytes(rec events.Record) int {
	if rec.IsZero() {
		return 0
	}
	return 256 + len(rec.Payload) + len(rec.ID.Topic) + len(rec.SourceType) +
		len(rec.SourceID) + len(rec.SourceEventID) + len(rec.SchemaID)
}

func (s *liveSubscriber) reserve(rec events.Record) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if s.maxBytes == 0 {
		return true
	}
	size := recordBytes(rec)
	if size > s.maxBytes-s.pendingBytes {
		return false
	}
	s.pendingBytes += size
	return true
}

func (s *liveSubscriber) release(rec events.Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maxBytes > 0 {
		s.pendingBytes -= recordBytes(rec)
	}
}
