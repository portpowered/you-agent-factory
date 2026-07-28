package http

// ActivateHTTPRequest is the adapter-owned HTTP request shape for Visualization
// lifecycle Activate. PSS-I02 later fans this into shared OpenAPI route
// registration; this packet proves decode/map at the owner-local adapter edge.
type ActivateHTTPRequest struct {
	Mode string `json:"mode"`
}

// LifecycleHTTPResponse is the adapter-owned HTTP success response shape for
// Visualization lifecycle operations that publish lifecycle state.
type LifecycleHTTPResponse struct {
	State string `json:"state"`
}
