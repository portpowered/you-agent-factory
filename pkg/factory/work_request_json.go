package factory

import (
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type canonicalWorkRequestJSON struct {
	WorkTypeID json.RawMessage             `json:"work_type_id"`
	Works      []canonicalWorkRequestEntry `json:"works"`
}

type canonicalWorkRequestEntry struct {
	WorkTypeID             json.RawMessage `json:"work_type_id"`
	TargetState            json.RawMessage `json:"target_state"`
	CurrentChainingTraceID json.RawMessage `json:"currentChainingTraceId"`
	LegacyCurrentChaining  json.RawMessage `json:"current_chaining_trace_id"`
	TraceID                json.RawMessage `json:"traceId"`
	LegacyTraceID          json.RawMessage `json:"trace_id"`
}

// ParseCanonicalWorkRequestJSON parses a public FACTORY_REQUEST_BATCH JSON
// payload and rejects retired aliases that must not be accepted on public
// submit boundaries.
func ParseCanonicalWorkRequestJSON(data []byte) (interfaces.WorkRequest, error) {
	if err := ValidateCanonicalWorkRequestJSON(data); err != nil {
		return interfaces.WorkRequest{}, err
	}

	var request interfaces.WorkRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return interfaces.WorkRequest{}, err
	}
	return request, nil
}

// ValidateCanonicalWorkRequestJSON rejects unsupported public aliases and
// conflicting trace-field combinations before canonical work-request JSON is
// normalized into the runtime request model.
func ValidateCanonicalWorkRequestJSON(data []byte) error {
	var raw canonicalWorkRequestJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := validateRetiredWorkRequestFieldAliases(raw); err != nil {
		return err
	}
	if err := validateConflictingWorkRequestTraceFields(raw); err != nil {
		return err
	}
	return nil
}

// RejectRetiredWorkRequestFieldAliases rejects retired public submit fields
// that should no longer be accepted on canonical work-request JSON inputs.
func RejectRetiredWorkRequestFieldAliases(data []byte) error {
	var raw canonicalWorkRequestJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse work request retired fields: %w", err)
	}
	return validateRetiredWorkRequestFieldAliases(raw)
}

func RejectConflictingWorkRequestTraceFields(data []byte) error {
	var raw canonicalWorkRequestJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse work request chaining traces: %w", err)
	}
	return validateConflictingWorkRequestTraceFields(raw)
}

func validateRetiredWorkRequestFieldAliases(raw canonicalWorkRequestJSON) error {
	if raw.WorkTypeID != nil {
		return fmt.Errorf("work request batch uses retired work_type_id field; use workTypeName")
	}
	for i := range raw.Works {
		if raw.Works[i].WorkTypeID != nil {
			return fmt.Errorf("work request batch works[%d] uses retired work_type_id field; use workTypeName", i)
		}
		if raw.Works[i].TargetState != nil {
			return fmt.Errorf("work request batch works[%d] uses retired target_state field; use state", i)
		}
	}
	return nil
}

func validateConflictingWorkRequestTraceFields(raw canonicalWorkRequestJSON) error {
	for i := range raw.Works {
		if err := ValidateWorkRequestTraceFieldAliases(
			raw.Works[i].CurrentChainingTraceID,
			raw.Works[i].LegacyCurrentChaining,
			raw.Works[i].TraceID,
			raw.Works[i].LegacyTraceID,
		); err != nil {
			return fmt.Errorf("work request batch works[%d] %w", i, err)
		}
	}
	return nil
}
