package observe

import "time"

// ObserveHTTPRequest is the adapter-owned HTTP request shape for Visualization
// Observe. PSS-I02 later fans this into shared OpenAPI route registration; this
// packet proves decode/map at the owner-local adapter edge.
type ObserveHTTPRequest struct {
	Mode      string                             `json:"mode"`
	Reconnect *ObserveReconnectCursorHTTPRequest `json:"reconnect,omitempty"`
}

// ObserveReconnectCursorHTTPRequest is the adapter-owned reconnect cursor shape
// for Visualization Observe HTTP requests.
type ObserveReconnectCursorHTTPRequest struct {
	AfterEventID  string `json:"after_event_id,omitempty"`
	AfterSequence *int   `json:"after_sequence,omitempty"`
}

// ObserveHTTPResponse is the adapter-owned HTTP success response shape for
// Visualization Observe. Plain projected-view facts are preserved from the root
// contract outcome.
type ObserveHTTPResponse struct {
	View ObserveHTTPProjectedView `json:"view"`
}

// ObserveHTTPProjectedView is the adapter-owned detached live view facts encoded
// into Observe HTTP success responses.
type ObserveHTTPProjectedView struct {
	TickCount          int       `json:"tick_count"`
	RetainedEventCount int       `json:"retained_event_count"`
	ObservedAt         time.Time `json:"observed_at"`
}
