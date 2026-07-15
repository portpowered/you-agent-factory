package factorycontracts

import (
	"encoding/json"
	"testing"
)

func TestFactorySnapshotPreservesUnknownFieldsAndCloneIsolation(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"name": "factory-a",
		"futureField": map[string]any{
			"enabled": true,
		},
	}
	snapshot, err := NewFactorySnapshot(source)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	clone := snapshot.Clone()
	(*snapshot)[0] = '['

	var decoded map[string]any
	if err := clone.Decode(&decoded); err != nil {
		t.Fatalf("Decode clone: %v", err)
	}
	if decoded["name"] != "factory-a" {
		t.Fatalf("name = %#v, want factory-a", decoded["name"])
	}
	future, ok := decoded["futureField"].(map[string]any)
	if !ok || future["enabled"] != true {
		t.Fatalf("futureField = %#v, want preserved unknown object", decoded["futureField"])
	}
}

func TestFactorySnapshotMarshalsAsFactoryObject(t *testing.T) {
	t.Parallel()

	snapshot, err := NewFactorySnapshot(map[string]any{"name": "factory-a"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	encoded, err := json.Marshal(struct {
		Factory *FactorySnapshot `json:"factory"`
	}{Factory: snapshot})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(encoded), `{"factory":{"name":"factory-a"}}`; got != want {
		t.Fatalf("Marshal = %s, want %s", got, want)
	}
}

func TestFactorySnapshotRejectsNonObjectJSON(t *testing.T) {
	t.Parallel()

	if _, err := NewFactorySnapshot([]string{"not", "a", "factory"}); err == nil {
		t.Fatal("NewFactorySnapshot(non-object) error = nil, want actionable validation error")
	}
	var snapshot FactorySnapshot
	if err := json.Unmarshal([]byte(`null`), &snapshot); err == nil {
		t.Fatal("UnmarshalJSON(null) error = nil, want Factory object validation error")
	}
}
