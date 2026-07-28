package checkpoint_recovery

// LoadRequest loads one versioned opaque checkpoint envelope for inspect or
// compatibility checking without restoring mutable execution state.
type LoadRequest struct {
	CheckpointID          string
	ExpectedSchemaVersion int
}

// LoadResult reports the stored opaque checkpoint envelope and whether it
// matches ExpectedSchemaVersion when that field is non-zero.
type LoadResult struct {
	Envelope   Envelope
	Compatible bool
}

// CompatibilityForSchema reports whether a stored schema version matches the
// caller's expected schema. A zero expected schema skips compatibility checks.
func CompatibilityForSchema(storedSchemaVersion, expectedSchemaVersion int) bool {
	if expectedSchemaVersion == 0 {
		return false
	}
	return storedSchemaVersion == expectedSchemaVersion
}
