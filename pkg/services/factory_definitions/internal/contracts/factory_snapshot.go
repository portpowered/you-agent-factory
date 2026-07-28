package factorycontracts

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// DefaultCurrentFactoryName is the reserved identity used when the editable
// Factory snapshot is read directly from the current Factory root.
const DefaultCurrentFactoryName = "UNDEFINED"

// FactorySnapshot is the canonical serialized Factory definition carried by
// Factory events and projections. The Factory domain owns the snapshot bytes;
// transport adapters decode them into generated public response types.
//
// Keeping the snapshot as validated JSON preserves unknown public fields during
// replay and projection while avoiding a dependency from the Factory domain to
// generated HTTP contracts.
type FactorySnapshot json.RawMessage

// MarshalJSON preserves the captured Factory object instead of encoding the
// underlying bytes as a base64 string.
func (s FactorySnapshot) MarshalJSON() ([]byte, error) {
	if err := validateFactorySnapshotJSON(s); err != nil {
		return nil, fmt.Errorf("marshal factory snapshot: %w", err)
	}
	return bytes.Clone(s), nil
}

// UnmarshalJSON validates and detaches a Factory object read from an event or
// projection document.
func (s *FactorySnapshot) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("unmarshal factory snapshot: destination is required")
	}
	if err := validateFactorySnapshotJSON(data); err != nil {
		return fmt.Errorf("unmarshal factory snapshot: %w", err)
	}
	*s = FactorySnapshot(bytes.Clone(data))
	return nil
}

// NewFactorySnapshot captures one Factory definition as detached canonical
// JSON. The input may be a domain definition, a transport-boundary value, or a
// previously captured snapshot.
func NewFactorySnapshot(value any) (*FactorySnapshot, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode factory snapshot: %w", err)
	}
	if err := validateFactorySnapshotJSON(encoded); err != nil {
		return nil, fmt.Errorf("encode factory snapshot: %w", err)
	}
	snapshot := FactorySnapshot(bytes.Clone(encoded))
	return &snapshot, nil
}

func validateFactorySnapshotJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if !json.Valid(trimmed) {
		return fmt.Errorf("invalid JSON")
	}
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("Factory object is required")
	}
	return nil
}

// Clone returns a detached copy of the snapshot.
func (s *FactorySnapshot) Clone() *FactorySnapshot {
	if s == nil {
		return nil
	}
	clone := FactorySnapshot(bytes.Clone(*s))
	return &clone
}

// Decode projects the canonical snapshot into a boundary-owned destination.
func (s *FactorySnapshot) Decode(destination any) error {
	if s == nil {
		return fmt.Errorf("decode factory snapshot: snapshot is required")
	}
	if destination == nil {
		return fmt.Errorf("decode factory snapshot: destination is required")
	}
	if err := json.Unmarshal(*s, destination); err != nil {
		return fmt.Errorf("decode factory snapshot: %w", err)
	}
	return nil
}

// WithName returns a detached snapshot whose Factory name is replaced with the
// canonical read-model identity. Unknown fields remain present in the snapshot.
func (s *FactorySnapshot) WithName(name string) (*FactorySnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("set factory snapshot name: snapshot is required")
	}
	var object map[string]json.RawMessage
	if err := s.Decode(&object); err != nil {
		return nil, fmt.Errorf("set factory snapshot name: %w", err)
	}
	encodedName, err := json.Marshal(name)
	if err != nil {
		return nil, fmt.Errorf("set factory snapshot name: %w", err)
	}
	object["name"] = encodedName
	updated, err := NewFactorySnapshot(object)
	if err != nil {
		return nil, fmt.Errorf("set factory snapshot name: %w", err)
	}
	return updated, nil
}
