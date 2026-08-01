package presentation

// OpenPresentationHTTPRequest is the adapter-owned HTTP request shape for
// Visualization presentation open. PSS-I02 later fans this into shared OpenAPI
// route registration; this packet proves decode/map at the owner-local adapter
// edge.
type OpenPresentationHTTPRequest struct {
	Mode string `json:"mode"`
}

// OpenPresentationHTTPResponse is the adapter-owned HTTP success response shape
// for Visualization presentation open.
type OpenPresentationHTTPResponse struct {
	SessionID string `json:"session_id"`
	Mode      string `json:"mode"`
}

// PresentProgressHTTPRequest is the adapter-owned HTTP request shape for
// Visualization presentation progress enqueue.
type PresentProgressHTTPRequest struct {
	SessionID string                      `json:"session_id"`
	Records   []ProgressRecordHTTPRequest `json:"records"`
}

// ProgressRecordHTTPRequest is one adapter-owned progress payload in a
// PresentProgress HTTP request.
type ProgressRecordHTTPRequest struct {
	Payload []byte `json:"payload"`
}

// PresentProgressHTTPResponse is the adapter-owned HTTP success response shape
// for Visualization presentation progress enqueue.
type PresentProgressHTTPResponse struct {
	AcceptedCount int `json:"accepted_count"`
}

// FinalizePresentationHTTPRequest is the adapter-owned HTTP request shape for
// Visualization presentation finalize.
type FinalizePresentationHTTPRequest struct {
	SessionID string                    `json:"session_id"`
	Terminal  *TerminalWriteHTTPRequest `json:"terminal,omitempty"`
}

// TerminalWriteHTTPRequest is the adapter-owned terminal payload in a
// FinalizePresentation HTTP request.
type TerminalWriteHTTPRequest struct {
	Payload []byte `json:"payload"`
}

// FinalizePresentationHTTPResponse is the adapter-owned HTTP success response
// shape for Visualization presentation finalize.
type FinalizePresentationHTTPResponse struct {
	Finalized    bool `json:"finalized"`
	ProgressSeen bool `json:"progress_seen"`
}

// ClosePresentationHTTPRequest is the adapter-owned HTTP request shape for
// Visualization presentation close-and-drain.
type ClosePresentationHTTPRequest struct {
	SessionID string `json:"session_id"`
}

// ClosePresentationHTTPResponse is the adapter-owned HTTP success response shape
// for Visualization presentation close-and-drain.
type ClosePresentationHTTPResponse struct {
	DroppedCount int `json:"dropped_count"`
}
