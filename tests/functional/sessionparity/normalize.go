package sessionparity

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// NormalizationError identifies a captured customer-boundary value that cannot
// represent the parity contract. It intentionally preserves the interface and
// stable field path instead of manufacturing parity from partial observations.
type NormalizationError struct {
	Interface string
	Field     string
	Reason    string
}

func (e *NormalizationError) Error() string {
	return fmt.Sprintf("%s observation %s: %s", e.Interface, e.Field, e.Reason)
}

// NormalizeREST normalizes a captured REST body whose Factory Session is in
// its session field. HTTP headers, status, and request metadata are excluded.
func NormalizeREST(observation []byte) (Projection, error) {
	return normalizeEnvelope("REST", observation, []string{"session"})
}

// NormalizeCLIJSON normalizes captured CLI JSON whose Factory Session is in
// its factorySession field. Rendering and command diagnostics are excluded.
func NormalizeCLIJSON(observation []byte) (Projection, error) {
	return normalizeEnvelope("CLI JSON", observation, []string{"factorySession"})
}

// NormalizeMCP normalizes a captured MCP JSON-RPC result whose Factory Session
// is in result.factorySession. JSON-RPC envelope and correlation fields are
// deliberately excluded.
func NormalizeMCP(observation []byte) (Projection, error) {
	return normalizeEnvelope("MCP", observation, []string{"result", "factorySession"})
}

func normalizeEnvelope(name string, observation []byte, path []string) (Projection, error) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(observation, &value); err != nil {
		return Projection{}, normalizationError(name, "$", "must be a JSON object")
	}
	for _, field := range path {
		raw, ok := value[field]
		if !ok {
			return Projection{}, normalizationError(name, field, "is required")
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return Projection{}, normalizationError(name, field, "must be a JSON object")
		}
	}
	return decodeProjection(name, value)
}

func decodeProjection(name string, value map[string]json.RawMessage) (Projection, error) {
	var projection Projection
	if err := decodeRequired(value, "identity", &projection.Identity); err != nil {
		return Projection{}, normalizationError(name, "identity", err.Error())
	}
	if err := decodeRequired(value, "lifecycle", &projection.Lifecycle); err != nil {
		return Projection{}, normalizationError(name, "lifecycle", err.Error())
	}
	if err := decodeRequired(value, "hashes", &projection.Hashes); err != nil {
		return Projection{}, normalizationError(name, "hashes", err.Error())
	}
	if err := decodeRequired(value, "progress", &projection.Progress); err != nil {
		return Projection{}, normalizationError(name, "progress", err.Error())
	}
	if err := decodeRequired(value, "dispatches", &projection.Dispatches); err != nil {
		return Projection{}, normalizationError(name, "dispatches", err.Error())
	}
	if err := decodeRequired(value, "artifacts", &projection.Artifacts); err != nil {
		return Projection{}, normalizationError(name, "artifacts", err.Error())
	}
	if err := decodeRequired(value, "results", &projection.Results); err != nil {
		return Projection{}, normalizationError(name, "results", err.Error())
	}
	if err := decodeRequired(value, "failures", &projection.Failures); err != nil {
		return Projection{}, normalizationError(name, "failures", err.Error())
	}
	if err := decodeRequired(value, "eventCursors", &projection.EventCursors); err != nil {
		return Projection{}, normalizationError(name, "eventCursors", err.Error())
	}
	if err := validateProjection(projection); err != nil {
		return Projection{}, normalizationError(name, err.Field, err.Reason)
	}
	return projection, nil
}

func decodeRequired(value map[string]json.RawMessage, field string, destination any) error {
	raw, ok := value[field]
	if !ok {
		return fmt.Errorf("is required")
	}
	if bytes.Equal(raw, []byte("null")) {
		return fmt.Errorf("must not be null")
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return fmt.Errorf("has incompatible value")
	}
	return nil
}

func validateProjection(projection Projection) *NormalizationError {
	if projection.Identity.SessionID == "" {
		return &NormalizationError{Field: "identity.sessionId", Reason: "is required"}
	}
	if projection.Lifecycle.Status == "" {
		return &NormalizationError{Field: "lifecycle.status", Reason: "is required"}
	}
	if projection.Hashes.SourceHash == "" {
		return &NormalizationError{Field: "hashes.sourceHash", Reason: "is required"}
	}
	if err := validateDispatches(projection.Identity.SessionID, projection.Dispatches); err != nil {
		return err
	}
	if err := validateArtifacts(projection.Identity.SessionID, projection.Artifacts); err != nil {
		return err
	}
	if err := validateResults(projection.Identity.SessionID, projection.Results); err != nil {
		return err
	}
	if err := validateFailures(projection.Identity.SessionID, projection.Failures); err != nil {
		return err
	}
	return validateEventCursors(projection.Identity.SessionID, projection.EventCursors)
}

func validateDispatches(sessionID string, facts []DispatchFact) *NormalizationError {
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.ID == "" || fact.Order != index+1 || fact.Status == "" || fact.Kind == "" {
			return &NormalizationError{Field: fmt.Sprintf("dispatches[%d]", index), Reason: "must retain a correlated, ordered dispatch fact"}
		}
	}
	return nil
}

func validateArtifacts(sessionID string, facts []ArtifactFact) *NormalizationError {
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.ID == "" || fact.Order != index+1 || fact.Kind == "" {
			return &NormalizationError{Field: fmt.Sprintf("artifacts[%d]", index), Reason: "must retain a correlated, ordered artifact fact"}
		}
	}
	return nil
}

func validateResults(sessionID string, facts []ResultFact) *NormalizationError {
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.ID == "" || fact.Order != index+1 || fact.Status == "" {
			return &NormalizationError{Field: fmt.Sprintf("results[%d]", index), Reason: "must retain a correlated, ordered result fact"}
		}
	}
	return nil
}

func validateFailures(sessionID string, facts []FailureFact) *NormalizationError {
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.ID == "" || fact.Order != index+1 || fact.Code == "" || fact.Message == "" {
			return &NormalizationError{Field: fmt.Sprintf("failures[%d]", index), Reason: "must retain a correlated, ordered failure fact"}
		}
	}
	return nil
}

func validateEventCursors(sessionID string, facts []FactoryEventCursor) *NormalizationError {
	var previous int64
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.Cursor == "" || fact.EventType == "" || (index > 0 && fact.Sequence <= previous) {
			return &NormalizationError{Field: fmt.Sprintf("eventCursors[%d]", index), Reason: "must retain a correlated, strictly ordered event cursor"}
		}
		previous = fact.Sequence
	}
	return nil
}

func normalizationError(name, field, reason string) *NormalizationError {
	return &NormalizationError{Interface: name, Field: field, Reason: reason}
}
