package stdio

import "sync"

// cancelFlight orders an in-flight prompt's terminal response after the
// captured cancellation and replacement runtime finish. The Chat Session
// remains the authority for the turn and its control intent.
type cancelFlight struct {
	done     chan struct{}
	once     sync.Once
	accepted bool
}

func (f *cancelFlight) finish(accepted bool) {
	f.once.Do(func() {
		f.accepted = accepted
		close(f.done)
	})
}

func cancelFlightKey(sessionID, turnID string) string {
	return sessionID + "\x00" + turnID
}

func (s *Server) startCancelFlight(sessionID, turnID string) (*cancelFlight, bool) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	if s.cancelFlights == nil {
		s.cancelFlights = make(map[string]*cancelFlight)
	}
	key := cancelFlightKey(sessionID, turnID)
	if existing := s.cancelFlights[key]; existing != nil {
		select {
		case <-existing.done:
			if existing.accepted {
				return existing, false
			}
			// A failed downstream control leaves the captured intent retryable.
		default:
			return existing, false
		}
	}
	flight := &cancelFlight{done: make(chan struct{})}
	s.cancelFlights[key] = flight
	return flight, true
}

func (s *Server) cancelFlightForTurn(sessionID, turnID string) *cancelFlight {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	return s.cancelFlights[cancelFlightKey(sessionID, turnID)]
}

func (s *Server) clearCancelFlight(sessionID, turnID string, flight *cancelFlight) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	key := cancelFlightKey(sessionID, turnID)
	if s.cancelFlights[key] == flight {
		delete(s.cancelFlights, key)
	}
}
