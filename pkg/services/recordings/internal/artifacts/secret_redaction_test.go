package artifacts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestBuildPortableRecordingRedactsDeclaredResultBeforeReturning(t *testing.T) {
	t.Parallel()

	facts := minimalCanonicalFacts()
	facts.Result = &CanonicalResult{
		Status:        "FINAL",
		Mode:          "final",
		PrimaryResult: json.RawMessage(`{"credential":"portable-build-secret-002","control":"portable-control"}`),
	}
	facts.SecretProvenance = []recordings.RecordingSecret{{
		JSONPointer: "/result/primaryResult/credential",
		Provenance:  recordings.RecordingSecretProvenanceDeclared,
	}}

	value, err := Build(facts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if value.Redaction.SecretsRedacted != 1 {
		t.Fatalf("secretsRedacted = %d, want 1", value.Redaction.SecretsRedacted)
	}
	assertPortableResultRedacted(t, value, "portable-control")
}

func TestAtomicWriterRedactsBeforeTemporaryWrite(t *testing.T) {
	t.Parallel()

	facts := minimalCanonicalFacts()
	facts.Result = &CanonicalResult{
		Status:        "FINAL",
		Mode:          "final",
		PrimaryResult: json.RawMessage(`{"credential":"portable-write-secret-002","control":"portable-write-control"}`),
	}
	value, err := Build(facts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	value.SecretProvenance = []recordings.RecordingSecret{{
		JSONPointer: "/result/primaryResult/credential",
		Provenance:  recordings.RecordingSecretProvenanceDeclared,
	}}

	writer, err := NewAtomicWriter(
		os.MkdirAll,
		func(dir, pattern string) (TemporaryFile, error) { return os.CreateTemp(dir, pattern) },
		os.Remove,
		os.Rename,
	)
	if err != nil {
		t.Fatalf("NewAtomicWriter: %v", err)
	}
	path := filepath.Join(t.TempDir(), "nested", "recording.json")
	if err := writer.Write(path, value); err != nil {
		t.Fatalf("Write: %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted Recording
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatalf("decode persisted recording: %v", err)
	}
	if persisted.Redaction.SecretsRedacted != 1 {
		t.Fatalf("persisted secretsRedacted = %d, want 1", persisted.Redaction.SecretsRedacted)
	}
	assertPortableResultRedacted(t, persisted, "portable-write-control")
}

func TestAtomicWriterRejectsClassifiedPathBeforeCreatingDestination(t *testing.T) {
	t.Parallel()

	const declaredSecret = "portable-write-failure-secret-002"
	facts := minimalCanonicalFacts()
	facts.Result = &CanonicalResult{
		Status:        "FINAL",
		Mode:          "final",
		PrimaryResult: json.RawMessage(`{"credential":"portable-write-failure-secret-002"}`),
	}
	value, err := Build(facts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	value.SecretProvenance = []recordings.RecordingSecret{{
		JSONPointer: "/result/primaryResult/missing",
		Provenance:  recordings.RecordingSecretProvenanceDeclared,
	}}
	writer, err := NewAtomicWriter(
		os.MkdirAll,
		func(dir, pattern string) (TemporaryFile, error) { return os.CreateTemp(dir, pattern) },
		os.Remove,
		os.Rename,
	)
	if err != nil {
		t.Fatalf("NewAtomicWriter: %v", err)
	}
	path := filepath.Join(t.TempDir(), "not-created", "recording.json")
	err = writer.Write(path, value)
	if !errors.Is(err, recordings.ErrRecordingSecretPathNotFound) {
		t.Fatalf("Write error = %v, want missing classified path", err)
	}
	if strings.Contains(err.Error(), declaredSecret) {
		t.Fatalf("Write error exposed declared secret: %v", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination stat = %v, want not exist", statErr)
	}
}

func assertPortableResultRedacted(t *testing.T, value Recording, wantControl string) {
	t.Helper()
	if value.Result == nil {
		t.Fatal("portable result is nil")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value.Result.PrimaryResult, &fields); err != nil {
		t.Fatalf("decode portable result: %v", err)
	}
	var marker recordings.RecordingRedactedValue
	if err := json.Unmarshal(fields["credential"], &marker); err != nil {
		t.Fatalf("decode portable redaction marker: %v", err)
	}
	if err := marker.Validate(); err != nil {
		t.Fatalf("portable redaction marker: %v", err)
	}
	var control string
	if err := json.Unmarshal(fields["control"], &control); err != nil || control != wantControl {
		t.Fatalf("control = %q, want %q (err=%v)", control, wantControl, err)
	}
	if err := Validate(value); err != nil {
		t.Fatalf("redacted portable result failed validation: %v", err)
	}
}
