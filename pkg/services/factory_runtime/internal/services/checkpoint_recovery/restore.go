package checkpoint_recovery

// RestoreRequest restores one versioned opaque checkpoint envelope into mutable
// execution state through the private store port.
type RestoreRequest struct {
	Envelope Envelope
}

// RestoreResult reports the stored opaque checkpoint envelope after restore.
type RestoreResult struct {
	Envelope Envelope
	Facts    ExecutionCaptureFacts
}
