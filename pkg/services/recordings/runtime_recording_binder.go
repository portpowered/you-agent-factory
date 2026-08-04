package recordings

// RuntimeRecordingBinder is implemented by RuntimeRecorder producers that
// bind to an already-constructed RecordingLifecycle capability once a caller
// makes one available, instead of receiving the broad Service for
// lifecycle-only use or being discovered through a caller-local type
// assertion. Binding is optional: not every RuntimeRecorder needs to bind
// (for example, a disabled or replay-only recorder), so callers type-assert
// against this published interface rather than requiring it on every
// RuntimeRecorder.
type RuntimeRecordingBinder interface {
	// BindRecordingLifecycle binds this runtime recorder to the given
	// RecordingLifecycle capability and Factory Session scope. Implementations
	// preserve RecordingLifecycle's idempotent-rebind and binding-conflict
	// semantics for repeated calls with identical or differing facts.
	BindRecordingLifecycle(RecordingLifecycle, CanonicalEventScope) error
}
