package factory

import "errors"

var errConflictingWorkRequestTraceFields = errors.New("currentChainingTraceId and traceId must match when both are provided")

// ResolveWorkRequestCurrentChainingTraceID returns the effective chaining
// trace, preferring the current field while preserving legacy traceId fallback.
func ResolveWorkRequestCurrentChainingTraceID(current string, legacy string) string {
	if current != "" {
		return current
	}
	return legacy
}

// ValidateWorkRequestTraceFields rejects mismatched currentChainingTraceId and
// traceId values when both are explicitly populated.
func ValidateWorkRequestTraceFields(current string, legacy string) error {
	if current != "" && legacy != "" && current != legacy {
		return errConflictingWorkRequestTraceFields
	}
	return nil
}
