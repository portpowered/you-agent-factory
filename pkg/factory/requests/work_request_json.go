package requests

import (
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/work"
)

type canonicalWorkRequestJSON struct {
	WorkTypeID             json.RawMessage             `json:"work_type_id"`
	TargetState            json.RawMessage             `json:"target_state"`
	CurrentChainingTraceID json.RawMessage             `json:"currentChainingTraceId"`
	LegacyCurrentChaining  json.RawMessage             `json:"current_chaining_trace_id"`
	TraceID                json.RawMessage             `json:"traceId"`
	LegacyTraceID          json.RawMessage             `json:"trace_id"`
	Works                  []canonicalWorkRequestEntry `json:"works"`
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
func ParseCanonicalWorkRequestJSON(data []byte) (work.WorkRequest, error) {
	if err := ValidateCanonicalWorkRequestJSON(data); err != nil {
		return work.WorkRequest{}, err
	}

	var request work.WorkRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return work.WorkRequest{}, err
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

func validateRetiredWorkRequestFieldAliases(raw canonicalWorkRequestJSON) error {
	if raw.WorkTypeID != nil {
		return fmt.Errorf("work_type_id is not supported; use workTypeName")
	}
	if raw.TargetState != nil {
		return fmt.Errorf("target_state is not supported; use state")
	}
	for i := range raw.Works {
		if raw.Works[i].WorkTypeID != nil {
			return fmt.Errorf("works[%d].work_type_id is not supported; use workTypeName", i)
		}
		if raw.Works[i].TargetState != nil {
			return fmt.Errorf("works[%d].target_state is not supported; use state", i)
		}
	}
	return nil
}

func validateConflictingWorkRequestTraceFields(raw canonicalWorkRequestJSON) error {
	if err := ValidateWorkRequestTraceFieldAliases(
		raw.CurrentChainingTraceID,
		raw.LegacyCurrentChaining,
		raw.TraceID,
		raw.LegacyTraceID,
	); err != nil {
		return err
	}
	for i := range raw.Works {
		if err := ValidateWorkRequestTraceFieldAliases(
			raw.Works[i].CurrentChainingTraceID,
			raw.Works[i].LegacyCurrentChaining,
			raw.Works[i].TraceID,
			raw.Works[i].LegacyTraceID,
		); err != nil {
			return fmt.Errorf("works[%d].%w", i, err)
		}
	}
	return nil
}
