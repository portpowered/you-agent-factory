package factory

import (
	"encoding/json"
	"errors"
	"fmt"
)

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

// ValidateWorkRequestTraceFieldAliases resolves supported trace field aliases
// and rejects mismatched currentChainingTraceId versus traceId values.
func ValidateWorkRequestTraceFieldAliases(currentRaw json.RawMessage, legacyCurrentRaw json.RawMessage, traceRaw json.RawMessage, legacyTraceRaw json.RawMessage) error {
	if currentRaw == nil {
		currentRaw = legacyCurrentRaw
	}
	if traceRaw == nil {
		traceRaw = legacyTraceRaw
	}
	if currentRaw == nil || traceRaw == nil {
		return nil
	}

	var current string
	if err := json.Unmarshal(currentRaw, &current); err != nil {
		return fmt.Errorf("parse currentChainingTraceId: %w", err)
	}
	var legacy string
	if err := json.Unmarshal(traceRaw, &legacy); err != nil {
		return fmt.Errorf("parse traceId: %w", err)
	}
	return ValidateWorkRequestTraceFields(current, legacy)
}
